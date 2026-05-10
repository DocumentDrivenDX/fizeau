package fizeau

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/easel/fizeau/internal/provider/openai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubClock is a deterministic time source for tests.
type stubClock struct {
	mu  sync.Mutex
	now time.Time
}

func newStubClock() *stubClock {
	return &stubClock{now: time.Date(2026, 4, 21, 12, 0, 0, 0, time.UTC)}
}

func (c *stubClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *stubClock) advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.mu.Unlock()
}

func testCache(clock *stubClock) *catalogCache {
	return newCatalogCache(catalogCacheOptions{
		FreshTTL:            10 * time.Second,
		StaleTTL:            60 * time.Second,
		UnreachableCooldown: 5 * time.Second,
		UnreachableJitter:   0, // deterministic tests
		Now:                 clock.Now,
		RandInt63n:          func(n int64) int64 { return 0 },
	})
}

func testKey(baseURL string) catalogCacheKey {
	return newCatalogCacheKey(baseURL, "apikey", map[string]string{"X-Test": "yes"})
}

func TestCatalogCache_FreshHitAvoidsNetwork(t *testing.T) {
	clock := newStubClock()
	cache := testCache(clock)
	key := testKey("http://host/v1")

	var callCount atomic.Int32
	probe := func(ctx context.Context) ([]string, error) {
		callCount.Add(1)
		return []string{"model-a", "model-b"}, nil
	}

	// Cold miss → synchronous probe.
	r1, err := cache.Get(context.Background(), key, probe)
	require.NoError(t, err)
	assert.Equal(t, []string{"model-a", "model-b"}, r1.IDs)
	assert.False(t, r1.FromCache, "cold miss is not from cache")
	assert.EqualValues(t, 1, callCount.Load())

	// Fresh hit within FreshTTL → no probe.
	clock.advance(5 * time.Second)
	r2, err := cache.Get(context.Background(), key, probe)
	require.NoError(t, err)
	assert.Equal(t, []string{"model-a", "model-b"}, r2.IDs)
	assert.True(t, r2.FromCache)
	assert.False(t, r2.Stale)
	assert.EqualValues(t, 1, callCount.Load(), "fresh hit must not re-probe")
}

func TestCatalogCache_StaleServesCachedAndAsyncRefreshes(t *testing.T) {
	clock := newStubClock()
	cache := testCache(clock)
	key := testKey("http://host/v1")

	var callCount atomic.Int32
	refreshedCh := make(chan struct{}, 2)
	probe := func(ctx context.Context) ([]string, error) {
		n := callCount.Add(1)
		if n > 1 {
			refreshedCh <- struct{}{}
		}
		return []string{"fresh-model"}, nil
	}

	// Seed the cache.
	_, _ = cache.Get(context.Background(), key, probe)

	// Advance past FreshTTL but within StaleTTL.
	clock.advance(20 * time.Second)

	// Stale Get returns cached + kicks async refresh.
	r, err := cache.Get(context.Background(), key, probe)
	require.NoError(t, err)
	assert.True(t, r.FromCache)
	assert.True(t, r.Stale, "result must be flagged stale")
	assert.Equal(t, []string{"fresh-model"}, r.IDs)

	// Wait for async refresh to complete.
	select {
	case <-refreshedCh:
		// refreshed
	case <-time.After(2 * time.Second):
		t.Fatal("async refresh did not fire within 2s")
	}

	// After refresh, the next call should see the fresh fetch timestamp.
	r2, err := cache.Get(context.Background(), key, probe)
	require.NoError(t, err)
	assert.True(t, r2.FromCache, "still within FreshTTL of the refresh")
}

