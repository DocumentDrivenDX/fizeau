package routing

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestRoutingPolicyInvariants(t *testing.T) {
	cases := []struct {
		policyStatement string
		req             Request
		inputs          Inputs
		wantHarness     string
		wantProvider    string
		wantEndpoint    string
		wantModel       string
		wantOnlyHarness string
		wantErrContains string
		wantRejected    map[string]FilterReason
	}{
		{
			policyStatement: "config only sets up provider sources; routing uses discovered inventory",
			req:             Request{Model: "configured-default"},
			inputs:          policyBaseInputs(),
			wantErrContains: "no live endpoint offers a match",
			wantRejected: map[string]FilterReason{
				"fiz/local": FilterReasonScoredBelowTop,
				"fiz/paid":  FilterReasonScoredBelowTop,
				"codex/":    FilterReasonScoredBelowTop,
			},
		},
		{
			policyStatement: "if no power provider is configured, use the best cheapest viable model",
			req:             Request{},
			inputs:          policyInputsWithoutPower(),
			wantHarness:     "fiz",
			wantProvider:    "local",
			wantModel:       "local-good",
		},
		{
			policyStatement: "soft bounded power can still prefer a healthier subscription route",
			req:             Request{MinPower: 7, MaxPower: 7},
			inputs:          policyInputsWithDiscoveredLocalDefault(),
			wantHarness:     "fiz",
			wantProvider:    "local",
			wantModel:       "local-good",
		},
		{
			policyStatement: "if local/free is unavailable, fall back to the best viable non-local candidate",
			req:             Request{MinPower: 7},
			inputs:          policyInputsWithLocalUnavailable(),
			wantHarness:     "codex",
			wantModel:       "cloud-frontier",
			wantRejected: map[string]FilterReason{
				"fiz/": FilterReasonUnhealthy,
			},
		},
		{
			policyStatement: "max power excludes overpowered candidates while min power remains soft",
			req:             Request{MinPower: 8, MaxPower: 8},
			inputs:          policyInputsWithDiscoveredLocalDefault(),
			wantHarness:     "fiz",
			wantProvider:    "local",
			wantModel:       "local-good",
			wantRejected: map[string]FilterReason{
				"codex/": FilterReasonAboveMaxPower,
			},
		},
		{
			policyStatement: "an exact model pin overrides local preference",
			req:             Request{Model: "paid-strong"},
			inputs:          policyInputsWithDiscoveredLocalDefault(),
			wantHarness:     "fiz",
			wantProvider:    "paid",
			wantModel:       "paid-strong",
			wantRejected: map[string]FilterReason{
				"fiz/local": FilterReasonScoredBelowTop,
			},
		},
		{
			policyStatement: "a provider endpoint pin is exclusive",
			req:             Request{Provider: "paid@secondary", Model: "paid-strong"},
			inputs:          policyInputsWithProviderEndpoints(),
			wantHarness:     "fiz",
			wantProvider:    "paid@secondary",
			wantEndpoint:    "secondary",
			wantModel:       "paid-strong",
			wantRejected: map[string]FilterReason{
				"fiz/paid@primary": FilterReasonPinMismatch,
			},
		},
		{
			policyStatement: "a harness pin is exclusive",
			req:             Request{Harness: "codex", Model: "cloud-frontier"},
			inputs:          policyBaseInputs(),
			wantHarness:     "codex",
			wantModel:       "cloud-frontier",
			wantOnlyHarness: "codex",
		},
		{
			policyStatement: "unknown-power models remain routable when no power bound is requested",
			req:             Request{},
			inputs:          policyInputsWithUnknownPowerModel(),
			wantHarness:     "fiz",
			wantProvider:    "local",
			wantModel:       "local-good",
		},
		{
			policyStatement: "unknown-power exact pins are allowed",
			req:             Request{Model: "unknown-model"},
			inputs:          policyInputsWithUnknownPowerModel(),
			wantHarness:     "fiz",
			wantProvider:    "unknown",
			wantModel:       "unknown-model",
		},
		{
			policyStatement: "provider/deployment class keeps local community models below cloud frontier when benchmarks tie",
			req:             Request{},
			inputs:          policyInputsWithTiedBenchmarks(),
			wantHarness:     "codex",
			wantModel:       "cloud-frontier",
		},
	}

	for _, tc := range cases {
		t.Run(tc.policyStatement, func(t *testing.T) {
			dec, err := Resolve(tc.req, tc.inputs)
			if tc.wantErrContains != "" {
				if err == nil {
					t.Fatalf("policy_statement=%q: Resolve succeeded with decision=%#v, want error containing %q", tc.policyStatement, dec, tc.wantErrContains)
				}
				if !strings.Contains(err.Error(), tc.wantErrContains) {
					t.Fatalf("policy_statement=%q: error=%q, want contains %q", tc.policyStatement, err.Error(), tc.wantErrContains)
				}
			} else if err != nil {
				t.Fatalf("policy_statement=%q: Resolve error: %v", tc.policyStatement, err)
			}

			if tc.wantHarness != "" && dec.Harness != tc.wantHarness {
				t.Fatalf("policy_statement=%q: harness=%q, want %q; candidates=%s", tc.policyStatement, dec.Harness, tc.wantHarness, policyCandidateSummary(dec.Candidates))
			}
			if tc.wantProvider != "" && dec.Provider != tc.wantProvider {
				t.Fatalf("policy_statement=%q: provider=%q, want %q; candidates=%s", tc.policyStatement, dec.Provider, tc.wantProvider, policyCandidateSummary(dec.Candidates))
			}
			if tc.wantEndpoint != "" && dec.Endpoint != tc.wantEndpoint {
				t.Fatalf("policy_statement=%q: endpoint=%q, want %q; candidates=%s", tc.policyStatement, dec.Endpoint, tc.wantEndpoint, policyCandidateSummary(dec.Candidates))
			}
			if tc.wantModel != "" && dec.Model != tc.wantModel {
				t.Fatalf("policy_statement=%q: model=%q, want %q; candidates=%s", tc.policyStatement, dec.Model, tc.wantModel, policyCandidateSummary(dec.Candidates))
			}
			if tc.wantOnlyHarness != "" {
				for _, candidate := range dec.Candidates {
					if candidate.Harness != tc.wantOnlyHarness {
						t.Fatalf("policy_statement=%q: candidate harness=%q, want only %q; candidates=%s", tc.policyStatement, candidate.Harness, tc.wantOnlyHarness, policyCandidateSummary(dec.Candidates))
					}
				}
			}
			for key, want := range tc.wantRejected {
				candidate, ok := policyCandidateByKey(dec.Candidates, key)
				if !ok {
					t.Fatalf("policy_statement=%q: candidate %q not found in %s", tc.policyStatement, key, policyCandidateSummary(dec.Candidates))
				}
				if candidate.Eligible {
					t.Fatalf("policy_statement=%q: candidate %q remained eligible, want rejected with %q", tc.policyStatement, key, want)
				}
				if candidate.FilterReason != want {
					t.Fatalf("policy_statement=%q: candidate %q filter_reason=%q, want %q; reason=%q", tc.policyStatement, key, candidate.FilterReason, want, candidate.Reason)
				}
			}
		})
	}
}

