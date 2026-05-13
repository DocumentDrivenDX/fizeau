package routing

import (
	"testing"
	"time"
)

func TestScorePowerSelectsPrepaidFrontierWhenQuotaHealthy(t *testing.T) {
	in := scorePowerInputs()

	dec, err := Resolve(Request{Policy: "smart"}, in)
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
	if dec.Harness != "fiz" || dec.Provider != "local" {
		t.Fatalf("winner=%s/%s, want fiz/local", dec.Harness, dec.Provider)
	}
	for _, c := range dec.Candidates {
		if c.Provider == "paid" && c.Eligible && c.Score >= dec.Candidates[0].Score {
			t.Fatalf("paid cloud should not beat sufficient local/free candidate: %#v", c)
		}
	}
}

func TestSoftPowerScoring_Power9BeatsPower7WhenHealthy(t *testing.T) {
	in := softPowerInputs(map[string]int{
		"power-7": 7,
		"power-9": 9,
	})

	dec, err := Resolve(Request{MinPower: 8, MaxPower: 10}, in)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if dec.Provider != "power-9" {
		t.Fatalf("Provider=%q, want power-9", dec.Provider)
	}
}

func TestSoftPowerScoring_AsymmetricUndershootHeavier(t *testing.T) {
	in := softPowerInputs(map[string]int{
		"power-7":  7,
		"power-11": 11,
	})

	dec, err := Resolve(Request{MinPower: 8, MaxPower: 10}, in)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if dec.Provider != "power-11" {
		t.Fatalf("Provider=%q, want power-11 because undershoot is penalized more heavily than overshoot", dec.Provider)
	}
}

func TestSoftPowerScoring_DefaultPolicyPrefersInBandOverUnderpoweredFree(t *testing.T) {
	in := scorePowerInputs()
	in.Harnesses = in.Harnesses[:2]
	in.ModelEligibility = testPowerLookup(map[string]int{
		"local-good":  5,
		"paid-strong": 8,
	})

	dec, err := Resolve(Request{Policy: "default", MinPower: 7, MaxPower: 8}, in)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if dec.Provider != "paid" {
		t.Fatalf("Provider=%q, want paid in-band candidate to beat free underpowered local route", dec.Provider)
	}
}

func TestSoftPowerScoring_HealthySubscriptionBonusStopsBelowEffectiveMax(t *testing.T) {
	in := scorePowerInputs()

	dec, err := Resolve(Request{Policy: "default", MinPower: 7, MaxPower: 8}, in)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	var codex Candidate
	for _, c := range dec.Candidates {
		if c.Harness == "codex" {
			codex = c
			break
		}
	}
	if codex.Harness != "codex" {
		t.Fatalf("codex candidate not found: %+v", dec.Candidates)
	}
	if got := codex.ScoreComponents["power"]; got != -2 {
		t.Fatalf("codex power component=%v, want -2 with max_power=8 (bonus suppressed)", got)
	}
}

func TestSoftPowerScoring_BelowMinPenaltyIsSteeperPerPoint(t *testing.T) {
	in := softPowerInputs(map[string]int{
		"below-min": 7,
		"above-max": 11,
	})

	dec, err := Resolve(Request{MinPower: 8, MaxPower: 10}, in)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	var below, above Candidate
	for _, c := range dec.Candidates {
		switch c.Provider {
		case "below-min":
			below = c
		case "above-max":
			above = c
		}
	}
	if below.Provider != "below-min" || above.Provider != "above-max" {
		t.Fatalf("candidates=%+v, want both power probes", dec.Candidates)
	}
	belowPerPoint := -below.ScoreComponents["power"] / float64(8-below.Power)
	abovePerPoint := -above.ScoreComponents["power"] / float64(above.Power-10)
	if belowPerPoint <= abovePerPoint {
		t.Fatalf("below-min penalty per point=%v, want steeper than above-max=%v (below=%#v above=%#v)", belowPerPoint, abovePerPoint, below, above)
	}
}