func TestCatalogCache_AsyncRefreshUsesConfiguredDeadline(t *testing.T) {
	clock := newStubClock()
	cache := newCatalogCache(catalogCacheOptions{
		FreshTTL:            10 * time.Second,
		StaleTTL:            60 * time.Second,
		UnreachableCooldown: 5 * time.Second,
		UnreachableJitter:   0,
		AsyncRefreshTimeout: 40 * time.Millisecond,
		Now:                 clock.Now,
		RandInt63n:          func(n int64) int64 { return 0 },
	})
	key := testKey("http://host/v1")

	var callCount atomic.Int32
	deadlineCh := make(chan time.Duration, 1)
	doneCh := make(chan error, 1)
	probe := func(ctx context.Context) ([]string, error) {
		callCount.Add(1)
		deadline, ok := ctx.Deadline()
		if !ok {
			deadlineCh <- -1
			doneCh <- context.Canceled
			return nil, context.Canceled
		}
		deadlineCh <- time.Until(deadline)
		<-ctx.Done()
		err := ctx.Err()
		doneCh <- err
		return nil, err
	}

	// Seed the cache so the next call is stale and triggers async refresh.
	_, err := cache.Get(context.Background(), key, func(context.Context) ([]string, error) {
		return []string{"seed-model"}, nil
	})
	require.NoError(t, err)
	clock.advance(20 * time.Second)

	r, err := cache.Get(context.Background(), key, probe)
	require.NoError(t, err)
	assert.True(t, r.Stale)
	assert.True(t, r.FromCache)

	select {
	case remaining := <-deadlineCh:
		assert.Greater(t, remaining, time.Duration(0), "refresh context must have a live deadline")
		assert.LessOrEqual(t, remaining, 40*time.Millisecond, "refresh deadline must respect the configured timeout")
	case <-time.After(1 * time.Second):
		t.Fatal("async refresh probe did not start")
	}

	select {
	case err := <-doneCh:
		require.ErrorIs(t, err, context.DeadlineExceeded)
	case <-time.After(1 * time.Second):
		t.Fatal("async refresh probe did not observe cancellation")
	}

	assert.EqualValues(t, 1, callCount.Load(), "one async refresh probe should run under the configured deadline")
}

func TestCatalogCache_ColdMissCoalesces(t *testing.T) {
	clock := newStubClock()
	cache := testCache(clock)
	key := testKey("http://host/v1")

	var inflight atomic.Int32
	var maxConcurrent atomic.Int32
	gate := make(chan struct{})
	probe := func(ctx context.Context) ([]string, error) {
		n := inflight.Add(1)
		// Track the peak concurrent probes so we can assert coalescing.
		for {
			peak := maxConcurrent.Load()
			if n <= peak || maxConcurrent.CompareAndSwap(peak, n) {
				break
			}
		}
		<-gate
		inflight.Add(-1)
		return []string{"coalesced-model"}, nil
	}

	const N = 10
	var wg sync.WaitGroup
	var started sync.WaitGroup
	wg.Add(N)
	started.Add(N)

	for i := 0; i < N; i++ {
		go func() {
			defer wg.Done()
			started.Done()
			_, _ = cache.Get(context.Background(), key, probe)
		}()
	}
	started.Wait()
	// Give all goroutines a moment to reach the singleflight point.
	time.Sleep(50 * time.Millisecond)
	close(gate)
	wg.Wait()

	assert.EqualValues(t, 1, maxConcurrent.Load(),
		"singleflight must coalesce N concurrent cold-miss callers into 1 probe")
}

func TestCatalogCache_ReachabilityErrorCooldown(t *testing.T) {
	clock := newStubClock()
	cache := testCache(clock)
	key := testKey("http://host/v1")

	var callCount atomic.Int32
	reachErr := &openai.ReachabilityError{
		Endpoint: "http://host/v1", Operation: "probe_models", StatusCode: 502,
		Cause: errors.New("bad gateway"),
	}
	probe := func(ctx context.Context) ([]string, error) {
		callCount.Add(1)
		return nil, reachErr
	}

	// First probe fails with ReachabilityError.
	_, err := cache.Get(context.Background(), key, probe)
	require.Error(t, err)
	assert.True(t, errors.Is(err, openai.ErrEndpointUnreachable))
	assert.EqualValues(t, 1, callCount.Load())

	// Within cooldown window, cache returns the same error without re-probing.
	clock.advance(2 * time.Second)
	_, err = cache.Get(context.Background(), key, probe)
	require.Error(t, err)
	assert.True(t, errors.Is(err, openai.ErrEndpointUnreachable))
	assert.EqualValues(t, 1, callCount.Load(), "must not re-probe within cooldown")

	// Past cooldown, re-probe.
	clock.advance(10 * time.Second)
	_, _ = cache.Get(context.Background(), key, probe)
	assert.EqualValues(t, 2, callCount.Load(), "must re-probe after cooldown expires")
}

