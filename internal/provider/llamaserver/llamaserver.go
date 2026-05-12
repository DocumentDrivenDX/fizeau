// Package llamaserver wraps the OpenAI-compatible HTTP surface exposed by
// llama.cpp's built-in server.
package llamaserver

import (
	"context"
	"time"

	agentcore "github.com/easel/fizeau/internal/core"
	"github.com/easel/fizeau/internal/provider/openai"
	"github.com/easel/fizeau/internal/provider/registry"
	"github.com/easel/fizeau/internal/reasoning"
)

const DefaultBaseURL = "http://localhost:8080/v1"

func init() {
	registry.Register(registry.Descriptor{
		Type: "llama-server",
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
		DefaultPort:     8080,
		IntrospectionFn: Introspect,
	})
}

// ProtocolCapabilities is the L3 static default for llama-server. It is used
// as the base when no live introspection is available. llama.cpp's llama-server
// accepts Qwen-family enable_thinking + thinking_budget controls when the
// model's chat template is wired for thinking mode (Qwen3, DeepSeek R1, etc.)
// — and silently ignores them on non-thinking models. The default
// ThinkingWireFormatQwen matches what Qwen GGUFs expect via the
// chat_template_kwargs envelope.
//
// Without Thinking: true, fizeau's openai provider rejects any explicit
// reasoning param with "openai: reasoning=... is not supported by provider
// type llama-server" — biting us on sindri-llamacpp until 2026-05-11.
var ProtocolCapabilities = openai.ProtocolCapabilities{
	Tools:            true,
	Stream:           true,
	StructuredOutput: true,
	Thinking:         true,
	ThinkingFormat:   openai.ThinkingWireFormatQwen,
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

// New constructs a llama-server provider. It first attempts L1 live
// introspection via /props to override static capability defaults
// (e.g. ThinkingFormat, server-side reasoning_format). On introspection
// failure the static ProtocolCapabilities are used unchanged.
func New(cfg Config) *openai.Provider {
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}

	// Start from the L3 static defaults; L1 introspection may override.
	caps := ProtocolCapabilities

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if intro, ok := registry.IntrospectProvider(ctx, "llama-server", baseURL, cfg.Model); ok {
		if intro.EffectiveThinkingFormat != "" {
			caps.ThinkingFormat = openai.ThinkingWireFormat(intro.EffectiveThinkingFormat)
		}
	}

	return openai.New(openai.Config{
		BaseURL:        baseURL,
		APIKey:         cfg.APIKey,
		Model:          cfg.Model,
		ProviderName:   "llama-server",
		ProviderSystem: "llama-server",
		ModelPattern:   cfg.ModelPattern,
		KnownModels:    cfg.KnownModels,
		Headers:        cfg.Headers,
		Reasoning:      cfg.Reasoning,
		Capabilities:   &caps,
	})
}
