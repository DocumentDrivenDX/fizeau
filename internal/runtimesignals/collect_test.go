package runtimesignals

import (
	"context"
	"net/http"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/easel/fizeau/internal/discoverycache"
	"github.com/easel/fizeau/internal/harnesses"
	"github.com/easel/fizeau/internal/harnesses/harnesstest"
)

// TestCollect_OpenRouter_RateLimitHeaders verifies that Collect parses
// OpenRouter rate-limit headers into an available runtime signal.
func TestCollect_OpenRouter_RateLimitHeaders(t *testing.T) {
	store := NewStore()

	h := http.Header{}
	h.Set("x-ratelimit-remaining", "42")
	// OpenRouter uses Unix-ms for reset; set it one minute in the future.
	h.Set("x-ratelimit-reset", strconv.FormatInt(time.Now().Add(time.Minute).UnixMilli(), 10))

	store.RecordResponse("openrouter", h, 100*time.Millisecond, "openrouter")

	sig, err := store.Collect(context.Background(), "openrouter", CollectInput{Type: "openrouter"})
	require.NoError(t, err)
	assert.Equal(t, StatusAvailable, sig.Status)
	require.NotNil(t, sig.QuotaRemaining, "QuotaRemaining should be set from x-ratelimit-remaining")
	assert.Equal(t, 42, *sig.QuotaRemaining)
}

// TestCollect_OpenRouter_Exhausted verifies that a zero remaining-requests
// header drives StatusExhausted.
func TestCollect_OpenRouter_Exhausted(t *testing.T) {
	store := NewStore()

	h := http.Header{}
	h.Set("x-ratelimit-remaining", "0")
	h.Set("x-ratelimit-reset", strconv.FormatInt(time.Now().Add(time.Minute).UnixMilli(), 10))

	store.RecordResponse("or-exhausted", h, 50*time.Millisecond, "openrouter")

	sig, err := store.Collect(context.Background(), "or-exhausted", CollectInput{Type: "openrouter"})
	require.NoError(t, err)
	assert.Equal(t, StatusExhausted, sig.Status)
	require.NotNil(t, sig.QuotaRemaining)
	assert.Equal(t, 0, *sig.QuotaRemaining)
}

func TestCollect_QuotaHarnessAvailable(t *testing.T) {
	tests := []struct {
		name         string
		providerType string
		limitID      string
	}{
		{name: "claude", providerType: "claude", limitID: "session"},
		{name: "codex", providerType: "codex", limitID: "codex"},
		{name: "gemini", providerType: "gemini", limitID: "gemini-pro"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setHarnessByNameForTest(t, tt.providerType, harnesstest.NewSyntheticQuotaHarness(tt.providerType, harnesses.QuotaStatus{
				Source:            "test",
				CapturedAt:        time.Now().UTC(),
				Fresh:             true,
				State:             harnesses.QuotaOK,
				RoutingPreference: harnesses.RoutingPreferenceAvailable,
				Windows: []harnesses.QuotaWindow{{
					Name:          tt.limitID,
					LimitID:       tt.limitID,
					WindowMinutes: 300,
					UsedPercent:   25,
					State:         "ok",
				}},
			}, []string{tt.limitID}))

			store := NewStore()
			sig, err := store.Collect(context.Background(), tt.providerType+"-subscription", CollectInput{Type: tt.providerType})
			require.NoError(t, err)
			assert.Equal(t, StatusAvailable, sig.Status)
			assert.Nil(t, sig.QuotaRemaining)
		})
	}
}

func TestCollect_QuotaHarnessExhausted(t *testing.T) {
	tests := []struct {
		name         string
		providerType string
		limitID      string
	}{
		{name: "claude", providerType: "claude", limitID: "session"},
		{name: "codex", providerType: "codex", limitID: "codex"},
		{name: "gemini", providerType: "gemini", limitID: "gemini-pro"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setHarnessByNameForTest(t, tt.providerType, harnesstest.NewSyntheticQuotaHarness(tt.providerType, harnesses.QuotaStatus{
				Source:            "test",
				CapturedAt:        time.Now().UTC(),
				Fresh:             true,
				State:             harnesses.QuotaBlocked,
				RoutingPreference: harnesses.RoutingPreferenceBlocked,
				Windows: []harnesses.QuotaWindow{{
					Name:          tt.limitID,
					LimitID:       tt.limitID,
					WindowMinutes: 300,
					UsedPercent:   100,
					State:         "blocked",
				}},
			}, []string{tt.limitID}))

			store := NewStore()
			sig, err := store.Collect(context.Background(), tt.providerType+"-subscription", CollectInput{Type: tt.providerType})
			require.NoError(t, err)
			assert.Equal(t, StatusExhausted, sig.Status)
			require.NotNil(t, sig.QuotaRemaining)
			assert.Equal(t, 0, *sig.QuotaRemaining)
		})
	}
}

