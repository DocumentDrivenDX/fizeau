package anthropic_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	agent "github.com/easel/fizeau/internal/core"
	"github.com/easel/fizeau/internal/discoverycache"
	"github.com/easel/fizeau/internal/provider/anthropic"
	"github.com/easel/fizeau/internal/runtimesignals"
)

func TestMain(m *testing.M) {
	os.Exit(runAnthropicRuntimeSignalTests(m))
}

func runAnthropicRuntimeSignalTests(m *testing.M) int {
	tmp, err := os.MkdirTemp("", "anthropic-runtime-cache-*")
	if err != nil {
		panic(err)
	}

	os.Setenv("FIZEAU_CACHE_DIR", filepath.Join(tmp, "cache"))
	code := m.Run()
	_ = os.RemoveAll(tmp)
	return code
}

func TestProvider_Chat_RuntimeSignalWritesExhaustedCacheOnFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("anthropic-ratelimit-requests-remaining", "0")
		w.Header().Set("anthropic-ratelimit-requests-reset", time.Now().Add(time.Second).UTC().Format(time.RFC3339))
		http.Error(w, `{"type":"error","error":{"type":"rate_limit_error","message":"boom"}}`, http.StatusTooManyRequests)
	}))
	defer srv.Close()

	p := anthropic.New(anthropic.Config{
		APIKey:  "test-key",
		Model:   "claude-sonnet-4-20250514",
		BaseURL: srv.URL,
	})

	_, err := p.Chat(context.Background(), []agent.Message{
		{Role: agent.RoleUser, Content: "hello"},
	}, nil, agent.Options{})
	if err == nil {
		t.Fatal("expected Chat to fail")
	}

	cacheRoot := filepath.Join(os.Getenv("FIZEAU_CACHE_DIR"))
	sig, ok := runtimesignals.ReadCached(&discoverycache.Cache{Root: cacheRoot}, "anthropic")
	if !ok {
		t.Fatal("expected runtime cache entry for anthropic")
	}
	if sig.Status != runtimesignals.StatusExhausted {
		t.Fatalf("Status = %q, want exhausted", sig.Status)
	}
	if sig.QuotaRemaining == nil || *sig.QuotaRemaining != 0 {
		t.Fatalf("QuotaRemaining = %#v, want 0", sig.QuotaRemaining)
	}
	if sig.RecentP50Latency <= 0 {
		t.Fatalf("RecentP50Latency = %v, want > 0", sig.RecentP50Latency)
	}
}
