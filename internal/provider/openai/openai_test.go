package openai_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	agent "github.com/easel/fizeau/internal/core"
	"github.com/easel/fizeau/internal/provider/openai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testLogHandler captures slog records for assertion in tests.
type testLogHandler struct {
	mu      sync.Mutex
	records []slog.Record
}

func (h *testLogHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }
func (h *testLogHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	h.records = append(h.records, r.Clone())
	h.mu.Unlock()
	return nil
}
func (h *testLogHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }
func (h *testLogHandler) WithGroup(_ string) slog.Handler      { return h }
func (h *testLogHandler) Messages() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	msgs := make([]string, len(h.records))
	for i, r := range h.records {
		msgs[i] = r.Message
	}
	return msgs
}

// streamSSE writes a sequence of SSE data lines followed by a final [DONE] event.
func streamSSE(w http.ResponseWriter, events []string) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Transfer-Encoding", "chunked")
	flusher, _ := w.(http.Flusher)
	for _, ev := range events {
		fmt.Fprintf(w, "data: %s\n\n", ev)
		if flusher != nil {
			flusher.Flush()
		}
	}
	fmt.Fprintf(w, "data: [DONE]\n\n")
	if flusher != nil {
		flusher.Flush()
	}
}

// writeRawSSE lets a test emit arbitrary SSE framing including `:` comment
// frames (keep-alive probes) and inter-frame sleeps. `frames` are written in
// order; each string is written verbatim (the caller provides terminators),
// followed by a flush. A positive `sleep` inserts a wall-clock delay between
// frames so tests can reproduce the "long silence then data" shape that
// reasoning-model warmup produces.
func writeRawSSE(w http.ResponseWriter, frames []string, sleep time.Duration) {
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Transfer-Encoding", "chunked")
	flusher, _ := w.(http.Flusher)
	for _, f := range frames {
		_, _ = io.WriteString(w, f)
		if flusher != nil {
			flusher.Flush()
		}
		if sleep > 0 {
			time.Sleep(sleep)
		}
	}
}

// TestChatStream_ToolCallIndexIDMapping verifies that the OpenAI provider
// carries the tool call ID forward using the chunk index when OpenAI omits
// the ID on all but the first argument chunk.
func TestChatStream_ToolCallIndexIDMapping(t *testing.T) {
	// OpenAI streaming format: first chunk has id+name, subsequent chunks have
	// index but empty id, and carry argument fragments.
	chunks := []string{
		// chunk 0: tool call header — id and name present
		`{"id":"chatcmpl-1","model":"gpt-4o","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_abc","type":"function","function":{"name":"read","arguments":""}}]},"finish_reason":null}]}`,
		// chunk 1: first arg fragment — no id
		`{"id":"chatcmpl-1","model":"","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"path\":"}}]},"finish_reason":null}]}`,
		// chunk 2: second arg fragment — no id
		`{"id":"chatcmpl-1","model":"","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"main.go\"}"}}]},"finish_reason":null}]}`,
		// chunk 3: finish
		`{"id":"chatcmpl-1","model":"","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`,
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		streamSSE(w, chunks)
	}))
	defer srv.Close()

	p := openai.New(openai.Config{
		BaseURL: srv.URL + "/v1",
		APIKey:  "test",
		Model:   "gpt-4o",
	})

	ch, err := p.ChatStream(context.Background(), []agent.Message{
		{Role: agent.RoleUser, Content: "call the read tool"},
	}, nil, agent.Options{})
	require.NoError(t, err)

	// Drain the channel and collect all ToolCallArgs by ID
	argsByID := make(map[string]string)
	idNames := make(map[string]string)
	for delta := range ch {
		if delta.Err != nil {
			t.Fatalf("unexpected stream error: %v", delta.Err)
		}
		if delta.ToolCallID != "" {
			argsByID[delta.ToolCallID] += delta.ToolCallArgs
			if delta.ToolCallName != "" {
				idNames[delta.ToolCallID] = delta.ToolCallName
			}
		}
	}

	require.Contains(t, argsByID, "call_abc", "tool call ID must be present on all arg deltas")
	assert.Equal(t, `{"path":"main.go"}`, argsByID["call_abc"], "arguments must be assembled from all chunks")
	assert.Equal(t, "read", idNames["call_abc"])
}

// TestChatStream_SurvivesSSECommentFramesAndLongSilence reproduces the
// omlx/reasoning-model streaming defect tracked by bead agent-f237e07b.
//
// The real failure mode is:
//  1. Server sends a `: keep-alive\n\n` SSE comment frame while the reasoning
//     model warms up (several seconds before the first content frame arrives).
//  2. openai-go's ssestream decoder treats that comment's trailing blank line
//     as an event dispatch with empty Data. Stream.Next then tries to
//     json.Unmarshal empty bytes and surfaces "unexpected end of JSON input",
//     which propagates up as a user-visible error — even though the wire
//     stream is well-formed per the SSE spec (which requires empty-data
//     events to be silently ignored).
//
// This test reproduces the exact frame shape captured against a vidar-omlx
// server: a keep-alive comment first, then the role delta, then (after a
// silence) the first content delta. It asserts that the stream completes
// without error and delivers the content.
func TestChatStream_SurvivesSSECommentFramesAndLongSilence(t *testing.T) {
	// Frames mirror the wire capture from /tmp/vidar-omlx-wire2.jsonl:
	// ": keep-alive" comment, then a role delta, then content.
	frames := []string{
		": keep-alive\n\n",
		`data: {"id":"chatcmpl-1","model":"qwen3","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}` + "\n\n",
		": keep-alive\n\n",
		`data: {"id":"chatcmpl-1","model":"qwen3","choices":[{"index":0,"delta":{"content":"warmup-done"},"finish_reason":null}]}` + "\n\n",
		`data: {"id":"chatcmpl-1","model":"qwen3","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":12,"completion_tokens":5,"total_tokens":17}}` + "\n\n",
		"data: [DONE]\n\n",
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// A short inter-frame sleep is enough to exercise the per-chunk
		// arrival shape; we do not need a full 9s warmup to trigger the
		// decoder bug because the empty-event dispatch happens on the
		// first keep-alive frame regardless of timing.
		writeRawSSE(w, frames, 10*time.Millisecond)
	}))
	defer srv.Close()

	p := openai.New(openai.Config{
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

	require.NoError(t, streamErr, "keep-alive SSE comment frames must not corrupt stream parsing")
	assert.Contains(t, content, "warmup-done", "content delta that follows a keep-alive frame must still be delivered")
}

func TestChat_AttemptMetadataIncludesServerIdentityAndCacheUsage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"chatcmpl-1",
			"model":"gpt-4o",
			"choices":[{"index":0,"message":{"role":"assistant","content":"done"},"finish_reason":"stop"}],
			"usage":{
				"prompt_tokens":12,
				"completion_tokens":5,
				"total_tokens":17,
				"prompt_tokens_details":{"cached_tokens":3}
			}
		}`))
	}))
	defer srv.Close()

	parsed, err := url.Parse(srv.URL)
	require.NoError(t, err)

	p := openai.New(openai.Config{
		BaseURL: srv.URL + "/v1",
		APIKey:  "test",
		Model:   "gpt-4o",
	})

	resp, err := p.Chat(context.Background(), []agent.Message{
		{Role: agent.RoleUser, Content: "hello"},
	}, nil, agent.Options{})
	require.NoError(t, err)

	require.NotNil(t, resp.Attempt)
	assert.Equal(t, "openai", resp.Attempt.ProviderName)
	assert.Equal(t, "openai", resp.Attempt.ProviderSystem)
	assert.Equal(t, parsed.Hostname(), resp.Attempt.ServerAddress)
	assert.NotZero(t, resp.Attempt.ServerPort)
	assert.Equal(t, "gpt-4o", resp.Attempt.RequestedModel)
	assert.Equal(t, "gpt-4o", resp.Attempt.ResponseModel)
	assert.Equal(t, "gpt-4o", resp.Attempt.ResolvedModel)
	assert.Equal(t, 3, resp.Usage.CacheRead)
}

func TestChat_SingleAttemptPerCall(t *testing.T) {
	var requests int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requests, 1)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"boom"}`))
	}))
	defer srv.Close()

	p := openai.New(openai.Config{
		BaseURL: srv.URL + "/v1",
		APIKey:  "test",
		Model:   "gpt-4o",
	})

	_, err := p.Chat(context.Background(), []agent.Message{
		{Role: agent.RoleUser, Content: "hello"},
	}, nil, agent.Options{})
	require.Error(t, err)
	assert.Equal(t, int32(1), atomic.LoadInt32(&requests))
}

