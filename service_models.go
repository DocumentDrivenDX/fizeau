package fizeau

// This file implements ListModels for the FizeauService service.
// It lives in the root package to avoid import cycles; provider and catalog
// data is injected via ServiceConfig (defined in service.go).
//
// Provider-backed models are discovered through /v1/models. Codex and Claude
// expose a separate harness-native surface backed by PTY/CLI evidence.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/easel/fizeau/internal/compaction"
	"github.com/easel/fizeau/internal/harnesses"
	claudeharness "github.com/easel/fizeau/internal/harnesses/claude"
	codexharness "github.com/easel/fizeau/internal/harnesses/codex"
	geminiharness "github.com/easel/fizeau/internal/harnesses/gemini"
	"github.com/easel/fizeau/internal/modelcatalog"
	"github.com/easel/fizeau/internal/provider/lmstudio"
	"github.com/easel/fizeau/internal/provider/omlx"
	"github.com/easel/fizeau/internal/provider/openrouter"
	"github.com/easel/fizeau/internal/serverinstance"
)

// ListModels returns models matching the filter, with full metadata.
// Empty filter returns all models from every reachable provider.
func (s *service) ListModels(ctx context.Context, filter ModelFilter) ([]ModelInfo, error) {
	if filter.Harness != "" && harnesses.ResolveHarnessAlias(filter.Harness) != "fiz" {
		return s.listModelsForSubprocessHarness(filter), nil
	}

	sc := s.opts.ServiceConfig
	if sc == nil {
		return nil, fmt.Errorf("service: no ServiceConfig provided; pass ServiceOptions.ServiceConfig")
	}

	// Load the model catalog once for cross-referencing.
	cat, _ := modelcatalog.Default() // ignore error: catalog miss is non-fatal

	defaultProviderName := sc.DefaultProviderName()

	names := sc.ProviderNames()

	type indexedModels struct {
		idx    int
		models []ModelInfo
	}
	results := make([]indexedModels, len(names))
	var wg sync.WaitGroup

	for i, name := range names {
		// Apply provider filter.
		if filter.Provider != "" && filter.Provider != name {
			results[i] = indexedModels{idx: i, models: nil}
			continue
		}
		// Apply harness filter: providers are served by the "fiz" harness.
		if filter.Harness != "" && harnesses.ResolveHarnessAlias(filter.Harness) != "fiz" {
			results[i] = indexedModels{idx: i, models: nil}
			continue
		}

		wg.Add(1)
		go func(idx int, providerName string) {
			defer wg.Done()

			entry, ok := sc.Provider(providerName)
			if !ok {
				results[idx] = indexedModels{idx: idx, models: nil}
				return
			}

			isDefaultProvider := providerName == defaultProviderName
			models := listModelsForProvider(ctx, providerName, entry, isDefaultProvider, sc, cat, s.routeUtilizationEvidence)
			results[idx] = indexedModels{idx: idx, models: models}
		}(i, name)
	}
	wg.Wait()

	// Flatten in stable provider order.
	var out []ModelInfo
	for _, r := range results {
		out = append(out, r.models...)
	}
	return out, nil
}

func (s *service) listModelsForSubprocessHarness(filter ModelFilter) []ModelInfo {
	name := harnesses.ResolveHarnessAlias(filter.Harness)
	cfg, ok := s.registry.Get(name)
	modelIDs := subprocessHarnessModelIDs(name, cfg)
	if !ok || harnessRunsInProcessOrHTTP(cfg) || len(modelIDs) == 0 {
		return nil
	}
	if filter.Provider != "" && filter.Provider != name {
		return nil
	}
	cat, _ := modelcatalog.Default()
	out := make([]ModelInfo, 0, len(modelIDs))
	for i, id := range modelIDs {
		info := ModelInfo{
			ID:             id,
			Provider:       name,
			Harness:        name,
			EndpointName:   name,
			ServerInstance: name,
			Capabilities:   []string{"streaming", "tool_use"},
			Available:      true,
			IsDefault:      cfg.DefaultModel != "" && id == cfg.DefaultModel,
			Billing:        harnessPaymentKind(name, cfg),
			RankPosition:   i,
		}
		if cat != nil {
			info.ContextLength, info.ContextSource = resolveContextEvidence(context.Background(), ServiceProviderEntry{}, id, cat)
			info.Cost, info.PerfSignal = catalogCostAndPerf(cat, id)
			info.Power, info.AutoRoutable, info.ExactPinOnly = catalogPowerEligibility(cat, id)
		}
		info.Utilization = s.routeUtilizationEvidence(name, info.ServerInstance, info.EndpointName, id)
		out = append(out, info)
	}
	return out
}

