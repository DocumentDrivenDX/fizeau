package harnesses

import (
	"context"
	"encoding/json"
	"time"
)

// AccountInfo captures provider account metadata from local auth files.
type AccountInfo struct {
	Email    string `json:"email,omitempty"`
	PlanType string `json:"plan_type,omitempty"`
	OrgName  string `json:"org_name,omitempty"`
}

// QuotaWindow captures one quota window (e.g. 5h, weekly, model-specific).
type QuotaWindow struct {
	Name          string  `json:"name"`               // e.g. "5h", "7d", "spark"
	LimitID       string  `json:"limit_id,omitempty"` // provider limit_id
	LimitName     string  `json:"limit_name,omitempty"`
	WindowMinutes int     `json:"window_minutes"`
	UsedPercent   float64 `json:"used_percent"`
	ResetsAt      string  `json:"resets_at,omitempty"`      // human-readable
	ResetsAtUnix  int64   `json:"resets_at_unix,omitempty"` // unix timestamp
	State         string  `json:"state"`
}

// QuotaStateFromUsedPercent maps a usage percentage to a quota state string.
func QuotaStateFromUsedPercent(usedPercent int) string {
	if usedPercent >= 95 {
		return "blocked"
	}
	if usedPercent >= 0 {
		return "ok"
	}
	return "unknown"
}

// ModelDiscoverySnapshot captures model and reasoning capability evidence for
// harnesses whose source of truth is a CLI/TUI surface instead of /v1/models.
type ModelDiscoverySnapshot struct {
	CapturedAt      time.Time `json:"captured_at"`
	Models          []string  `json:"models,omitempty"`
	ReasoningLevels []string  `json:"reasoning_levels,omitempty"`
	Source          string    `json:"source"`
	FreshnessWindow string    `json:"freshness_window,omitempty"`
	Detail          string    `json:"detail,omitempty"`
}

// EventType identifies the kind of event a harness emits during execution.
//
// The set is the closed union defined by CONTRACT-003 ("Event JSON shapes"):
// every backend (native + subprocess) emits these identically so the agent
// loop can multiplex them onto a single channel.
type EventType string

const (
	EventTypeTextDelta       EventType = "text_delta"
	EventTypeToolCall        EventType = "tool_call"
	EventTypeToolResult      EventType = "tool_result"
	EventTypeCompaction      EventType = "compaction"
	EventTypeProgress        EventType = "progress"
	EventTypeRoutingDecision EventType = "routing_decision"
	EventTypeStall           EventType = "stall"
	EventTypeFinal           EventType = "final"
)

// Event is the structured event a harness emits during Execute. It mirrors
// the shape defined in CONTRACT-003 §"Event JSON shapes". The Data field is
// a JSON-encoded payload whose schema is determined by Type.
type Event struct {
	Type     EventType         `json:"type"`
	Sequence int64             `json:"sequence"`
	Time     time.Time         `json:"time"`
	Metadata map[string]string `json:"metadata,omitempty"`
	Data     json.RawMessage   `json:"data"`
}

// TextDeltaData is the payload for type=text_delta events.
type TextDeltaData struct {
	Text string `json:"text"`
}

// ToolCallData is the payload for type=tool_call events.
type ToolCallData struct {
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input,omitempty"`
}

// ToolResultData is the payload for type=tool_result events.
type ToolResultData struct {
	ID         string `json:"id"`
	Output     string `json:"output,omitempty"`
	Error      string `json:"error,omitempty"`
	DurationMS int64  `json:"duration_ms,omitempty"`
}

// FinalData is the payload for type=final events.
type FinalData struct {
	Status         string            `json:"status"` // success|iteration_limit|failed|stalled|timed_out|cancelled
	ExitCode       int               `json:"exit_code"`
	Error          string            `json:"error,omitempty"`
	FinalText      string            `json:"final_text,omitempty"`
	DurationMS     int64             `json:"duration_ms"`
	Usage          *FinalUsage       `json:"usage,omitempty"`
	Warnings       []FinalWarning    `json:"warnings,omitempty"`
	CostUSD        float64           `json:"cost_usd,omitempty"`
	SessionLogPath string            `json:"session_log_path,omitempty"`
	RoutingActual  *RoutingActual    `json:"routing_actual,omitempty"`
	Extra          map[string]string `json:"-"`
}

// FinalUsage carries token totals on a final event. Count fields are pointers
// so unavailable token dimensions are omitted instead of serialized as zero.
// A present pointer to 0 means the harness explicitly reported zero usage.
type FinalUsage struct {
	InputTokens      *int                  `json:"input_tokens,omitempty"`
	OutputTokens     *int                  `json:"output_tokens,omitempty"`
	CacheReadTokens  *int                  `json:"cache_read_tokens,omitempty"`
	CacheWriteTokens *int                  `json:"cache_write_tokens,omitempty"`
	CacheTokens      *int                  `json:"cache_tokens,omitempty"`
	ReasoningTokens  *int                  `json:"reasoning_tokens,omitempty"`
	TotalTokens      *int                  `json:"total_tokens,omitempty"`
	Source           string                `json:"source,omitempty"`
	Fresh            *bool                 `json:"fresh,omitempty"`
	CapturedAt       string                `json:"captured_at,omitempty"`
	Sources          []UsageSourceEvidence `json:"sources,omitempty"`
}

// FinalWarning is normalized metadata about non-fatal final-event issues.
type FinalWarning struct {
	Code    string                `json:"code"`
	Message string                `json:"message,omitempty"`
	Sources []UsageSourceEvidence `json:"sources,omitempty"`
}

