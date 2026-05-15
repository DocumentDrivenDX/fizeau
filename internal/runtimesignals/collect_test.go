package runtimesignals_test

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/easel/fizeau/internal/discoverycache"
	"github.com/easel/fizeau/internal/harnesses"
	"github.com/easel/fizeau/internal/runtimesignals"
)

// TestCollect_OpenRouter_RateLimitHeaders verifies AC2:
// Collect() against a mock OR provider with rate-limit headers parses
// QuotaRemaining correctly.
func TestCollect_OpenRouter_RateLimitHeaders(t *testing.T) {
	store := runtimesignals.NewStore()

	h := http.Header{}
	h.Set("x-ratelimit-remaining", "42")
	// OpenRouter uses Unix-ms for reset; set it one minute in the future.
	h.Set("x-ratelimit-reset", strconv.FormatInt(time.Now().Add(time.Minute).UnixMilli(), 10))

	store.RecordResponse("openrouter", h, 100*time.Millisecond, "openrouter")

	sig, err := store.Collect(context.Background(), "openrouter", runtimesignals.CollectInput{Type: "openrouter"})
	require.NoError(t, err)
	assert.Equal(t, runtimesignals.StatusAvailable, sig.Status)
	require.NotNil(t, sig.QuotaRemaining, "QuotaRemaining should be set from x-ratelimit-remaining")
	assert.Equal(t, 42, *sig.QuotaRemaining)
}

// TestCollect_OpenRouter_Exhausted verifies that a zero remaining-requests
// header drives StatusExhausted.
func TestCollect_OpenRouter_Exhausted(t *testing.T) {
	store := runtimesignals.NewStore()

	h := http.Header{}
	h.Set("x-ratelimit-remaining", "0")
	h.Set("x-ratelimit-reset", strconv.FormatInt(time.Now().Add(time.Minute).UnixMilli(), 10))

	store.RecordResponse("or-exhausted", h, 50*time.Millisecond, "openrouter")

	sig, err := store.Collect(context.Background(), "or-exhausted", runtimesignals.CollectInput{Type: "openrouter"})
	require.NoError(t, err)
	assert.Equal(t, runtimesignals.StatusExhausted, sig.Status)
	require.NotNil(t, sig.QuotaRemaining)
	assert.Equal(t, 0, *sig.QuotaRemaining)
}

// TestCollect_Claude_QuotaCache verifies AC3:
// Collect() against a mock claude harness reads the existing quota_cache.go data.
func TestCollect_Claude_QuotaCache(t *testing.T) {
	tmpDir := t.TempDir()
	cachePath := filepath.Join(tmpDir, "claude-quota.json")
	t.Setenv("FIZEAU_CLAUDE_QUOTA_CACHE", cachePath)

	snap := claudeQuotaCacheFixture{
		CapturedAt:        time.Now().UTC(),
		FiveHourRemaining: 500,
		FiveHourLimit:     1000,
		WeeklyRemaining:   5000,
		WeeklyLimit:       10000,
		Source:            "test",
	}
	writeClaudeQuotaCacheFixture(t, cachePath, snap)

	store := runtimesignals.NewStore()
	sig, err := store.Collect(context.Background(), "claude-subscription", runtimesignals.CollectInput{Type: "claude"})
	require.NoError(t, err)
	assert.Equal(t, runtimesignals.StatusAvailable, sig.Status)
	require.NotNil(t, sig.QuotaRemaining, "QuotaRemaining should reflect FiveHourRemaining")
	assert.Equal(t, 500, *sig.QuotaRemaining)
}

