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

func TestRefreshStorm(t *testing.T) {
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

	started := make(chan error, 2)
	go func() {
		_, err := Warmup(context.Background(), cfg, cat, cache)
		started <- err
	}()
	go func() {
		_, err := Warmup(context.Background(), cfg, cat, cache)
		started <- err
	}()

	select {
	case <-refreshStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("expected one refresh request to start")
	}

	close(releaseRefresh)

	for i := 0; i < 2; i++ {
		select {
		case err := <-started:
			require.NoError(t, err)
		case <-time.After(2 * time.Second):
			t.Fatal("warmup did not complete")
		}
	}

	require.Equal(t, int32(1), atomic.LoadInt32(&requests), "refresh storm should coalesce to one live request")
}
