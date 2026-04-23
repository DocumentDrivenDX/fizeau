package openrouter

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	agent "github.com/DocumentDrivenDX/agent/internal/core"
	"github.com/DocumentDrivenDX/agent/internal/provider/limits"
	"github.com/DocumentDrivenDX/agent/internal/provider/openai"
	"github.com/DocumentDrivenDX/agent/internal/reasoning"
)

const DefaultBaseURL = "https://openrouter.ai/api/v1"

var ProtocolCapabilities = openai.ProtocolCapabilities{
	Tools:            true,
	Stream:           true,
	StructuredOutput: true,
	Thinking:         true,
	ThinkingFormat:   openai.ThinkingWireFormatOpenRouter,
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
		BaseURL:              baseURL,
		APIKey:               cfg.APIKey,
		Model:                cfg.Model,
		ProviderName:         "openrouter",
		ProviderSystem:       "openrouter",
		ModelPattern:         cfg.ModelPattern,
		KnownModels:          cfg.KnownModels,
		Headers:              cfg.Headers,
		Reasoning:            cfg.Reasoning,
		Capabilities:         &ProtocolCapabilities,
		UsageCostAttribution: UsageCostAttribution,
	})
}

func UsageCostAttribution(rawUsage string) (*agent.CostAttribution, bool) {
	if strings.TrimSpace(rawUsage) == "" {
		return nil, false
	}

	// OpenRouter extends the OpenAI-compatible usage object with a billed USD
	// cost field at usage.cost. Preserve it when present instead of guessing from
	// a local pricing table.
	var usage struct {
		Cost *float64 `json:"cost"`
	}
	if err := json.Unmarshal([]byte(rawUsage), &usage); err != nil || usage.Cost == nil || *usage.Cost < 0 {
		return nil, false
	}

	return &agent.CostAttribution{
		Source:     agent.CostSourceGatewayReported,
		Currency:   "USD",
		Amount:     usage.Cost,
		PricingRef: "openrouter/usage.cost",
		Raw:        json.RawMessage(rawUsage),
	}, true
}

func LookupModelLimits(ctx context.Context, baseURL, apiKey string, headers map[string]string, model string) limits.ModelLimits {
	var list struct {
		Data []struct {
			ID            string `json:"id"`
			ContextLength int    `json:"context_length"`
			TopProvider   struct {
				MaxCompletionTokens int `json:"max_completion_tokens"`
			} `json:"top_provider"`
		} `json:"data"`
	}
	endpoint := strings.TrimRight(baseURL, "/") + "/models"
	if err := getAndDecode(ctx, 10*time.Second, endpoint, apiKey, headers, &list); err != nil {
		return limits.ModelLimits{}
	}

	normalizeID := func(s string) string {
		return strings.ToLower(strings.ReplaceAll(s, "-", "."))
	}
	normModel := normalizeID(model)
	for _, m := range list.Data {
		if strings.EqualFold(m.ID, model) || normalizeID(m.ID) == normModel {
			return limits.ModelLimits{
				ContextLength:       m.ContextLength,
				MaxCompletionTokens: m.TopProvider.MaxCompletionTokens,
			}
		}
	}
	return limits.ModelLimits{}
}

func getAndDecode(ctx context.Context, timeout time.Duration, endpoint, apiKey string, headers map[string]string, out any) error {
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