func TestChatStream_PartialContentPreservedWhenStreamErrors(t *testing.T) {
	chunks := []string{
		`{"id":"chatcmpl-1","model":"gpt-4o","choices":[{"index":0,"delta":{"content":"partial-response"},"finish_reason":null}]}`,
		`{"id":"chatcmpl-1","model":"gpt-4o","choices":[{"index":0,"delta":{"content":"oops"}`,
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		streamSSE(w, chunks)
	}))
	defer srv.Close()

	p := openai.New(openai.Config{
		BaseURL: srv.URL + "/v1",
		APIKey:  "test",
		Model:   "gpt-4o",
	})

	ch, err := p.ChatStream(context.Background(), []agent.Message{
		{Role: agent.RoleUser, Content: "stream"},
	}, nil, agent.Options{})
	require.NoError(t, err)

	var content string
	var streamErr error
	for delta := range ch {
		content += delta.Content
		if delta.Err != nil {
			streamErr = delta.Err
		}
	}

	assert.Contains(t, content, "partial-response")
	require.Error(t, streamErr)
}

func TestChat_UnreachableEndpointFailsQuicklyAndMentionsEndpoint(t *testing.T) {
	baseURL := "http://127.0.0.1:1/v1"
	p := openai.New(openai.Config{
		BaseURL: baseURL,
		APIKey:  "test",
		Model:   "gpt-4o",
	})

	ctx, cancel := context.WithTimeout(context.Background(), 800*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := p.Chat(ctx, []agent.Message{
		{Role: agent.RoleUser, Content: "hello"},
	}, nil, agent.Options{})
	elapsed := time.Since(start)

	require.Error(t, err)
	assert.Less(t, elapsed, 2*time.Second)
	assert.Contains(t, err.Error(), "openai:")
	assert.Contains(t, err.Error(), "127.0.0.1:1")
}

func TestChat_MissingAPIKeyFailsAtCallTime(t *testing.T) {
	var requests int32
	var authHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requests, 1)
		authHeader = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"invalid api key","type":"invalid_request_error"}}`))
	}))
	defer srv.Close()

	p := openai.New(openai.Config{
		BaseURL: srv.URL + "/v1",
		Model:   "gpt-4o",
	})

	_, err := p.Chat(context.Background(), []agent.Message{
		{Role: agent.RoleUser, Content: "hello"},
	}, nil, agent.Options{})

	require.Error(t, err)
	assert.Equal(t, int32(1), atomic.LoadInt32(&requests), "constructor should not fail; request should fail at call time")
	assert.Equal(t, "Bearer not-needed", authHeader)
	assert.Contains(t, err.Error(), "401")
}

func TestChat_ToolDefinitionsAreSentToAPI(t *testing.T) {
	var capturedBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		capturedBody = body
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"chatcmpl-1",
			"model":"gpt-4o",
			"choices":[{"index":0,"message":{"role":"assistant","content":"done"},"finish_reason":"stop"}],
			"usage":{"prompt_tokens":12,"completion_tokens":5,"total_tokens":17}
		}`))
	}))
	defer srv.Close()

	p := openai.New(openai.Config{
		BaseURL: srv.URL + "/v1",
		APIKey:  "test",
		Model:   "gpt-4o",
	})

	toolDefs := []agent.ToolDef{
		{
			Name:        "read",
			Description: "Read file contents",
			Parameters:  json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}`),
		},
		{
			Name:        "bash",
			Description: "Run shell commands",
			Parameters:  json.RawMessage(`{"type":"object","properties":{"command":{"type":"string"}},"required":["command"]}`),
		},
	}

	_, err := p.Chat(context.Background(), []agent.Message{
		{Role: agent.RoleUser, Content: "read the file"},
	}, toolDefs, agent.Options{})
	require.NoError(t, err)

	var reqBody map[string]interface{}
	require.NoError(t, json.Unmarshal(capturedBody, &reqBody))

	tools, ok := reqBody["tools"].([]interface{})
	require.True(t, ok, "request must include 'tools' array")
	assert.Len(t, tools, 2)

	first := tools[0].(map[string]interface{})["function"].(map[string]interface{})
	assert.Equal(t, "read", first["name"])
	assert.Equal(t, "Read file contents", first["description"])

	second := tools[1].(map[string]interface{})["function"].(map[string]interface{})
	assert.Equal(t, "bash", second["name"])
}

func TestChatStream_ToolDefinitionsAreSentToAPI(t *testing.T) {
	var capturedBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		capturedBody = body
		streamSSE(w, []string{
			`{"id":"chatcmpl-1","model":"gpt-4o","choices":[{"index":0,"delta":{"content":"done"},"finish_reason":"stop"}],"usage":{"prompt_tokens":12,"completion_tokens":5,"total_tokens":17}}`,
		})
	}))
	defer srv.Close()

	p := openai.New(openai.Config{
		BaseURL: srv.URL + "/v1",
		APIKey:  "test",
		Model:   "gpt-4o",
	})

	toolDefs := []agent.ToolDef{
		{
			Name:        "read",
			Description: "Read file contents",
			Parameters:  json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}}}`),
		},
	}

	ch, err := p.ChatStream(context.Background(), []agent.Message{
		{Role: agent.RoleUser, Content: "read the file"},
	}, toolDefs, agent.Options{})
	require.NoError(t, err)
	for range ch { /* drain */
	}

	var reqBody map[string]interface{}
	require.NoError(t, json.Unmarshal(capturedBody, &reqBody))

	tools, ok := reqBody["tools"].([]interface{})
	require.True(t, ok, "streaming request must include 'tools' array")
	assert.Len(t, tools, 1)

	fn := tools[0].(map[string]interface{})["function"].(map[string]interface{})
	assert.Equal(t, "read", fn["name"])
	assert.Equal(t, "Read file contents", fn["description"])
}