// TestCollect_Claude_Exhausted verifies that a zero 5h window drives StatusExhausted.
func TestCollect_Claude_Exhausted(t *testing.T) {
	tmpDir := t.TempDir()
	cachePath := filepath.Join(tmpDir, "claude-quota-exhausted.json")
	t.Setenv("FIZEAU_CLAUDE_QUOTA_CACHE", cachePath)

	snap := claudeQuotaCacheFixture{
		CapturedAt:        time.Now().UTC(),
		FiveHourRemaining: 0,
		FiveHourLimit:     1000,
		WeeklyRemaining:   5000,
		WeeklyLimit:       10000,
		Source:            "test",
	}
	writeClaudeQuotaCacheFixture(t, cachePath, snap)

	store := runtimesignals.NewStore()
	sig, err := store.Collect(context.Background(), "claude-subscription", runtimesignals.CollectInput{Type: "claude"})
	require.NoError(t, err)
	assert.Equal(t, runtimesignals.StatusExhausted, sig.Status)
}

// TestCollect_NoHeaders_Unknown verifies that a provider with no observed
// headers or quota cache is reported as StatusUnknown.
func TestCollect_NoHeaders_Unknown(t *testing.T) {
	store := runtimesignals.NewStore()

	sig, err := store.Collect(context.Background(), "unknown-provider", runtimesignals.CollectInput{Type: "openai"})
	require.NoError(t, err)
	assert.Equal(t, runtimesignals.StatusUnknown, sig.Status)
}

// TestCollect_LatencyRecorded verifies that a recorded latency appears in
// the collected signal's RecentP50Latency.
func TestCollect_LatencyRecorded(t *testing.T) {
	store := runtimesignals.NewStore()

	for _, d := range []time.Duration{10, 20, 30} {
		store.RecordResponse("latency-test", nil, d*time.Millisecond, "openai")
	}

	sig, err := store.Collect(context.Background(), "latency-test", runtimesignals.CollectInput{Type: "openai"})
	require.NoError(t, err)
	// Three samples: 10ms, 20ms, 30ms. P50 = sorted[3/2] = sorted[1] = 20ms.
	assert.Equal(t, 20*time.Millisecond, sig.RecentP50Latency)
}

// TestCacheRoundTrip verifies AC5:
// Runtime cache file path is runtime/<provider>.json and Signal data
// round-trips through M1's cache API without loss.
func TestCacheRoundTrip(t *testing.T) {
	cacheDir := t.TempDir()
	cache := &discoverycache.Cache{Root: cacheDir}

	remaining := 100
	resetAt := time.Now().UTC().Truncate(time.Second).Add(5 * time.Minute)
	sig := runtimesignals.Signal{
		Provider:         "test-provider",
		Status:           runtimesignals.StatusAvailable,
		QuotaRemaining:   &remaining,
		QuotaResetAt:     &resetAt,
		RecentP50Latency: 50 * time.Millisecond,
		RecordedAt:       time.Now().UTC().Truncate(time.Second),
	}

	require.NoError(t, runtimesignals.Write(cache, sig))

	// Verify the file lands at runtime/<provider>.json.
	expectedPath := filepath.Join(cacheDir, "runtime", "test-provider.json")
	require.FileExists(t, expectedPath, "cache file must be at runtime/<provider>.json")

	// Read back and verify the data round-trips intact.
	got, ok := runtimesignals.ReadCached(cache, "test-provider")
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

	got, ok := runtimesignals.ReadCached(cache, "nonexistent")
	assert.False(t, ok)
	assert.Nil(t, got)
}

type claudeQuotaCacheFixture struct {
	CapturedAt        time.Time               `json:"captured_at"`
	FiveHourRemaining int                     `json:"five_hour_remaining"`
	FiveHourLimit     int                     `json:"five_hour_limit"`
	WeeklyRemaining   int                     `json:"weekly_remaining"`
	WeeklyLimit       int                     `json:"weekly_limit"`
	Windows           []harnesses.QuotaWindow `json:"windows,omitempty"`
	Source            string                  `json:"source"`
}

func writeClaudeQuotaCacheFixture(t *testing.T, path string, snap claudeQuotaCacheFixture) {
	t.Helper()
	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		t.Fatalf("marshal claude quota cache: %v", err)
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir claude quota cache dir: %v", err)
	}
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o600); err != nil {
		t.Fatalf("write claude quota cache: %v", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		t.Fatalf("rename claude quota cache: %v", err)
	}
}
