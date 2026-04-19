package occompat

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strings"

	agentConfig "github.com/DocumentDrivenDX/agent/internal/config"
	"github.com/DocumentDrivenDX/agent/internal/safefs"
)

// TranslationResult contains the result of translating opencode config to agent config.
type TranslationResult struct {
	Provider agentConfig.ProviderConfig
	Warnings []string
}

// Translate converts opencode configuration to agent provider config per SD-007.
func Translate(opencodeDir, authKey string) *TranslationResult {
	result := &TranslationResult{
		Provider: agentConfig.ProviderConfig{
			Type: "lmstudio",
		},
	}

	// Load opencode config
	cfg, err := LoadConfig(opencodeDir)
	if err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("could not load opencode config: %v", err))
		return result
	}

	// Map options.baseURL → base_url
	if cfg.Options.BaseURL != "" {
		result.Provider.BaseURL = cfg.Options.BaseURL
	}

	// Map options.apiKey or auth.json key → api_key
	if cfg.Options.APIKey != "" {
		result.Provider.APIKey = cfg.Options.APIKey
	} else if authKey != "" {
		result.Provider.APIKey = authKey
	}

	result.Provider.Type = concreteProviderType(cfg.Options.NPM, result.Provider.BaseURL)

	// Map npm → type
	if cfg.Options.NPM != "" {
		if cfg.Options.NPM != "@ai-sdk/openai-compatible" {
			result.Warnings = append(result.Warnings, fmt.Sprintf("unsupported npm provider %q; inferred type %q", cfg.Options.NPM, result.Provider.Type))
		}
	}

	// Map options.headers
	if len(cfg.Options.Headers) > 0 {
		result.Provider.Headers = cfg.Options.Headers
	}

	return result
}

func concreteProviderType(npm, baseURL string) string {
	lowURL := strings.ToLower(strings.TrimSpace(baseURL))
	switch {
	case strings.Contains(lowURL, "openrouter.ai"):
		return "openrouter"
	case strings.Contains(lowURL, "openai.com"):
		return "openai"
	case strings.Contains(lowURL, "minimaxi.chat"):
		return "minimax"
	case strings.Contains(lowURL, "dashscope.aliyuncs.com"):
		return "qwen"
	case strings.Contains(lowURL, "z.ai"):
		return "zai"
	case strings.Contains(lowURL, ":11434"):
		return "ollama"
	case strings.Contains(lowURL, ":1235"):
		return "omlx"
	case strings.Contains(lowURL, ":1234"):
		return "lmstudio"
	}
	if strings.TrimSpace(npm) == "@ai-sdk/openai-compatible" {
		return "lmstudio"
	}
	return "openai"
}

// ComputeSourceHash computes a truncated SHA-256 hash of the source files.
func ComputeSourceHash(opencodeDir string) (string, error) {
	authPath := opencodeDir + "/auth.json"
	// Try project config first
	configPath := "opencode.json"
	home, _ := os.UserHomeDir()
	if home != "" {
		configPath = home + "/.config/opencode/opencode.json"
	}

	authData, err := safefs.ReadFile(authPath)
	if err != nil {
		return "", err
	}
	configData, err := safefs.ReadFile(configPath)
	if err != nil {
		return "", err
	}

	combined := append(authData, configData...)
	h := sha256.Sum256(combined)
	return hex.EncodeToString(h[:])[:8], nil
}