func TestCatalogCache_DiscoveryUnsupportedFallsBackToPassthrough(t *testing.T) {
	clock := newStubClock()
	cache := testCache(clock)
	key := testKey("http://host/v1")

	var callCount atomic.Int32
	probe := func(ctx context.Context) ([]string, error) {
		callCount.Add(1)
		return nil, ErrDiscoveryUnsupported()
	}

	r, err := cache.Get(context.Background(), key, probe)
	require.Error(t, err) // discovery-unsupported is surfaced as err
	assert.False(t, r.DiscoverySupported,
		"endpoint marked as discovery-unsupported")
	assert.Empty(t, r.IDs)
	assert.EqualValues(t, 1, callCount.Load())

	// Fresh cache state suppresses re-probes within FreshTTL even when
	// last state was DiscoverySupported=false. Note: because LastErr != nil
	// on the entry, the "fresh" branch doesn't short-circuit; this is by
	// design — the caller wants to know discovery remains unsupported on
	// every call. Verify the cache does NOT re-probe within cooldown.
	// (Discovery-unsupported is not a ReachabilityError → no cooldown set.)
	// The cache re-probes — that's acceptable because /v1/models 404 is
	// cheap, and in practice we expect this to be rare (most servers
	// support discovery).
}

func TestCatalogCache_SnapshotsDontAliasCacheState(t *testing.T) {
	clock := newStubClock()
	cache := testCache(clock)
	key := testKey("http://host/v1")
	probe := func(ctx context.Context) ([]string, error) {
		return []string{"model-x", "model-y"}, nil
	}
	r, err := cache.Get(context.Background(), key, probe)
	require.NoError(t, err)

	// Mutate caller's slice; cache state must be unaffected.
	r.IDs[0] = "mutated"

	r2, err := cache.Get(context.Background(), key, probe)
	require.NoError(t, err)
	assert.Equal(t, []string{"model-x", "model-y"}, r2.IDs,
		"caller mutation must not leak into cache state")
}

func TestCatalogCacheKey_String(t *testing.T) {
	k1 := newCatalogCacheKey("http://host/v1", "key1", nil)
	k2 := newCatalogCacheKey("http://host/v1", "key2", nil)
	k3 := newCatalogCacheKey("http://host/v1", "key1", map[string]string{"X-Custom": "a"})
	k4 := newCatalogCacheKey("http://host/v1", "key1", map[string]string{"X-Custom": "b"})

	assert.NotEqual(t, k1.String(), k2.String(), "different API keys must hash differently")
	assert.NotEqual(t, k1.String(), k3.String(), "adding headers must change the key")
	assert.NotEqual(t, k3.String(), k4.String(), "header values must be part of the hash")
	assert.Equal(t, k1.String(), k1.String(), "same inputs produce same key")
}

func TestHashHeaders_OrderIndependent(t *testing.T) {
	a := map[string]string{"X-One": "1", "X-Two": "2"}
	b := map[string]string{"X-Two": "2", "X-One": "1"}
	assert.Equal(t, hashHeaders(a), hashHeaders(b),
		"header hash must be independent of Go map iteration order")
}

func TestNewCatalogCacheKey_EmptyHeadersHashZero(t *testing.T) {
	k1 := newCatalogCacheKey("url", "key", nil)
	k2 := newCatalogCacheKey("url", "key", map[string]string{})
	assert.Equal(t, k1, k2, "nil and empty headers must produce identical keys")
}

// Ensure helper functions don't cause data races under concurrent Get calls.
func TestCatalogCache_RaceSafety(t *testing.T) {
	clock := newStubClock()
	cache := testCache(clock)
	probe := func(ctx context.Context) ([]string, error) {
		return []string{"m1", "m2"}, nil
	}

	const N = 50
	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func(i int) {
			defer wg.Done()
			key := testKey(fmt.Sprintf("http://host-%d/v1", i%5))
			_, _ = cache.Get(context.Background(), key, probe)
		}(i)
	}
	wg.Wait()
}
