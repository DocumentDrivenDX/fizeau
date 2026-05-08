package routing

import (
	"testing"
	"time"
)

func TestSubscriptionPreferenceFavorsHighestHeadroom(t *testing.T) {
	in := Inputs{
		Harnesses: []HarnessEntry{
			{
				Name:                "codex-low-headroom",
				Surface:             "codex",
				CostClass:           "medium",
				IsSubscription:      true,
				AutoRoutingEligible: true,
				Available:           true,
				QuotaOK:             true,
				QuotaPercentUsed:    85,
				QuotaTrend:          QuotaTrendHealthy,
				SubscriptionOK:      true,
				ExactPinSupport:     true,
				DefaultModel:        "frontier-low",
				SupportsTools:       true,
			},
			{
				Name:                "codex-high-headroom",
				Surface:             "codex",
				CostClass:           "medium",
				IsSubscription:      true,
				AutoRoutingEligible: true,
				Available:           true,
				QuotaOK:             true,
				QuotaPercentUsed:    15,
				QuotaTrend:          QuotaTrendHealthy,
				SubscriptionOK:      true,
				ExactPinSupport:     true,
				DefaultModel:        "frontier-high",
				SupportsTools:       true,
			},
		},
		ModelEligibility: testPowerLookup(map[string]int{
			"frontier-low":  10,
			"frontier-high": 10,
		}),
		Now: time.Date(2026, 4, 30, 0, 0, 0, 0, time.UTC),
	}

	dec, err := Resolve(Request{Policy: "smart", ProviderPreference: ProviderPreferenceSubscriptionFirst}, in)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if dec.Harness != "codex-high-headroom" {
		t.Fatalf("Harness=%q, want codex-high-headroom", dec.Harness)
	}
}
