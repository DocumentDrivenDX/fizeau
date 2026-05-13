package routing

import (
	"testing"
	"time"

	"github.com/easel/fizeau/internal/modelcatalog"
)

func TestCheapPolicySelectsLowestEffectiveCostSufficientCandidate(t *testing.T) {
	in := effectiveCostRoutingInputs(true, 7, 95)

	dec, err := Resolve(Request{Policy: "cheap", MinPower: 7}, in)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if dec.Harness != "fiz" || dec.Provider != "local" || dec.Model != "gpt-5.4-nano" {
		t.Fatalf("winner=%s/%s/%s, want fiz/local/gpt-5.4-nano", dec.Harness, dec.Provider, dec.Model)
	}
	wantFrontier := "gpt-5.5"
	if got := dec.Model; got == wantFrontier {
		t.Fatal("cheap policy selected gpt-5.5, want the lower effective-cost sufficient route")
	}
	assertEligibleCandidate(t, dec.Candidates, "codex", "", "gpt-5.5")
}

func TestDefaultPolicySelectsLowerEffectiveCostRoutineCandidateOverFrontier(t *testing.T) {
	in := effectiveCostRoutingInputs(false, 7, 95)

	dec, err := Resolve(Request{Policy: "default", MinPower: 7}, in)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if dec.Harness != "mini" || dec.Provider != "mini" || dec.Model != "gpt-5.4-mini" {
		t.Fatalf("winner=%s/%s/%s, want mini/mini/gpt-5.4-mini", dec.Harness, dec.Provider, dec.Model)
	}
	wantFrontier := "gpt-5.5"
	if got := dec.Model; got == wantFrontier {
		t.Fatal("default policy selected gpt-5.5, want the lower effective-cost routine route")
	}
	assertEligibleCandidate(t, dec.Candidates, "codex", "", "gpt-5.5")
}

func TestSmartPolicyCanSelectGPT55WhenQualityJustifiesIt(t *testing.T) {
	in := effectiveCostRoutingInputs(false, 10, 10)

	dec, err := Resolve(Request{Policy: "smart", MinPower: 7}, in)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	wantHarness, wantModel := "codex", "gpt-5.5"
	if dec.Harness != wantHarness || dec.Model != wantModel {
		t.Fatalf("winner=%s/%s, want codex/gpt-5.5", dec.Harness, dec.Model)
	}
	assertEligibleCandidate(t, dec.Candidates, "mini", "mini", "gpt-5.4-mini")
}

func TestMeteredOptInRequiresActualCashSpend(t *testing.T) {
	in := Inputs{
		Harnesses: []HarnessEntry{
			{
				Name:                "mini",
				Surface:             "embedded-openai",
				CostClass:           "cheap",
				AutoRoutingEligible: true,
				Available:           true,
				ExactPinSupport:     true,
				SupportsTools:       true,
				Providers: []ProviderEntry{{
					Name:                      "payg",
					DefaultModel:              "gpt-5.4-mini",
					Billing:                   modelcatalog.BillingModelPerToken,
					ActualCashSpend:           true,
					CostUSDPer1kTokens:        0.003,
					CostSource:                CostSourceCatalog,
					ExcludeFromDefaultRouting: true,
					SupportsTools:             true,
				}},
			},
			{
				Name:                "codex",
				Surface:             "codex",
				CostClass:           "medium",
				IsSubscription:      true,
				AutoRoutingEligible: true,
				Available:           true,
				QuotaOK:             true,
				QuotaPercentUsed:    10,
				QuotaTrend:          QuotaTrendHealthy,
				SubscriptionOK:      true,
				ExactPinSupport:     true,
				SupportsTools:       true,
				DefaultModel:        "gpt-5.5",
				Providers: []ProviderEntry{{
					Billing:            modelcatalog.BillingModelSubscription,
					CostUSDPer1kTokens: 0.009,
					CostSource:         CostSourceSubscription,
				}},
			},
		},
		ModelEligibility: testPowerLookup(map[string]int{
			"gpt-5.4-mini": 7,
			"gpt-5.5":      10,
		}),
		Now: time.Date(2026, 4, 30, 0, 0, 0, 0, time.UTC),
	}

	dec, err := Resolve(Request{Policy: "default", MinPower: 7}, in)
	if err != nil {
		t.Fatalf("Resolve default: %v", err)
	}
	payg, ok := candidateByIdentity(dec.Candidates, "mini", "payg", "gpt-5.4-mini")
	if !ok {
		t.Fatalf("payg candidate not found in %#v", dec.Candidates)
	}
	if payg.Eligible {
		t.Fatalf("payg candidate should be rejected without opt-in: %#v", payg)
	}
	if payg.FilterReason != FilterReasonMeteredOptInRequired {
		t.Fatalf("payg FilterReason=%q, want %q", payg.FilterReason, FilterReasonMeteredOptInRequired)
	}

	dec, err = Resolve(Request{Policy: "default", MinPower: 7, Provider: "payg"}, in)
	if err != nil {
		t.Fatalf("Resolve pinned payg: %v", err)
	}
	if dec.Provider != "payg" || dec.Model != "gpt-5.4-mini" {
		t.Fatalf("winner=%s/%s, want payg/gpt-5.4-mini", dec.Provider, dec.Model)
	}
}

