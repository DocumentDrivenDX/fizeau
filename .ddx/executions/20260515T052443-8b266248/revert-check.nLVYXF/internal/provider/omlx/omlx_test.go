package omlx

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	agent "github.com/easel/fizeau/internal/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func omlxServer(modelID string, maxContext, maxTokens int) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models/status" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"models": []map[string]any{
				{
					"id":                 modelID,
					"max_context_window": maxContext,
					"max_tokens":         maxTokens,
				},
			},
		})
	}))
}

func TestLookupModelLimits(t *testing.T) {
	srv := omlxServer("Qwen3.5-27B-4bit", 262_144, 32_768)
	defer srv.Close()

	got := LookupModelLimits(context.Background(), srv.URL+"/v1", "Qwen3.5-27B-4bit")
	assert.Equal(t, 262_144, got.ContextLength)
	assert.Equal(t, 32_768, got.MaxCompletionTokens)
}

func TestLookupModelLimits_ModelMatchIsCaseInsensitive(t *testing.T) {
	srv := omlxServer("Qwen3.5-27B-4bit", 262_144, 32_768)
	defer srv.Close()

	got := LookupModelLimits(context.Background(), srv.URL+"/v1", "qwen3.5-27b-4bit")
	assert.Equal(t, 262_144, got.ContextLength)
}

func TestLookupModelLimits_UnknownModelReturnsZero(t *testing.T) {
	srv := omlxServer("foo-model", 262_144, 32_768)
	defer srv.Close()

	got := LookupModelLimits(context.Background(), srv.URL+"/v1", "bar-model")
	assert.Zero(t, got.ContextLength)
	assert.Zero(t, got.MaxCompletionTokens)
}

func TestProtocolCapabilities(t *testing.T) {
	p := New(Config{BaseURL: "http://localhost:1235/v1"})
	assert.True(t, p.SupportsTools())
	assert.True(t, p.SupportsStream())
	assert.True(t, p.SupportsStructuredOutput())
	assert.True(t, p.SupportsThinking())
}

func TestChatStream_SurvivesKeepAliveCommentFrames(t *testing.T) {
	frames := []string{
		": keep-alive\n\n",
		`data: {"id":"chatcmpl-1","model":"qwen3","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}` + "\n\n",
		": keep-alive\n\n",
		`data: {"id":"chatcmpl-1","model":"qwen3","choices":[{"index":0,"delta":{"content":"warmup-done"},"finish_reason":null}]}` + "\n\n",
		`data: {"id":"chatcmpl-1","model":"qwen3","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":12,"completion_tokens":5,"total_tokens":17}}` + "\n\n",
		"data: [DONE]\n\n",
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		flusher, _ := w.(http.Flusher)
		for _, frame := range frames {
			_, _ = io.WriteString(w, frame)
			if flusher != nil {
				flusher.Flush()
			}
			time.Sleep(time.Millisecond)
		}
	}))
	defer srv.Close()

	p := New(Config{
		BaseURL: srv.URL + "/v1",
		APIKey:  "test",
		Model:   "qwen3",
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ch, err := p.ChatStream(ctx, []agent.Message{
		{Role: agent.RoleUser, Content: "hello"},
	}, nil, agent.Options{})
	require.NoError(t, err)

	var content string
	var streamErr error
	for delta := range ch {
		if delta.Err != nil {
			streamErr = delta.Err
		}
		content += delta.Content
	}

	require.NoError(t, streamErr, "omlx keep-alive SSE comment frames must not corrupt stream parsing")
	assert.Contains(t, content, "warmup-done")
}
