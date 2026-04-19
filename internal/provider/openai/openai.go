// Package openai implements a agent.Provider for any OpenAI-compatible API
// endpoint (LM Studio, Ollama, OpenAI, Azure, Groq, Together, OpenRouter).
package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/DocumentDrivenDX/agent"
	reasoningpolicy "github.com/DocumentDrivenDX/agent/internal/reasoning"
	"github.com/DocumentDrivenDX/agent/internal/sdk/openaicompat"
	"github.com/openai/openai-go/option"
)

// Provider implements agent.Provider for OpenAI-compatible APIs.
type Provider struct {
	client           *openaicompat.Client
	model            string
	modelPattern     string            // regex filter for auto-discovery; "" means first model
	knownModels      map[string]string // catalog-recognized model IDs (modelID → catalogRef)
	baseURL          string            // stored for lazy model discovery
	apiKey           string            // stored for lazy model discovery
	providerName     string
	providerSystem   string // URL-heuristic tag; eager, zero-cost, used in hot telemetry paths
	configFlavor     string // explicit Config.Flavor when set; empty means auto-detect via probe
	serverAddress    string
	serverPort       int
	reasoningDefault reasoningpolicy.Reasoning

	// lazy model discovery — runs at most once per Provider instance
	discoverOnce     sync.Once
	discoverErr      error
	discoveredModels []ScoredModel // full ranked list; populated on first use when model == ""

	// lazy flavor detection — probe runs at most once per Provider instance
	flavorOnce     sync.Once
	detectedFlavor string
}

// Config holds configuration for the OpenAI-compatible provider.
type Config struct {
	BaseURL      string // e.g., "http://localhost:1234/v1" for LM Studio
	APIKey       string // optional for local providers
	Model        string // e.g., "qwen3.5-7b", "gpt-4o". Empty = auto-discover.
	ModelPattern string // case-insensitive regex to prefer among auto-discovered models
	// KnownModels maps concrete model IDs to catalog target IDs for the
	// agent.openai surface. Models present in this map are ranked higher during
	// auto-selection. Populated by the config layer from the model catalog;
	// nil disables catalog-aware ranking.
	KnownModels map[string]string
	Headers     map[string]string // extra HTTP headers (OpenRouter, Azure, etc.)
	Reasoning   reasoningpolicy.Reasoning
	// Flavor is an optional explicit server-type hint ("lmstudio", "omlx",
	// "openrouter", "ollama"). When set, DetectedFlavor() returns this value
	// without probing. When empty, DetectedFlavor() runs a one-time probe.
	Flavor string
}

// New creates a new OpenAI-compatible provider.
func New(cfg Config) *Provider {
	providerSystem, serverAddress, serverPort := openAIIdentity(cfg.BaseURL)
	return &Provider{
		client: openaicompat.NewClient(openaicompat.Config{
			BaseURL: cfg.BaseURL,
			APIKey:  cfg.APIKey,
			Headers: cfg.Headers,
		}),
		model:            cfg.Model,
		modelPattern:     cfg.ModelPattern,
		knownModels:      cfg.KnownModels,
		baseURL:          cfg.BaseURL,
		apiKey:           cfg.APIKey,
		providerName:     "openai-compat",
		providerSystem:   providerSystem,
		configFlavor:     cfg.Flavor,
		serverAddress:    serverAddress,
		serverPort:       serverPort,
		reasoningDefault: cfg.Reasoning,
	}
}

