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
// alongside `content`. The provider's existing reasoning_content handling on
// the openai-compat path covers this case. Live `/props` introspection is used
// when available to discover whether a build also exposes request-side
// reasoning controls.
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
	"context"
	"time"

	agentcore "github.com/easel/fizeau/internal/core"
	"github.com/easel/fizeau/internal/provider/openai"
	"github.com/easel/fizeau/internal/provider/registry"
	"github.com/easel/fizeau/internal/reasoning"
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
		DefaultBaseURL:  DefaultBaseURL,
		DefaultPort:     1236,
		IntrospectionFn: Introspect,
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
	caps := ProtocolCapabilities
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if intro, ok := registry.IntrospectProvider(ctx, "lucebox", baseURL, cfg.Model); ok {
		if intro.EffectiveThinkingFormat != "" {
			caps.ThinkingFormat = openai.ThinkingWireFormat(intro.EffectiveThinkingFormat)
		}
		caps.EffectiveReasoningLevels = intro.EffectiveReasoningLevels
		caps.ReasoningAliasMap = intro.AliasMap
		caps.SupportedRequestParams = intro.SupportedRequestParams
		caps.ServerSideReasoningFormat = intro.ServerSideReasoningFormat
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
		Capabilities:   &caps,
	})
}
