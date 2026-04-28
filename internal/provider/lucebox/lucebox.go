// Package lucebox wraps the OpenAI-compat HTTP shape exposed by the
// lucebox-hub dflash server (https://github.com/Luce-Org/lucebox-hub). The
// server runs hand-tuned CUDA inference (DFlash speculative decoding +
// DDTree verify) behind an OpenAI-compat HTTP API on :1236. For routing
// and benchmarking purposes treat lucebox like any other openai-compat
// local provider — same shape as lmstudio (:1234) and omlx (:1235), just
// a different runtime underneath.
//
// Capabilities mirror lmstudio for tool calling / streaming / structured
// output, and additionally declare Thinking=true: the server returns
// Qwen3-style thinking traces in a separate `reasoning_content` field
// alongside `content` (verified empirically against Qwen3.5-27B-Q4_K_M
// on bragi:1236 — a non-streaming chat with sufficient max_tokens
// produces both fields). The provider's existing reasoning_content
// handling on the openai-compat path covers this case; no separate
// thinking wire format is set because the server doesn't expose a
// request-side toggle (no enable_thinking / reasoning_effort field).
//
// Sampling: the server accepts the standard OpenAI sampler fields. Catalog
// entries for lucebox-served models should not set sampling_control unless
// a specific model id is observed to pin samplers server-side; in the
// absence of that signal, leaving sampling_control unset (=
// client_settable) lets internal/sampling.Resolve push catalog profile
// values to the wire as usual.
//
// Default port 1236 fits alongside the existing :1234 (lmstudio) and
// :1235 (omlx) conventions on the LAN.
package lucebox

import (
	agentcore "github.com/DocumentDrivenDX/agent/internal/core"
	"github.com/DocumentDrivenDX/agent/internal/provider/openai"
	"github.com/DocumentDrivenDX/agent/internal/provider/registry"
	"github.com/DocumentDrivenDX/agent/internal/reasoning"
)

const DefaultBaseURL = "http://localhost:1236/v1"

func init() {
	registry.Register(registry.Descriptor{
		Type: "lucebox",
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
		DefaultPort:    1236,
	})
}

// ProtocolCapabilities mirrors lmstudio's openai-compat surface — the
// routing engine treats lucebox as a full participant rather than a narrow
// special case. Per-model exceptions (e.g., a model that pins samplers
// server-side) belong on the catalog ModelEntry, not here.
var ProtocolCapabilities = openai.ProtocolCapabilities{
	Tools:            true,
	Stream:           true,
	StructuredOutput: true,
	Thinking:         true,
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
		ProviderName:   "lucebox",
		ProviderSystem: "lucebox",
		ModelPattern:   cfg.ModelPattern,
		KnownModels:    cfg.KnownModels,
		Headers:        cfg.Headers,
		Reasoning:      cfg.Reasoning,
		Capabilities:   &ProtocolCapabilities,
	})
}