func subprocessHarnessModelIDs(name string, cfg harnesses.HarnessConfig) []string {
	models := append([]string(nil), cfg.Models...)
	switch name {
	case "claude":
		snapshot := claudeharness.DefaultClaudeModelDiscovery()
		models = appendUniqueModelIDs(models, snapshot.Models...)
		for _, family := range []string{"sonnet", "opus", "haiku"} {
			resolved := claudeharness.ResolveClaudeFamilyAlias(family, snapshot)
			if resolved != family {
				models = appendUniqueModelIDs(models, resolved)
			}
		}
	case "codex":
		snapshot := codexharness.DefaultCodexModelDiscovery()
		models = appendUniqueModelIDs(models, snapshot.Models...)
		for _, family := range []string{"gpt", "gpt-5"} {
			resolved := codexharness.ResolveCodexModelAlias(family, snapshot)
			if resolved != family {
				models = appendUniqueModelIDs(models, resolved)
			}
		}
	case "gemini":
		snapshot := geminiharness.DefaultGeminiModelDiscovery()
		models = appendUniqueModelIDs(models, snapshot.Models...)
		for _, family := range []string{"gemini", "gemini-2.5"} {
			resolved := geminiharness.ResolveGeminiModelAlias(family, snapshot)
			if resolved != family {
				models = appendUniqueModelIDs(models, resolved)
			}
		}
	}
	return models
}

func resolveSubprocessModelAlias(harness, model string) string {
	switch harness {
	case "claude":
		return claudeCLIExecutableModel(model)
	case "codex":
		return codexharness.ResolveCodexModelAlias(model, codexharness.DefaultCodexModelDiscovery())
	case "gemini":
		return geminiharness.ResolveGeminiModelAlias(model, geminiharness.DefaultGeminiModelDiscovery())
	default:
		return model
	}
}

func claudeCLIExecutableModel(model string) string {
	normalized := strings.ToLower(strings.TrimSpace(model))
	switch {
	case normalized == "sonnet" || strings.HasPrefix(normalized, "sonnet-") || strings.HasPrefix(normalized, "claude-sonnet-"):
		return "sonnet"
	case normalized == "opus" || strings.HasPrefix(normalized, "opus-") || strings.HasPrefix(normalized, "claude-opus-"):
		return "opus"
	case normalized == "haiku" || strings.HasPrefix(normalized, "haiku-") || strings.HasPrefix(normalized, "claude-haiku-"):
		return "haiku"
	default:
		return model
	}
}

func appendUniqueModelIDs(values []string, additions ...string) []string {
	for _, value := range additions {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		found := false
		for _, existing := range values {
			if existing == value {
				found = true
				break
			}
		}
		if !found {
			values = append(values, value)
		}
	}
	return values
}

// listModelsForProvider discovers and annotates models for a single provider.
func listModelsForProvider(
	ctx context.Context,
	providerName string,
	entry ServiceProviderEntry,
	isDefaultProvider bool,
	sc ServiceConfig,
	cat *modelcatalog.Catalog,
	utilizationEvidence func(provider, serverInstance, endpoint, model string) RouteUtilizationState,
) []ModelInfo {
	if entry.ConfigError != "" {
		return nil
	}
	// Discover model IDs from the provider.
	discoveries := discoverAndRankModels(ctx, entry)
	if len(discoveries) == 0 {
		return nil
	}

	configuredDefaultModel := entry.Model
	providerType := normalizeServiceProviderType(entry.Type)

	outLen := 0
	for _, discovery := range discoveries {
		outLen += len(discovery.IDs)
	}
	out := make([]ModelInfo, 0, outLen)
	for _, discovery := range discoveries {
		// Build a position map from the ranked list.
		rankPos := make(map[string]int, len(discovery.Ranked))
		for pos, sm := range discovery.Ranked {
			rankPos[sm.ID] = pos
		}

		for _, id := range discovery.IDs {
			info := ModelInfo{
				ID:              id,
				Provider:        providerName,
				ProviderType:    providerType,
				Harness:         "fiz",
				EndpointName:    discovery.EndpointName,
				EndpointBaseURL: discovery.EndpointBaseURL,
				ServerInstance:  discovery.ServerInstance,
				Available:       true,
				Billing:         serviceProviderBilling(entry),
			}

			// Resolve context length: provider config > provider API > catalog > default.
			info.ContextLength, info.ContextSource = resolveContextEvidence(ctx, entry, id, cat)

			// Capabilities from provider type.
			info.Capabilities = providerCapabilities(entry)

			// Cost and PerfSignal from catalog.
			if cat != nil {
				info.Cost, info.PerfSignal = catalogCostAndPerf(cat, id)
				info.Power, info.AutoRoutable, info.ExactPinOnly = catalogPowerEligibility(cat, id)
			}
			if utilizationEvidence != nil {
				info.Utilization = utilizationEvidence(providerName, info.ServerInstance, info.EndpointName, id)
			}

			// IsDefault: provider is default AND this model is the configured default model.
			info.IsDefault = isDefaultProvider && configuredDefaultModel != "" && id == configuredDefaultModel

			// RankPosition from discovery ranking.
			if pos, ok := rankPos[id]; ok {
				info.RankPosition = pos
			} else {
				info.RankPosition = -1
			}

			out = append(out, info)
		}
	}
	return out
}

