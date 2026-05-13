package modelregistry

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/easel/fizeau/internal/config"
	"github.com/easel/fizeau/internal/discoverycache"
	"github.com/easel/fizeau/internal/modelcatalog"
	"github.com/stretchr/testify/require"
)

func TestAssembleRefreshBackgroundCoalescesStaleRequests(t *testing.T) {
	t.Setenv("PATH", "")
	cache := &discoverycache.Cache{Root: t.TempDir()}
	capturedAt := time.Date(2026, 5, 12, 15, 0, 0, 0, time.UTC)

	var requests int32
	refreshStarted := make(chan struct{})
	releaseRefresh := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			http.NotFound(w, r)
			return
		}
		if atomic.AddInt32(&requests, 1) == 1 {
			close(refreshStarted)
		}
		<-releaseRefresh
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"fresh-model"}]}`))
	}))
	t.Cleanup(server.Close)
	source := endpointSourceName("studio", "alpha", server.URL+"/v1", "")
	writeDiscoveryFixture(t, cache, source, capturedAt, []string{"stale-model"})
	stalePath := filepath.Join(cache.Root, "discovery", source+".json")
	past := time.Now().Add(-2 * time.Hour)
	require.NoError(t, os.Chtimes(stalePath, past, past))

	cfg := &config.Config{Providers: map[string]config.ProviderConfig{
		"studio": {
			Type: "lmstudio",
			Endpoints: []config.ProviderEndpoint{{
				Name:    "alpha",
				BaseURL: server.URL + "/v1",
			}},
			Billing: string(modelcatalog.BillingModelFixed),
		},
	}}
	cat := loadTestCatalog(t)

	start := time.Now()
	snapshot, err := AssembleWithOptions(context.Background(), cfg, cat, cache, AssembleOptions{Refresh: RefreshBackground})
	require.NoError(t, err)
	require.Len(t, snapshot.Models, 1)
	require.Equal(t, "stale-model", snapshot.Models[0].ID)
	require.Less(t, time.Since(start), 100*time.Millisecond, "background refresh should not block stale read")

	select {
	case <-refreshStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("expected background refresh to reach /v1/models")
	}

	second, err := AssembleWithOptions(context.Background(), cfg, cat, cache, AssembleOptions{Refresh: RefreshBackground})
	require.NoError(t, err)
	require.Len(t, second.Models, 1)
	require.Equal(t, "stale-model", second.Models[0].ID)
	time.Sleep(50 * time.Millisecond)
	require.Equal(t, int32(1), atomic.LoadInt32(&requests), "second stale read must not launch another refresh")

	close(releaseRefresh)

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		fresh, err := AssembleWithOptions(context.Background(), cfg, cat, cache, AssembleOptions{Refresh: RefreshNone})
		require.NoError(t, err)
		if len(fresh.Models) == 1 && fresh.Models[0].ID == "fresh-model" {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}

	t.Fatal("background refresh did not replace stale rows with fresh rows")
}

func TestAssembleRefreshFailureRecordsRefreshFailedAndDoesNotRetryImmediately(t *testing.T) {
	t.Setenv("PATH", "")
	cache := &discoverycache.Cache{Root: t.TempDir()}
	capturedAt := time.Date(2026, 5, 12, 16, 0, 0, 0, time.UTC)

	var requests int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			http.NotFound(w, r)
			return
		}
		atomic.AddInt32(&requests, 1)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"boom"}`))
	}))
	t.Cleanup(server.Close)
	source := endpointSourceName("studio", "alpha", server.URL+"/v1", "")
	writeDiscoveryFixture(t, cache, source, capturedAt, []string{"stale-model"})
	stalePath := filepath.Join(cache.Root, "discovery", source+".json")
	past := time.Now().Add(-2 * time.Hour)
	require.NoError(t, os.Chtimes(stalePath, past, past))

	cfg := &config.Config{Providers: map[string]config.ProviderConfig{
		"studio": {
			Type: "lmstudio",
			Endpoints: []config.ProviderEndpoint{{
				Name:    "alpha",
				BaseURL: server.URL + "/v1",
			}},
			Billing: string(modelcatalog.BillingModelFixed),
		},
	}}
	cat := loadTestCatalog(t)

	first, err := AssembleWithOptions(context.Background(), cfg, cat, cache, AssembleOptions{Refresh: RefreshBackground})
	require.NoError(t, err)
	require.Len(t, first.Models, 1)
	require.Equal(t, "stale-model", first.Models[0].ID)

	stateSource := discoverycache.Source{Tier: "discovery", Name: source, TTL: time.Hour, RefreshDeadline: discoveryRefreshDeadlineHTTP}
	require.Eventually(t, func() bool {
		state, stateErr := cache.RefreshState(stateSource)
		return stateErr == nil && state.Failed
	}, 2*time.Second, 10*time.Millisecond)

	second, err := AssembleWithOptions(context.Background(), cfg, cat, cache, AssembleOptions{Refresh: RefreshBackground})
	require.NoError(t, err)
	require.Len(t, second.Models, 1)
	require.Equal(t, "stale-model", second.Models[0].ID)
	require.Contains(t, second.Sources[source].Error, "refresh_failed")

	time.Sleep(50 * time.Millisecond)
	require.Equal(t, int32(1), atomic.LoadInt32(&requests), "failed refresh must not immediately retry")
}

