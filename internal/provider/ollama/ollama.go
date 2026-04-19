package ollama

import (
	"github.com/DocumentDrivenDX/agent/internal/provider/openai"
	"github.com/DocumentDrivenDX/agent/internal/reasoning"
)

const DefaultBaseURL = "http://localhost:11434/v1"

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
		ProviderName:   "ollama",
		ProviderSystem: "ollama",
		ModelPattern:   cfg.ModelPattern,
		KnownModels:    cfg.KnownModels,
		Headers:        cfg.Headers,
		Reasoning:      cfg.Reasoning,
		Flavor:         "ollama",
	})
}
