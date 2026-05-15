package serviceimpl

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type ProviderProbeResult struct {
	Status         string
	ModelCount     int
	Capabilities   []string
	Detail         string
	EndpointName   string
	BaseURL        string
	ServerInstance string
}

func ProbeServiceProviderDetailed(ctx context.Context, entry ProviderEntry) ProviderProbeResult {
	switch entry.Type {
	case "anthropic":
		if entry.APIKey == "" {
			return ProviderProbeResult{Status: "error: api_key not configured", Detail: "api_key not configured"}
		}
		return ProviderProbeResult{Status: "connected", Capabilities: ProviderCapabilities(entry)}
	case "openai", "openrouter", "lmstudio", "llama-server", "ds4", "omlx", "rapid-mlx", "vllm", "ollama", "minimax", "qwen", "zai", "":
		if entry.BaseURL == "" {
			return ProviderProbeResult{Status: "error: base_url not configured", Detail: "base_url not configured"}
		}
		modelCount, err := DiscoverOpenAIModels(ctx, entry.BaseURL, entry.APIKey)
		if err != nil {
			msg := err.Error()
			if ServiceIsUnreachable(msg) {
				return ProviderProbeResult{Status: "unreachable", Detail: ServiceTrimError(msg)}
			}
			detail := ServiceTrimError(msg)
			return ProviderProbeResult{Status: "error: " + detail, Detail: detail}
		}
		return ProviderProbeResult{Status: "connected", ModelCount: modelCount, Capabilities: ProviderCapabilities(entry)}
	default:
		detail := "unknown provider type " + entry.Type
		return ProviderProbeResult{Status: "error: " + detail, Detail: detail}
	}
}

func ShouldPreferProviderProbe(candidate, current ProviderProbeResult) bool {
	return ProviderProbePriority(candidate.Status) < ProviderProbePriority(current.Status)
}

func ProviderProbePriority(status string) int {
	switch endpointStatus(status) {
	case "connected":
		return 0
	case "unauthenticated":
		return 1
	case "unreachable":
		return 2
	default:
		return 3
	}
}

func DiscoverOpenAIModels(ctx context.Context, baseURL, apiKey string) (int, error) {
	base := strings.TrimRight(baseURL, "/")
	endpoint := base + "/models"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return 0, fmt.Errorf("discovery: build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return 0, fmt.Errorf("discovery: %s returned HTTP %d: %s", endpoint, resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var response struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return 0, fmt.Errorf("discovery: decode response from %s: %w", endpoint, err)
	}
	return len(response.Data), nil
}

func ServiceIsUnreachable(msg string) bool {
	lower := strings.ToLower(msg)
	return strings.Contains(lower, "connection refused") ||
		strings.Contains(lower, "no such host") ||
		strings.Contains(lower, "dial tcp") ||
		strings.Contains(lower, "unreachable") ||
		strings.Contains(lower, "connection reset") ||
		strings.Contains(lower, "i/o timeout")
}

func ServiceTrimError(msg string) string {
	const maxLen = 120
	if len(msg) > maxLen {
		return msg[:maxLen] + "..."
	}
	return msg
}

func endpointStatus(status string) string {
	switch {
	case status == "connected":
		return "connected"
	case strings.HasPrefix(status, "error: api_key not configured"),
		strings.HasPrefix(status, "error: authentication"),
		strings.Contains(status, "401"),
		strings.Contains(status, "403"):
		return "unauthenticated"
	case status == "unreachable":
		return "unreachable"
	default:
		return "error"
	}
}