// UsageSourceEvidence records one usage source considered by the resolver.
type UsageSourceEvidence struct {
	Source     string            `json:"source"`
	Fresh      *bool             `json:"fresh,omitempty"`
	CapturedAt string            `json:"captured_at,omitempty"`
	Usage      *UsageTokenCounts `json:"usage,omitempty"`
	Warning    string            `json:"warning,omitempty"`
}

// UsageTokenCounts is the normalized token-count vocabulary shared by
// subprocess harnesses and CONTRACT-003 final metadata.
type UsageTokenCounts struct {
	InputTokens      *int `json:"input_tokens,omitempty"`
	OutputTokens     *int `json:"output_tokens,omitempty"`
	CacheReadTokens  *int `json:"cache_read_tokens,omitempty"`
	CacheWriteTokens *int `json:"cache_write_tokens,omitempty"`
	CacheTokens      *int `json:"cache_tokens,omitempty"`
	ReasoningTokens  *int `json:"reasoning_tokens,omitempty"`
	TotalTokens      *int `json:"total_tokens,omitempty"`
}

// Any reports whether at least one token dimension is known.
func (c UsageTokenCounts) Any() bool {
	return c.InputTokens != nil ||
		c.OutputTokens != nil ||
		c.CacheReadTokens != nil ||
		c.CacheWriteTokens != nil ||
		c.CacheTokens != nil ||
		c.ReasoningTokens != nil ||
		c.TotalTokens != nil
}

func IntPtr(v int) *int {
	return &v
}

func BoolPtr(v bool) *bool {
	return &v
}

// RoutingActual captures the resolved fallback chain on a final event.
type RoutingActual struct {
	Harness            string   `json:"harness"`
	Provider           string   `json:"provider,omitempty"`
	ServerInstance     string   `json:"server_instance,omitempty"`
	Model              string   `json:"model"`
	FallbackChainFired []string `json:"fallback_chain_fired,omitempty"`
	FailureClass       string   `json:"failure_class,omitempty"`
	// Power is the catalog-projected power of the actually-dispatched
	// Model. 0 means unknown/exact-pin-only/no catalog entry.
	Power int `json:"power,omitempty"`
}

// HarnessInfo describes a registered harness. Mirrors the public
// HarnessInfo type defined in CONTRACT-003. Internal callers use this to
// implement the public ListHarnesses surface without re-declaring the shape.
type HarnessInfo struct {
	Name                 string
	Type                 string // "native" | "subprocess"
	Available            bool
	Path                 string
	Error                string
	IsLocal              bool
	IsSubscription       bool
	AutoRoutingEligible  bool
	ExactPinSupport      bool
	DefaultModel         string
	SupportedPermissions []string
	SupportedReasoning   []string
	CostClass            string
}

// ExecuteRequest is the internal request carried into Harness.Execute. It
// is intentionally narrower than the public ExecuteRequest in CONTRACT-003:
// the agent's routing layer is expected to resolve provider/model/reasoning
// /permissions/timeouts before invoking a harness, so the harness sees a
// concrete, ready-to-run request.
type ExecuteRequest struct {
	// Prompt is the resolved user prompt sent to the model.
	Prompt string

	// SystemPrompt is the resolved system prompt; empty means harness default.
	SystemPrompt string

	// Provider is the resolved provider identifier when applicable. May be
	// empty for harnesses that have no provider concept (e.g. claude CLI).
	Provider string

	// Model is the resolved model identifier; empty means harness default.
	Model string

	// WorkDir is the working directory for tool operations. Required when
	// the chosen harness uses tools.
	WorkDir string

	// Permissions is "safe" | "supervised" | "unrestricted". Empty defaults to "safe".
	Permissions string

	// Temperature is the model sampling temperature requested by the caller.
	// Harness adapters may ignore it when their CLI has no equivalent control.
	Temperature float32

	// Seed is the requested sampling seed. Zero means unset/provider chooses.
	// Harness adapters may ignore it when their CLI has no equivalent control.
	Seed int64

	// Reasoning is the normalized public reasoning scalar. Empty/off means no
	// adapter flag should be emitted.
	Reasoning string

	// Timeout is the wall-clock cap for the entire request. 0 disables.
	Timeout time.Duration

	// IdleTimeout is the streaming-quiet cap. 0 uses harness default.
	IdleTimeout time.Duration

	// SessionLogDir overrides the per-run session-log directory; harness
	// uses this to direct progress traces into a per-bundle evidence dir.
	SessionLogDir string

	// SessionID is a stable identifier for the run, used in progress log
	// filenames and event metadata. Empty means the harness generates one.
	SessionID string

	// Metadata is echoed back into Event.Metadata (e.g. bead_id, attempt_id).
	Metadata map[string]string
}

// Harness is the internal contract every harness implementation in
// internal/harnesses/<name> satisfies. It is the minimal surface the agent
// dispatcher needs to route a resolved request into a backend.
//
// A Harness is responsible for emitting events on the returned channel until
// execution completes; the channel MUST be closed after the final event so
// downstream consumers can detect end-of-stream. The final event is always
// of type EventTypeFinal.
type Harness interface {
	// Info returns identity + capability metadata for this harness.
	Info() HarnessInfo

	// HealthCheck triggers a fresh probe (binary present, auth ok, etc.)
	// and returns nil if the harness is ready to execute.
	HealthCheck(ctx context.Context) error

	// Execute runs one resolved request. Events stream on the returned
	// channel; a single final event closes the stream. The first error
	// return is reserved for setup failures (binary missing, etc.) — once
	// the channel is returned, all per-run failures are reported via a
	// final event with Status != "success".
	Execute(ctx context.Context, req ExecuteRequest) (<-chan Event, error)
}
