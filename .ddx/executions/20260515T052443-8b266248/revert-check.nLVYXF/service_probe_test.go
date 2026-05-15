package fizeau

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/easel/fizeau/internal/provider/openai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProbeOpenAIModels_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/models", r.URL.Path)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"object": "list",
			"data": []map[string]any{
				{"id": "Qwen3.6-35B-A3B-4bit"},
				{"id": "Qwen3.6-35B-A3B-nvfp4"},
			},
		})
	}))
	defer srv.Close()
	ids, err := probeOpenAIModels(context.Background(), srv.URL+"/v1", "")
	require.NoError(t, err)
	assert.Equal(t, []string{"Qwen3.6-35B-A3B-4bit", "Qwen3.6-35B-A3B-nvfp4"}, ids)
}

func TestProbeOpenAIModels_404ReturnsDiscoveryUnsupported(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()
	_, err := probeOpenAIModels(context.Background(), srv.URL+"/v1", "")
	require.Error(t, err)
	assert.True(t, isDiscoveryUnsupported(err),
		"404 must classify as discovery-unsupported so the cache enables passthrough")
}

func TestProbeOpenAIModels_502ReturnsReachabilityError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
	}))
	defer srv.Close()
	_, err := probeOpenAIModels(context.Background(), srv.URL+"/v1", "")
	require.Error(t, err)
	assert.True(t, errors.Is(err, openai.ErrEndpointUnreachable),
		"5xx must wrap as ReachabilityError so routing can skip")
}

func TestProbeOpenAIModels_DialFailureReturnsReachabilityError(t *testing.T) {
	// http://127.0.0.1:1 is RFC-invalid and will reliably fail to dial.
	_, err := probeOpenAIModels(context.Background(), "http://127.0.0.1:1/v1", "")
	require.Error(t, err)
	assert.True(t, errors.Is(err, openai.ErrEndpointUnreachable),
		"dial failure must wrap as ReachabilityError")
}

func TestProbeOpenAIModels_AuthErrorBubbles(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer srv.Close()
	_, err := probeOpenAIModels(context.Background(), srv.URL+"/v1", "")
	require.Error(t, err)
	// 401 is neither reachability nor discovery-unsupported; it's a plain error.
	assert.False(t, errors.Is(err, openai.ErrEndpointUnreachable))
	assert.False(t, isDiscoveryUnsupported(err))
}

func TestRunQuotaRecoveryProbePass_ElapsedRetryTriggersProbe(t *testing.T) {
	store := NewProviderQuotaStateStore()
	now := time.Date(2026, 5, 2, 12, 0, 0, 0, time.UTC)
	// retry_after already in the past — probe must run.
	store.MarkQuotaExhausted("openai", now.Add(-time.Minute))

	var calls []string
	probe := func(_ context.Context, name string) error {
		calls = append(calls, name)
		return nil
	}
	backoffs := map[string]time.Duration{}
	next := runQuotaRecoveryProbePass(context.Background(), store, probe, time.Minute, func() time.Time { return now }, backoffs)

	require.Equal(t, []string{"openai"}, calls)
	state, _ := store.State("openai", now)
	assert.Equal(t, ProviderQuotaStateAvailable, state, "successful probe must restore provider")
	assert.Equal(t, time.Minute, next, "with no remaining exhausted entries the loop should sleep for fallback")
}

func TestRunQuotaRecoveryProbePass_FutureRetrySkipsProbe(t *testing.T) {
	store := NewProviderQuotaStateStore()
	now := time.Date(2026, 5, 2, 12, 0, 0, 0, time.UTC)
	store.MarkQuotaExhausted("openai", now.Add(2*time.Minute))

	called := false
	probe := func(_ context.Context, _ string) error {
		called = true
		return nil
	}
	backoffs := map[string]time.Duration{}
	next := runQuotaRecoveryProbePass(context.Background(), store, probe, 5*time.Minute, func() time.Time { return now }, backoffs)

	assert.False(t, called, "future retry_after must not trigger probe")
	assert.Equal(t, 2*time.Minute, next, "next wake should be the soonest retry_after")
	state, _ := store.State("openai", now)
	assert.Equal(t, ProviderQuotaStateQuotaExhausted, state)
}

