package lmstudio

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	agent "github.com/easel/fizeau/internal/core"
	"github.com/easel/fizeau/internal/reasoning"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func lmStudioServer(loaded, max int) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/v0/models/") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":                    strings.TrimPrefix(r.URL.Path, "/api/v0/models/"),
			"loaded_context_length": loaded,
			"max_context_length":    max,
		})
	}))
}

func TestLookupModelLimits_PrefersLoadedContextLength(t *testing.T) {
	srv := lmStudioServer(100_000, 131_072)
	defer srv.Close()

	got := LookupModelLimits(context.Background(), srv.URL+"/v1", "qwen3.5-27b")
	assert.Equal(t, 100_000, got.ContextLength)
	assert.Equal(t, 0, got.MaxCompletionTokens)
}

func TestLookupModelLimits_FallsBackToMaxContextLength(t *testing.T) {
	srv := lmStudioServer(0, 131_072)
	defer srv.Close()

	got := LookupModelLimits(context.Background(), srv.URL+"/v1", "qwen3.5-27b")
	assert.Equal(t, 131_072, got.ContextLength)
}

func TestProtocolCapabilities(t *testing.T) {
	p := New(Config{BaseURL: "http://localhost:1234/v1"})
	assert.True(t, p.SupportsTools())
	assert.True(t, p.SupportsStream())
	assert.True(t, p.SupportsStructuredOutput())
	assert.False(t, p.SupportsThinking())
}

// bodyCapturingServer returns an httptest server that records the last
// /chat/completions request body and replies with a minimal success payload.
func bodyCapturingServer(t *testing.T, captured *[]byte) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		*captured = body
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"chatcmpl-1",
			"model":"qwen/qwen3.6-35b-a3b",
			"choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],
			"usage":{"prompt_tokens":3,"completion_tokens":1,"total_tokens":4}
		}`))
	}))
}

// LM Studio's OpenAI-compatible surface is intentionally classified as not
// supporting request-level reasoning control. Even for Qwen-family models, the
// provider must strip reasoning-control fields rather than advertising support
// the runtime cannot rely on.
func TestReasoningSerialization_QwenModelStripsReasoningControls(t *testing.T) {
	cases := []struct {
		name              string
		reasoning         agent.Reasoning
		wantErr           bool
		wantNoHTTPRequest bool
	}{
		{name: "off strips fields", reasoning: agent.ReasoningOff},
		{name: "low is rejected", reasoning: agent.ReasoningLow, wantErr: true, wantNoHTTPRequest: true},
		{name: "medium is rejected", reasoning: agent.ReasoningMedium, wantErr: true, wantNoHTTPRequest: true},
		{name: "numeric tokens rejected", reasoning: agent.ReasoningTokens(321), wantErr: true, wantNoHTTPRequest: true},
		{name: "unset omits fields", reasoning: agent.Reasoning("")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var captured []byte
			srv := bodyCapturingServer(t, &captured)
			defer srv.Close()

			p := New(Config{
				BaseURL: srv.URL + "/v1",
				APIKey:  "test",
				Model:   "qwen/qwen3.6-35b-a3b",
			})
			opts := agent.Options{Reasoning: tc.reasoning}
			_, err := p.Chat(context.Background(), []agent.Message{{Role: agent.RoleUser, Content: "hi"}}, nil, opts)
			if tc.wantErr {
				require.Error(t, err)
				if tc.wantNoHTTPRequest {
					assert.Nil(t, captured)
				}
				return
			}
			require.NoError(t, err)
			require.NotNil(t, captured)

			var body map[string]any
			require.NoError(t, json.Unmarshal(captured, &body))
			assert.NotContains(t, body, "enable_thinking")
			assert.NotContains(t, body, "thinking_budget")
			assert.NotContains(t, body, "thinking")
		})
	}
}

// TestReasoningSerialization_NonQwenModelStripsQwenControls verifies that
// non-Qwen models served by LM Studio do not carry Qwen-specific reasoning
// fields. This preserves the invariant that Qwen wire controls are only
// emitted for Qwen-family models.
func TestReasoningSerialization_NonQwenModelStripsQwenControls(t *testing.T) {
	var captured []byte
	srv := bodyCapturingServer(t, &captured)
	defer srv.Close()

	p := New(Config{
		BaseURL:   srv.URL + "/v1",
		APIKey:    "test",
		Model:     "google/gemma-3-27b",
		Reasoning: reasoning.ReasoningMedium,
	})
	_, err := p.Chat(context.Background(), []agent.Message{{Role: agent.RoleUser, Content: "hi"}}, nil, agent.Options{})
	require.NoError(t, err)
	require.NotNil(t, captured)

	var body map[string]any
	require.NoError(t, json.Unmarshal(captured, &body))
	assert.NotContains(t, body, "enable_thinking")
	assert.NotContains(t, body, "thinking_budget")
	assert.NotContains(t, body, "thinking")
}