func TestRoutingPolicyNoCandidateCarriesRejectedTrace(t *testing.T) {
	policyStatement := "no-candidate errors expose rejected candidate reasons for caller retry decisions"
	dec, err := Resolve(Request{Provider: "missing-provider"}, policyBaseInputs())
	if err == nil {
		t.Fatalf("policy_statement=%q: Resolve succeeded with decision=%#v, want no viable candidate", policyStatement, dec)
	}
	var noViable *NoViableCandidateError
	if !errors.As(err, &noViable) {
		t.Fatalf("policy_statement=%q: error=%T %v, want NoViableCandidateError", policyStatement, err, err)
	}
	if noViable.Provider != "missing-provider" {
		t.Fatalf("policy_statement=%q: provider=%q, want missing-provider", policyStatement, noViable.Provider)
	}
	if len(dec.Candidates) == 0 {
		t.Fatalf("policy_statement=%q: rejected trace is empty", policyStatement)
	}
	for _, c := range dec.Candidates {
		if c.Eligible || c.FilterReason == FilterReasonEligible {
			t.Fatalf("policy_statement=%q: candidate=%#v, want rejected candidate with typed reason", policyStatement, c)
		}
	}
}

func policyBaseInputs() Inputs {
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
						Name:               "local",
						DefaultModel:       "configured-default",
						DiscoveredIDs:      []string{"local-good"},
						DiscoveryAttempted: true,
						SupportsTools:      true,
					},
					{
						Name:               "paid",
						DefaultModel:       "paid-strong",
						DiscoveredIDs:      []string{"paid-strong"},
						DiscoveryAttempted: true,
						CostUSDPer1kTokens: 0.05,
						CostSource:         CostSourceCatalog,
						SupportsTools:      true,
					},
				},
			},
			{
				Name:                "codex",
				Surface:             "codex",
				CostClass:           "medium",
				IsSubscription:      true,
				AutoRoutingEligible: true,
				ExactPinSupport:     true,
				Available:           true,
				QuotaOK:             true,
				QuotaPercentUsed:    10,
				QuotaTrend:          QuotaTrendHealthy,
				SubscriptionOK:      true,
				DefaultModel:        "cloud-frontier",
				SupportedModels:     []string{"cloud-frontier"},
				SupportsTools:       true,
				Providers: []ProviderEntry{{
					CostSource: CostSourceSubscription,
				}},
			},
		},
		ModelEligibility: policyPowerLookup(map[string]ModelEligibility{
			"local-good":     {Power: 7, AutoRoutable: true},
			"paid-strong":    {Power: 8, AutoRoutable: true},
			"cloud-frontier": {Power: 10, AutoRoutable: true},
		}),
		Now: time.Date(2026, 4, 30, 0, 0, 0, 0, time.UTC),
	}
}

