package runtimesignals_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/easel/fizeau/internal/discoverycache"
	"github.com/easel/fizeau/internal/runtimesignals"
	"github.com/stretchr/testify/require"
)

func TestWarmupCoalescesConcurrentRefreshes(t *testing.T) {
	cache := &discoverycache.Cache{Root: t.TempDir()}
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
		_, _ = w.Write([]byte(`{"data":[{"id":"warmup-model"}]}`))
	}))
	t.Cleanup(server.Close)

	cfg := runtimesignals.CollectInput{Type: "lmstudio", BaseURL: server.URL}

	firstErr := make(chan error, 1)
	go func() {
		_, err := runtimesignals.Warmup(context.Background(), cache, "studio", cfg)
		firstErr <- err
	}()

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("expected first warmup to reach /v1/models")
	}

	secondErr := make(chan error, 1)
	go func() {
		_, err := runtimesignals.Warmup(context.Background(), cache, "studio", cfg)
		secondErr <- err
	}()

	time.Sleep(50 * time.Millisecond)
	require.Equal(t, int32(1), atomic.LoadInt32(&requests), "warmup must stay coalesced while refresh is active")

	close(release)
	require.NoError(t, <-firstErr)
	require.NoError(t, <-secondErr)

	sig, ok := runtimesignals.ReadCached(cache, "studio")
	require.True(t, ok)
	require.NotNil(t, sig)
	require.Equal(t, runtimesignals.StatusAvailable, sig.Status)

	expected := filepath.Join(cache.Root, "runtime", "studio.json")
	require.FileExists(t, expected)
}

func TestWarmupNormalizesOpenAICompatibleBaseURL(t *testing.T) {
	cache := &discoverycache.Cache{Root: t.TempDir()}
	var requests int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			http.NotFound(w, r)
			return
		}
		atomic.AddInt32(&requests, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"warmup-model"}]}`))
	}))
	t.Cleanup(server.Close)

	cfg := runtimesignals.CollectInput{Type: "lmstudio", BaseURL: server.URL + "/v1"}
	_, err := runtimesignals.Warmup(context.Background(), cache, "studio", cfg)
	require.NoError(t, err)
	require.Equal(t, int32(1), atomic.LoadInt32(&requests), "warmup should request /v1/models, not /v1/v1/models")
}
