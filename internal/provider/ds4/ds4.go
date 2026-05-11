// Package ds4 wraps the OpenAI-compatible HTTP surface exposed by DwarfStar 4
// (antirez/ds4) — a single-model native inference engine for DeepSeek V4 Flash
// with Metal/CUDA execution. The /v1/chat/completions and /v1/completions
// routes are OpenAI-compatible; /v1/messages also speaks Anthropic Messages.
// Other endpoints (no /health, /metrics, /slots, no auth) are minimal —
// see internal/provider/ds4/utilization_probe.go for the liveness-only probe.
package ds4

import (
	agentcore "github.com/easel/fizeau/internal/core"
	"github.com/easel/fizeau/internal/provider/openai"
	"github.com/easel/fizeau/internal/provider/registry"
	"github.com/easel/fizeau/internal/reasoning"
)

// DefaultBaseURL matches the ds4-server default listen (127.0.0.1:8000) plus
// the /v1 prefix. Operators usually override the host portion in their
// fizeau provider config; the port stays 8000 unless the server was started
// with --port.
const DefaultBaseURL = "http://localhost:8000/v1"

func init() {
	registry.Register(registry.Descriptor{
		Type: "ds4",
		Factory: func(in registry.Inputs) agentcore.Provider {
			return New(Config{
				BaseURL:      in.BaseURL,
				APIKey:       in.APIKey,
				Model:        in.Model,
				ModelPattern: in.ModelPattern,
				KnownModels:  in.KnownModels,
				Headers:      in.Headers,
				Reasoning:    in.Reasoning,
			})
		},
		DefaultBaseURL: DefaultBaseURL,
		DefaultPort:    8000,
	})
}

// ProtocolCapabilities extends the standard OpenAI-compatible surface with
// thinking-control support. ds4's verified wire surface (via /props):
//   - `reasoning_effort: "low"|"medium"|"high"|"max"|"xhigh"` (flat top-level, OpenAI-style)
//   - `think: false` (boolean shortcut for direct-reply / disable)
//   - model alias `deepseek-chat` for non-thinking
//
// ds4 /props.reasoning.aliases declares {low→high, medium→high, xhigh→high};
// only `high` and `max` are practically distinct effort levels.
// fizeau emits ThinkingWireFormatOpenAIEffort (flat reasoning_effort) for this provider.
//
// Other ds4 quirks worth knowing about (handled elsewhere): finish_reason
// is only "stop" or "length" — never "tool_calls" — but the agent loop
// already keys on empty resp.ToolCalls for natural-stop detection.
var ProtocolCapabilities = openai.ProtocolCapabilities{
	Tools:            true,
	Stream:           true,
	StructuredOutput: true,
	Thinking:         true,
	ThinkingFormat:   openai.ThinkingWireFormatOpenAIEffort,
}

type Config struct {
	BaseURL      string
	APIKey       string
	Model        string
	ModelPattern string
	KnownModels  map[string]string
	Headers      map[string]string
	Reasoning    reasoning.Reasoning
}

func New(cfg Config) *openai.Provider {
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	return openai.New(openai.Config{
		BaseURL:        baseURL,
		APIKey:         cfg.APIKey,
		Model:          cfg.Model,
		ProviderName:   "ds4",
		ProviderSystem: "ds4",
		ModelPattern:   cfg.ModelPattern,
		KnownModels:    cfg.KnownModels,
		Headers:        cfg.Headers,
		Reasoning:      cfg.Reasoning,
		Capabilities:   &ProtocolCapabilities,
	})
}
