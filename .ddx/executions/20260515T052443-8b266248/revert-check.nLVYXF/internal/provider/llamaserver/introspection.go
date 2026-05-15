package llamaserver

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

// llamaProps mirrors the relevant fields of the llama-server /props response.
type llamaProps struct {
	DefaultGenerationSettings struct {
		Params struct {
			ReasoningFormat string `json:"reasoning_format"`
		} `json:"params"`
	} `json:"default_generation_settings"`
	ChatTemplate     string `json:"chat_template"`
	ChatTemplateCaps struct {
		SupportsPreserveReasoning bool `json:"supports_preserve_reasoning"`
	} `json:"chat_template_caps"`
	BuildInfo map[string]any `json:"build_info"`
}

// Introspect fetches GET /props from a llama-server and returns structured
// ProviderIntrospection. Returns an error if the endpoint is unreachable or
// the response is malformed; callers should fall through to static defaults.
//
// The chat_template is inspected for the "enable_thinking" substring to
// confirm the Qwen chat_template_kwargs envelope (vs bare top-level fields).
func Introspect(ctx context.Context, baseURL, model string, client *http.Client) (*registry.ProviderIntrospection, error) {
	if client == nil {
		client = http.DefaultClient
	}
	endpoint := utilization.ServerRoot(baseURL) + "/props"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("llama-server introspection: build request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("llama-server introspection: GET %s: %w", endpoint, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return nil, fmt.Errorf("llama-server introspection: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("llama-server introspection: read body: %w", err)
	}

	var props llamaProps
	if err := json.Unmarshal(body, &props); err != nil {
		return nil, fmt.Errorf("llama-server introspection: unmarshal: %w", err)
	}

	// Detect Qwen chat_template_kwargs envelope via substring match.
	// Top-level enable_thinking is silently dropped by llama-server; the
	// kwargs envelope (chat_template_kwargs.enable_thinking) is required.
	thinkingFormat := openai.ThinkingWireFormatQwen
	if !strings.Contains(props.ChatTemplate, "enable_thinking") {
		// Template doesn't reference enable_thinking at all — reasoning
		// controls may not apply. Qwen remains the default for this provider
		// type; callers can override via catalog L2.
		thinkingFormat = openai.ThinkingWireFormatQwen
	}

	var raw map[string]any
	_ = json.Unmarshal(body, &raw)

	return &registry.ProviderIntrospection{
		EffectiveThinkingFormat:   string(thinkingFormat),
		ServerSideReasoningFormat: props.DefaultGenerationSettings.Params.ReasoningFormat,
		Raw:                       raw,
	}, nil
}