func TestThinkingSerializationReasoningPolicy(t *testing.T) {
	tests := []struct {
		name              string
		configReasoning   agent.Reasoning
		opts              agent.Options
		wantThinking      bool
		wantBudget        int
		wantErr           bool
		wantNoHTTPRequest bool
	}{
		{
			name:            "unset preserves provider config",
			configReasoning: agent.ReasoningTokens(8192),
			wantThinking:    true,
			wantBudget:      8192,
		},
		{
			name:            "explicit off suppresses provider config",
			configReasoning: agent.ReasoningTokens(8192),
			opts:            agent.Options{Reasoning: agent.ReasoningOff},
		},
		{
			name:            "numeric zero suppresses provider config",
			configReasoning: agent.ReasoningTokens(8192),
			opts:            agent.Options{Reasoning: agent.ReasoningTokens(0)},
		},
		{
			name:            "explicit request wins over provider default",
			configReasoning: agent.ReasoningTokens(8192),
			opts:            agent.Options{Reasoning: agent.ReasoningTokens(1234)},
			wantThinking:    true,
			wantBudget:      1234,
		},
		{
			name:         "low maps to portable budget",
			opts:         agent.Options{Reasoning: agent.ReasoningLow},
			wantThinking: true,
			wantBudget:   2048,
		},
		{
			name:         "medium maps to portable budget",
			opts:         agent.Options{Reasoning: agent.ReasoningMedium},
			wantThinking: true,
			wantBudget:   8192,
		},
		{
			name:         "high maps to portable budget",
			opts:         agent.Options{Reasoning: agent.ReasoningHigh},
			wantThinking: true,
			wantBudget:   32768,
		},
		{
			name:         "numeric tokens pass through",
			opts:         agent.Options{Reasoning: agent.ReasoningTokens(4321)},
			wantThinking: true,
			wantBudget:   4321,
		},
		{
			name:              "unsupported extended value fails before request",
			opts:              agent.Options{Reasoning: agent.ReasoningXHigh},
			wantErr:           true,
			wantNoHTTPRequest: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name+"/chat", func(t *testing.T) {
			body, err := captureOpenAIChatBody(t, "thinking-map", tt.configReasoning, tt.opts)
			if tt.wantErr {
				require.Error(t, err)
				if tt.wantNoHTTPRequest {
					assert.Nil(t, body)
				}
				return
			}
			require.NoError(t, err)
			assertReasoningWireBudget(t, body, tt.wantThinking, tt.wantBudget)
		})
		t.Run(tt.name+"/stream", func(t *testing.T) {
			body, err := captureOpenAIStreamBody(t, "thinking-map", tt.configReasoning, tt.opts)
			if tt.wantErr {
				require.Error(t, err)
				if tt.wantNoHTTPRequest {
					assert.Nil(t, body)
				}
				return
			}
			require.NoError(t, err)
			assertReasoningWireBudget(t, body, tt.wantThinking, tt.wantBudget)
		})
	}
}

func TestReasoningSerializationUnsupportedProviders(t *testing.T) {
	for _, providerType := range []string{"openai", "ollama"} {
		t.Run(providerType+"/default provider budget drops", func(t *testing.T) {
			body, err := captureOpenAIChatBody(t, providerType, agent.ReasoningTokens(8192), agent.Options{})
			require.NoError(t, err)
			assertReasoningWireBudget(t, body, false, 0)
		})
		t.Run(providerType+"/explicit request fails before serialization", func(t *testing.T) {
			body, err := captureOpenAIChatBody(t, providerType, "", agent.Options{Reasoning: agent.ReasoningLow})
			require.Error(t, err)
			assert.Nil(t, body)
		})
	}
}

func TestOpenRouterReasoningSerialization(t *testing.T) {
	tests := []struct {
		name          string
		opts          agent.Options
		wantEffort    string
		wantMaxTokens int
	}{
		{
			name:       "medium maps to nested effort",
			opts:       agent.Options{Reasoning: agent.ReasoningMedium},
			wantEffort: "medium",
		},
		{
			name:       "explicit off maps to effort none",
			opts:       agent.Options{Reasoning: agent.ReasoningOff},
			wantEffort: "none",
		},
		{
			name:       "max maps to xhigh effort",
			opts:       agent.Options{Reasoning: agent.ReasoningMax},
			wantEffort: "xhigh",
		},
		{
			name:          "numeric budget maps to max_tokens",
			opts:          agent.Options{Reasoning: agent.ReasoningTokens(4321)},
			wantMaxTokens: 4321,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name+"/chat", func(t *testing.T) {
			body, err := captureOpenAIChatBody(t, "openrouter", "", tt.opts)
			require.NoError(t, err)
			assertOpenRouterReasoningWire(t, body, tt.wantEffort, tt.wantMaxTokens)
		})
		t.Run(tt.name+"/stream", func(t *testing.T) {
			body, err := captureOpenAIStreamBody(t, "openrouter", "", tt.opts)
			require.NoError(t, err)
			assertOpenRouterReasoningWire(t, body, tt.wantEffort, tt.wantMaxTokens)
		})
	}
}