func policyInputsWithoutPower() Inputs {
	in := policyBaseInputs()
	in.ModelEligibility = nil
	in.Harnesses = in.Harnesses[:1]
	in.Harnesses[0].Providers[0].DefaultModel = "local-good"
	return in
}

func policyInputsWithDiscoveredLocalDefault() Inputs {
	in := policyBaseInputs()
	in.Harnesses[0].Providers[0].DefaultModel = "local-good"
	return in
}

func policyInputsWithLocalUnavailable() Inputs {
	in := policyBaseInputs()
	in.Harnesses[0].Available = false
	return in
}

func policyInputsWithProviderEndpoints() Inputs {
	in := policyBaseInputs()
	in.Harnesses = in.Harnesses[:1]
	in.Harnesses[0].Providers = []ProviderEntry{
		{
			Name:               "paid@primary",
			EndpointName:       "primary",
			DefaultModel:       "paid-strong",
			DiscoveredIDs:      []string{"paid-strong"},
			DiscoveryAttempted: true,
			CostUSDPer1kTokens: 0.05,
			CostSource:         CostSourceCatalog,
			SupportsTools:      true,
		},
		{
			Name:               "paid@secondary",
			EndpointName:       "secondary",
			DefaultModel:       "paid-strong",
			DiscoveredIDs:      []string{"paid-strong"},
			DiscoveryAttempted: true,
			CostUSDPer1kTokens: 0.05,
			CostSource:         CostSourceCatalog,
			SupportsTools:      true,
		},
	}
	return in
}

func policyInputsWithUnknownPowerModel() Inputs {
	in := policyInputsWithDiscoveredLocalDefault()
	in.Harnesses = in.Harnesses[:1]
	in.Harnesses[0].Providers = append(in.Harnesses[0].Providers, ProviderEntry{
		Name:               "unknown",
		DefaultModel:       "unknown-model",
		DiscoveredIDs:      []string{"unknown-model"},
		DiscoveryAttempted: true,
		SupportsTools:      true,
	})
	return in
}

func policyInputsWithTiedBenchmarks() Inputs {
	in := policyBaseInputs()
	in.Harnesses[0].Providers = []ProviderEntry{
		{
			Name:               "community",
			DefaultModel:       "gpt-oss-120b-local",
			DiscoveredIDs:      []string{"gpt-oss-120b-local"},
			DiscoveryAttempted: true,
			SupportsTools:      true,
		},
	}
	in.ModelEligibility = policyPowerLookup(map[string]ModelEligibility{
		"gpt-oss-120b-local": {Power: 7, AutoRoutable: true},
		"cloud-frontier":     {Power: 10, AutoRoutable: true},
	})
	in.ProviderSuccessRate = map[string]float64{
		ProviderModelKey("community", "", "gpt-oss-120b-local"): 0.9,
		ProviderModelKey("", "", "cloud-frontier"):              0.9,
	}
	in.ObservedSpeedTPS = map[string]float64{
		ProviderModelKey("community", "", "gpt-oss-120b-local"): 100,
		ProviderModelKey("", "", "cloud-frontier"):              100,
	}
	return in
}

func policyPowerLookup(entries map[string]ModelEligibility) func(string) (ModelEligibility, bool) {
	return func(model string) (ModelEligibility, bool) {
		eligibility, ok := entries[model]
		return eligibility, ok
	}
}

func policyCandidateByKey(candidates []Candidate, key string) (Candidate, bool) {
	for _, c := range candidates {
		if c.Harness+"/"+c.Provider == key {
			return c, true
		}
	}
	return Candidate{}, false
}

func policyCandidateSummary(candidates []Candidate) string {
	var parts []string
	for _, c := range candidates {
		parts = append(parts, fmt.Sprintf("%s/%s/%s eligible=%v reason=%s score=%.1f", c.Harness, c.Provider, c.Model, c.Eligible, c.FilterReason, c.Score))
	}
	return strings.Join(parts, "; ")
}
