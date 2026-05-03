package openai

import (
	agent "github.com/DocumentDrivenDX/fizeau/internal/core"
	"github.com/DocumentDrivenDX/fizeau/internal/provider/registry"
)

// init registers the openai-flavored provider types whose factories call
// directly into openai.New rather than going through a wrapper package
// (omlx / lmstudio / luce / vllm / openrouter / ollama all wrap openai
// and register themselves; the types below are the ones served by this
// package directly with a custom ProviderSystem string).
func init() {
	for _, typ := range []string{"openai", "minimax", "qwen", "zai"} {
		t := typ // local copy for the closure
		registry.Register(registry.Descriptor{
			Type: t,
			Factory: func(in registry.Inputs) agent.Provider {
				return New(Config{
					BaseURL:             in.BaseURL,
					APIKey:              in.APIKey,
					Model:               in.Model,
					ProviderName:        in.ProviderName,
					ProviderSystem:      t,
					ModelPattern:        in.ModelPattern,
					KnownModels:         in.KnownModels,
					Headers:             in.Headers,
					Reasoning:           in.Reasoning,
					ModelReasoningWire:  in.ModelReasoningWire,
					QuotaSignalObserver: in.QuotaSignalObserver,
				})
			},
			// Built-in BaseURL only for "openai" (api.openai.com); the
			// other openai-compat types require an explicit endpoint.
			DefaultBaseURL: defaultBaseURLForType(t),
		})
	}
}

func defaultBaseURLForType(t string) string {
	if t == "openai" {
		return "https://api.openai.com/v1"
	}
	return ""
}