func TestQwenReasoningSerialization(t *testing.T) {
	tests := []struct {
		name              string
		configReasoning   agent.Reasoning
		opts              agent.Options
		wantEnabled       bool
		wantBudget        int
		wantAbsent        bool
		wantErr           bool
		wantNoHTTPRequest bool
	}{
		{
			name:       "unset omits qwen reasoning fields",
			wantAbsent: true,
		},
		{
			name:        "low maps to qwen thinking budget",
			opts:        agent.Options{Reasoning: agent.ReasoningLow},
			wantEnabled: true,
			wantBudget:  2048,
		},
		{
			name:            "provider default sends qwen thinking budget",
			configReasoning: agent.ReasoningMedium,
			wantEnabled:     true,
			wantBudget:      8192,
		},
		{
			name:        "high maps to qwen thinking budget",
			opts:        agent.Options{Reasoning: agent.ReasoningHigh},
			wantEnabled: true,
			wantBudget:  32768,
		},
		{
			name:        "numeric budget maps to qwen thinking budget",
			opts:        agent.Options{Reasoning: agent.ReasoningTokens(4321)},
			wantEnabled: true,
			wantBudget:  4321,
		},
		{
			name:            "explicit off disables qwen thinking",
			configReasoning: agent.ReasoningMedium,
			opts:            agent.Options{Reasoning: agent.ReasoningOff},
			wantEnabled:     false,
			wantBudget:      0,
		},
		{
			name:              "unsupported extended value fails before request",
			opts:              agent.Options{Reasoning: agent.ReasoningXHigh},
			wantErr:           true,
			wantNoHTTPRequest: true,
		},
	}
	for _, tt := range tests {
		t.Run("omlx/"+tt.name+"/chat", func(t *testing.T) {
			body, err := captureOpenAIChatBody(t, "omlx", tt.configReasoning, tt.opts)
			if tt.wantErr {
				require.Error(t, err)
				if tt.wantNoHTTPRequest {
					assert.Nil(t, body)
				}
				return
			}
			require.NoError(t, err)
			if tt.wantAbsent {
				assertNoQwenReasoningWire(t, body)
				return
			}
			assertQwenReasoningWireBudget(t, body, tt.wantEnabled, tt.wantBudget)
		})
		t.Run("omlx/"+tt.name+"/stream", func(t *testing.T) {
			body, err := captureOpenAIStreamBody(t, "omlx", tt.configReasoning, tt.opts)
			if tt.wantErr {
				require.Error(t, err)
				if tt.wantNoHTTPRequest {
					assert.Nil(t, body)
				}
				return
			}
			require.NoError(t, err)
			if tt.wantAbsent {
				assertNoQwenReasoningWire(t, body)
				return
			}
			assertQwenReasoningWireBudget(t, body, tt.wantEnabled, tt.wantBudget)
		})
	}
}

// TestQwenReasoningSerializationRejectsNonQwenModels covers strict providers
// (OMLX): a Qwen-wire provider that only hosts Qwen models must fail the
// request when an explicit reasoning policy is sent against a non-Qwen
// model, so misconfiguration surfaces loudly instead of silently sending a
// control the template will ignore.
func TestQwenReasoningSerializationRejectsNonQwenModels(t *testing.T) {
	for _, opts := range []agent.Options{
		{Reasoning: agent.ReasoningMedium},
		{Reasoning: agent.ReasoningOff},
	} {
		t.Run(string(opts.Reasoning)+"/chat", func(t *testing.T) {
			body, err := captureOpenAIChatBodyWithModel(t, "omlx", "gpt-oss-20b-MXFP4-Q8", "", opts)
			require.Error(t, err)
			assert.Nil(t, body)
			assert.Contains(t, err.Error(), "qwen reasoning control")
			assert.Contains(t, err.Error(), "gpt-oss-20b")
		})
		t.Run(string(opts.Reasoning)+"/stream", func(t *testing.T) {
			body, err := captureOpenAIStreamBodyWithModel(t, "omlx", "gpt-oss-20b-MXFP4-Q8", "", opts)
			require.Error(t, err)
			assert.Nil(t, body)
			assert.Contains(t, err.Error(), "qwen reasoning control")
			assert.Contains(t, err.Error(), "gpt-oss-20b")
		})
	}
}

// TestQwenReasoningSerializationLMStudioMixedFamilyPolicy covers LM Studio's
// current contract: explicit "off" is allowed and strips Qwen-specific wire
// fields for non-Qwen models, but higher reasoning controls are rejected
// because LM Studio is not a verified request-level reasoning-control surface.
func TestQwenReasoningSerializationLMStudioMixedFamilyPolicy(t *testing.T) {
	t.Run("off/chat", func(t *testing.T) {
		body, err := captureOpenAIChatBodyWithModel(t, "lmstudio", "google/gemma-3-27b", "", agent.Options{Reasoning: agent.ReasoningOff})
		require.NoError(t, err)
		assertNoQwenReasoningWire(t, body)
	})
	t.Run("off/stream", func(t *testing.T) {
		body, err := captureOpenAIStreamBodyWithModel(t, "lmstudio", "google/gemma-3-27b", "", agent.Options{Reasoning: agent.ReasoningOff})
		require.NoError(t, err)
		assertNoQwenReasoningWire(t, body)
	})
	for _, opts := range []agent.Options{
		{Reasoning: agent.ReasoningMedium},
		{Reasoning: agent.ReasoningTokens(4321)},
	} {
		t.Run(string(opts.Reasoning)+"/chat", func(t *testing.T) {
			body, err := captureOpenAIChatBodyWithModel(t, "lmstudio", "google/gemma-3-27b", "", opts)
			require.Error(t, err)
			assert.Nil(t, body)
			assert.Contains(t, err.Error(), "provider type \"lmstudio\"")
		})
		t.Run(string(opts.Reasoning)+"/stream", func(t *testing.T) {
			body, err := captureOpenAIStreamBodyWithModel(t, "lmstudio", "google/gemma-3-27b", "", opts)
			require.Error(t, err)
			assert.Nil(t, body)
			assert.Contains(t, err.Error(), "provider type \"lmstudio\"")
		})
	}
}

func TestSamplingOptionsSerialization(t *testing.T) {
	temperature := 0.25
	opts := agent.Options{Temperature: &temperature, Seed: 12345}

	t.Run("chat", func(t *testing.T) {
		body, err := captureOpenAIChatBody(t, "openai", "", opts)
		require.NoError(t, err)
		assertSamplingWireOptions(t, body, temperature, 12345)
	})

	t.Run("stream", func(t *testing.T) {
		body, err := captureOpenAIStreamBody(t, "openai", "", opts)
		require.NoError(t, err)
		assertSamplingWireOptions(t, body, temperature, 12345)
	})
}

// TestSamplingProfilePassesThroughSeam pins ADR-007 §3 v1 behavior: the
// seeded "code" profile values flow unchanged to the wire regardless of
// the reasoning state. The provider seam is the architectural home for
// future (model_family × reasoning_state × profile) clipping rules; v1
// ships without one because the seeded code-profile values happen to be
// safe in both thinking and non-thinking states for Qwen3.x. When this
// test starts asserting different values across reasoning states, the
// seam-side rule has been added and ADR-007 §3 should be revisited.
func TestSamplingProfilePassesThroughSeam(t *testing.T) {
	temp := 0.6
	topP := 0.95
	topK := 20
	codeProfile := agent.Options{
		Temperature: &temp,
		TopP:        &topP,
		TopK:        &topK,
	}

	cases := []struct {
		name      string
		reasoning agent.Reasoning
	}{
		{"reasoning_off", agent.ReasoningOff},
		{"reasoning_unset", ""},
		{"reasoning_low", agent.ReasoningLow},
		{"reasoning_high", agent.ReasoningHigh},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opts := codeProfile
			opts.Reasoning = tc.reasoning
			body, err := captureOpenAIChatBody(t, "omlx", "", opts)
			require.NoError(t, err)
			var got map[string]any
			require.NoError(t, json.Unmarshal(body, &got))

			// All five sampler fields land on the wire as-supplied,
			// regardless of reasoning state. JSON numbers decode as float64.
			gotTemp, ok := got["temperature"].(float64)
			require.True(t, ok, "temperature must be present")
			assert.InDelta(t, 0.6, gotTemp, 1e-9)

			gotTopP, ok := got["top_p"].(float64)
			require.True(t, ok, "top_p must be present")
			assert.InDelta(t, 0.95, gotTopP, 1e-9)

			gotTopK, ok := got["top_k"].(float64)
			require.True(t, ok, "top_k must be present")
			assert.Equal(t, 20.0, gotTopK)

			// Unset fields stay unset — the v1 contract for "leave-unset
			// → server default applies".
			_, hasMinP := got["min_p"]
			assert.False(t, hasMinP, "min_p was nil → must not appear on wire")
			_, hasRep := got["repetition_penalty"]
			assert.False(t, hasRep, "repetition_penalty was nil → must not appear on wire")
		})
	}
}

