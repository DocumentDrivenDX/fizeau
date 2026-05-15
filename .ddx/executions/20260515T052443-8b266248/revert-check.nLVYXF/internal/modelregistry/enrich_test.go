package modelregistry

import (
	"testing"

	"github.com/easel/fizeau/internal/modelcatalog"
)

func TestEnrichSnapshotPopulatesCatalogMetadataAndLeavesUnknownZero(t *testing.T) {
	cat := loadTestCatalog(t)

	enriched := EnrichModel(KnownModel{
		Provider: "openrouter",
		ID:       "gpt-5.5",
		Status:   StatusAvailable,
	}, true, cat)
	if enriched.Power != 10 {
		t.Fatalf("Power = %d, want 10", enriched.Power)
	}
	if enriched.CostInputPerM != 1.25 || enriched.CostOutputPerM != 10.5 {
		t.Fatalf("cost = %v/%v, want 1.25/10.5", enriched.CostInputPerM, enriched.CostOutputPerM)
	}
	if enriched.ContextWindow != 400000 {
		t.Fatalf("ContextWindow = %d, want 400000", enriched.ContextWindow)
	}
	if len(enriched.ReasoningLevels) != 3 || enriched.ReasoningLevels[2] != "high" {
		t.Fatalf("ReasoningLevels = %#v, want [low medium high]", enriched.ReasoningLevels)
	}
	if enriched.QuotaPool != "openai-frontier" {
		t.Fatalf("QuotaPool = %q, want openai-frontier", enriched.QuotaPool)
	}
	if !enriched.SupportsTools {
		t.Fatalf("SupportsTools = false, want true")
	}
	if enriched.EffectiveCost <= 0 {
		t.Fatalf("EffectiveCost = %v, want positive", enriched.EffectiveCost)
	}
	if enriched.EffectiveCostSource != "catalog" && enriched.EffectiveCostSource != "subscription_shadow" {
		t.Fatalf("EffectiveCostSource = %q, want catalog or subscription_shadow", enriched.EffectiveCostSource)
	}

	unknown := EnrichModel(KnownModel{
		Provider: "openrouter",
		ID:       "uncataloged-model",
		Status:   StatusAvailable,
	}, true, cat)
	if unknown.Power != 0 || unknown.CostInputPerM != 0 || unknown.CostOutputPerM != 0 || unknown.ContextWindow != 0 || len(unknown.ReasoningLevels) != 0 || unknown.QuotaPool != "" {
		t.Fatalf("uncataloged metadata = %#v, want zero/empty", unknown)
	}
	if unknown.SupportsTools || unknown.EffectiveCost != 0 || unknown.EffectiveCostSource != "" || unknown.ActualCashSpend {
		t.Fatalf("uncataloged routing facts = %#v, want zero/false/empty", unknown)
	}
}

func TestEnrichSnapshotEffectiveCostAndCashSpend(t *testing.T) {
	cat := loadTestCatalog(t)

	subscription := EnrichModel(KnownModel{
		Provider: "codex-subscription",
		ID:       "gpt-5.5",
		Billing:  modelcatalog.BillingModelSubscription,
		Status:   StatusAvailable,
	}, true, cat)
	lowerTier := EnrichModel(KnownModel{
		Provider: "codex-subscription",
		ID:       "gpt-5.4-mini",
		Billing:  modelcatalog.BillingModelSubscription,
		Status:   StatusAvailable,
	}, true, cat)
	metered := EnrichModel(KnownModel{
		Provider: "openrouter",
		ID:       "gpt-5.5",
		Billing:  modelcatalog.BillingModelPerToken,
		Status:   StatusAvailable,
	}, true, cat)

	if subscription.ActualCashSpend {
		t.Fatalf("subscription.ActualCashSpend = true, want false")
	}
	if subscription.EffectiveCostSource != "subscription_shadow" {
		t.Fatalf("subscription.EffectiveCostSource = %q, want subscription_shadow", subscription.EffectiveCostSource)
	}
	if subscription.EffectiveCost <= lowerTier.EffectiveCost {
		t.Fatalf("subscription.EffectiveCost = %v, want greater than lower-tier %v", subscription.EffectiveCost, lowerTier.EffectiveCost)
	}
	if lowerTier.ActualCashSpend {
		t.Fatalf("lowerTier.ActualCashSpend = true, want false")
	}
	if metered.Billing != modelcatalog.BillingModelPerToken {
		t.Fatalf("metered.Billing = %q, want per_token", metered.Billing)
	}
	if !metered.ActualCashSpend {
		t.Fatalf("metered.ActualCashSpend = false, want true")
	}
	if metered.EffectiveCostSource != "catalog" {
		t.Fatalf("metered.EffectiveCostSource = %q, want catalog", metered.EffectiveCostSource)
	}
}

func TestAutoRoutableComposition(t *testing.T) {
	cat := loadTestCatalog(t)
	tests := []struct {
		name             string
		modelID          string
		includeByDefault bool
		status           ModelStatus
		wantAutoRoutable bool
		wantExclusion    string
	}{
		{
			name:             "eligible included available",
			modelID:          "gpt-5.5",
			includeByDefault: true,
			status:           StatusAvailable,
			wantAutoRoutable: true,
		},
		{
			name:             "catalog unknown",
			modelID:          "uncataloged-model",
			includeByDefault: true,
			status:           StatusAvailable,
			wantExclusion:    exclusionCatalogUnknown,
		},
		{
			name:             "provider excluded",
			modelID:          "gpt-5.5",
			includeByDefault: false,
			status:           StatusAvailable,
			wantExclusion:    exclusionProviderExcluded,
		},
		{
			name:             "unknown status",
			modelID:          "gpt-5.5",
			includeByDefault: true,
			status:           StatusUnknown,
			wantExclusion:    exclusionStatusUnavailable,
		},
		{
			name:             "unreachable status",
			modelID:          "gpt-5.5",
			includeByDefault: true,
			status:           StatusUnreachable,
			wantExclusion:    exclusionStatusUnavailable,
		},
		{
			name:             "rate limited still reachable",
			modelID:          "gpt-5.5",
			includeByDefault: true,
			status:           StatusRateLimited,
			wantAutoRoutable: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EnrichModel(KnownModel{
				Provider: "openrouter",
				ID:       tt.modelID,
				Status:   tt.status,
			}, tt.includeByDefault, cat)
			if got.AutoRoutable != tt.wantAutoRoutable {
				t.Fatalf("AutoRoutable = %v, want %v (model %#v)", got.AutoRoutable, tt.wantAutoRoutable, got)
			}
			if got.ExclusionReason != tt.wantExclusion {
				t.Fatalf("ExclusionReason = %q, want %q", got.ExclusionReason, tt.wantExclusion)
			}
		})
	}
}

func TestKnownModelSnapshotMarksExactPinOnlyCatalogModels(t *testing.T) {
	cat := loadTestCatalog(t)

	known := EnrichModel(KnownModel{
		Provider: "openrouter",
		ID:       "catalog-only-model",
		Status:   StatusAvailable,
	}, true, cat)
	if !known.ExactPinOnly {
		t.Fatalf("ExactPinOnly = false, want true for catalog-only-model: %#v", known)
	}
	if known.AutoRoutable {
		t.Fatalf("AutoRoutable = true, want false for exact-pin-only model: %#v", known)
	}
}
