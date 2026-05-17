package routing

import (
	"testing"
	"time"

	"github.com/easel/fizeau/internal/modelcatalog"
)

// multiTierSubscriptionInputsWithEligibility returns the multi-tier claude
// fixture with a caller-supplied ModelEligibility resolver. Sibling tests use
// this to simulate broken catalog metadata for one tier (e.g. sonnet-4.6 with
// no power field) without disturbing the rest of the fixture.
func multiTierSubscriptionInputsWithEligibility(elig func(string) (ModelEligibility, bool)) Inputs {
	in := multiTierSubscriptionInputs()
	in.ModelEligibility = elig
	return in
}

// TestExcludedTierEmitsFilterReason locks in the bead-fizeau-d37752b9 contract:
// when one tier of a multi-tier subscription harness has broken catalog
// metadata (no power, not auto-routable), the routing_decision candidates
// list MUST still contain a row for that tier with eligible:false and
// FilterReasonPowerMissing — never a silent drop.
func TestExcludedTierEmitsFilterReason(t *testing.T) {
	in := multiTierSubscriptionInputsWithEligibility(func(model string) (ModelEligibility, bool) {
		switch model {
		case "opus-4.7":
			return ModelEligibility{Power: 10, AutoRoutable: true}, true
		case "sonnet-4.6":
			// Catalog known but power metadata removed.
			return ModelEligibility{Power: 0, AutoRoutable: false}, true
		default:
			return ModelEligibility{}, false
		}
	})
	dec, err := Resolve(Request{Policy: "default"}, in)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	var sonnet *Candidate
	for i := range dec.Candidates {
		c := &dec.Candidates[i]
		if c.Harness == "claude" && c.Model == "sonnet-4.6" {
			sonnet = c
			break
		}
	}
	if sonnet == nil {
		t.Fatalf("claude/sonnet-4.6 row missing from routing_decision candidates; got: %#v", dec.Candidates)
	}
	if sonnet.Eligible {
		t.Errorf("claude/sonnet-4.6 Eligible=true, want false (broken catalog metadata)")
	}
	if sonnet.FilterReason != FilterReasonPowerMissing {
		t.Errorf("claude/sonnet-4.6 FilterReason=%q, want %q", sonnet.FilterReason, FilterReasonPowerMissing)
	}
	if sonnet.Reason == "" {
		t.Errorf("claude/sonnet-4.6 Reason is empty; want human-readable diagnostic")
	}
}

// TestNoSilentDropForCatalogKnownTier asserts that for every tier listed in a
// configured subscription harness's SupportedModels, the routing_decision
// candidates list contains exactly one row keyed by that tier — either
// eligible or with a non-empty FilterReason. No tier may be silently absent.
func TestNoSilentDropForCatalogKnownTier(t *testing.T) {
	// Build a harness with three tiers; mark the middle tier as broken
	// metadata to ensure all three rows survive regardless of eligibility.
	in := Inputs{
		Harnesses: []HarnessEntry{{
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
			SupportedModels:     []string{"opus-4.7", "sonnet-4.6", "haiku-4.5"},
			AutoRoutingModels:   []string{"opus-4.7", "sonnet-4.6", "haiku-4.5"},
			Providers: []ProviderEntry{{
				Billing:            modelcatalog.BillingModelSubscription,
				CostUSDPer1kTokens: 0.045,
				CostSource:         CostSourceSubscription,
				CostUSDPer1kTokensByModel: map[string]float64{
					"opus-4.7":   0.045,
					"sonnet-4.6": 0.009,
					"haiku-4.5":  0.002,
				},
				SupportsTools: true,
			}},
		}},
		ModelEligibility: func(model string) (ModelEligibility, bool) {
			switch model {
			case "opus-4.7":
				return ModelEligibility{Power: 10, AutoRoutable: true}, true
			case "sonnet-4.6":
				return ModelEligibility{Power: 0, AutoRoutable: false}, true
			case "haiku-4.5":
				return ModelEligibility{Power: 6, AutoRoutable: true}, true
			default:
				return ModelEligibility{}, false
			}
		},
		Now: time.Date(2026, 5, 16, 0, 0, 0, 0, time.UTC),
	}
	dec, err := Resolve(Request{Policy: "default"}, in)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	cases := []struct {
		tier         string
		wantEligible bool
	}{
		{"opus-4.7", true},
		{"sonnet-4.6", false},
		{"haiku-4.5", true},
	}
	for _, tc := range cases {
		t.Run(tc.tier, func(t *testing.T) {
			matches := 0
			var got Candidate
			for _, c := range dec.Candidates {
				if c.Harness == "claude" && c.Model == tc.tier {
					matches++
					got = c
				}
			}
			if matches == 0 {
				t.Fatalf("claude/%s missing from candidates (silent drop)", tc.tier)
			}
			if matches > 1 {
				t.Fatalf("claude/%s appears %d times, want exactly 1", tc.tier, matches)
			}
			if got.Eligible != tc.wantEligible {
				t.Errorf("claude/%s Eligible=%v, want %v", tc.tier, got.Eligible, tc.wantEligible)
			}
			if !got.Eligible && got.FilterReason == FilterReasonEligible {
				t.Errorf("claude/%s ineligible row has empty FilterReason — must carry an explicit reason", tc.tier)
			}
		})
	}
}