func TestOpenAIProviderDoesNotSendNonStandardSamplingExtras(t *testing.T) {
	temp := 0.6
	topP := 0.95
	topK := 20
	minP := 0.05
	rep := 1.1
	opts := agent.Options{
		Temperature:       &temp,
		TopP:              &topP,
		TopK:              &topK,
		MinP:              &minP,
		RepetitionPenalty: &rep,
	}
	body, err := captureOpenAIChatBody(t, "openai", "", opts)
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(body, &got))

	gotTemp, ok := got["temperature"].(float64)
	require.True(t, ok, "temperature must be present")
	assert.InDelta(t, 0.6, gotTemp, 1e-9)
	gotTopP, ok := got["top_p"].(float64)
	require.True(t, ok, "top_p must be present")
	assert.InDelta(t, 0.95, gotTopP, 1e-9)

	_, hasTopK := got["top_k"]
	assert.False(t, hasTopK, "OpenAI rejects non-standard top_k")
	_, hasMinP := got["min_p"]
	assert.False(t, hasMinP, "OpenAI rejects non-standard min_p")
	_, hasRep := got["repetition_penalty"]
	assert.False(t, hasRep, "OpenAI rejects non-standard repetition_penalty")
}

func TestNativeOpenAIGPT5DoesNotSendUnsupportedSamplingControls(t *testing.T) {
	temp := 0.0
	topP := 0.95
	topK := 20
	minP := 0.05
	rep := 1.1
	opts := agent.Options{
		Temperature:       &temp,
		TopP:              &topP,
		TopK:              &topK,
		MinP:              &minP,
		RepetitionPenalty: &rep,
	}
	body, err := captureOpenAIChatBodyWithModel(t, "openai", "gpt-5.5", "", opts)
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(body, &got))

	_, hasTemp := got["temperature"]
	assert.False(t, hasTemp, "native OpenAI GPT-5 models reject non-default temperature")
	_, hasTopP := got["top_p"]
	assert.False(t, hasTopP, "native OpenAI GPT-5 models reject non-default top_p")
	_, hasTopK := got["top_k"]
	assert.False(t, hasTopK, "OpenAI rejects non-standard top_k")
	_, hasMinP := got["min_p"]
	assert.False(t, hasMinP, "OpenAI rejects non-standard min_p")
	_, hasRep := got["repetition_penalty"]
	assert.False(t, hasRep, "OpenAI rejects non-standard repetition_penalty")
}

func TestOpenRouterGPT5KeepsOpenAICompatSamplingControls(t *testing.T) {
	temp := 0.0
	topP := 0.95
	topK := 20
	minP := 0.05
	rep := 1.1
	opts := agent.Options{
		Temperature:       &temp,
		TopP:              &topP,
		TopK:              &topK,
		MinP:              &minP,
		RepetitionPenalty: &rep,
	}
	body, err := captureOpenAIChatBodyWithModel(t, "openrouter", "openai/gpt-5.5", "", opts)
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(body, &got))

	gotTemp, ok := got["temperature"].(float64)
	require.True(t, ok, "OpenRouter should keep temperature on OpenAI-compatible wire")
	assert.InDelta(t, 0.0, gotTemp, 1e-9)
	gotTopP, ok := got["top_p"].(float64)
	require.True(t, ok, "OpenRouter should keep top_p on OpenAI-compatible wire")
	assert.InDelta(t, 0.95, gotTopP, 1e-9)
	gotTopK, ok := got["top_k"].(float64)
	require.True(t, ok, "OpenRouter should keep top_k on OpenAI-compatible wire")
	assert.Equal(t, 20.0, gotTopK)
	gotMinP, ok := got["min_p"].(float64)
	require.True(t, ok, "OpenRouter should keep min_p on OpenAI-compatible wire")
	assert.InDelta(t, 0.05, gotMinP, 1e-9)
	gotRep, ok := got["repetition_penalty"].(float64)
	require.True(t, ok, "OpenRouter should keep repetition_penalty on OpenAI-compatible wire")
	assert.InDelta(t, 1.1, gotRep, 1e-9)
}

func captureOpenAIChatBody(t *testing.T, providerType string, providerReasoning agent.Reasoning, opts agent.Options) ([]byte, error) {
	return captureOpenAIChatBodyWithModel(t, providerType, testModelForProvider(providerType), providerReasoning, opts)
}

