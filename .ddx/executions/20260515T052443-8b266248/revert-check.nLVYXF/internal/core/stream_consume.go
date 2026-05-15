package core

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// DefaultReasoningByteLimit is the default maximum number of bytes of pure
// reasoning_content allowed before the stream is aborted with
// ErrReasoningOverflow. Only fires when no content or tool_call delta has been
// seen yet. Configurable via config.yaml reasoning_byte_limit; 0 = unlimited.
const DefaultReasoningByteLimit = 256 * 1024 // 256KB

// DefaultReasoningStallTimeout is the fallback stall deadline used when no
// reasoning budget is available to drive the adaptive computation. Sized as
// the worst-case absolute ceiling: max reasoning budget (32 768 tokens) at
// the slowest plausible local inference rate (2 tok/s) = 16 384 s (~4.5 h).
// The adaptive mechanism in consumeStream computes a tighter, budget-aware
// deadline for any run where the reasoning budget is known.
const DefaultReasoningStallTimeout = 16384 * time.Second

// DefaultReasoningTailBytes is the default size of the reasoning-tail buffer
// captured for inclusion in ReasoningStallError. Sized to roughly the last
// 500 reasoning tokens at ~4 chars/token. Configurable via
// streamThresholds.reasoningTailBytes; 0 falls back to this default.
const DefaultReasoningTailBytes = 2000

// streamThresholds holds the configurable reasoning-loop detection thresholds
// passed into consumeStream.
type streamThresholds struct {
	reasoningByteLimit    int
	reasoningStallTimeout time.Duration
	// reasoningTailBytes bounds the rolling buffer of reasoning_content
	// retained for the ReasoningStallError. Zero means
	// DefaultReasoningTailBytes; negative disables the buffer.
	reasoningTailBytes int
	modelName          string // included in error messages
	// promptID is included in the reasoning.stall event payload and the
	// returned ReasoningStallError so callers can correlate the stall to a
	// specific prompt/turn. May be empty.
	promptID string
	// reasoningBudgetTokens is the thinking_budget sent to the model for this
	// turn. When non-zero, the stall deadline is extended forward as reasoning
	// tokens arrive: remaining_budget_bytes / observed_bytes_per_sec * 2x,
	// ensuring slow local providers are not cut off before the budget is spent.
	// The static reasoningStallTimeout acts as a floor.
	reasoningBudgetTokens int
}

