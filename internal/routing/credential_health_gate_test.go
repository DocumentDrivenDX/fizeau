package routing

import (
	"testing"
	"time"
)

// fourFilterReasons is the set of FilterReason values fizeau-458f10b0 adds for
// the provider health gate. The credential-check and credit-probe children
// rely on every constant being routable through the generic eligibility
// override plumbing, so the table-driven tests exercise each one.
var fourFilterReasons = []FilterReason{
	FilterReasonCredentialMissing,
	FilterReasonCredentialInvalid,
	FilterReasonCreditExhausted,
	FilterReasonProviderUnreachable,
}

// healthGateInputs builds a two-provider routing inputs fixture: openrouter
// is the gated provider under test and claude is a healthy fallback so
// Resolve has a viable winner whenever openrouter is filtered out.
func healthGateInputs(now time.Time, overrides map[string]ProviderEligibilityOverride) Inputs {
	return Inputs{
		Now: now,
		Harnesses: []HarnessEntry{
			{
				Name:                "fiz",
				Surface:             "embedded-openai",
				CostClass:           "remote",
				AutoRoutingEligible: true,
				Available:           true,
				QuotaOK:             true,
				SubscriptionOK:      true,
				SupportsTools:       true,
				Providers: []ProviderEntry{
					{
						Name:          "openrouter",
						BaseURL:       "https://openrouter.ai/api/v1",
						DefaultModel:  "openrouter/auto",
						DiscoveredIDs: []string{"openrouter/auto"},
						SupportsTools: true,
					},
				},
			},
			{
				Name:                "claude",
				Surface:             "claude",
				CostClass:           "medium",
				IsSubscription:      true,
				AutoRoutingEligible: true,
				Available:           true,
				QuotaOK:             true,
				SubscriptionOK:      true,
				SupportsTools:       true,
				DefaultModel:        "claude-sonnet-4-6",
			},
		},
		ProviderEligibilityOverrides: overrides,
	}
}

func findCandidate(t *testing.T, dec *Decision, provider string) Candidate {
	t.Helper()
	for _, c := range dec.Candidates {
		if c.Provider == provider {
			return c
		}
	}
	t.Fatalf("provider %q missing from decision candidates: %+v", provider, dec.Candidates)
	return Candidate{}
}

// TestEligibilityOverride_GatesProviderForEachFilterReason covers AC 4a:
// every new FilterReason gates a matching provider when no pin is present
// and routing falls through to the healthy fallback harness.
func TestEligibilityOverride_GatesProviderForEachFilterReason(t *testing.T) {
	now := time.Date(2026, 5, 16, 10, 0, 0, 0, time.UTC)
	probeAt := now.Add(-90 * time.Second)

	for _, reason := range fourFilterReasons {
		reason := reason
		t.Run(string(reason), func(t *testing.T) {
			in := healthGateInputs(now, map[string]ProviderEligibilityOverride{
				"openrouter": {FilterReason: reason, ProbeAt: probeAt},
			})

			dec, err := Resolve(Request{Policy: "default"}, in)
			if err != nil {
				t.Fatalf("Resolve: %v", err)
			}
			if dec.Provider == "openrouter" {
				t.Fatalf("openrouter selected despite %s override; decision=%+v", reason, dec)
			}
			if dec.Harness != "claude" {
				t.Errorf("expected fallback to claude harness, got harness=%q provider=%q", dec.Harness, dec.Provider)
			}

			cand := findCandidate(t, dec, "openrouter")
			if cand.Eligible {
				t.Errorf("openrouter eligible=true under %s override; want false", reason)
			}
			if cand.FilterReason != reason {
				t.Errorf("openrouter FilterReason = %q, want %q", cand.FilterReason, reason)
			}
			if cand.Reason == "" {
				t.Error("override gate did not populate candidate Reason")
			}
		})
	}
}

// TestEligibilityOverride_ProviderPinBypassesGate covers AC 3 and AC 4b: an
// explicit provider pin reaches the gated provider regardless of which
// FilterReason the override carries, mirroring the endpoint_unreachable gate.
func TestEligibilityOverride_ProviderPinBypassesGate(t *testing.T) {
	now := time.Date(2026, 5, 16, 10, 0, 0, 0, time.UTC)
	probeAt := now.Add(-90 * time.Second)

	for _, reason := range fourFilterReasons {
		reason := reason
		t.Run(string(reason), func(t *testing.T) {
			in := healthGateInputs(now, map[string]ProviderEligibilityOverride{
				"openrouter": {FilterReason: reason, ProbeAt: probeAt},
			})

			dec, err := Resolve(Request{Policy: "default", Provider: "openrouter"}, in)
			if err != nil {
				t.Fatalf("Resolve with explicit pin under %s override: %v", reason, err)
			}
			if dec == nil || dec.Provider != "openrouter" {
				t.Fatalf("expected openrouter to win under explicit pin (%s), got %+v", reason, dec)
			}
			cand := findCandidate(t, dec, "openrouter")
			if !cand.Eligible {
				t.Errorf("openrouter eligible=false despite explicit pin under %s", reason)
			}
			if cand.FilterReason == reason {
				t.Errorf("pinned openrouter still recorded %s FilterReason; want override skipped", reason)
			}
		})
	}
}

// TestEligibilityOverride_NilAndAbsentProviderAreNoops covers AC 4c: the gate
// is nil-safe (no panic on a missing map), an empty map is a no-op, and a
// provider that does not appear in the override map is unaffected.
func TestEligibilityOverride_NilAndAbsentProviderAreNoops(t *testing.T) {
	now := time.Date(2026, 5, 16, 10, 0, 0, 0, time.UTC)

	cases := []struct {
		name      string
		overrides map[string]ProviderEligibilityOverride
	}{
		{name: "nil map", overrides: nil},
		{name: "empty map", overrides: map[string]ProviderEligibilityOverride{}},
		{
			name: "unrelated provider",
			overrides: map[string]ProviderEligibilityOverride{
				"some-other-provider": {
					FilterReason: FilterReasonCredentialMissing,
					ProbeAt:      now,
				},
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			in := healthGateInputs(now, tc.overrides)
			dec, err := Resolve(Request{Policy: "default"}, in)
			if err != nil {
				t.Fatalf("Resolve: %v", err)
			}
			cand := findCandidate(t, dec, "openrouter")
			if !cand.Eligible {
				t.Errorf("openrouter eligible=false with overrides=%v; want true (no gating applies)", tc.overrides)
			}
			for _, fr := range fourFilterReasons {
				if cand.FilterReason == fr {
					t.Errorf("openrouter recorded %s FilterReason with no matching override entry", fr)
				}
			}
		})
	}
}