// TestFilterReasonStringsAreStable guards against ad-hoc filter_reason strings
// drifting in via new emitter sites. Every ineligible candidate's FilterReason
// must be one of the documented FilterReason* constants in engine.go.
func TestFilterReasonStringsAreStable(t *testing.T) {
	in := multiTierSubscriptionInputsWithEligibility(func(model string) (ModelEligibility, bool) {
		switch model {
		case "opus-4.7":
			return ModelEligibility{Power: 10, AutoRoutable: true}, true
		case "sonnet-4.6":
			return ModelEligibility{Power: 0, AutoRoutable: false}, true
		default:
			return ModelEligibility{}, false
		}
	})
	dec, err := Resolve(Request{Policy: "default"}, in)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	known := map[FilterReason]struct{}{
		FilterReasonEligible:                    {},
		FilterReasonContextTooSmall:             {},
		FilterReasonNoToolSupport:               {},
		FilterReasonReasoningUnsupported:        {},
		FilterReasonUnhealthy:                   {},
		FilterReasonScoredBelowTop:              {},
		FilterReasonPinMismatch:                 {},
		FilterReasonPowerMissing:                {},
		FilterReasonBelowMinPower:               {},
		FilterReasonAboveMaxPower:               {},
		FilterReasonExactPinOnly:                {},
		FilterReasonNotAutoRoutable:             {},
		FilterReasonQuotaExhausted:              {},
		FilterReasonPolicyFiltered:              {},
		FilterReasonProviderExcludedFromDefault: {},
		FilterReasonMeteredOptInRequired:        {},
		FilterReasonCallerExcluded:              {},
		FilterReasonEndpointUnreachable:         {},
		FilterReasonCredentialMissing:           {},
	}
	sawIneligible := false
	for _, c := range dec.Candidates {
		if _, ok := known[c.FilterReason]; !ok {
			t.Errorf("candidate %s/%s/%s carries ad-hoc FilterReason=%q (not in the enumerated set)",
				c.Harness, c.Provider, c.Model, c.FilterReason)
		}
		if !c.Eligible {
			sawIneligible = true
			if c.FilterReason == FilterReasonEligible {
				t.Errorf("ineligible candidate %s/%s/%s has FilterReasonEligible (empty); want explicit reason",
					c.Harness, c.Provider, c.Model)
			}
		}
	}
	if !sawIneligible {
		t.Fatal("test fixture failed to produce any ineligible candidate; tier-exclusion path not exercised")
	}
}