func captureOpenAIChatBodyWithModel(t *testing.T, providerType string, model string, providerReasoning agent.Reasoning, opts agent.Options) ([]byte, error) {
	t.Helper()
	var capturedBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		capturedBody = body
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"chatcmpl-1",
			"model":"gpt-4o",
			"choices":[{"index":0,"message":{"role":"assistant","content":"done"},"finish_reason":"stop"}],
			"usage":{"prompt_tokens":12,"completion_tokens":5,"total_tokens":17}
		}`))
	}))
	defer srv.Close()

	p := openai.New(openai.Config{
		BaseURL:        srv.URL + "/v1",
		APIKey:         "test",
		Model:          model,
		ProviderSystem: providerType,
		Capabilities:   capabilitiesForTestProvider(providerType),
		Reasoning:      providerReasoning,
	})
	_, err := p.Chat(context.Background(), []agent.Message{{Role: agent.RoleUser, Content: "hello"}}, nil, opts)
	return capturedBody, err
}

func captureOpenAIStreamBody(t *testing.T, providerType string, providerReasoning agent.Reasoning, opts agent.Options) ([]byte, error) {
	return captureOpenAIStreamBodyWithModel(t, providerType, testModelForProvider(providerType), providerReasoning, opts)
}

func captureOpenAIStreamBodyWithModel(t *testing.T, providerType string, model string, providerReasoning agent.Reasoning, opts agent.Options) ([]byte, error) {
	t.Helper()
	var capturedBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		capturedBody = body
		streamSSE(w, []string{
			`{"id":"chatcmpl-1","model":"gpt-4o","choices":[{"index":0,"delta":{"content":"done"},"finish_reason":"stop"}],"usage":{"prompt_tokens":12,"completion_tokens":5,"total_tokens":17}}`,
		})
	}))
	defer srv.Close()

	p := openai.New(openai.Config{
		BaseURL:        srv.URL + "/v1",
		APIKey:         "test",
		Model:          model,
		ProviderSystem: providerType,
		Capabilities:   capabilitiesForTestProvider(providerType),
		Reasoning:      providerReasoning,
	})
	ch, err := p.ChatStream(context.Background(), []agent.Message{{Role: agent.RoleUser, Content: "hello"}}, nil, opts)
	if err != nil {
		return capturedBody, err
	}
	for delta := range ch {
		if delta.Err != nil {
			return capturedBody, delta.Err
		}
	}
	return capturedBody, nil
}

func testModelForProvider(providerType string) string {
	switch providerType {
	case "omlx":
		return "Qwen3.6-27B-MLX-8bit"
	case "lmstudio":
		return "qwen/qwen3.6-35b-a3b"
	case "thinking-map":
		return "anthropic-compat-claude"
	}
	return "gpt-4o"
}

func capabilitiesForTestProvider(providerType string) *openai.ProtocolCapabilities {
	caps := openai.OpenAIProtocolCapabilities
	switch providerType {
	case "omlx":
		caps.Thinking = true
		caps.ThinkingFormat = openai.ThinkingWireFormatQwen
		caps.StrictThinkingModelMatch = true
	case "openrouter":
		caps.Thinking = true
		caps.ThinkingFormat = openai.ThinkingWireFormatOpenRouter
	case "ollama":
		caps.StructuredOutput = false
	case "thinking-map":
		caps.Thinking = true
		caps.ThinkingFormat = openai.ThinkingWireFormatThinkingMap
	}
	return &caps
}

func assertReasoningWireBudget(t *testing.T, body []byte, wantThinking bool, wantBudget int) {
	t.Helper()
	require.NotNil(t, body)
	var reqBody map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &reqBody))
	thinking, ok := reqBody["thinking"].(map[string]interface{})
	if !wantThinking {
		assert.False(t, ok, "request body must not include thinking: %s", string(body))
		return
	}
	require.True(t, ok, "request body must include thinking: %s", string(body))
	assert.Equal(t, "enabled", thinking["type"])
	assert.Equal(t, float64(wantBudget), thinking["budget_tokens"])
}

func assertOpenRouterReasoningWire(t *testing.T, body []byte, wantEffort string, wantMaxTokens int) {
	t.Helper()
	require.NotNil(t, body)
	var reqBody map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &reqBody))
	reasoning, ok := reqBody["reasoning"].(map[string]interface{})
	require.True(t, ok, "request body must include reasoning: %s", string(body))
	if wantEffort != "" {
		assert.Equal(t, wantEffort, reasoning["effort"])
		assert.NotContains(t, reasoning, "max_tokens")
	} else {
		assert.Equal(t, float64(wantMaxTokens), reasoning["max_tokens"])
		assert.NotContains(t, reasoning, "effort")
	}
	assert.NotContains(t, reqBody, "thinking")
	assert.NotContains(t, reqBody, "reasoning_effort")
}

func assertQwenReasoningWireBudget(t *testing.T, body []byte, wantEnabled bool, wantBudget int) {
	t.Helper()
	require.NotNil(t, body)
	var reqBody map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &reqBody))
	assert.NotContains(t, reqBody, "enable_thinking", "qwen controls must use chat_template_kwargs envelope: %s", string(body))
	assert.NotContains(t, reqBody, "thinking_budget", "qwen controls must use chat_template_kwargs envelope: %s", string(body))
	ctk, ok := reqBody["chat_template_kwargs"].(map[string]interface{})
	require.True(t, ok, "request body must include chat_template_kwargs: %s", string(body))
	assert.Equal(t, wantEnabled, ctk["enable_thinking"])
	if wantEnabled {
		assert.Equal(t, float64(wantBudget), ctk["thinking_budget"])
	}
	if _, ok := reqBody["thinking"]; ok {
		t.Fatalf("qwen reasoning controls must not use thinking map: %s", string(body))
	}
}

func assertNoQwenReasoningWire(t *testing.T, body []byte) {
	t.Helper()
	require.NotNil(t, body)
	var reqBody map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &reqBody))
	assert.NotContains(t, reqBody, "enable_thinking")
	assert.NotContains(t, reqBody, "thinking_budget")
	assert.NotContains(t, reqBody, "thinking")
	assert.NotContains(t, reqBody, "reasoning")
	assert.NotContains(t, reqBody, "chat_template_kwargs")
}

func assertOpenAIEffortReasoningWire(t *testing.T, body []byte, wantEffort string) {
	t.Helper()
	require.NotNil(t, body)
	var reqBody map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &reqBody))
	gotEffort, ok := reqBody["reasoning_effort"].(string)
	require.True(t, ok, "request body must include reasoning_effort string: %s", string(body))
	assert.Equal(t, wantEffort, gotEffort)
	assert.NotContains(t, reqBody, "thinking")
	assert.NotContains(t, reqBody, "think")
	assert.NotContains(t, reqBody, "enable_thinking")
}

func assertOpenAIEffortOffWire(t *testing.T, body []byte) {
	t.Helper()
	require.NotNil(t, body)
	var reqBody map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &reqBody))
	thinkVal, ok := reqBody["think"].(bool)
	require.True(t, ok, "request body must include think: false: %s", string(body))
	assert.False(t, thinkVal)
	assert.NotContains(t, reqBody, "reasoning_effort")
	assert.NotContains(t, reqBody, "thinking")
}

func assertSamplingWireOptions(t *testing.T, body []byte, wantTemperature float64, wantSeed int64) {
	t.Helper()
	require.NotNil(t, body)
	var reqBody map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &reqBody))
	assert.Equal(t, wantTemperature, reqBody["temperature"])
	assert.Equal(t, float64(wantSeed), reqBody["seed"])
}

func TestNew_BaseURLControlsEndpointMetadataOnly(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
		host    string
		port    int
	}{
		{
			name:    "lmstudio default local endpoint",
			baseURL: "http://localhost:1234/v1",
			host:    "localhost",
			port:    1234,
		},
		{
			name:    "ollama compatible endpoint",
			baseURL: "http://127.0.0.1:11434/v1",
			host:    "127.0.0.1",
			port:    11434,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := openai.New(openai.Config{
				BaseURL: tt.baseURL,
				Model:   "gpt-4o",
			})
			system, host, port := p.ChatStartMetadata()
			assert.Equal(t, "openai", system)
			assert.Equal(t, tt.host, host)
			assert.Equal(t, tt.port, port)
		})
	}
}

func TestNew_ExplicitProviderSystemControlsIdentity(t *testing.T) {
	p := openai.New(openai.Config{
		BaseURL:        "http://vidar:1234/v1",
		Model:          "qwen",
		ProviderName:   "studio",
		ProviderSystem: "lmstudio",
	})

	system, host, port := p.ChatStartMetadata()
	assert.Equal(t, "lmstudio", system)
	assert.Equal(t, "vidar", host)
	assert.Equal(t, 1234, port)
}

// TestOpenAIRespectsReasoningWireModelID verifies that a model whose catalog
// reasoning_wire is "model_id" (the model name encodes the reasoning level —
// e.g. fixed-variant Qwen3.6 plus) does NOT receive a reasoning field on the
// wire even when the caller passes Reasoning=high. The upstream endpoint for
// such models cannot honor an external reasoning toggle, so emitting the
// field would be silently ignored or rejected by the provider.
func TestOpenAIRespectsReasoningWireModelID(t *testing.T) {
	const model = "qwen/qwen3.6-plus"
	body, err := captureOpenAIChatBodyWithReasoningWire(t, model, map[string]string{
		model: "model_id",
	}, agent.Options{Reasoning: agent.ReasoningHigh})
	require.NoError(t, err)
	require.NotNil(t, body)
	var reqBody map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &reqBody))
	assert.NotContains(t, reqBody, "reasoning", "reasoning_wire=model_id must strip reasoning field: %s", string(body))
	assert.NotContains(t, reqBody, "thinking")
	assert.NotContains(t, reqBody, "enable_thinking")
	assert.NotContains(t, reqBody, "thinking_budget")
}

// TestOpenAIRespectsReasoningWireProvider verifies that a model whose catalog
// reasoning_wire is "provider" preserves the existing OpenRouter wire
// behavior: the nested reasoning object is emitted with the requested effort.
func TestOpenAIRespectsReasoningWireProvider(t *testing.T) {
	const model = "anthropic/claude-sonnet-4.6"
	body, err := captureOpenAIChatBodyWithReasoningWire(t, model, map[string]string{
		model: "provider",
	}, agent.Options{Reasoning: agent.ReasoningHigh})
	require.NoError(t, err)
	require.NotNil(t, body)
	var reqBody map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &reqBody))
	reasoning, ok := reqBody["reasoning"].(map[string]interface{})
	require.True(t, ok, "reasoning_wire=provider must emit reasoning field: %s", string(body))
	assert.Equal(t, "high", reasoning["effort"])
}

// TestOpenAIRejectsReasoningWireNoneWithLevel verifies that a model whose
// catalog reasoning_wire is "none" (no reasoning surface at all) fails
// pre-flight when the caller asks for an explicit non-off reasoning level —
// surfacing the catalog/wire mismatch instead of silently dropping the
// request.
func TestOpenAIRejectsReasoningWireNoneWithLevel(t *testing.T) {
	const model = "no-reasoning-model"
	body, err := captureOpenAIChatBodyWithReasoningWire(t, model, map[string]string{
		model: "none",
	}, agent.Options{Reasoning: agent.ReasoningHigh})
	require.Error(t, err)
	assert.Nil(t, body, "request must not be sent when reasoning_wire=none rejects the call")
	assert.Contains(t, err.Error(), "reasoning_wire=none")
	assert.Contains(t, err.Error(), model)
}

// captureOpenAIChatBodyWithReasoningWire constructs an openrouter-style
// provider (Thinking=true, OpenRouter wire format) with the given per-model
// reasoning_wire metadata and returns the captured request body.
func captureOpenAIChatBodyWithReasoningWire(t *testing.T, model string, modelWire map[string]string, opts agent.Options) ([]byte, error) {
	t.Helper()
	var capturedBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		capturedBody = body
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"chatcmpl-1",
			"model":"` + model + `",
			"choices":[{"index":0,"message":{"role":"assistant","content":"done"},"finish_reason":"stop"}],
			"usage":{"prompt_tokens":12,"completion_tokens":5,"total_tokens":17}
		}`))
	}))
	defer srv.Close()

	caps := openai.OpenAIProtocolCapabilities
	caps.Thinking = true
	caps.ThinkingFormat = openai.ThinkingWireFormatOpenRouter

	p := openai.New(openai.Config{
		BaseURL:            srv.URL + "/v1",
		APIKey:             "test",
		Model:              model,
		ProviderSystem:     "openrouter",
		Capabilities:       &caps,
		ModelReasoningWire: modelWire,
	})
	_, err := p.Chat(context.Background(), []agent.Message{{Role: agent.RoleUser, Content: "hello"}}, nil, opts)
	return capturedBody, err
}