func effectiveCostRoutingInputs(includeLocal bool, frontierPower, frontierQuota int) Inputs {
	harnesses := []HarnessEntry{
		{
			Name:                "mini",
			Surface:             "openrouter",
			CostClass:           "cheap",
			AutoRoutingEligible: true,
			Available:           true,
			ExactPinSupport:     true,
			SupportsTools:       true,
			Providers: []ProviderEntry{{
				Name:               "mini",
				DefaultModel:       "gpt-5.4-mini",
				Billing:            modelcatalog.BillingModelPerToken,
				ActualCashSpend:    true,
				CostUSDPer1kTokens: 0.003,
				CostSource:         CostSourceCatalog,
				SupportsTools:      true,
			}},
		},
		{
			Name:                "codex",
			Surface:             "codex",
			CostClass:           "medium",
			IsSubscription:      true,
			AutoRoutingEligible: true,
			Available:           true,
			QuotaOK:             true,
			QuotaPercentUsed:    frontierQuota,
			QuotaTrend:          QuotaTrendHealthy,
			SubscriptionOK:      true,
			ExactPinSupport:     true,
			SupportsTools:       true,
			DefaultModel:        "gpt-5.5",
			Providers: []ProviderEntry{{
				Billing:            modelcatalog.BillingModelSubscription,
				CostUSDPer1kTokens: 0.009,
				CostSource:         CostSourceSubscription,
			}},
		},
	}
	if includeLocal {
		harnesses = append([]HarnessEntry{{
			Name:                "fiz",
			Surface:             "embedded-openai",
			CostClass:           "local",
			IsLocal:             true,
			AutoRoutingEligible: true,
			Available:           true,
			ExactPinSupport:     true,
			SupportsTools:       true,
			Providers: []ProviderEntry{{
				Name:               "local",
				DefaultModel:       "gpt-5.4-nano",
				Billing:            modelcatalog.BillingModelFixed,
				CostUSDPer1kTokens: 0.001,
				CostSource:         CostSourceUserConfig,
				SupportsTools:      true,
			}},
		}}, harnesses...)
	}
	return Inputs{
		Harnesses: harnesses,
		ModelEligibility: testPowerLookup(map[string]int{
			"gpt-5.4-nano": 7,
			"gpt-5.4-mini": 7,
			"gpt-5.5":      frontierPower,
		}),
		Now: time.Date(2026, 4, 30, 0, 0, 0, 0, time.UTC),
	}
}

func candidateByIdentity(candidates []Candidate, harness, provider, model string) (Candidate, bool) {
	for _, candidate := range candidates {
		if candidate.Harness == harness && candidate.Provider == provider && candidate.Model == model {
			return candidate, true
		}
	}
	return Candidate{}, false
}

func assertEligibleCandidate(t *testing.T, candidates []Candidate, harness, provider, model string) {
	t.Helper()
	candidate, ok := candidateByIdentity(candidates, harness, provider, model)
	if !ok {
		t.Fatalf("candidate %s/%s/%s not found in %#v", harness, provider, model, candidates)
	}
	if !candidate.Eligible {
		t.Fatalf("candidate %s/%s/%s should be eligible: %#v", harness, provider, model, candidate)
	}
}
