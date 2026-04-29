package core

import (
	"errors"
	"fmt"
	"time"
)

// ReasoningStallCode is the stable, machine-matchable identifier for a
// reasoning-stall failure. Callers and downstream observers (benchmark
// harnesses, dashboards) can match on this constant rather than parsing
// the wrapped error string.
const ReasoningStallCode = "REASONING_STALL"

// ErrCompactionNoFit reports that compaction was needed but could not produce
// a message history that fits within the effective context window.
var ErrCompactionNoFit = errors.New("agent: compaction could not fit within the effective context window")

// ErrReasoningOverflow is returned by consumeStream when the model has emitted
// more than reasoningByteLimit bytes of pure reasoning_content without
// producing any content or tool_call deltas. The model is stuck in a runaway
// reasoning loop and the stream is aborted early.
var ErrReasoningOverflow = errors.New("agent: reasoning overflow: model produced only reasoning tokens past byte limit")

// ErrReasoningStall is returned by consumeStream when only reasoning_content
// deltas have arrived for longer than reasoningStallTimeout with no content or
// tool_call delta. The model appears to be making no forward progress.
//
// Stall failures are also wrapped in a ReasoningStallError carrying structured
// fields (model, timeout, reasoning tail, prompt id). Callers that need the
// machine-matchable code or the captured reasoning context should use
// errors.As with *ReasoningStallError.
var ErrReasoningStall = errors.New("agent: reasoning stall: model produced only reasoning tokens past stall timeout")

// ReasoningStallError is the structured form of ErrReasoningStall. It carries
// the fields needed to (a) count stall rate as a metric across runs/models and
// (b) debug what the model was reasoning about when the stall fired.
type ReasoningStallError struct {
	// Model is the resolved/concrete model ID that stalled.
	Model string
	// Timeout is the stall threshold that was exceeded.
	Timeout time.Duration
	// ReasoningTail is the last N bytes of reasoning_content seen on the
	// stream prior to the stall. May be empty if the stall fired before any
	// reasoning bytes accumulated.
	ReasoningTail string
	// PromptID is a caller-supplied identifier correlating this stall to the
	// prompt or turn that produced it. Empty when not provided.
	PromptID string
}

// Code returns the stable, machine-matchable identifier (ReasoningStallCode).
func (e *ReasoningStallError) Code() string { return ReasoningStallCode }

// Error renders a human-readable form compatible with the legacy flat string,
// preserving model and timeout for log greppability.
func (e *ReasoningStallError) Error() string {
	model := e.Model
	if model == "" {
		model = "unknown"
	}
	return fmt.Sprintf("%s (model=%s, timeout=%s)", ErrReasoningStall.Error(), model, e.Timeout)
}

// Unwrap returns ErrReasoningStall so existing errors.Is callers continue to
// match.
func (e *ReasoningStallError) Unwrap() error { return ErrReasoningStall }

// ErrToolCallLoop reports that the agent produced identical tool calls for
// toolCallLoopLimit consecutive turns, indicating a non-converging loop.
var ErrToolCallLoop = errors.New("agent: identical tool calls repeated, aborting loop")

// ErrCompactionStuck reports that compaction was requested but failed to
// produce a compacted history for multiple consecutive attempts. This prevents
// a runaway loop where compaction.end(no_compaction=true) events fire
// indefinitely without making progress.
var ErrCompactionStuck = errors.New("agent: compaction stuck: consecutive attempts failed to produce a compacted history")