type discoveredModelSet struct {
	EndpointName    string
	EndpointBaseURL string
	ServerInstance  string
	IDs             []string
	Ranked          []scoredModel
}

// discoverAndRankModels fetches the model list from each provider endpoint.
// IDs and rank positions preserve discovery order per endpoint.
func discoverAndRankModels(ctx context.Context, entry ServiceProviderEntry) []discoveredModelSet {
	switch normalizeServiceProviderType(entry.Type) {
	case "openai", "openrouter", "lmstudio", "llama-server", "ds4", "omlx", "rapid-mlx", "vllm", "ollama", "minimax", "qwen", "zai":
		endpoints := modelDiscoveryEndpoints(entry)
		if len(endpoints) == 0 {
			return nil
		}
		out := make([]discoveredModelSet, 0, len(endpoints))
		for _, endpoint := range endpoints {
			ids, err := discoverModelsInline(ctx, endpoint.BaseURL, entry.APIKey)
			if err != nil || len(ids) == 0 {
				continue
			}
			out = append(out, discoveredModelSet{
				EndpointName:    endpoint.Name,
				EndpointBaseURL: endpoint.BaseURL,
				ServerInstance:  endpoint.ServerInstance,
				IDs:             ids,
				Ranked:          rankModelsInline(ids),
			})
		}
		return out

	case "anthropic":
		// Anthropic does not expose /v1/models for discovery.
		// If a default model is configured, surface it.
		if entry.Model != "" {
			sm := scoredModel{ID: entry.Model, RankPosition: 0}
			return []discoveredModelSet{{
				EndpointName:    "default",
				EndpointBaseURL: entry.BaseURL,
				ServerInstance:  serverinstance.Normalize(entry.BaseURL, entry.ServerInstance),
				IDs:             []string{entry.Model},
				Ranked:          []scoredModel{sm},
			}}
		}
		return nil

	default:
		return nil
	}
}

type modelDiscoveryEndpoint struct {
	Name           string
	BaseURL        string
	ServerInstance string
}

func modelDiscoveryEndpoints(entry ServiceProviderEntry) []modelDiscoveryEndpoint {
	if len(entry.Endpoints) > 0 {
		out := make([]modelDiscoveryEndpoint, 0, len(entry.Endpoints))
		for _, ep := range entry.Endpoints {
			if strings.TrimSpace(ep.BaseURL) == "" {
				continue
			}
			out = append(out, modelDiscoveryEndpoint{
				Name:           endpointDisplayName(ep.Name, ep.BaseURL),
				BaseURL:        ep.BaseURL,
				ServerInstance: serverinstance.Normalize(ep.BaseURL, ep.ServerInstance),
			})
		}
		return out
	}
	if strings.TrimSpace(entry.BaseURL) == "" {
		return nil
	}
	return []modelDiscoveryEndpoint{{
		Name:           endpointDisplayName("default", entry.BaseURL),
		BaseURL:        entry.BaseURL,
		ServerInstance: serverinstance.Normalize(entry.BaseURL, entry.ServerInstance),
	}}
}

func endpointDisplayName(name, baseURL string) string {
	if trimmed := strings.TrimSpace(name); trimmed != "" {
		return trimmed
	}
	u, err := url.Parse(baseURL)
	if err == nil && u.Host != "" {
		return u.Host
	}
	return "default"
}

// discoverModelsInline queries /v1/models and returns model IDs.
// Mirrors the inline impl in service_providers.go to avoid import cycle.
func discoverModelsInline(ctx context.Context, baseURL, apiKey string) ([]string, error) {
	base := strings.TrimRight(baseURL, "/")
	endpoint := base + "/models"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("discovery: build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("discovery: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var mr struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&mr); err != nil {
		return nil, fmt.Errorf("discovery: decode response: %w", err)
	}

	ids := make([]string, 0, len(mr.Data))
	for _, m := range mr.Data {
		if m.ID != "" {
			ids = append(ids, m.ID)
		}
	}
	return ids, nil
}

// scoredModel mirrors provider/openai.ScoredModel to avoid the import cycle.
type scoredModel struct {
	ID           string
	RankPosition int
}

