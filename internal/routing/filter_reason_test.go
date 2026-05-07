package routing

import (
	"testing"
)

// TestCandidateFilterReasonAtRejectionSite verifies that each rejection
// site in the routing engine sets a typed FilterReason on the produced
// Candidate at the point the rejection decision is made — not derived
// later by parsing free-form Reason text (fiz-2c55b8a4).
func TestCandidateFilterReasonAtRejectionSite(t *testing.T) {
	t.Run("context window too small", func(t *testing.T) {
		in := Inputs{
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
							Name:           "local",
							DefaultModel:   "tiny-model",
							ContextWindows: map[string]int{"tiny-model": 4096},
						},
					},
				},
			},
		}
		dec, _ := Resolve(Request{
			Harness:               "fiz",
			EstimatedPromptTokens: 100_000,
		}, in)
		assertRejection(t, dec, FilterReasonContextTooSmall)
	})

	t.Run("no tool support", func(t *testing.T) {
		in := Inputs{
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
					SupportsTools:       false,
					Providers: []ProviderEntry{
						{Name: "local", DefaultModel: "no-tools", SupportsTools: false},
					},
				},
			},
		}
		dec, _ := Resolve(Request{Harness: "fiz", RequiresTools: true}, in)
		assertRejection(t, dec, FilterReasonNoToolSupport)
	})

	t.Run("reasoning unsupported", func(t *testing.T) {
		in := Inputs{
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
					SupportedReasoning:  []string{"low"},
					Providers: []ProviderEntry{
						{Name: "local", DefaultModel: "model"},
					},
				},
			},
		}
		dec, _ := Resolve(Request{Harness: "fiz", Reasoning: "high"}, in)
		assertRejection(t, dec, FilterReasonReasoningUnsupported)
	})

	t.Run("harness not available is unhealthy", func(t *testing.T) {
		in := Inputs{
			Harnesses: []HarnessEntry{
				{
					Name:                "fiz",
					Surface:             "embedded-openai",
					CostClass:           "local",
					IsLocal:             true,
					AutoRoutingEligible: true,
					Available:           false, // unavailable
					QuotaOK:             true,
					SubscriptionOK:      true,
				},
			},
		}
		dec, _ := Resolve(Request{Harness: "fiz"}, in)
		assertRejection(t, dec, FilterReasonUnhealthy)
	})

	t.Run("subscription quota exhausted is unhealthy", func(t *testing.T) {
		in := Inputs{
			Harnesses: []HarnessEntry{
				{
					Name:                "claude",
					Surface:             "claude",
					CostClass:           "medium",
					IsSubscription:      true,
					AutoRoutingEligible: true,
					ExactPinSupport:     true,
					Available:           true,
					QuotaOK:             false,
					SubscriptionOK:      false, // quota exhausted
					SupportsTools:       true,
					DefaultModel:        "claude-sonnet-4-6",
				},
			},
		}
		dec, _ := Resolve(Request{Harness: "claude"}, in)
		assertRejection(t, dec, FilterReasonUnhealthy)
	})

	t.Run("provider preference local-only is unhealthy", func(t *testing.T) {
		in := Inputs{
			Harnesses: []HarnessEntry{
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
					DefaultModel:        "claude-sonnet-4-6",
				},
			},
		}
		dec, _ := Resolve(Request{
			Harness:            "claude",
			ProviderPreference: ProviderPreferenceLocalOnly,
		}, in)
		assertRejection(t, dec, FilterReasonUnhealthy)
	})

	t.Run("eligible candidate has no filter reason", func(t *testing.T) {
		in := Inputs{
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
						{Name: "local", DefaultModel: "model", SupportsTools: true},
					},
				},
			},
		}
		dec, err := Resolve(Request{Harness: "fiz"}, in)
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}
		if len(dec.Candidates) == 0 {
			t.Fatal("expected at least one candidate")
		}
		c := dec.Candidates[0]
		if !c.Eligible {
			t.Fatalf("expected eligible candidate, got Reason=%q FilterReason=%q", c.Reason, c.FilterReason)
		}
		if c.FilterReason != FilterReasonEligible {
			t.Errorf("eligible candidate FilterReason=%q, want empty", c.FilterReason)
		}
	})
}

func assertRejection(t *testing.T, dec *Decision, want FilterReason) {
	t.Helper()
	if dec == nil || len(dec.Candidates) == 0 {
		t.Fatal("expected at least one candidate in decision")
	}
	for _, c := range dec.Candidates {
		if c.Eligible {
			continue
		}
		if c.FilterReason != want {
			t.Errorf("candidate %s/%s/%s: FilterReason=%q (Reason=%q), want %q",
				c.Harness, c.Provider, c.Model, c.FilterReason, c.Reason, want)
		}
		return
	}
	t.Fatalf("no rejected candidate found in decision; candidates=%+v", dec.Candidates)
}
