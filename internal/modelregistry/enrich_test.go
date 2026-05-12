package modelregistry

import (
	"testing"
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

	unknown := EnrichModel(KnownModel{
		Provider: "openrouter",
		ID:       "uncataloged-model",
		Status:   StatusAvailable,
	}, true, cat)
	if unknown.Power != 0 || unknown.CostInputPerM != 0 || unknown.CostOutputPerM != 0 || unknown.ContextWindow != 0 || len(unknown.ReasoningLevels) != 0 || unknown.QuotaPool != "" {
		t.Fatalf("uncataloged metadata = %#v, want zero/empty", unknown)
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
