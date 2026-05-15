package ds4

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

// ds4Props mirrors the relevant fields of the ds4-server /props response.
type ds4Props struct {
	Reasoning struct {
		SupportedEfforts   []string          `json:"supported_efforts"`
		Aliases            map[string]string `json:"aliases"`
		Default            string            `json:"default"`
		EffectiveDefault   string            `json:"effective_default"`
		ThinkMaxMinContext int               `json:"think_max_min_context"`
	} `json:"reasoning"`
	API struct {
		SupportedRequestParameters []string `json:"supported_request_parameters"`
	} `json:"api"`
	Sampling map[string]any `json:"sampling"`
	Runtime  map[string]any `json:"runtime"`
}

// Introspect fetches GET /props from a ds4-server and returns structured
// ProviderIntrospection. Returns an error if the endpoint is unreachable or
// the response is malformed; callers should fall through to static defaults.
//
// Effective reasoning levels are derived by removing alias source keys from
// supported_efforts (e.g. low/medium/xhigh all alias to high, leaving ["high", "max"]).
func Introspect(ctx context.Context, baseURL, model string, client *http.Client) (*registry.ProviderIntrospection, error) {
	if client == nil {
		client = http.DefaultClient
	}
	endpoint := utilization.ServerRoot(baseURL) + "/props"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("ds4 introspection: build request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ds4 introspection: GET %s: %w", endpoint, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return nil, fmt.Errorf("ds4 introspection: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("ds4 introspection: read body: %w", err)
	}

	var props ds4Props
	if err := json.Unmarshal(body, &props); err != nil {
		return nil, fmt.Errorf("ds4 introspection: unmarshal: %w", err)
	}

	// Effective levels = supported_efforts minus alias source keys.
	effectiveLevels := make([]string, 0, len(props.Reasoning.SupportedEfforts))
	for _, effort := range props.Reasoning.SupportedEfforts {
		if _, isAlias := props.Reasoning.Aliases[effort]; !isAlias {
			effectiveLevels = append(effectiveLevels, effort)
		}
	}

	var raw map[string]any
	_ = json.Unmarshal(body, &raw)

	return &registry.ProviderIntrospection{
		EffectiveThinkingFormat:  string(openai.ThinkingWireFormatOpenAIEffort),
		EffectiveReasoningLevels: effectiveLevels,
		AliasMap:                 props.Reasoning.Aliases,
		SupportedRequestParams:   props.API.SupportedRequestParameters,
		Raw:                      raw,
	}, nil
}
