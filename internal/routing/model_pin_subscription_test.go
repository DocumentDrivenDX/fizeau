package routing

import (
	"testing"
	"time"

	"github.com/easel/fizeau/internal/modelcatalog"
)

// modelPinSubscriptionFixture returns Inputs with both:
//   - a claude subscription harness whose SupportedModels covers sonnet-4.6
//   - a fiz harness with an openrouter provider whose DiscoveredIDs also
//     covers sonnet-4.6 (the catalog-known openrouter_id surface)
//
// This mirrors the fizeau-84b485d2 setup where pinning --model sonnet-4.6
// against a configured subscription harness was silently routing to
// fiz/openrouter on score, dropping the operator's subscription auth.
func modelPinSubscriptionFixture() Inputs {
	return Inputs{
		Harnesses: []HarnessEntry{
			{
				Name:                "fiz",
				Surface:             "embedded-openai",
				CostClass:           "local",
				IsLocal:             true,
				AutoRoutingEligible: true,
				ExactPinSupport:     true,
				Available:           true,
				QuotaOK:             true,
				SubscriptionOK:      true,
				SupportsTools:       true,
				Providers: []ProviderEntry{
					{
						Name:               "openrouter",
						BaseURL:            "https://openrouter.ai/api/v1",
						DefaultModel:       "anthropic/claude-sonnet-4-6",
						DiscoveredIDs:      []string{"sonnet-4.6", "anthropic/claude-sonnet-4-6", "qwen/qwen3.6"},
						SupportsTools:      true,
						Billing:            modelcatalog.BillingModelPerToken,
						CostUSDPer1kTokens: 0.003,
						CostSource:         CostSourceCatalog,
						ActualCashSpend:    true,
					},
				},
			},
			{
				Name:                "claude",
				Surface:             "claude",
				CostClass:           "medium",
				IsSubscription:      true,
				AutoRoutingEligible: true,
				ExactPinSupport:     true,
				Available:           true,
				QuotaOK:             true,
				SubscriptionOK:      true,
				SupportsTools:       true,
				DefaultModel:        "opus-4.7",
				SupportedModels:     []string{"opus-4.7", "sonnet-4.6"},
				AutoRoutingModels:   []string{"opus-4.7", "sonnet-4.6"},
				Providers: []ProviderEntry{{
					Billing:            modelcatalog.BillingModelSubscription,
					CostUSDPer1kTokens: 0.045,
					CostSource:         CostSourceSubscription,
					CostUSDPer1kTokensByModel: map[string]float64{
						"opus-4.7":   0.045,
						"sonnet-4.6": 0.009,
					},
					SupportsTools: true,
				}},
			},
		},
		ModelEligibility: func(model string) (ModelEligibility, bool) {
			switch model {
			case "opus-4.7":
				return ModelEligibility{Power: 10, AutoRoutable: true}, true
			case "sonnet-4.6":
				return ModelEligibility{Power: 8, AutoRoutable: true}, true
			case "qwen/qwen3.6":
				return ModelEligibility{Power: 7, AutoRoutable: true}, true
			default:
				return ModelEligibility{}, false
			}
		},
		Now: time.Date(2026, 5, 16, 0, 0, 0, 0, time.UTC),
	}
}

// TestModelPinPrefersSubscriptionHarnessOverOpenrouter pins fizeau-84b485d2:
// when --model sonnet-4.6 is pinned (no --harness/--provider override) and a
// configured subscription harness lists that model in its SupportedModels,
// dispatch must resolve to the subscription harness rather than the
// fiz/openrouter route that happens to score higher on the catalog-known
// openrouter_id.
func TestModelPinPrefersSubscriptionHarnessOverOpenrouter(t *testing.T) {
	in := modelPinSubscriptionFixture()
	dec, err := Resolve(Request{Model: "sonnet-4.6"}, in)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if dec.Harness != "claude" {
		t.Fatalf("dispatched harness=%q, want claude; candidates=%+v", dec.Harness, dec.Candidates)
	}
	if dec.Model != "sonnet-4.6" {
		t.Fatalf("dispatched model=%q, want sonnet-4.6", dec.Model)
	}
	// The fiz/openrouter candidate must still appear in the trace, but as
	// ineligible — the routing engine recorded that the subscription
	// preference suppressed it.
	var sawOpenrouter bool
	for _, c := range dec.Candidates {
		if c.Harness != "fiz" || c.Provider != "openrouter" {
			continue
		}
		sawOpenrouter = true
		if c.Eligible {
			t.Errorf("fiz/openrouter candidate stayed eligible despite subscription preference: %+v", c)
		}
	}
	if !sawOpenrouter {
		t.Fatalf("expected fiz/openrouter candidate in trace; candidates=%+v", dec.Candidates)
	}
}

// TestModelPinFallsBackWhenNoSubscriptionHarnessAdvertisesModel pins the
// guardrail half of fizeau-84b485d2: if no configured subscription harness
// lists the pinned model in its SupportedModels, dispatch must fall back to
// fiz/openrouter as before. Without this assertion the preference could
// over-fire and strand operators whose subscription tiers don't cover the
// pin.
func TestModelPinFallsBackWhenNoSubscriptionHarnessAdvertisesModel(t *testing.T) {
	in := modelPinSubscriptionFixture()
	dec, err := Resolve(Request{Model: "qwen/qwen3.6"}, in)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if dec.Harness != "fiz" || dec.Provider != "openrouter" {
		t.Fatalf("dispatched harness/provider=%q/%q, want fiz/openrouter; candidates=%+v",
			dec.Harness, dec.Provider, dec.Candidates)
	}
	if dec.Model != "qwen/qwen3.6" {
		t.Fatalf("dispatched model=%q, want qwen/qwen3.6", dec.Model)
	}
}