func TestRunQuotaRecoveryProbePass_FailureExtendsWithBackoff(t *testing.T) {
	store := NewProviderQuotaStateStore()
	now := time.Date(2026, 5, 2, 12, 0, 0, 0, time.UTC)
	store.MarkQuotaExhausted("openai", now.Add(-time.Second))

	probeErr := errors.New("still 429")
	probe := func(_ context.Context, _ string) error { return probeErr }
	backoffs := map[string]time.Duration{}

	next := runQuotaRecoveryProbePass(context.Background(), store, probe, time.Hour, func() time.Time { return now }, backoffs)
	assert.Equal(t, quotaRecoveryBackoffInitial, next)
	assert.Equal(t, quotaRecoveryBackoffInitial, backoffs["openai"])
	state, retry := store.State("openai", now)
	require.Equal(t, ProviderQuotaStateQuotaExhausted, state)
	assert.True(t, retry.Equal(now.Add(quotaRecoveryBackoffInitial)),
		"retry_after must extend by initial backoff after first failure: got %v", retry)

	// Second failure (advance the clock past the new retry) doubles.
	now2 := retry.Add(time.Second)
	next2 := runQuotaRecoveryProbePass(context.Background(), store, probe, time.Hour, func() time.Time { return now2 }, backoffs)
	assert.Equal(t, quotaRecoveryBackoffInitial*2, next2)
	assert.Equal(t, quotaRecoveryBackoffInitial*2, backoffs["openai"])
}

func TestNextQuotaRecoveryBackoffCaps(t *testing.T) {
	assert.Equal(t, quotaRecoveryBackoffInitial, nextQuotaRecoveryBackoff(0))
	assert.Equal(t, quotaRecoveryBackoffMax, nextQuotaRecoveryBackoff(quotaRecoveryBackoffMax))
	assert.Equal(t, quotaRecoveryBackoffMax, nextQuotaRecoveryBackoff(quotaRecoveryBackoffMax*2))
}

func TestRunQuotaRecoveryProbePass_FallbackIntervalWhenEmpty(t *testing.T) {
	store := NewProviderQuotaStateStore()
	probe := func(_ context.Context, _ string) error { return nil }
	backoffs := map[string]time.Duration{}
	next := runQuotaRecoveryProbePass(context.Background(), store, probe, 7*time.Minute, time.Now, backoffs)
	assert.Equal(t, 7*time.Minute, next, "empty store must wake at the fallback interval")
}

func TestRunQuotaRecoveryProbeLoop_CancellableViaContext(t *testing.T) {
	store := NewProviderQuotaStateStore()
	store.MarkQuotaExhausted("openai", time.Now().Add(-time.Second))

	probe := func(_ context.Context, _ string) error { return errors.New("down") }

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		runQuotaRecoveryProbeLoop(ctx, store, probe, time.Hour, nil, func(c context.Context, _ time.Duration) bool {
			// Block until ctx cancels so the loop must exit via the
			// cancellation path, not by completing the sleep.
			<-c.Done()
			return false
		})
		close(done)
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("probe loop did not exit after context cancel")
	}
}

func TestRunQuotaRecoveryProbeLoop_RecoveryRestoresProvider(t *testing.T) {
	store := NewProviderQuotaStateStore()
	now := time.Date(2026, 5, 2, 12, 0, 0, 0, time.UTC)
	store.MarkQuotaExhausted("openai", now.Add(-time.Second))

	probe := func(_ context.Context, _ string) error { return nil }
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// One sleep allows the loop to make exactly one pass, then we cancel.
	sleepCount := 0
	sleep := func(c context.Context, _ time.Duration) bool {
		sleepCount++
		if sleepCount >= 1 {
			cancel()
			return false
		}
		return c.Err() == nil
	}
	runQuotaRecoveryProbeLoop(ctx, store, probe, time.Minute, func() time.Time { return now }, sleep)

	state, _ := store.State("openai", now)
	assert.Equal(t, ProviderQuotaStateAvailable, state)
}

func TestExtractStatusCode(t *testing.T) {
	cases := map[string]int{
		"HTTP 502: Bad Gateway":     502,
		"HTTP 404: not found":       404,
		"HTTP 200: ok":              200,
		"HTTP 999: weird":           999,
		"nothing here":              0,
		"HTTP foo: oops":            0,
		"context deadline exceeded": 0,
	}
	for input, want := range cases {
		t.Run(input, func(t *testing.T) {
			assert.Equal(t, want, extractStatusCode(input))
		})
	}
}
