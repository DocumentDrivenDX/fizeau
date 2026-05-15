package fizeau

// This file implements ListModels for the FizeauService service.
// It lives in the root package to avoid import cycles; provider and catalog
// data is injected via ServiceConfig (defined in service.go).
//
// Provider-backed models are assembled from the unified model snapshot used by
// the CLI model inventory path. Codex and Claude expose a separate
// harness-native surface backed by PTY/CLI evidence.

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/easel/fizeau/internal/compaction"
	"github.com/easel/fizeau/internal/harnesses"
	claudeharness "github.com/easel/fizeau/internal/harnesses/claude"
	codexharness "github.com/easel/fizeau/internal/harnesses/codex"
	geminiharness "github.com/easel/fizeau/internal/harnesses/gemini"
	"github.com/easel/fizeau/internal/modelcatalog"
	"github.com/easel/fizeau/internal/modelsnapshot"
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
	cacheRoot, err := serviceSnapshotCacheRoot()
	if err != nil {
		return nil, err
	}
	snapshot, err := assembleModelSnapshotFromServiceConfigWithOptions(ctx, sc, cat, cacheRoot, modelsnapshot.AssembleOptions{Refresh: modelsnapshot.RefreshForce})
	if err != nil {
		return nil, err
	}
	return s.listModelsFromSnapshot(ctx, sc, cat, snapshot, filter), nil
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
		runner := &claudeharness.Runner{}
		snapshot := runner.DefaultModelSnapshot()
		models = appendUniqueModelIDs(models, snapshot.Models...)
		for _, family := range runner.SupportedAliases() {
			if resolved, err := runner.ResolveModelAlias(family, snapshot); err == nil && resolved != family {
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

func (s *service) listModelsFromSnapshot(ctx context.Context, sc ServiceConfig, cat *modelcatalog.Catalog, snapshot modelsnapshot.ModelSnapshot, filter ModelFilter) []ModelInfo {
	defaultProviderName := sc.DefaultProviderName()
	entries := make(map[string]ServiceProviderEntry, len(sc.ProviderNames()))
	for _, name := range sc.ProviderNames() {
		if filter.Provider != "" && filter.Provider != name {
			continue
		}
		entry, ok := sc.Provider(name)
		if !ok {
			continue
		}
		entries[name] = entry
	}

	modelsByProvider := make(map[string][]modelsnapshot.KnownModel, len(snapshot.Models))
	for _, model := range snapshot.Models {
		if filter.Provider != "" && filter.Provider != model.Provider {
			continue
		}
		if _, ok := entries[model.Provider]; !ok {
			continue
		}
		modelsByProvider[model.Provider] = append(modelsByProvider[model.Provider], model)
	}

	rankByEndpoint := make(map[string]int, len(snapshot.Models))
	out := make([]ModelInfo, 0, len(snapshot.Models))
	for _, providerName := range sc.ProviderNames() {
		if filter.Provider != "" && filter.Provider != providerName {
			continue
		}
		entry, ok := entries[providerName]
		if !ok {
			continue
		}
		for _, model := range modelsByProvider[providerName] {
			normalizedModel := model
			normalizedModel.ServerInstance = serverinstance.Normalize(model.EndpointBaseURL, model.ServerInstance)
			rankKey := strings.Join([]string{normalizedModel.Provider, normalizedModel.EndpointName, normalizedModel.EndpointBaseURL, normalizedModel.ServerInstance}, "\x00")
			rank := rankByEndpoint[rankKey]
			rankByEndpoint[rankKey] = rank + 1
			out = append(out, s.modelInfoFromSnapshotModel(ctx, providerName, entry, defaultProviderName, cat, normalizedModel, rank))
		}
	}
	return out
}

func (s *service) modelInfoFromSnapshotModel(ctx context.Context, providerName string, entry ServiceProviderEntry, defaultProviderName string, cat *modelcatalog.Catalog, model modelsnapshot.KnownModel, rankPosition int) ModelInfo {
	info := ModelInfo{
		ID:              model.ID,
		Provider:        providerName,
		ProviderType:    model.ProviderType,
		Harness:         model.Harness,
		EndpointName:    model.EndpointName,
		EndpointBaseURL: model.EndpointBaseURL,
		ServerInstance:  model.ServerInstance,
		ContextLength:   model.ContextWindow,
		Utilization:     s.routeUtilizationEvidence(providerName, model.ServerInstance, model.EndpointName, model.ID),
		Capabilities:    providerCapabilities(entry),
		Cost: CostInfo{
			InputPerMTok:  model.CostInputPerM,
			OutputPerMTok: model.CostOutputPerM,
		},
		Power:                         model.Power,
		AutoRoutable:                  model.AutoRoutable,
		ExactPinOnly:                  model.ExactPinOnly,
		Billing:                       model.Billing,
		ActualCashSpend:               model.ActualCashSpend,
		EffectiveCost:                 model.EffectiveCost,
		EffectiveCostSource:           model.EffectiveCostSource,
		SupportsTools:                 model.SupportsTools,
		DeploymentClass:               model.DeploymentClass,
		HealthFreshnessAt:             model.HealthFreshnessAt,
		HealthFreshnessSource:         model.HealthFreshnessSource,
		QuotaFreshnessAt:              model.QuotaFreshnessAt,
		QuotaFreshnessSource:          model.QuotaFreshnessSource,
		ModelDiscoveryFreshnessAt:     model.DiscoveredAt,
		ModelDiscoveryFreshnessSource: string(model.DiscoveredVia),
		Available:                     true,
		RankPosition:                  rankPosition,
	}
	if info.Harness == "" {
		info.Harness = "fiz"
	}
	info.ContextLength, info.ContextSource = resolveContextEvidence(ctx, entry, model.ID, cat)
	if cat != nil {
		_, info.PerfSignal = catalogCostAndPerf(cat, model.ID)
	}
	info.IsDefault = providerName == defaultProviderName && entry.Model != "" && model.ID == entry.Model
	return info
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
	if !ok {
		return CostInfo{}, PerfSignal{}
	}
	return CostInfo{
			InputPerMTok:  catalogEntryInputCost(entry),
			OutputPerMTok: catalogEntryOutputCost(entry),
		}, PerfSignal{
			SpeedTokensPerSec: entry.SpeedTokensPerSec,
			SWEBenchVerified:  entry.SWEBenchVerified,
		}
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
