package openai_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	agent "github.com/easel/fizeau/internal/core"
	"github.com/easel/fizeau/internal/discoverycache"
	"github.com/easel/fizeau/internal/provider/openai"
	"github.com/easel/fizeau/internal/runtimesignals"
)

func TestMain(m *testing.M) {
	os.Exit(runOpenAIRuntimeSignalTests(m))
}

func runOpenAIRuntimeSignalTests(m *testing.M) int {
	tmp, err := os.MkdirTemp("", "openai-runtime-cache-*")
	if err != nil {
		panic(err)
	}

	os.Setenv("FIZEAU_CACHE_DIR", filepath.Join(tmp, "cache"))
	code := m.Run()
	_ = os.RemoveAll(tmp)
	return code
}

func TestChat_RuntimeSignalWritesAvailableCache(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("x-ratelimit-remaining-requests", "42")
		w.Header().Set("x-ratelimit-remaining-tokens", "100")
		w.Header().Set("x-ratelimit-reset-requests", "100ms")
		time.Sleep(10 * time.Millisecond)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":     "chatcmpl-1",
			"model":  "gpt-4o",
			"object": "chat.completion",
			"choices": []map[string]any{
				{
					"index": 0,
					"message": map[string]any{
						"role":    "assistant",
						"content": "done",
					},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]int{
				"prompt_tokens":     12,
				"completion_tokens": 5,
				"total_tokens":      17,
			},
		})
	}))
	defer srv.Close()

	p := openai.New(openai.Config{
		BaseURL: srv.URL + "/v1",
		APIKey:  "test",
		Model:   "gpt-4o",
	})

	resp, err := p.Chat(context.Background(), []agent.Message{{Role: agent.RoleUser, Content: "hello"}}, nil, agent.Options{})
	if err != nil {
		t.Fatalf("Chat returned error: %v", err)
	}
	if resp.Content != "done" {
		t.Fatalf("Content = %q, want done", resp.Content)
	}

	cacheRoot := filepath.Join(os.Getenv("FIZEAU_CACHE_DIR"))
	sig, ok := runtimesignals.ReadCached(&discoverycache.Cache{Root: cacheRoot}, "openai")
	if !ok {
		t.Fatal("expected runtime cache entry for openai")
	}
	if sig.Status != runtimesignals.StatusAvailable {
		t.Fatalf("Status = %q, want available", sig.Status)
	}
	if sig.QuotaRemaining == nil || *sig.QuotaRemaining != 42 {
		t.Fatalf("QuotaRemaining = %#v, want 42", sig.QuotaRemaining)
	}
	if sig.RecentP50Latency <= 0 {
		t.Fatalf("RecentP50Latency = %v, want > 0", sig.RecentP50Latency)
	}
	if sig.QuotaResetAt == nil {
		t.Fatal("expected quota reset time to be recorded")
	}
}

func TestChat_RuntimeSignalWritesExhaustedCacheOnFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("x-ratelimit-remaining-requests", "0")
		w.Header().Set("x-ratelimit-reset-requests", "1s")
		http.Error(w, `{"error":"boom"}`, http.StatusInternalServerError)
	}))
	defer srv.Close()

	p := openai.New(openai.Config{
		BaseURL: srv.URL + "/v1",
		APIKey:  "test",
		Model:   "gpt-4o",
	})

	_, err := p.Chat(context.Background(), []agent.Message{{Role: agent.RoleUser, Content: "hello"}}, nil, agent.Options{})
	if err == nil {
		t.Fatal("expected Chat to fail")
	}

	cacheRoot := filepath.Join(os.Getenv("FIZEAU_CACHE_DIR"))
	sig, ok := runtimesignals.ReadCached(&discoverycache.Cache{Root: cacheRoot}, "openai")
	if !ok {
		t.Fatal("expected runtime cache entry for openai")
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
