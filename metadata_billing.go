package fizeau

import (
	"github.com/easel/fizeau/internal/harnesses"
	"github.com/easel/fizeau/internal/modelcatalog"
	"github.com/easel/fizeau/internal/routing"
)

func harnessPaymentKind(name string, cfg harnesses.HarnessConfig) BillingModel {
	if name == "" {
		name = cfg.Name
	}
	if billing := modelcatalog.BillingForHarness(name); billing != modelcatalog.BillingModelUnknown {
		return billing
	}
	if billing := modelcatalog.BillingForProviderSystem(name); billing != modelcatalog.BillingModelUnknown {
		return billing
	}
	if cfg.IsSubscription {
		return modelcatalog.BillingModelSubscription
	}
	if cfg.IsLocal {
		return modelcatalog.BillingModelFixed
	}
	return modelcatalog.BillingModelUnknown
}

func harnessRunsInProcessOrHTTP(cfg harnesses.HarnessConfig) bool {
	return cfg.IsHTTPProvider || cfg.IsLocal
}

func serviceProviderBilling(entry ServiceProviderEntry) BillingModel {
	if entry.Billing != modelcatalog.BillingModelUnknown {
		return entry.Billing
	}
	if billing := modelcatalog.BillingForProviderSystem(entry.Type); billing != modelcatalog.BillingModelUnknown {
		return billing
	}
	return modelcatalog.BillingForHarness(entry.Type)
}

func serviceProviderDefaultInclusion(entry ServiceProviderEntry) bool {
	if entry.IncludeByDefaultSet {
		return entry.IncludeByDefault
	}
	switch serviceProviderBilling(entry) {
	case modelcatalog.BillingModelFixed, modelcatalog.BillingModelSubscription:
		return true
	default:
		return false
	}
}

func providerTypeUsesFixedBilling(providerType string) bool {
	return modelcatalog.BillingForProviderSystem(providerType) == modelcatalog.BillingModelFixed
}

func routingHarnessEntryFromMetadata(name string, cfg harnesses.HarnessConfig, st harnesses.HarnessStatus) routing.HarnessEntry {
	billing := harnessPaymentKind(name, cfg)
	return routing.HarnessEntry{
		Name:                name,
		Surface:             cfg.Surface,
		CostClass:           cfg.CostClass,
		IsLocal:             billing == modelcatalog.BillingModelFixed,
		IsSubscription:      billing == modelcatalog.BillingModelSubscription,
		IsHTTPProvider:      cfg.IsHTTPProvider,
		AutoRoutingEligible: cfg.AutoRoutingEligible,
		TestOnly:            cfg.TestOnly,
		ExactPinSupport:     cfg.ExactPinSupport,
		DefaultModel:        cfg.DefaultModel,
		SupportedModels:     subprocessHarnessModelIDs(name, cfg),
		SupportedReasoning:  supportedReasoning(cfg),
		MaxReasoningTokens:  cfg.MaxReasoningTokens,
		SupportedPerms:      supportedPermissions(cfg),
		SupportsTools:       true,
		Available:           st.Available,
		QuotaOK:             true,
		QuotaTrend:          routing.QuotaTrendUnknown,
		SubscriptionOK:      true,
	}
}

func routingHarnessUsesAccountBilling(entry *routing.HarnessEntry) bool {
	return entry != nil && entry.IsSubscription
}
