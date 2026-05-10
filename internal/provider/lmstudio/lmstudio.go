package lmstudio

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	agentcore "github.com/easel/fizeau/internal/core"
	"github.com/easel/fizeau/internal/provider/limits"
	"github.com/easel/fizeau/internal/provider/openai"
	"github.com/easel/fizeau/internal/provider/registry"
	"github.com/easel/fizeau/internal/reasoning"
)

const DefaultBaseURL = "http://localhost:1234/v1"

func init() {
	registry.Register(registry.Descriptor{
		Type: "lmstudio",
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
		DefaultPort:    1234,
	})
}

// ProtocolCapabilities reflects what an LM Studio server exposes on the
// OpenAI-compatible surface. Although LM Studio can host models with native
// reasoning controls on `/api/v1/chat`, the OpenAI-compatible surface is not a
// verified reasoning-control surface for routing or benchmark use.
//
// Evidence (2026-04-23) against Bragi LM Studio serving
// `qwen/qwen3.6-35b-a3b` (arch=qwen35moe, Q4_K_M) shows LM Studio accepts
// multiple reasoning-related request shapes on `/v1/chat/completions` but does
// not reliably honor them in the model template. Treat this provider as
// tool-capable and streaming-capable, but not as supporting request-level
// reasoning control on the OpenAI-compatible wire.
var ProtocolCapabilities = openai.ProtocolCapabilities{
	Tools:            true,
	Stream:           true,
	StructuredOutput: true,
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
		BaseURL:        baseURL,
		APIKey:         cfg.APIKey,
		Model:          cfg.Model,
		ProviderName:   "lmstudio",
		ProviderSystem: "lmstudio",
		ModelPattern:   cfg.ModelPattern,
		KnownModels:    cfg.KnownModels,
		Headers:        cfg.Headers,
		Reasoning:      cfg.Reasoning,
		Capabilities:   &ProtocolCapabilities,
	})
}

func LookupModelLimits(ctx context.Context, baseURL, model string) limits.ModelLimits {
	root := strings.TrimSuffix(strings.TrimRight(baseURL, "/"), "/v1")
	endpoint := root + "/api/v0/models/" + url.PathEscape(model)

	var info struct {
		LoadedContextLength int `json:"loaded_context_length"`
		MaxContextLength    int `json:"max_context_length"`
	}
	if err := getAndDecode(ctx, 5*time.Second, endpoint, &info); err != nil {
		return limits.ModelLimits{}
	}

	contextLen := info.LoadedContextLength
	if contextLen == 0 {
		contextLen = info.MaxContextLength
	}
	return limits.ModelLimits{ContextLength: contextLen}
}

func getAndDecode(ctx context.Context, timeout time.Duration, endpoint string, out any) error {
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")

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