// captureOpenAIChatBodyWithReasoningWireQwen constructs a Qwen-style provider
// (ThinkingWireFormatQwen) with the given per-model reasoning_wire metadata,
// an optional logger, and returns the captured request body.
func captureOpenAIChatBodyWithReasoningWireQwen(t *testing.T, model string, modelWire map[string]string, logger *slog.Logger, opts agent.Options) ([]byte, error) {
	t.Helper()
	var capturedBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		capturedBody = body
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"chatcmpl-1",
			"model":"` + model + `",
			"choices":[{"index":0,"message":{"role":"assistant","content":"done"},"finish_reason":"stop"}],
			"usage":{"prompt_tokens":12,"completion_tokens":5,"total_tokens":17}
		}`))
	}))
	defer srv.Close()

	caps := openai.OpenAIProtocolCapabilities
	caps.Thinking = true
	caps.ThinkingFormat = openai.ThinkingWireFormatQwen

	p := openai.New(openai.Config{
		BaseURL:            srv.URL + "/v1",
		APIKey:             "test",
		Model:              model,
		ProviderSystem:     "omlx",
		Capabilities:       &caps,
		ModelReasoningWire: modelWire,
		Logger:             logger,
	})
	_, err := p.Chat(context.Background(), []agent.Message{{Role: agent.RoleUser, Content: "hello"}}, nil, opts)
	return capturedBody, err
}

// TestOpenRouterReasoningWireCatalog verifies the five wire-form table cases
// from the bead spec and the backwards-compat case.
func TestOpenRouterReasoningWireCatalog(t *testing.T) {
	const model = "qwen/qwen3-235b-a22b"

	tests := []struct {
		name          string
		opts          agent.Options
		wire          string
		wantEffort    string
		wantMaxTokens int
	}{
		// AC1: low + tokens wire → max_tokens: 2048
		{
			name:          "low+tokens wire emits max_tokens 2048",
			opts:          agent.Options{Reasoning: agent.ReasoningLow},
			wire:          "tokens",
			wantMaxTokens: 2048,
		},
		// AC2: 4096 tokens + effort wire → effort: "medium" (round-up on tie)
		{
			name:       "4096tokens+effort wire emits effort medium",
			opts:       agent.Options{Reasoning: agent.ReasoningTokens(4096)},
			wire:       "effort",
			wantEffort: "medium",
		},
		// AC3: high + effort wire → effort: "high"
		{
			name:       "high+effort wire emits effort high",
			opts:       agent.Options{Reasoning: agent.ReasoningHigh},
			wire:       "effort",
			wantEffort: "high",
		},
		// AC4 (backwards-compat): high + provider wire → effort: "high" (no change)
		{
			name:       "high+provider wire emits effort high (backwards-compat)",
			opts:       agent.Options{Reasoning: agent.ReasoningHigh},
			wire:       "provider",
			wantEffort: "high",
		},
		// AC4 (backwards-compat): high + unset wire → effort: "high" (no change)
		{
			name:       "high+unset wire emits effort high (backwards-compat)",
			opts:       agent.Options{Reasoning: agent.ReasoningHigh},
			wire:       "",
			wantEffort: "high",
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			wireMap := map[string]string{}
			if tt.wire != "" {
				wireMap[model] = tt.wire
			}
			body, err := captureOpenAIChatBodyWithReasoningWire(t, model, wireMap, tt.opts)
			require.NoError(t, err)
			assertOpenRouterReasoningWire(t, body, tt.wantEffort, tt.wantMaxTokens)
		})
	}
}

// TestQwenReasoningWireCatalogEffortDegrades verifies AC5: a Qwen-format
// provider whose model is catalog-flagged as wire=effort degrades to budget
// wire with the nearest PortableBudgets tier, and emits a structured warning.
func TestQwenReasoningWireCatalogEffortDegrades(t *testing.T) {
	const model = "Qwen3.6-27B-MLX-8bit"
	logHandler := &testLogHandler{}
	logger := slog.New(logHandler)

	body, err := captureOpenAIChatBodyWithReasoningWireQwen(t, model, map[string]string{
		model: "effort",
	}, logger, agent.Options{Reasoning: agent.ReasoningTokens(4096)})
	require.NoError(t, err)
	// 4096 snaps to medium tier → thinking_budget: 8192
	assertQwenReasoningWireBudget(t, body, true, 8192)

	// Verify the structured warning was emitted.
	msgs := logHandler.Messages()
	require.NotEmpty(t, msgs, "expected a structured warning log for effort wire on Qwen provider")
	assert.Contains(t, msgs[0], "catalog declares effort wire but provider only supports budget wire")
}

// TestQwenReasoningWireCatalogTokensPassthrough verifies that wire=tokens on a
// Qwen-format provider behaves identically to the default (no degradation, no
// warning), satisfying the backwards-compat requirement for Qwen+tokens.
func TestQwenReasoningWireCatalogTokensPassthrough(t *testing.T) {
	const model = "Qwen3.6-27B-MLX-8bit"
	logHandler := &testLogHandler{}
	logger := slog.New(logHandler)

	body, err := captureOpenAIChatBodyWithReasoningWireQwen(t, model, map[string]string{
		model: "tokens",
	}, logger, agent.Options{Reasoning: agent.ReasoningHigh})
	require.NoError(t, err)
	assertQwenReasoningWireBudget(t, body, true, 32768)

	assert.Empty(t, logHandler.Messages(), "wire=tokens on Qwen must not emit a warning")
}

// captureOpenAIChatBodyWithOpenAIEffort constructs an OpenAIEffort-format provider
// (ThinkingWireFormatOpenAIEffort) and returns the captured request body.
func captureOpenAIChatBodyWithOpenAIEffort(t *testing.T, model string, opts agent.Options) ([]byte, error) {
	t.Helper()
	var capturedBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		capturedBody = body
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"chatcmpl-1",
			"model":"` + model + `",
			"choices":[{"index":0,"message":{"role":"assistant","content":"done"},"finish_reason":"stop"}],
			"usage":{"prompt_tokens":12,"completion_tokens":5,"total_tokens":17}
		}`))
	}))
	defer srv.Close()

	caps := openai.OpenAIProtocolCapabilities
	caps.Thinking = true
	caps.ThinkingFormat = openai.ThinkingWireFormatOpenAIEffort

	p := openai.New(openai.Config{
		BaseURL:        srv.URL + "/v1",
		APIKey:         "test",
		Model:          model,
		ProviderSystem: "ds4",
		Capabilities:   &caps,
	})
	_, err := p.Chat(context.Background(), []agent.Message{{Role: agent.RoleUser, Content: "hello"}}, nil, opts)
	return capturedBody, err
}

