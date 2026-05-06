package routing

import (
	"testing"
	"time"
)

func TestScorePowerSelectsPrepaidFrontierWhenQuotaHealthy(t *testing.T) {
	in := scorePowerInputs()

	dec, err := Resolve(Request{Profile: "smart"}, in)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if dec.Harness != "codex" || dec.Model != "frontier" {
		t.Fatalf("winner=%s/%s, want codex/frontier", dec.Harness, dec.Model)
	}
	assertCandidatePower(t, dec.Candidates, "codex", "", 10)
}

func TestScorePowerPrefersLocalFreeWhenPowerSufficient(t *testing.T) {
	in := scorePowerInputs()
	in.Harnesses = in.Harnesses[:2]

	dec, err := Resolve(Request{MinPower: 7}, in)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if dec.Harness != "agent" || dec.Provider != "local" {
		t.Fatalf("winner=%s/%s, want agent/local", dec.Harness, dec.Provider)
	}
	for _, c := range dec.Candidates {
		if c.Provider == "paid" && c.Eligible && c.Score >= dec.Candidates[0].Score {
			t.Fatalf("paid cloud should not beat sufficient local/free candidate: %#v", c)
		}
	}
}

func TestScorePowerMinPowerAndPinsRemainConstraints(t *testing.T) {
	in := scorePowerInputs()

	dec, err := Resolve(Request{MinPower: 9}, in)
	if err != nil {
		t.Fatalf("Resolve MinPower: %v", err)
	}
	if dec.Harness != "codex" || dec.Model != "frontier" {
		t.Fatalf("MinPower winner=%s/%s, want codex/frontier", dec.Harness, dec.Model)
	}
	for _, c := range dec.Candidates {
		if c.Provider == "local" && c.FilterReason != FilterReasonBelowMinPower {
			t.Fatalf("local FilterReason=%q, want %q", c.FilterReason, FilterReasonBelowMinPower)
		}
	}

	dec, err = Resolve(Request{Harness: "agent", Provider: "local", Model: "local-good", MinPower: 9}, in)
	if err != nil {
		t.Fatalf("Resolve exact pin: %v", err)
	}
	if dec.Harness != "agent" || dec.Provider != "local" || dec.Model != "local-good" {
		t.Fatalf("exact pin winner=%s/%s/%s, want agent/local/local-good", dec.Harness, dec.Provider, dec.Model)
	}

	dec, err = Resolve(Request{Provider: "local", MinPower: 9}, in)
	if err != nil {
		t.Fatalf("Resolve pinned provider: %v", err)
	}
	if dec.Provider != "local" || dec.Model != "local-good" {
		t.Fatalf("pinned provider winner=%s/%s, want local/local-good", dec.Provider, dec.Model)
	}
	for _, c := range dec.Candidates {
		if c.Provider == "local" && !c.Eligible {
			t.Fatalf("pinned provider must remain eligible under MinPower: %#v", c)
		}
	}
}

func TestScorePowerCostAndSpeedBreakTies(t *testing.T) {
	t.Run("cost", func(t *testing.T) {
		in := tieBreakInputs()
		dec, err := Resolve(Request{MinPower: 7}, in)
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}
		if dec.Provider != "cheap" {
			t.Fatalf("cost tie winner=%q, want cheap", dec.Provider)
		}
	})

	t.Run("speed", func(t *testing.T) {
		in := tieBreakInputs()
		in.Harnesses[0].Providers[0].CostUSDPer1kTokens = 0.01
		in.Harnesses[0].Providers[1].CostUSDPer1kTokens = 0.01
		in.ObservedSpeedTPS = map[string]float64{
			ProviderModelKey("cheap", "", "tie-model"): 40,
			ProviderModelKey("fast", "", "tie-model"):  120,
		}
		dec, err := Resolve(Request{MinPower: 7}, in)
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}
		if dec.Provider != "fast" {
			t.Fatalf("speed tie winner=%q, want fast", dec.Provider)
		}
	})
}

func scorePowerInputs() Inputs {
	return Inputs{
		Harnesses: []HarnessEntry{
			{
				Name:                "agent",
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
					Name:               "local",
					DefaultModel:       "local-good",
					DiscoveredIDs:      []string{"local-good"},
					DiscoveryAttempted: true,
					SupportsTools:      true,
				}},
			},
			{
				Name:                "paid-cloud",
				Surface:             "embedded-openai",
				CostClass:           "medium",
				AutoRoutingEligible: true,
				ExactPinSupport:     true,
				Available:           true,
				QuotaOK:             true,
				SubscriptionOK:      true,
				SupportsTools:       true,
				Providers: []ProviderEntry{{
					Name:               "paid",
					DefaultModel:       "paid-strong",
					DiscoveredIDs:      []string{"paid-strong"},
					DiscoveryAttempted: true,
					CostUSDPer1kTokens: 0.04,
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
				ExactPinSupport:     true,
				Available:           true,
				QuotaOK:             true,
				QuotaPercentUsed:    10,
				QuotaTrend:          QuotaTrendHealthy,
				SubscriptionOK:      true,
				DefaultModel:        "frontier",
				SupportedModels:     []string{"frontier"},
				SupportsTools:       true,
				Providers: []ProviderEntry{{
					CostSource: CostSourceSubscription,
				}},
			},
		},
		ModelEligibility: testPowerLookup(map[string]int{
			"local-good":  7,
			"paid-strong": 8,
			"frontier":    10,
		}),
		Now: time.Date(2026, 4, 30, 0, 0, 0, 0, time.UTC),
	}
}

func tieBreakInputs() Inputs {
	return Inputs{
		Harnesses: []HarnessEntry{{
			Name:                "agent",
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
				{Name: "cheap", DefaultModel: "tie-model", DiscoveredIDs: []string{"tie-model"}, CostUSDPer1kTokens: 0.01, CostSource: CostSourceCatalog, SupportsTools: true},
				{Name: "fast", DefaultModel: "tie-model", DiscoveredIDs: []string{"tie-model"}, CostUSDPer1kTokens: 0.03, CostSource: CostSourceCatalog, SupportsTools: true},
			},
		}},
		ModelEligibility: testPowerLookup(map[string]int{"tie-model": 7}),
		Now:              time.Date(2026, 4, 30, 0, 0, 0, 0, time.UTC),
	}
}

func testPowerLookup(powers map[string]int) func(string) (ModelEligibility, bool) {
	return func(model string) (ModelEligibility, bool) {
		power, ok := powers[model]
		if !ok {
			return ModelEligibility{}, false
		}
		return ModelEligibility{Power: power, AutoRoutable: true}, true
	}
}

func assertCandidatePower(t *testing.T, candidates []Candidate, harness, provider string, want int) {
	t.Helper()
	for _, c := range candidates {
		if c.Harness == harness && c.Provider == provider {
			if c.Power != want {
				t.Fatalf("%s/%s Power=%d, want %d", harness, provider, c.Power, want)
			}
			return
		}
	}
	t.Fatalf("candidate %s/%s not found", harness, provider)
}