// DetectedFlavor returns the effective server flavor for this provider.
// Resolution order:
//
//  1. Config.Flavor (if set at construction) — returned verbatim, no probe.
//  2. Cached probe result — computed on first call by contacting
//     /v1/models/status (omlx) and /api/v0/models (lmstudio).
//  3. URL-heuristic providerSystem — fallback when probe is inconclusive.
//
// This accessor is intended for pre-dispatch gating (capability introspection,
// routing decisions) where the caller is willing to block once on a short
// network probe. Do not use it in per-response hot paths; use
// ChatStartMetadata() for telemetry, which is eager and non-blocking.
func (p *Provider) DetectedFlavor() string {
	p.flavorOnce.Do(func() {
		if p.configFlavor != "" {
			p.detectedFlavor = strings.ToLower(strings.TrimSpace(p.configFlavor))
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		p.detectedFlavor = resolveProviderFlavor(ctx, p.baseURL, "")
	})
	if p.detectedFlavor != "" {
		return p.detectedFlavor
	}
	return p.providerSystem
}

// DiscoveredModels returns the full ranked list of models discovered from the
// server's /v1/models endpoint. Returns nil if the provider has a statically
// configured model or if discovery has not yet run (i.e. no request has been
// made yet). Call EnsureDiscovered to force discovery without making a chat
// request.
func (p *Provider) DiscoveredModels() []ScoredModel {
	return p.discoveredModels
}

// EnsureDiscovered probes the server's /v1/models endpoint and caches the
// full ranked model list. It is a no-op when the provider has a statically
// configured model or when discovery has already run.
func (p *Provider) EnsureDiscovered(ctx context.Context) error {
	if p.model != "" {
		return nil
	}
	_, err := p.resolveModel(ctx)
	return err
}

// resolveModel returns the model to use for a request. If the provider was
// configured without a model it queries /v1/models once, ranks results, and
// caches both the full list and the selected model.
// Subsequent calls return the cached value without hitting the network.
func (p *Provider) resolveModel(ctx context.Context) (string, error) {
	if p.model != "" {
		return p.model, nil
	}
	p.discoverOnce.Do(func() {
		candidates, err := DiscoverModels(ctx, p.baseURL, p.apiKey)
		if err != nil {
			p.discoverErr = err
			return
		}
		ranked, err := RankModels(candidates, p.knownModels, p.modelPattern)
		if err != nil {
			p.discoverErr = err
			return
		}
		p.discoveredModels = ranked
		selected := SelectModel(ranked)
		if selected == "" {
			p.discoverErr = fmt.Errorf("openai: no models returned by %s/models", p.baseURL)
			return
		}
		p.model = selected
	})
	return p.model, p.discoverErr
}

func (p *Provider) Chat(ctx context.Context, messages []agent.Message, tools []agent.ToolDef, opts agent.Options) (agent.Response, error) {
	model, err := p.resolveModel(ctx)
	if err != nil {
		return agent.Response{}, err
	}
	if opts.Model != "" {
		model = opts.Model
	}

	reqOpts, err := p.compatRequestOptions(opts)
	if err != nil {
		return agent.Response{}, err
	}

	result, err := p.client.Chat(ctx, model, messages, tools, reqOpts)
	if err != nil {
		return agent.Response{}, fmt.Errorf("openai: %w", err)
	}

	resp := agent.Response{
		Model:        result.Model,
		Content:      result.Content,
		FinishReason: result.FinishReason,
		ToolCalls:    result.ToolCalls,
		Usage:        result.Usage,
	}
	resp.Attempt = p.attemptMetadata(model, result.Model, &agent.CostAttribution{
		Source: agent.CostSourceUnknown,
	})
	if cost := p.costAttribution(result.RawUsage); cost != nil {
		resp.Attempt.Cost = cost
	}
	return resp, nil
}

// SessionStartMetadata reports the broad provider identity and configured model
// that should be recorded on session.start events.
func (p *Provider) SessionStartMetadata() (string, string) {
	return p.providerName, p.model
}

// ChatStartMetadata reports the resolved provider system and upstream server
// identity known when the provider is constructed.
func (p *Provider) ChatStartMetadata() (string, string, int) {
	return p.providerSystem, p.serverAddress, p.serverPort
}

// ChatStream implements agent.StreamingProvider for token-level streaming.
func (p *Provider) ChatStream(ctx context.Context, messages []agent.Message, tools []agent.ToolDef, opts agent.Options) (<-chan agent.StreamDelta, error) {
	model, err := p.resolveModel(ctx)
	if err != nil {
		return nil, err
	}
	if opts.Model != "" {
		model = opts.Model
	}

	reqOpts, err := p.compatRequestOptions(opts)
	if err != nil {
		return nil, err
	}

	return p.client.ChatStream(ctx, model, messages, tools, reqOpts, openaicompat.StreamHooks{
		Cost: p.costAttribution,
		Attempt: func(responseModel string, cost *agent.CostAttribution) *agent.AttemptMetadata {
			return p.attemptMetadata(model, responseModel, streamAttemptCost(cost))
		},
	})
}

func (p *Provider) compatRequestOptions(opts agent.Options) (openaicompat.RequestOptions, error) {
	extra, err := p.reasoningRequestOptions(opts)
	if err != nil {
		return openaicompat.RequestOptions{}, err
	}
	return openaicompat.RequestOptions{
		MaxTokens:    opts.MaxTokens,
		Temperature:  opts.Temperature,
		Seed:         opts.Seed,
		Stop:         opts.Stop,
		ExtraOptions: extra,
	}, nil
}

func (p *Provider) attemptMetadata(requestedModel, responseModel string, cost *agent.CostAttribution) *agent.AttemptMetadata {
	if cost == nil {
		cost = &agent.CostAttribution{Source: agent.CostSourceUnknown}
	}
	return &agent.AttemptMetadata{
		ProviderName:   p.providerName,
		ProviderSystem: p.providerSystem,
		ServerAddress:  p.serverAddress,
		ServerPort:     p.serverPort,
		RequestedModel: requestedModel,
		ResponseModel:  responseModel,
		ResolvedModel:  responseModel,
		Cost:           cost,
	}
}

func (p *Provider) costAttribution(rawUsage string) *agent.CostAttribution {
	cost, _ := openRouterCostAttribution(p.providerSystem, rawUsage)
	return cost
}

// reasoningRequestOptions builds per-request options. For thinking models
// (Qwen3, DeepSeek-R1 etc.) apply a budget cap via the non-standard `thinking`
// body field only for flavors that tolerate it. Sending it to omlx causes
// silent SSE termination after the first delta (agent-04639431 wire evidence).
func (p *Provider) reasoningRequestOptions(opts agent.Options) ([]option.RequestOption, error) {
	policy, err := reasoningpolicy.Parse(opts.Reasoning)
	if err != nil {
		return nil, err
	}
	explicitRequest := policy.IsSet()
	if !explicitRequest {
		policy, err = reasoningpolicy.Parse(p.reasoningDefault)
		if err != nil {
			return nil, err
		}
	}

	if !policy.IsSet() || policy.Kind == reasoningpolicy.KindAuto || policy.IsExplicitOff() {
		return nil, nil
	}
	thinkingBudget, err := reasoningpolicy.BudgetFor(policy, nil, 0)
	if err != nil {
		return nil, fmt.Errorf("openai: %w", err)
	}
	if thinkingBudget <= 0 {
		return nil, nil
	}
	if !p.SupportsThinking() {
		if explicitRequest {
			return nil, fmt.Errorf("openai: reasoning=%q is not supported by flavor %q", policy.Value, p.DetectedFlavor())
		}
		return nil, nil
	}
	return []option.RequestOption{option.WithJSONSet("thinking", map[string]interface{}{
		"type":          "enabled",
		"budget_tokens": thinkingBudget,
	})}, nil
}

var _ agent.Provider = (*Provider)(nil)
var _ agent.StreamingProvider = (*Provider)(nil)

func openAIIdentity(baseURL string) (providerSystem, serverAddress string, serverPort int) {
	providerSystem = "openai"
	serverAddress = "api.openai.com"
	serverPort = 443

	if baseURL == "" {
		return providerSystem, serverAddress, serverPort
	}

	parsed, err := url.Parse(baseURL)
	if err != nil {
		return providerSystem, serverAddress, serverPort
	}

	host := parsed.Hostname()
	if host != "" {
		serverAddress = host
	}

	if port := parsed.Port(); port != "" {
		if parsedPort, err := strconv.Atoi(port); err == nil {
			serverPort = parsedPort
		}
	} else if strings.EqualFold(parsed.Scheme, "http") {
		serverPort = 80
	}

	switch {
	case strings.Contains(host, "openrouter.ai"):
		providerSystem = "openrouter"
	case host == "localhost" || host == "127.0.0.1":
		switch serverPort {
		case 11434:
			providerSystem = "ollama"
		case 1234:
			providerSystem = "lmstudio"
		case 1235:
			providerSystem = "omlx"
		default:
			providerSystem = "local"
		}
	case strings.Contains(host, "openai.com"):
		providerSystem = "openai"
	case strings.Contains(host, "minimaxi.chat"):
		providerSystem = "minimax"
	case strings.Contains(host, "dashscope.aliyuncs.com"):
		providerSystem = "qwen"
	case strings.Contains(host, "z.ai"):
		providerSystem = "zai"
	default:
		// Non-standard port on a named host → treat as local inference runtime.
		if serverPort != 0 && serverPort != 80 && serverPort != 443 {
			switch serverPort {
			case 11434:
				providerSystem = "ollama"
			case 1234:
				providerSystem = "lmstudio"
			case 1235:
				providerSystem = "omlx"
			default:
				providerSystem = "local"
			}
		}
		// Standard ports (0, 80, 443) on an unknown host fall through to "openai".
	}

	return providerSystem, serverAddress, serverPort
}

func streamAttemptCost(cost *agent.CostAttribution) *agent.CostAttribution {
	if cost != nil {
		return cost
	}
	return &agent.CostAttribution{Source: agent.CostSourceUnknown}
}

func openRouterCostAttribution(providerSystem, rawUsage string) (*agent.CostAttribution, bool) {
	if providerSystem != "openrouter" || strings.TrimSpace(rawUsage) == "" {
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
