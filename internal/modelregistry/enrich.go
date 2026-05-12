package modelregistry

import (
	"strings"

	"github.com/easel/fizeau/internal/config"
	"github.com/easel/fizeau/internal/discoverycache"
	"github.com/easel/fizeau/internal/modelcatalog"
	"github.com/easel/fizeau/internal/runtimesignals"
)

const (
	exclusionCatalogUnknown     = "catalog_unknown"
	exclusionCatalogNotRoutable = "catalog_not_auto_routable"
	exclusionProviderExcluded   = "provider_include_by_default_false"
	exclusionStatusUnavailable  = "status_unavailable"
)

func enrichModel(model KnownModel, includeByDefault bool, cat *modelcatalog.Catalog) KnownModel {
	parsed := modelcatalog.Parse(model.ID)
	model.Family = parsed.Family
	model.Version = append([]int(nil), parsed.Version...)
	model.Tier = parsed.Tier
	model.PreRelease = parsed.PreRelease

	var catalogAuto bool
	if cat != nil {
		if entry, ok := cat.LookupModel(model.ID); ok {
			model.Power = entry.Power
			model.CostInputPerM = catalogCostInput(entry)
			model.CostOutputPerM = catalogCostOutput(entry)
			model.ContextWindow = entry.ContextWindow
			model.ReasoningLevels = append([]string(nil), entry.ReasoningLevels...)
			model.QuotaPool = entry.EffectiveQuotaPool()
		}
		if eligibility, ok := cat.ModelEligibility(model.ID); ok {
			catalogAuto = eligibility.AutoRoutable
		}
	}
	if !catalogAuto {
		if model.Power == 0 && len(model.ReasoningLevels) == 0 && model.ContextWindow == 0 {
			model.ExclusionReason = exclusionCatalogUnknown
		} else {
			model.ExclusionReason = exclusionCatalogNotRoutable
		}
	}
	if catalogAuto && !includeByDefault {
		model.ExclusionReason = exclusionProviderExcluded
	}
	if catalogAuto && includeByDefault && !statusAllowsRouting(model.Status) {
		model.ExclusionReason = exclusionStatusUnavailable
	}
	model.AutoRoutable = catalogAuto && includeByDefault && statusAllowsRouting(model.Status)
	return model
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

func statusAllowsRouting(status ModelStatus) bool {
	return status != StatusUnreachable && status != StatusUnknown
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