func TestScorePowerMinPowerIsSoftAndPinsRemainConstraints(t *testing.T) {
	in := scorePowerInputs()

	dec, err := Resolve(Request{MinPower: 9}, in)
	if err != nil {
		t.Fatalf("Resolve MinPower: %v", err)
	}
	if dec.Harness != "codex" || dec.Model != "frontier" {
		t.Fatalf("MinPower winner=%s/%s, want codex/frontier", dec.Harness, dec.Model)
	}
	for _, c := range dec.Candidates {
		if c.Provider == "local" && !c.Eligible {
			t.Fatalf("local candidate must remain eligible under soft MinPower: %#v", c)
		}
	}

	dec, err = Resolve(Request{Harness: "fiz", Provider: "local", Model: "local-good", MinPower: 9}, in)
	if err != nil {
		t.Fatalf("Resolve exact pin: %v", err)
	}
	if dec.Harness != "fiz" || dec.Provider != "local" || dec.Model != "local-good" {
		t.Fatalf("exact pin winner=%s/%s/%s, want fiz/local/local-good", dec.Harness, dec.Provider, dec.Model)
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

func softPowerInputs(powers map[string]int) Inputs {
	providers := make([]ProviderEntry, 0, len(powers))
	for model := range powers {
		providers = append(providers, ProviderEntry{
			Name:          model,
			CostClass:     "medium",
			DefaultModel:  model,
			SupportsTools: true,
		})
	}
	return Inputs{
		Harnesses: []HarnessEntry{{
			Name:                "fiz",
			Surface:             "embedded-openai",
			CostClass:           "medium",
			AutoRoutingEligible: true,
			ExactPinSupport:     true,
			Available:           true,
			QuotaOK:             true,
			SubscriptionOK:      true,
			SupportsTools:       true,
			Providers:           providers,
		}},
		ModelEligibility: testPowerLookup(powers),
		Now:              time.Date(2026, 4, 30, 0, 0, 0, 0, time.UTC),
	}
}

func TestPolicyDefaultKeepsPowerFiveLocalCandidatesEligible(t *testing.T) {
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
					{Name: "grendel", DefaultModel: "grendel-5", DiscoveredIDs: []string{"grendel-5"}, DiscoveryAttempted: true, SupportsTools: true},
					{Name: "vidar", DefaultModel: "vidar-5", DiscoveredIDs: []string{"vidar-5"}, DiscoveryAttempted: true, SupportsTools: true},
					{Name: "sindri", DefaultModel: "sindri-5", DiscoveredIDs: []string{"sindri-5"}, DiscoveryAttempted: true, SupportsTools: true},
				},
			},
		},
		ModelEligibility: testPowerLookup(map[string]int{
			"grendel-5": 5,
			"vidar-5":   5,
			"sindri-5":  5,
		}),
		Now: time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
	}

	dec, err := Resolve(Request{Policy: "default"}, in)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(dec.Candidates) != 3 {
		t.Fatalf("Candidates len=%d, want 3: %#v", len(dec.Candidates), dec.Candidates)
	}
	for _, c := range dec.Candidates {
		if !c.Eligible {
			t.Fatalf("candidate unexpectedly ineligible under default: %#v", c)
		}
		if c.Power != 5 {
			t.Fatalf("candidate power=%d, want 5: %#v", c.Power, c)
		}
	}
}

func TestPolicyDefaultDoesNotBroadenExactModelPin(t *testing.T) {
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
					{Name: "grendel", DefaultModel: "grendel-5", DiscoveredIDs: []string{"grendel-5"}, DiscoveryAttempted: true, SupportsTools: true},
					{Name: "vidar", DefaultModel: "vidar-5", DiscoveredIDs: []string{"vidar-5"}, DiscoveryAttempted: true, SupportsTools: true},
				},
			},
		},
		ModelEligibility: testPowerLookup(map[string]int{
			"grendel-5": 5,
			"vidar-5":   5,
			"sindri-5":  5,
		}),
		Now: time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
	}

	dec, err := Resolve(Request{Policy: "default", Model: "grendel-5"}, in)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if dec.Model != "grendel-5" {
		t.Fatalf("Model=%q, want exact model pin", dec.Model)
	}
	if dec.Harness != "fiz" {
		t.Fatalf("Harness=%q, want fiz", dec.Harness)
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