// consumeStream reads from a StreamingProvider's channel, emits delta events,
// and assembles a complete Response. The thresholds parameter controls
// reasoning-loop detection limits; use streamThresholds{} for defaults.
func consumeStream(
	ctx context.Context,
	sp StreamingProvider,
	messages []Message,
	tools []ToolDef,
	opts Options,
	callback EventCallback,
	sessionID string,
	streamStart time.Time,
	seq *int,
	thresholds streamThresholds,
) (Response, error) {
	ch, err := sp.ChatStream(ctx, messages, tools, opts)
	if err != nil {
		return Response{}, err
	}

	var resp Response
	var contentBuf strings.Builder
	var firstOutputAt time.Time
	var lastOutputAt time.Time

	// 0 means unlimited for both thresholds (same pattern as MaxIterations).
	byteLimit := thresholds.reasoningByteLimit
	stallTimeout := thresholds.reasoningStallTimeout

	// Adaptive stall deadline constants.
	// Multiplier applied to full-budget projection: if the model runs at the
	// observed rate for the full budget and still hasn't produced output, allow
	// 2× that time before declaring a stall.
	const adaptiveSafetyFactor = 2.0
	// Minimum reasoning bytes received before the rate estimate is trusted.
	const adaptiveBootstrapBytes = 256
	// Approximate UTF-8 bytes per reasoning token for Qwen3-style models.
	const avgBytesPerToken = 4
	// Hard floor on the adaptive deadline: never abort sooner than this after
	// the first reasoning token, to protect against noisy early rate estimates.
	const adaptiveMinWindow = 30 * time.Second

	// Reasoning tail buffer: bounded ring of the most recent reasoning
	// content. On stall, the trailing slice is attached to ReasoningStallError
	// and emitted in the reasoning.stall event so callers can debug what the
	// model was reasoning about at the time.
	tailLimit := thresholds.reasoningTailBytes
	if tailLimit == 0 {
		tailLimit = DefaultReasoningTailBytes
	}
	var reasoningTail []byte

	// Runaway reasoning loop detection.
	// These track state only while the model is in pure-reasoning mode (no
	// content or tool_call deltas seen yet for this response).
	var reasoningBytes int
	var nonReasoningSeen bool
	reasoningStallStart := time.Now()
	// stallDeadline is the absolute time after which a pure-reasoning stream
	// fires ErrReasoningStall. Initialized from the static floor; extended
	// forward adaptively as tokens arrive when reasoningBudgetTokens is set.
	var stallDeadline time.Time
	if stallTimeout > 0 {
		stallDeadline = reasoningStallStart.Add(stallTimeout)
	}

	// Track tool call assembly — deltas arrive as fragments
	type toolCallState struct {
		ID      string
		Name    string
		ArgsBuf strings.Builder
	}
	toolCalls := make(map[string]*toolCallState)
	var toolCallOrder []string

	for delta := range ch {
		arrivalAt := delta.ArrivedAt
		if arrivalAt.IsZero() {
			arrivalAt = time.Now()
		}

		// Emit delta event
		if callback != nil {
			emitCallback(callback, Event{
				SessionID: sessionID,
				Seq:       *seq,
				Type:      EventLLMDelta,
				Timestamp: arrivalAt.UTC(),
				Data:      mustMarshal(delta),
			})
			*seq++
		}

		// Accumulate content
		if delta.Content != "" {
			contentBuf.WriteString(delta.Content)
		}

		// Accumulate tool call fragments
		if delta.ToolCallID != "" {
			tc, exists := toolCalls[delta.ToolCallID]
			if !exists {
				tc = &toolCallState{ID: delta.ToolCallID}
				toolCalls[delta.ToolCallID] = tc
				toolCallOrder = append(toolCallOrder, delta.ToolCallID)
			}
			if delta.ToolCallName != "" {
				tc.Name = delta.ToolCallName
			}
			if delta.ToolCallArgs != "" {
				tc.ArgsBuf.WriteString(delta.ToolCallArgs)
			}
		}

		// Capture model and finish reason from final delta
		if delta.Model != "" {
			resp.Model = delta.Model
		}
		if delta.Attempt != nil {
			attempt := *delta.Attempt
			resp.Attempt = &attempt
		}
		if delta.FinishReason != "" {
			resp.FinishReason = delta.FinishReason
		}
		if delta.Usage != nil {
			resp.Usage.Add(*delta.Usage)
		}

		if delta.Err != nil {
			return resp, delta.Err
		}

		// Runaway reasoning detection — only active while no content or tool_call
		// deltas have arrived yet for this response.
		if !nonReasoningSeen {
			if streamDeltaHasOutput(delta) {
				// First real output: disable reasoning loop checks for this response.
				nonReasoningSeen = true
			} else if delta.ReasoningContent != "" {
				reasoningBytes += len(delta.ReasoningContent)
				if tailLimit > 0 {
					reasoningTail = appendBoundedTail(reasoningTail, delta.ReasoningContent, tailLimit)
				}
				// 0 means unlimited (no limit).
				if byteLimit > 0 && reasoningBytes > byteLimit {
					return resp, reasoningOverflowError(thresholds.modelName, byteLimit, reasoningBytes)
				}
				// Adaptive deadline: when a budget is known and we have enough
				// data to estimate the token rate, recompute the deadline as:
				//   stall_start + (budget_tokens * avg_bytes_per_token) / rate * 2x
				// This is "how long the full budget would take at the observed rate,"
				// with a 2× safety margin. The result replaces the static floor so
				// the timeout scales correctly with both budget size and provider speed.
				// A hard minimum window (30s) guards against noisy early estimates.
				if thresholds.reasoningBudgetTokens > 0 && reasoningBytes >= adaptiveBootstrapBytes {
					elapsed := time.Since(reasoningStallStart)
					if elapsed > 0 {
						rate := float64(reasoningBytes) / elapsed.Seconds()
						totalBudgetBytes := float64(thresholds.reasoningBudgetTokens * avgBytesPerToken)
						projected := time.Duration(totalBudgetBytes / rate * adaptiveSafetyFactor * float64(time.Second))
						if projected < adaptiveMinWindow {
							projected = adaptiveMinWindow
						}
						stallDeadline = reasoningStallStart.Add(projected)
					}
				}
				if !stallDeadline.IsZero() && time.Now().After(stallDeadline) {
					effectiveTimeout := stallDeadline.Sub(reasoningStallStart)
					stallErr := newReasoningStallError(thresholds.modelName, effectiveTimeout, reasoningTail, thresholds.promptID)
					if callback != nil {
						emitCallback(callback, Event{
							SessionID: sessionID,
							Seq:       *seq,
							Type:      EventReasoningStall,
							Timestamp: time.Now().UTC(),
							Data: mustMarshal(map[string]any{
								"model":          stallErr.Model,
								"timeout_ms":     effectiveTimeout.Milliseconds(),
								"reasoning_tail": stallErr.ReasoningTail,
								"prompt_id":      stallErr.PromptID,
								"code":           ReasoningStallCode,
							}),
						})
						*seq++
					}
					return resp, stallErr
				}
			}
		}

		if streamDeltaHasOutput(delta) {
			if firstOutputAt.IsZero() {
				firstOutputAt = arrivalAt
			}
			lastOutputAt = arrivalAt
		}

		if delta.Done {
			break
		}
	}

	resp.Content = contentBuf.String()
	resp.Usage.Total = resp.Usage.Input + resp.Usage.Output

	if !firstOutputAt.IsZero() {
		if resp.Attempt == nil {
			resp.Attempt = &AttemptMetadata{}
		}
		if resp.Attempt.Timing == nil {
			resp.Attempt.Timing = &TimingBreakdown{}
		}
		firstToken := firstOutputAt.Sub(streamStart)
		generation := lastOutputAt.Sub(firstOutputAt)
		resp.Attempt.Timing.FirstToken = &firstToken
		resp.Attempt.Timing.Generation = &generation
	}

	// Assemble tool calls from fragments
	for _, id := range toolCallOrder {
		tc := toolCalls[id]
		resp.ToolCalls = append(resp.ToolCalls, ToolCall{
			ID:        tc.ID,
			Name:      tc.Name,
			Arguments: json.RawMessage(tc.ArgsBuf.String()),
		})
	}

	return resp, nil
}

