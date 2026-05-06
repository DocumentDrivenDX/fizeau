// Package llamaserver wraps the OpenAI-compatible HTTP surface exposed by
// llama.cpp's built-in server.
package llamaserver

import (
	agentcore "github.com/DocumentDrivenDX/fizeau/internal/core"
	"github.com/DocumentDrivenDX/fizeau/internal/provider/openai"
	"github.com/DocumentDrivenDX/fizeau/internal/provider/registry"
	"github.com/DocumentDrivenDX/fizeau/internal/reasoning"
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
		DefaultBaseURL: DefaultBaseURL,
		DefaultPort:    8080,
	})
}

// ProtocolCapabilities mirrors the standard OpenAI-compatible surface.
var ProtocolCapabilities = openai.OpenAIProtocolCapabilities

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
		ProviderName:   "llama-server",
		ProviderSystem: "llama-server",
		ModelPattern:   cfg.ModelPattern,
		KnownModels:    cfg.KnownModels,
		Headers:        cfg.Headers,
		Reasoning:      cfg.Reasoning,
		Capabilities:   &ProtocolCapabilities,
	})
}
