package openaicompat

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	agent "github.com/easel/fizeau/internal/core"
	"github.com/easel/fizeau/internal/provider/quotaheaders"
)

// TestQuotaHeaderMiddleware_WiresIntoDispatchPath verifies that the
// QuotaHeaderParser + QuotaSignalObserver pair gets called on every Chat
// response so the service-layer state machine can react to quota_exhausted
// signals. The test stands up a fake Chat Completions endpoint that emits
// OpenAI rate-limit headers and asserts the observer fires with the parsed
// signal.
func TestQuotaHeaderMiddleware_WiresIntoDispatchPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("x-ratelimit-remaining-requests", "0")
		w.Header().Set("x-ratelimit-remaining-tokens", "100")
		w.Header().Set("x-ratelimit-reset-requests", "30s")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"x","object":"chat.completion","model":"m",
			"choices":[{"index":0,"message":{"role":"assistant","content":"hi"},
			"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	defer srv.Close()

	var (
		mu       sync.Mutex
		captured []quotaheaders.Signal
	)
	client := NewClient(Config{
		BaseURL:           srv.URL,
		QuotaHeaderParser: quotaheaders.ParseOpenAI,
		QuotaSignalObserver: func(s quotaheaders.Signal) {
			mu.Lock()
			defer mu.Unlock()
			captured = append(captured, s)
		},
	})

	_, err := client.Chat(context.Background(), "m", []agent.Message{{Role: agent.RoleUser, Content: "hi"}}, nil, RequestOptions{})
	if err != nil {
		t.Fatalf("Chat returned error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(captured) == 0 {
		t.Fatal("expected QuotaSignalObserver to be called at least once")
	}
	sig := captured[0]
	if sig.RemainingRequests != 0 {
		t.Errorf("RemainingRequests = %d, want 0 (header parsed through middleware)", sig.RemainingRequests)
	}
	exhausted, retryAt := sig.IsExhausted(time.Now())
	if !exhausted {
		t.Error("zero remaining-requests should mark exhausted via middleware path")
	}
	if retryAt.IsZero() {
		t.Error("expected retryAt to be populated from x-ratelimit-reset-requests")
	}
}
