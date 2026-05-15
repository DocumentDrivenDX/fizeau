package routing

import (
	"testing"
	"time"

	"github.com/easel/fizeau/internal/modelcatalog"
)

func TestEffectiveCostRouting(t *testing.T) {
	t.Run("cheap chooses the lowest sufficient local route", func(t *testing.T) {
		in := effectiveCostRoutingInputs(true, 7, 95)
		dec, err := Resolve(Request{Policy: "cheap", MinPower: 7}, in)
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}
		if dec.Harness != "fiz" || dec.Provider != "local" || dec.Model != "gpt-5.4-nano" {
			t.Fatalf("winner=%s/%s/%s, want fiz/local/gpt-5.4-nano", dec.Harness, dec.Provider, dec.Model)
		}
	})

	t.Run("default prefers routine local capacity over frontier", func(t *testing.T) {
		in := effectiveCostRoutingInputs(false, 7, 95)
		dec, err := Resolve(Request{Policy: "default", MinPower: 7}, in)
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}
		if dec.Harness != "mini" || dec.Provider != "mini" || dec.Model != "gpt-5.4-mini" {
			t.Fatalf("winner=%s/%s/%s, want mini/mini/gpt-5.4-mini", dec.Harness, dec.Provider, dec.Model)
		}
	})

	t.Run("smart can still choose gpt-5.5 when it dominates", func(t *testing.T) {
		in := effectiveCostRoutingInputs(false, 10, 10)
		dec, err := Resolve(Request{Policy: "smart", MinPower: 7}, in)
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}
		if dec.Harness != "codex" || dec.Model != "gpt-5.5" {
			t.Fatalf("winner=%s/%s, want codex/gpt-5.5", dec.Harness, dec.Model)
		}
	})
}

func TestNoRemoteRouting(t *testing.T) {
	t.Run("prefers the only local route", func(t *testing.T) {
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
					Providers: []ProviderEntry{{
						Name:          "local",
						DefaultModel:  "local-model",
						SupportsTools: true,
					}},
				},
				{
					Name:                "remote",
					Surface:             "embedded-openai",
					CostClass:           "medium",
					AutoRoutingEligible: true,
					ExactPinSupport:     true,
					Available:           true,
					QuotaOK:             true,
					SubscriptionOK:      true,
					SupportsTools:       true,
					Providers: []ProviderEntry{{
						Name:                      "payg",
						DefaultModel:              "frontier-model",
						Billing:                   modelcatalog.BillingModelPerToken,
						ActualCashSpend:           true,
						ExcludeFromDefaultRouting: false,
						SupportsTools:             true,
					}},
				},
			},
			ModelEligibility: testPowerLookup(map[string]int{
				"local-model":    7,
				"frontier-model": 10,
			}),
			Now: time.Date(2026, 5, 12, 15, 0, 0, 0, time.UTC),
		}
		dec, err := Resolve(Request{Require: []string{"no_remote"}}, in)
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}
		if dec.Harness != "fiz" || dec.Provider != "local" || dec.Model != "local-model" {
			t.Fatalf("winner=%s/%s/%s, want fiz/local/local-model", dec.Harness, dec.Provider, dec.Model)
		}
	})

	t.Run("fails clearly when no local route is dispatchable", func(t *testing.T) {
		in := Inputs{
			Harnesses: []HarnessEntry{
				{
					Name:                "fiz",
					Surface:             "embedded-openai",
					CostClass:           "local",
					IsLocal:             true,
					AutoRoutingEligible: true,
					ExactPinSupport:     true,
					Available:           false,
					QuotaOK:             true,
					SubscriptionOK:      true,
					SupportsTools:       true,
					Providers: []ProviderEntry{{
						Name:          "local",
						DefaultModel:  "local-model",
						SupportsTools: true,
					}},
				},
				{
					Name:                "remote",
					Surface:             "embedded-openai",
					CostClass:           "medium",
					AutoRoutingEligible: true,
					ExactPinSupport:     true,
					Available:           true,
					QuotaOK:             true,
					SubscriptionOK:      true,
					SupportsTools:       true,
					Providers: []ProviderEntry{{
						Name:            "payg",
						DefaultModel:    "frontier-model",
						Billing:         modelcatalog.BillingModelPerToken,
						ActualCashSpend: true,
						SupportsTools:   true,
					}},
				},
			},
			ModelEligibility: testPowerLookup(map[string]int{
				"local-model":    7,
				"frontier-model": 10,
			}),
			Now: time.Date(2026, 5, 12, 15, 0, 0, 0, time.UTC),
		}
		_, err := Resolve(Request{Require: []string{"no_remote"}}, in)
		if err == nil {
			t.Fatal("Resolve succeeded, want a no_remote failure when the only local route is down")
		}
	})
}

func TestMeteredOptInRouting(t *testing.T) {
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
				Providers: []ProviderEntry{{
					Name:          "local",
					DefaultModel:  "gpt-5.4-mini",
					SupportsTools: true,
				}},
			},
			{
				Name:                "opt-out",
				Surface:             "embedded-openai",
				CostClass:           "medium",
				AutoRoutingEligible: true,
				ExactPinSupport:     true,
				Available:           true,
				QuotaOK:             true,
				SubscriptionOK:      true,
				SupportsTools:       true,
				Providers: []ProviderEntry{{
					Name:                      "remote-open",
					DefaultModel:              "gpt-5.5",
					Billing:                   modelcatalog.BillingModelPerToken,
					ActualCashSpend:           true,
					ExcludeFromDefaultRouting: true,
					SupportsTools:             true,
				}},
			},
			{
				Name:                "opt-in",
				Surface:             "embedded-openai",
				CostClass:           "medium",
				AutoRoutingEligible: true,
				ExactPinSupport:     true,
				Available:           true,
				QuotaOK:             true,
				SubscriptionOK:      true,
				SupportsTools:       true,
				Providers: []ProviderEntry{{
					Name:            "remote-optin",
					DefaultModel:    "gpt-5.5",
					Billing:         modelcatalog.BillingModelPerToken,
					ActualCashSpend: true,
					SupportsTools:   true,
				}},
			},
		},
		ModelEligibility: testPowerLookup(map[string]int{
			"gpt-5.4-mini": 7,
			"gpt-5.5":      10,
		}),
		Now: time.Date(2026, 5, 12, 15, 0, 0, 0, time.UTC),
	}

	dec, err := Resolve(Request{Policy: "smart", MinPower: 7}, in)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if dec.Harness != "opt-in" || dec.Provider != "remote-optin" || dec.Model != "gpt-5.5" {
		t.Fatalf("winner=%s/%s/%s, want opt-in/remote-optin/gpt-5.5", dec.Harness, dec.Provider, dec.Model)
	}
	var sawOptOut bool
	for _, c := range dec.Candidates {
		if c.Provider != "remote-open" {
			continue
		}
		sawOptOut = true
		if c.Eligible {
			t.Fatalf("remote-open candidate should be rejected without metered opt-in: %#v", c)
		}
		if c.FilterReason != FilterReasonMeteredOptInRequired {
			t.Fatalf("remote-open FilterReason=%q, want %q", c.FilterReason, FilterReasonMeteredOptInRequired)
		}
	}
	if !sawOptOut {
		t.Fatal("missing remote-open candidate in routed trace")
	}
}