// TestCollect_NoHeaders_Unknown verifies that a provider with no observed
// headers or quota cache is reported as StatusUnknown.
func TestCollect_NoHeaders_Unknown(t *testing.T) {
	store := NewStore()

	sig, err := store.Collect(context.Background(), "unknown-provider", CollectInput{Type: "openai"})
	require.NoError(t, err)
	assert.Equal(t, StatusUnknown, sig.Status)
}

// TestCollect_LatencyRecorded verifies that a recorded latency appears in
// the collected signal's RecentP50Latency.
func TestCollect_LatencyRecorded(t *testing.T) {
	store := NewStore()

	for _, d := range []time.Duration{10, 20, 30} {
		store.RecordResponse("latency-test", nil, d*time.Millisecond, "openai")
	}

	sig, err := store.Collect(context.Background(), "latency-test", CollectInput{Type: "openai"})
	require.NoError(t, err)
	// Three samples: 10ms, 20ms, 30ms. P50 = sorted[3/2] = sorted[1] = 20ms.
	assert.Equal(t, 20*time.Millisecond, sig.RecentP50Latency)
}

// TestCacheRoundTrip verifies that Runtime cache file path is
// runtime/<provider>.json and Signal data round-trips through M1's cache
// API without loss.
func TestCacheRoundTrip(t *testing.T) {
	cacheDir := t.TempDir()
	cache := &discoverycache.Cache{Root: cacheDir}

	remaining := 100
	resetAt := time.Now().UTC().Truncate(time.Second).Add(5 * time.Minute)
	sig := Signal{
		Provider:         "test-provider",
		Status:           StatusAvailable,
		QuotaRemaining:   &remaining,
		QuotaResetAt:     &resetAt,
		RecentP50Latency: 50 * time.Millisecond,
		RecordedAt:       time.Now().UTC().Truncate(time.Second),
	}

	require.NoError(t, Write(cache, sig))

	// Verify the file lands at runtime/<provider>.json.
	expectedPath := filepath.Join(cacheDir, "runtime", "test-provider.json")
	require.FileExists(t, expectedPath, "cache file must be at runtime/<provider>.json")

	// Read back and verify the data round-trips intact.
	got, ok := ReadCached(cache, "test-provider")
	require.True(t, ok)
	require.NotNil(t, got)
	assert.Equal(t, sig.Provider, got.Provider)
	assert.Equal(t, sig.Status, got.Status)
	require.NotNil(t, got.QuotaRemaining)
	assert.Equal(t, *sig.QuotaRemaining, *got.QuotaRemaining)
	assert.Equal(t, sig.RecentP50Latency, got.RecentP50Latency)
	require.NotNil(t, got.QuotaResetAt)
	assert.WithinDuration(t, *sig.QuotaResetAt, *got.QuotaResetAt, time.Second)
}

// TestReadCached_Missing verifies that ReadCached returns (nil, false) when
// no cache entry exists for the named provider.
func TestReadCached_Missing(t *testing.T) {
	cacheDir := t.TempDir()
	cache := &discoverycache.Cache{Root: cacheDir}

	got, ok := ReadCached(cache, "nonexistent")
	assert.False(t, ok)
	assert.Nil(t, got)
}

func setHarnessByNameForTest(t *testing.T, name string, h harnesses.Harness) {
	t.Helper()

	prev := harnessByName
	harnessByName = func(requested string) harnesses.Harness {
		if requested == name {
			return h
		}
		if prev == nil {
			return nil
		}
		return prev(requested)
	}
	t.Cleanup(func() { harnessByName = prev })
}
