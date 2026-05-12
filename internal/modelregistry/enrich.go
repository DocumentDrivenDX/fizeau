package modelregistry

import (
	"strings"

	"github.com/easel/fizeau/internal/config"
	"github.com/easel/fizeau/internal/discoverycache"
	"github.com/easel/fizeau/internal/modelcatalog"
	"github.com/easel/fizeau/internal/modeleligibility"
	"github.com/easel/fizeau/internal/runtimesignals"
)

const (
	exclusionCatalogUnknown     = "catalog_unknown"
	exclusionCatalogNotRoutable = "catalog_not_auto_routable"
	exclusionProviderExcluded   = "provider_include_by_default_false"
	exclusionStatusUnavailable  = "status_unavailable"
)

func EnrichModel(model KnownModel, includeByDefault bool, cat *modelcatalog.Catalog) KnownModel {
	parsed := modelcatalog.Parse(model.ID)
	model.Family = parsed.Family
	model.Version = append([]int(nil), parsed.Version...)
	model.Tier = parsed.Tier
	model.PreRelease = parsed.PreRelease

	view := modeleligibility.Resolve(model.ID, includeByDefault, string(model.Status), cat)
	model.Power = view.Power
	model.ExactPinOnly = view.ExactPinOnly
	model.AutoRoutable = view.AutoRoutable
	model.ExclusionReason = view.ExclusionReason
	if cat != nil {
		if entry, ok := cat.LookupModel(model.ID); ok {
			model.CostInputPerM = catalogCostInput(entry)
			model.CostOutputPerM = catalogCostOutput(entry)
			model.ContextWindow = entry.ContextWindow
			model.ReasoningLevels = append([]string(nil), entry.ReasoningLevels...)
			model.QuotaPool = entry.EffectiveQuotaPool()
		}
	}
	return model
}

func enrichModel(model KnownModel, includeByDefault bool, cat *modelcatalog.Catalog) KnownModel {
	return EnrichModel(model, includeByDefault, cat)
}

func attachRuntimeSignals(model KnownModel, cache *discoverycache.Cache) KnownModel {
	if cache == nil {
		return model
	}
	sig, ok := runtimesignals.ReadCached(cache, model.Provider)
	if !ok || sig == nil {
		return model
	}
	model.QuotaRemaining = sig.QuotaRemaining
	model.RecentP50Latency = sig.RecentP50Latency
	return model
}

func effectiveProviderIncludeByDefault(providerName string, pc config.ProviderConfig, cat *modelcatalog.Catalog) bool {
	if pc.IncludeByDefault != nil {
		return *pc.IncludeByDefault
	}
	if cat != nil {
		name := strings.ToLower(strings.TrimSpace(providerName))
		providerType := strings.ToLower(strings.TrimSpace(pc.Type))
		for _, provider := range cat.Providers() {
			catalogName := strings.ToLower(strings.TrimSpace(provider.Name))
			catalogType := strings.ToLower(strings.TrimSpace(provider.Type))
			if name != "" && catalogName == name {
				return provider.IncludeByDefault
			}
			if providerType != "" && (catalogType == providerType || catalogName == providerType) {
				return provider.IncludeByDefault
			}
		}
	}
	switch modelcatalog.BillingModel(strings.TrimSpace(pc.Billing)) {
	case modelcatalog.BillingModelFixed, modelcatalog.BillingModelSubscription:
		return true
	case modelcatalog.BillingModelPerToken:
		return false
	}
	if billing := modelcatalog.BillingForProviderSystem(pc.Type); billing != modelcatalog.BillingModelUnknown {
		return billing == modelcatalog.BillingModelFixed || billing == modelcatalog.BillingModelSubscription
	}
	billing := modelcatalog.BillingForHarness(pc.Type)
	return billing == modelcatalog.BillingModelFixed || billing == modelcatalog.BillingModelSubscription
}

func providerStatus(cache *discoverycache.Cache, providerName string) ModelStatus {
	if cache == nil {
		return StatusAvailable
	}
	sig, ok := runtimesignals.ReadCached(cache, providerName)
	if !ok || sig == nil {
		return StatusAvailable
	}
	switch sig.Status {
	case runtimesignals.StatusAvailable, runtimesignals.StatusDegraded:
		return StatusAvailable
	case runtimesignals.StatusExhausted:
		return StatusRateLimited
	case runtimesignals.StatusUnknown:
		return StatusUnknown
	default:
		return StatusUnknown
	}
}

func catalogCostInput(entry modelcatalog.ModelEntry) float64 {
	if entry.CostInputPerM != 0 {
		return entry.CostInputPerM
	}
	return entry.CostInputPerMTok
}

func catalogCostOutput(entry modelcatalog.ModelEntry) float64 {
	if entry.CostOutputPerM != 0 {
		return entry.CostOutputPerM
	}
	return entry.CostOutputPerMTok
}
