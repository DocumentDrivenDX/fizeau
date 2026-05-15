package lucebox

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/easel/fizeau/internal/provider/openai"
	"github.com/easel/fizeau/internal/provider/registry"
	"github.com/easel/fizeau/internal/provider/utilization"
)

// luceboxProps mirrors the stable parts of lucebox-hub /props. The endpoint is
// intentionally broader than this struct; unknown fields stay available through
// ProviderIntrospection.Raw for diagnostics.
type luceboxProps struct {
	Model struct {
		ID string `json:"id"`
	} `json:"model"`
	Runtime   map[string]any `json:"runtime"`
	Reasoning struct {
		Supported          *bool             `json:"supported"`
		DefaultEnabled     *bool             `json:"default_enabled"`
		SupportedEfforts   []string          `json:"supported_efforts"`
		Aliases            map[string]string `json:"aliases"`
		Default            string            `json:"default"`
		EffectiveDefault   string            `json:"effective_default"`
		ThinkMaxMinContext int               `json:"think_max_min_context"`
	} `json:"reasoning"`
	API struct {
		SupportedRequestParameters []string `json:"supported_request_parameters"`
		Endpoints                  []string `json:"endpoints"`
	} `json:"api"`
	Sampling map[string]any `json:"sampling"`
}

// Introspect fetches GET /props from a lucebox server and returns structured
// ProviderIntrospection. Current lucebox /props reports model/runtime and
// reasoning availability. Newer builds may also report DS4-style
// supported_request_parameters; when present, those fields drive the effective
// thinking wire format.
func Introspect(ctx context.Context, baseURL, model string, client *http.Client) (*registry.ProviderIntrospection, error) {
	if client == nil {
		client = http.DefaultClient
	}
	endpoint := utilization.ServerRoot(baseURL) + "/props"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("lucebox introspection: build request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("lucebox introspection: GET %s: %w", endpoint, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return nil, fmt.Errorf("lucebox introspection: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("lucebox introspection: read body: %w", err)
	}

	var props luceboxProps
	if err := json.Unmarshal(body, &props); err != nil {
		return nil, fmt.Errorf("lucebox introspection: unmarshal: %w", err)
	}

	var raw map[string]any
	_ = json.Unmarshal(body, &raw)

	return &registry.ProviderIntrospection{
		EffectiveThinkingFormat:  luceboxThinkingFormat(props.API.SupportedRequestParameters),
		EffectiveReasoningLevels: effectiveReasoningLevels(props.Reasoning.SupportedEfforts, props.Reasoning.Aliases),
		AliasMap:                 props.Reasoning.Aliases,
		SupportedRequestParams:   props.API.SupportedRequestParameters,
		Raw:                      raw,
	}, nil
}

func luceboxThinkingFormat(params []string) string {
	switch {
	case hasRequestParam(params, "reasoning_effort") || hasRequestParam(params, "think"):
		return string(openai.ThinkingWireFormatOpenAIEffort)
	case hasRequestParam(params, "enable_thinking") || hasRequestParam(params, "thinking_budget"):
		return string(openai.ThinkingWireFormatQwen)
	case hasRequestParam(params, "thinking") || hasRequestParam(params, "budget_tokens"):
		return string(openai.ThinkingWireFormatThinkingMap)
	default:
		return ""
	}
}

func hasRequestParam(params []string, target string) bool {
	for _, param := range params {
		if param == target {
			return true
		}
	}
	return false
}

func effectiveReasoningLevels(supported []string, aliases map[string]string) []string {
	if len(supported) == 0 {
		return nil
	}
	out := make([]string, 0, len(supported))
	for _, effort := range supported {
		if _, isAlias := aliases[effort]; isAlias {
			continue
		}
		out = append(out, effort)
	}
	return out
}