func TestAssembleRefreshForceAdvancesCapturedAt(t *testing.T) {
	t.Setenv("PATH", "")
	cache := &discoverycache.Cache{Root: t.TempDir()}
	oldCapturedAt := time.Date(2026, 5, 12, 14, 30, 0, 0, time.UTC)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"fresh-model"}]}`))
	}))
	t.Cleanup(server.Close)
	source := endpointSourceName("studio", "alpha", server.URL+"/v1", "")
	writeDiscoveryFixture(t, cache, source, oldCapturedAt, []string{"stale-model"})
	stalePath := filepath.Join(cache.Root, "discovery", source+".json")
	past := time.Now().Add(-2 * time.Hour)
	require.NoError(t, os.Chtimes(stalePath, past, past))

	cfg := &config.Config{Providers: map[string]config.ProviderConfig{
		"studio": {
			Type: "lmstudio",
			Endpoints: []config.ProviderEndpoint{{
				Name:    "alpha",
				BaseURL: server.URL + "/v1",
			}},
			Billing: string(modelcatalog.BillingModelFixed),
		},
	}}
	cat := loadTestCatalog(t)

	snapshot, err := AssembleWithOptions(context.Background(), cfg, cat, cache, AssembleOptions{Refresh: RefreshForce})
	require.NoError(t, err)
	require.Len(t, snapshot.Models, 1)
	require.Equal(t, "fresh-model", snapshot.Models[0].ID)
	require.True(t, snapshot.Models[0].DiscoveredAt.After(oldCapturedAt), "discovery refresh must advance captured_at metadata")
}

func TestRefreshModelsWarmupUsesLocks(t *testing.T) {
	t.Setenv("PATH", "")
	cache := &discoverycache.Cache{Root: t.TempDir()}
	oldCapturedAt := time.Date(2026, 5, 12, 13, 0, 0, 0, time.UTC)

	var requests int32
	started := make(chan struct{})
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			http.NotFound(w, r)
			return
		}
		if atomic.AddInt32(&requests, 1) == 1 {
			close(started)
		}
		<-release
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"warm-model"}]}`))
	}))
	t.Cleanup(server.Close)
	source := endpointSourceName("studio", "alpha", server.URL+"/v1", "")
	writeDiscoveryFixture(t, cache, source, oldCapturedAt, []string{"warm-model"})
	stalePath := filepath.Join(cache.Root, "discovery", source+".json")
	past := time.Now().Add(-2 * time.Hour)
	require.NoError(t, os.Chtimes(stalePath, past, past))

	cfg := &config.Config{Providers: map[string]config.ProviderConfig{
		"studio": {
			Type: "lmstudio",
			Endpoints: []config.ProviderEndpoint{{
				Name:    "alpha",
				BaseURL: server.URL + "/v1",
			}},
			Billing: string(modelcatalog.BillingModelFixed),
		},
	}}
	cat := loadTestCatalog(t)

	firstErr := make(chan error, 1)
	go func() {
		_, err := Warmup(context.Background(), cfg, cat, cache)
		firstErr <- err
	}()

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("expected warmup to reach /v1/models")
	}

	secondErr := make(chan error, 1)
	go func() {
		_, err := Warmup(context.Background(), cfg, cat, cache)
		secondErr <- err
	}()

	time.Sleep(50 * time.Millisecond)
	require.Equal(t, int32(1), atomic.LoadInt32(&requests), "warmup should coalesce through the refresh coordinator")

	close(release)
	require.NoError(t, <-firstErr)
	require.NoError(t, <-secondErr)

	snapshot, err := AssembleWithOptions(context.Background(), cfg, cat, cache, AssembleOptions{Refresh: RefreshNone})
	require.NoError(t, err)
	require.Len(t, snapshot.Models, 1)
	require.Equal(t, "warm-model", snapshot.Models[0].ID)
}