// TestQwenReasoningWireChatTemplateKwargsEnvelope verifies that the Qwen wire format
// emits enable_thinking and thinking_budget inside a chat_template_kwargs envelope,
// not at the top level. This is the sindri llama-server fix from the 2026-05-11 probe:
// top-level enable_thinking/thinking_budget are silently dropped by llama-server,
// but the chat_template_kwargs envelope activates thinking (250-526 completion tokens
// with extracted reasoning_content).
func TestQwenReasoningWireChatTemplateKwargsEnvelope(t *testing.T) {
	const model = "Qwen3.6-27B-UD-Q3_K_XL.gguf"

	t.Run("on/emits chat_template_kwargs envelope", func(t *testing.T) {
		body, err := captureOpenAIChatBodyWithReasoningWireQwen(t, model, nil, nil, agent.Options{Reasoning: agent.ReasoningTokens(4096)})
		require.NoError(t, err)
		assertQwenReasoningWireBudget(t, body, true, 4096)
	})

	t.Run("off/emits chat_template_kwargs.enable_thinking=false", func(t *testing.T) {
		body, err := captureOpenAIChatBodyWithReasoningWireQwen(t, model, nil, nil, agent.Options{Reasoning: agent.ReasoningOff})
		require.NoError(t, err)
		assertQwenReasoningWireBudget(t, body, false, 0)
	})
}

// TestOpenAIEffortReasoningWire verifies the flat top-level reasoning_effort wire
// emitted by OpenAIEffort-format providers (ds4 / deepseek-v4-flash).
func TestOpenAIEffortReasoningWire(t *testing.T) {
	const model = "deepseek-v4-flash"

	t.Run("high/emits reasoning_effort:high", func(t *testing.T) {
		body, err := captureOpenAIChatBodyWithOpenAIEffort(t, model, agent.Options{Reasoning: agent.ReasoningHigh})
		require.NoError(t, err)
		assertOpenAIEffortReasoningWire(t, body, "high")
	})

	t.Run("off/emits think:false", func(t *testing.T) {
		body, err := captureOpenAIChatBodyWithOpenAIEffort(t, model, agent.Options{Reasoning: agent.ReasoningOff})
		require.NoError(t, err)
		assertOpenAIEffortOffWire(t, body)
	})

	t.Run("tokens=4096/snaps to medium via NearestTierForTokens", func(t *testing.T) {
		body, err := captureOpenAIChatBodyWithOpenAIEffort(t, model, agent.Options{Reasoning: agent.ReasoningTokens(4096)})
		require.NoError(t, err)
		assertOpenAIEffortReasoningWire(t, body, "medium")
	})
}