// rankModelsInline records discovered model IDs in provider-returned order.
func rankModelsInline(ids []string) []scoredModel {
	scored := make([]scoredModel, 0, len(ids))
	for pos, id := range ids {
		scored = append(scored, scoredModel{ID: id, RankPosition: pos})
	}
	return scored
}

// resolveContextEvidence resolves the context window for a model using the
// precedence chain: provider config > provider API > catalog > default.
func resolveContextEvidence(ctx context.Context, entry ServiceProviderEntry, modelID string, cat *modelcatalog.Catalog) (int, string) {
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return 0, ContextSourceUnknown
	}
	if entry.ContextWindow > 0 {
		return entry.ContextWindow, ContextSourceProviderConfig
	}
	if limits, source := providerAPIContextEvidence(ctx, entry, modelID); limits > 0 {
		return limits, source
	}
	if cat != nil {
		if n := cat.ContextWindowForModel(modelID); n > 0 {
			return n, ContextSourceCatalog
		}
	}
	return compaction.DefaultContextWindow, ContextSourceDefault
}

func providerAPIContextEvidence(ctx context.Context, entry ServiceProviderEntry, modelID string) (int, string) {
	switch entry.Type {
	case "lmstudio":
		if entry.BaseURL == "" {
			return 0, ""
		}
		if limits := lmstudio.LookupModelLimits(ctx, entry.BaseURL, modelID); limits.ContextLength > 0 {
			return limits.ContextLength, ContextSourceProviderAPI
		}
	case "omlx":
		if entry.BaseURL == "" {
			return 0, ""
		}
		if limits := omlx.LookupModelLimits(ctx, entry.BaseURL, modelID); limits.ContextLength > 0 {
			return limits.ContextLength, ContextSourceProviderAPI
		}
	case "openrouter":
		if entry.BaseURL == "" {
			return 0, ""
		}
		if limits := openrouter.LookupModelLimits(ctx, entry.BaseURL, entry.APIKey, entry.Headers, modelID); limits.ContextLength > 0 {
			return limits.ContextLength, ContextSourceProviderAPI
		}
	}
	return 0, ""
}

// catalogCostAndPerf extracts CostInfo and PerfSignal for a model from the catalog.
func catalogCostAndPerf(cat *modelcatalog.Catalog, modelID string) (CostInfo, PerfSignal) {
	entry, ok := cat.LookupModel(modelID)
	if ok {
		return CostInfo{
				InputPerMTok:  catalogEntryInputCost(entry),
				OutputPerMTok: catalogEntryOutputCost(entry),
			}, PerfSignal{
				SpeedTokensPerSec: entry.SpeedTokensPerSec,
				SWEBenchVerified:  entry.SWEBenchVerified,
			}
	}

	// Fallback: try legacy pricing via PricingFor.
	pricing := cat.PricingFor()
	if p, ok := pricing[modelID]; ok {
		return CostInfo{
			InputPerMTok:  p.InputPerMTok,
			OutputPerMTok: p.OutputPerMTok,
		}, PerfSignal{}
	}

	return CostInfo{}, PerfSignal{}
}

func catalogPowerEligibility(cat *modelcatalog.Catalog, modelID string) (int, bool, bool) {
	eligibility, ok := cat.ModelEligibility(modelID)
	if !ok {
		return 0, false, false
	}
	return eligibility.Power, eligibility.AutoRoutable, eligibility.ExactPinOnly
}

// catalogPowerForModel returns the catalog-projected power for a model
// (CONTRACT-003 § Catalog Power Projection). Returns 0 when the catalog
// is nil or the model has no entry, which the contract documents as
// "unknown / exact-pin-only / no catalog entry" for the
// ServiceRoutingActual.Power surface.
func catalogPowerForModel(cat *modelcatalog.Catalog, modelID string) int {
	if cat == nil || modelID == "" {
		return 0
	}
	power, _, _ := catalogPowerEligibility(cat, modelID)
	return power
}

func catalogEntryInputCost(entry modelcatalog.ModelEntry) float64 {
	if entry.CostInputPerM != 0 {
		return entry.CostInputPerM
	}
	return entry.CostInputPerMTok
}

func catalogEntryOutputCost(entry modelcatalog.ModelEntry) float64 {
	if entry.CostOutputPerM != 0 {
		return entry.CostOutputPerM
	}
	return entry.CostOutputPerMTok
}

// providerCapabilities returns the capability set for a provider entry.
func providerCapabilities(entry ServiceProviderEntry) []string {
	switch normalizeServiceProviderType(entry.Type) {
	case "anthropic":
		return []string{"tool_use", "vision", "streaming"}
	case "omlx":
		return []string{"tool_use", "streaming", "json_mode", "reasoning_control"}
	default:
		return []string{"tool_use", "streaming", "json_mode"}
	}
}
