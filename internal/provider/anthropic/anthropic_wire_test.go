package anthropic_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	agent "github.com/DocumentDrivenDX/agent/internal/core"
	"github.com/DocumentDrivenDX/agent/internal/provider/anthropic"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// captureBody records the JSON body of the most recent /messages POST.
type capturedRequest struct {
	mu   sync.Mutex
	body map[string]any
}

func (c *capturedRequest) set(b map[string]any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.body = b
}

func (c *capturedRequest) get() map[string]any {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.body
}

func newJSONHandler(t *testing.T, captured *capturedRequest, respondWith func(w http.ResponseWriter, r *http.Request)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		var body map[string]any
		require.NoError(t, json.Unmarshal(raw, &body))
		captured.set(body)
		respondWith(w, r)
	}
}

func writeNonStreamingMessage(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{
		"id":"msg_wire",
		"type":"message",
		"role":"assistant",
		"model":"claude-sonnet-4-20250514",
		"content":[{"type":"text","text":"ok"}],
		"stop_reason":"end_turn",
		"usage":{"input_tokens":1,"output_tokens":1}
	}`))
}

func writeStreamingMessage(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	flusher, _ := w.(http.Flusher)
	events := []struct {
		name, data string
	}{
		{"message_start", `{"type":"message_start","message":{"id":"msg_wire","type":"message","role":"assistant","model":"claude-sonnet-4-20250514"},"usage":{"input_tokens":1}}`},
		{"content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`},
		{"content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"ok"}}`},
		{"content_block_stop", `{"type":"content_block_stop","index":0}`},
		{"message_delta", `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":1}}`},
		{"message_stop", `{"type":"message_stop"}`},
	}
	for _, e := range events {
		fmt.Fprintf(w, "event: %s\n", e.name)
		fmt.Fprintf(w, "data: %s\n\n", e.data)
		if flusher != nil {
			flusher.Flush()
		}
	}
}

// toolDefs returns three tool defs for the wire-body assertions.
func toolDefs() []agent.ToolDef {
	return []agent.ToolDef{
		{Name: "alpha", Description: "first", Parameters: json.RawMessage(`{"type":"object","properties":{}}`)},
		{Name: "bravo", Description: "second", Parameters: json.RawMessage(`{"type":"object","properties":{}}`)},
		{Name: "charlie", Description: "third", Parameters: json.RawMessage(`{"type":"object","properties":{}}`)},
	}
}

func messages() []agent.Message {
	return []agent.Message{
		{Role: agent.RoleSystem, Content: "system-one"},
		{Role: agent.RoleSystem, Content: "system-two"},
		{Role: agent.RoleUser, Content: "hi"},
	}
}

// assertCacheMarkers checks that the wire body contains cache_control:ephemeral
// at exactly the expected positions:
//   - tools[len-1] only
//   - system[len-1] only
func assertCacheMarkers(t *testing.T, body map[string]any) {
	t.Helper()

	tools, ok := body["tools"].([]any)
	require.True(t, ok, "tools should be array; body=%v", body)
	require.Len(t, tools, 3)
	for i, raw := range tools {
		tool := raw.(map[string]any)
		cc := tool["cache_control"]
		if i == len(tools)-1 {
			require.NotNil(t, cc, "last tool must have cache_control")
			assert.Equal(t, "ephemeral", cc.(map[string]any)["type"])
		} else {
			assert.Nil(t, cc, "tool %d should not have cache_control", i)
		}
	}

	system, ok := body["system"].([]any)
	require.True(t, ok, "system should be array; body=%v", body)
	require.Len(t, system, 2)
	for i, raw := range system {
		blk := raw.(map[string]any)
		cc := blk["cache_control"]
		if i == len(system)-1 {
			require.NotNil(t, cc, "last system block must have cache_control")
			assert.Equal(t, "ephemeral", cc.(map[string]any)["type"])
		} else {
			assert.Nil(t, cc, "system block %d should not have cache_control", i)
		}
	}
}

// countCacheControl walks the JSON body and counts occurrences of a
// cache_control object key (regardless of nesting / value).
func countCacheControl(v any) int {
	switch t := v.(type) {
	case map[string]any:
		n := 0
		for k, val := range t {
			if k == "cache_control" {
				n++
			}
			n += countCacheControl(val)
		}
		return n
	case []any:
		n := 0
		for _, val := range t {
			n += countCacheControl(val)
		}
		return n
	}
	return 0
}

func TestAnthropicWireBodyContainsCacheControlOnBothPaths(t *testing.T) {
	t.Run("Chat", func(t *testing.T) {
		captured := &capturedRequest{}
		srv := httptest.NewServer(newJSONHandler(t, captured, func(w http.ResponseWriter, r *http.Request) {
			writeNonStreamingMessage(w)
		}))
		defer srv.Close()

		p := anthropic.New(anthropic.Config{APIKey: "k", Model: "claude-sonnet-4-20250514", BaseURL: srv.URL})
		_, err := p.Chat(context.Background(), messages(), toolDefs(), agent.Options{})
		require.NoError(t, err)

		body := captured.get()
		require.NotNil(t, body)
		assertCacheMarkers(t, body)
	})

	t.Run("ChatStream", func(t *testing.T) {
		captured := &capturedRequest{}
		srv := httptest.NewServer(newJSONHandler(t, captured, func(w http.ResponseWriter, r *http.Request) {
			writeStreamingMessage(w)
		}))
		defer srv.Close()

		p := anthropic.New(anthropic.Config{APIKey: "k", Model: "claude-sonnet-4-20250514", BaseURL: srv.URL})
		ch, err := p.ChatStream(context.Background(), messages(), toolDefs(), agent.Options{})
		require.NoError(t, err)
		for range ch { // drain
		}

		body := captured.get()
		require.NotNil(t, body)
		assertCacheMarkers(t, body)
	})
}

func TestAnthropicCachePolicyOffEmitsNoCacheControl(t *testing.T) {
	t.Run("Chat", func(t *testing.T) {
		captured := &capturedRequest{}
		srv := httptest.NewServer(newJSONHandler(t, captured, func(w http.ResponseWriter, r *http.Request) {
			writeNonStreamingMessage(w)
		}))
		defer srv.Close()

		p := anthropic.New(anthropic.Config{APIKey: "k", Model: "claude-sonnet-4-20250514", BaseURL: srv.URL})
		_, err := p.Chat(context.Background(), messages(), toolDefs(), agent.Options{CachePolicy: "off"})
		require.NoError(t, err)

		body := captured.get()
		require.NotNil(t, body)
		assert.Equal(t, 0, countCacheControl(body),
			"CachePolicy=off must produce zero cache_control markers; body=%v", body)
	})

	t.Run("ChatStream", func(t *testing.T) {
		captured := &capturedRequest{}
		srv := httptest.NewServer(newJSONHandler(t, captured, func(w http.ResponseWriter, r *http.Request) {
			writeStreamingMessage(w)
		}))
		defer srv.Close()

		p := anthropic.New(anthropic.Config{APIKey: "k", Model: "claude-sonnet-4-20250514", BaseURL: srv.URL})
		ch, err := p.ChatStream(context.Background(), messages(), toolDefs(), agent.Options{CachePolicy: "off"})
		require.NoError(t, err)
		for range ch {
		}

		body := captured.get()
		require.NotNil(t, body)
		assert.Equal(t, 0, countCacheControl(body),
			"CachePolicy=off must produce zero cache_control markers; body=%v", body)
	})
}