func streamDeltaHasOutput(delta StreamDelta) bool {
	return delta.Content != "" ||
		delta.ToolCallID != "" ||
		delta.ToolCallName != "" ||
		delta.ToolCallArgs != ""
}

// reasoningOverflowError wraps ErrReasoningOverflow with model name and threshold.
func reasoningOverflowError(model string, limit, actual int) error {
	if model == "" {
		model = "unknown"
	}
	return fmt.Errorf("%w (model=%s, limit=%dKB, actual=%dKB)",
		ErrReasoningOverflow, model, limit/1024, actual/1024)
}

// newReasoningStallError builds a structured ReasoningStallError. The legacy
// helper reasoningStallError(model, timeout) used to return a flat
// fmt.Errorf wrapping; the structured form preserves errors.Is matching via
// the error's Unwrap method while exposing Model, Timeout, ReasoningTail, and
// PromptID for programmatic access (see ReasoningStallCode).
func newReasoningStallError(model string, timeout time.Duration, tail []byte, promptID string) *ReasoningStallError {
	if model == "" {
		model = "unknown"
	}
	tailStr := ""
	if len(tail) > 0 {
		tailStr = string(tail)
	}
	return &ReasoningStallError{
		Model:         model,
		Timeout:       timeout,
		ReasoningTail: tailStr,
		PromptID:      promptID,
	}
}

// appendBoundedTail keeps the last `limit` bytes of (existing + chunk).
// Avoids unbounded growth of the reasoning tail buffer over a long stream.
func appendBoundedTail(existing []byte, chunk string, limit int) []byte {
	if limit <= 0 || chunk == "" {
		return existing
	}
	if len(chunk) >= limit {
		// New chunk alone exceeds the limit: keep only its trailing bytes.
		out := make([]byte, limit)
		copy(out, chunk[len(chunk)-limit:])
		return out
	}
	combined := append(existing, chunk...)
	if len(combined) <= limit {
		return combined
	}
	return combined[len(combined)-limit:]
}
