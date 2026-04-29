package openai_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	agent "github.com/DocumentDrivenDX/agent/internal/core"
	"github.com/DocumentDrivenDX/agent/internal/provider/openai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestChatStream_ClassifiesNotImplementedErrorAsCapabilityMissing replays the
// SSE error frame captured against an mlx_lm/lmstudio backend that surfaced
// "NotImplementedError: RotatingKVCache Quantization NYI" mid-stream
// (bead agent-a8915e01). The decoder must hand the agent loop a typed
// ProviderCapabilityMissingError carrying the stable code, the parsed
// capability name, and the original server message — not the opaque
// "received error while streaming: ..." string the openai-go SDK produces.
func TestChatStream_ClassifiesNotImplementedErrorAsCapabilityMissing(t *testing.T) {
	// Wire shape: one SSE data frame with a top-level "error" object. The
	// openai-go ssestream decoder picks up the "error" key via gjson and
	// emits the JSON value as part of "received error while streaming: <ep>".
	frame := `{"error":{"message":"Error in iterating prediction stream: NotImplementedError: RotatingKVCache Quantization NYI"}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		streamSSE(w, []string{frame})
	}))
	defer srv.Close()

	p := openai.New(openai.Config{
		BaseURL: srv.URL + "/v1",
		APIKey:  "test",
		Model:   "qwen3.5-7b",
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ch, err := p.ChatStream(ctx, []agent.Message{
		{Role: agent.RoleUser, Content: "hello"},
	}, nil, agent.Options{})
	require.NoError(t, err)

	var streamErr error
	for delta := range ch {
		if delta.Err != nil {
			streamErr = delta.Err
		}
	}
	require.Error(t, streamErr, "stream must surface the upstream NotImplementedError")

	var typed *agent.ProviderCapabilityMissingError
	require.True(t, errors.As(streamErr, &typed),
		"err must be a *ProviderCapabilityMissingError; got %T: %v", streamErr, streamErr)
	assert.Equal(t, agent.ProviderCapabilityMissingErrorCode, typed.Code)
	assert.Equal(t, "RotatingKVCache Quantization", typed.Capability,
		"capability extracted from server message must drop the trailing NYI sentinel")
	assert.Contains(t, typed.ServerMessage, "RotatingKVCache Quantization NYI",
		"server message must preserve the upstream text for telemetry")
	assert.True(t, errors.Is(streamErr, agent.ErrProviderCapabilityMissing),
		"errors.Is must match the exported sentinel for routing-layer checks")
	require.NotNil(t, typed.UnwrapCause(),
		"cause should preserve the underlying SDK error for chained debugging")
	assert.Contains(t, typed.UnwrapCause().Error(), "received error while streaming",
		"underlying SDK error must remain accessible via UnwrapCause")
}

// TestChatStream_ClassifiesGenericNotImplementedError covers AC item 1's
// "or any NotImplementedError from the provider" clause: capability names
// other than RotatingKVCache must also be classified, with the parsed
// capability captured for telemetry.
func TestChatStream_ClassifiesGenericNotImplementedError(t *testing.T) {
	frame := `{"error":{"message":"NotImplementedError: SlidingWindowAttention NYI"}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		streamSSE(w, []string{frame})
	}))
	defer srv.Close()

	p := openai.New(openai.Config{
		BaseURL: srv.URL + "/v1",
		APIKey:  "test",
		Model:   "qwen3.5-7b",
	})

	ch, err := p.ChatStream(context.Background(), []agent.Message{
		{Role: agent.RoleUser, Content: "hello"},
	}, nil, agent.Options{})
	require.NoError(t, err)

	var streamErr error
	for delta := range ch {
		if delta.Err != nil {
			streamErr = delta.Err
		}
	}
	require.Error(t, streamErr)

	var typed *agent.ProviderCapabilityMissingError
	require.True(t, errors.As(streamErr, &typed))
	assert.Equal(t, agent.ProviderCapabilityMissingErrorCode, typed.Code)
	assert.Equal(t, "SlidingWindowAttention", typed.Capability)
}

// TestChatStream_UnrelatedStreamErrorIsNotReclassified is the negative case:
// the classifier must leave non-NotImplementedError stream errors alone so
// existing transient-error retry paths (rate limits, network blips) keep
// working unchanged.
func TestChatStream_UnrelatedStreamErrorIsNotReclassified(t *testing.T) {
	frame := `{"error":{"message":"rate limit reached"}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		streamSSE(w, []string{frame})
	}))
	defer srv.Close()

	p := openai.New(openai.Config{
		BaseURL: srv.URL + "/v1",
		APIKey:  "test",
		Model:   "qwen3.5-7b",
	})

	ch, err := p.ChatStream(context.Background(), []agent.Message{
		{Role: agent.RoleUser, Content: "hello"},
	}, nil, agent.Options{})
	require.NoError(t, err)

	var streamErr error
	for delta := range ch {
		if delta.Err != nil {
			streamErr = delta.Err
		}
	}
	require.Error(t, streamErr)

	var typed *agent.ProviderCapabilityMissingError
	assert.False(t, errors.As(streamErr, &typed),
		"non-NotImplementedError stream errors must remain opaque so transient-retry paths are unaffected")
}

// TestProviderCapabilityMissingError_IsNotTransient locks down AC item 2:
// the routing layer's transient-error classifier must return false for any
// error that wraps ErrProviderCapabilityMissing, so the agent loop does not
// silently retry the same provider+model on a deterministic upstream
// rejection.
func TestProviderCapabilityMissingError_IsNotTransient(t *testing.T) {
	err := &agent.ProviderCapabilityMissingError{
		Code:          agent.ProviderCapabilityMissingErrorCode,
		Capability:    "RotatingKVCache Quantization",
		ServerMessage: "received error while streaming: NotImplementedError: RotatingKVCache Quantization NYI",
	}
	assert.False(t, agent.IsTransientError(err),
		"capability-missing errors must never be classified as transient")

	wrapped := errors.Join(errors.New("agent: provider error"), err)
	assert.False(t, agent.IsTransientError(wrapped),
		"capability-missing must remain non-transient through error wrapping")
}
