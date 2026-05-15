package routing

import (
	"strings"
	"testing"
	"time"
)

// TestScorePolicy_MinPowerDoesNotDisableDefaultPolicy verifies that the default
// policy still applies the +15 subscription/free bonus to in-bounds candidates
// even when MinPower is set. Regression for fizeau-dc3cf359.
func TestScorePolicy_MinPowerDoesNotDisableDefaultPolicy(t *testing.T) {
	in := Inputs{
		Harnesses: []HarnessEntry{{
			Name:                "claude",
			Surface:             "claude",
			CostClass:           "medium",
			IsSubscription:      true,
			AutoRoutingEligible: true,
			Available:           true,
			QuotaOK:             true,
			SubscriptionOK:      true,
			ExactPinSupport:     true,
			DefaultModel:        "claude-sonnet-4-6",
			SupportedModels:     []string{"claude-sonnet-4-6"},
			SupportsTools:       true,
			Providers:           []ProviderEntry{{CostSource: CostSourceSubscription}},
		}},
		ModelEligibility: testPowerLookup(map[string]int{
			"claude-sonnet-4-6": 8,
		}),
		Now: time.Date(2026, 5, 14, 0, 0, 0, 0, time.UTC),
	}

	dec, err := Resolve(Request{MinPower: 8}, in)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(dec.Candidates) != 1 {
		t.Fatalf("Candidates len=%d, want 1", len(dec.Candidates))
	}
	cand := dec.Candidates[0]
	if !cand.Eligible {
		t.Fatalf("candidate ineligible: %#v", cand)
	}
	// Default policy gives +15 to subscription candidates within quota (quota_health
	// component). With MinPower set this must still apply to in-bounds candidates.
	quotaHealth := cand.ScoreComponents["quota_health"]
	if quotaHealth < 15 {
		t.Fatalf("quota_health=%.1f, want >= 15 (default policy +15 subscription bonus must apply with MinPower set)", quotaHealth)
	}
}

// TestScorePolicy_MinPowerSonnetBeatsOpus_DefaultPolicy verifies that with
// MinPower=8 and default policy, a subscription/free sonnet (power=8) scores
// higher than a metered opus (power=10, cost=$0.045). This is the exact bug
// reported in fizeau-dc3cf359: opus was winning due to policy scoring being
// skipped entirely when MinPower was set.
func TestScorePolicy_MinPowerSonnetBeatsOpus_DefaultPolicy(t *testing.T) {
	in := Inputs{
		Harnesses: []HarnessEntry{
			{
				Name:                "sonnet-harness",
				Surface:             "claude",
				CostClass:           "medium",
				IsSubscription:      true,
				AutoRoutingEligible: true,
				Available:           true,
				QuotaOK:             true,
				SubscriptionOK:      true,
				ExactPinSupport:     true,
				DefaultModel:        "sonnet",
				SupportedModels:     []string{"sonnet"},
				SupportsTools:       true,
				Providers:           []ProviderEntry{{CostSource: CostSourceSubscription}},
			},
			{
				Name:                "opus-harness",
				Surface:             "embedded-openai",
				CostClass:           "medium",
				AutoRoutingEligible: true,
				Available:           true,
				ExactPinSupport:     true,
				SupportsTools:       true,
				Providers: []ProviderEntry{{
					Name:               "opus-provider",
					DefaultModel:       "opus",
					CostUSDPer1kTokens: 0.045,
					CostSource:         CostSourceCatalog,
					SupportsTools:      true,
				}},
			},
		},
		ModelEligibility: testPowerLookup(map[string]int{
			"sonnet": 8,
			"opus":   10,
		}),
		Now: time.Date(2026, 5, 14, 0, 0, 0, 0, time.UTC),
	}

	dec, err := Resolve(Request{MinPower: 8}, in)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if dec.Harness != "sonnet-harness" {
		t.Fatalf("winner=%s, want sonnet-harness (subscription/free must beat metered opus with MinPower=8): candidates=%+v", dec.Harness, dec.Candidates)
	}
	var sonnetScore, opusScore float64
	for _, c := range dec.Candidates {
		switch c.Harness {
		case "sonnet-harness":
			sonnetScore = c.Score
		case "opus-harness":
			opusScore = c.Score
		}
	}
	if sonnetScore <= opusScore {
		t.Fatalf("sonnet score %.1f <= opus score %.1f; policy-preference scoring must apply to in-bounds candidates with MinPower set", sonnetScore, opusScore)
	}
}

// TestScorePolicy_MaxPowerDoesNotDisableProviderPreference verifies that with
// MaxPower=8 set, a subscription-first provider preference still adds the +30
// quota_health bonus to in-bounds subscription candidates.
func TestScorePolicy_MaxPowerDoesNotDisableProviderPreference(t *testing.T) {
	in := Inputs{
		Harnesses: []HarnessEntry{{
			Name:                "claude",
			Surface:             "claude",
			CostClass:           "medium",
			IsSubscription:      true,
			AutoRoutingEligible: true,
			Available:           true,
			QuotaOK:             true,
			SubscriptionOK:      true,
			ExactPinSupport:     true,
			DefaultModel:        "claude-sonnet-4-6",
			SupportedModels:     []string{"claude-sonnet-4-6"},
			SupportsTools:       true,
			Providers:           []ProviderEntry{{CostSource: CostSourceSubscription}},
		}},
		ModelEligibility: testPowerLookup(map[string]int{
			"claude-sonnet-4-6": 8,
		}),
		Now: time.Date(2026, 5, 14, 0, 0, 0, 0, time.UTC),
	}

	dec, err := Resolve(Request{MaxPower: 8, ProviderPreference: ProviderPreferenceSubscriptionFirst}, in)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(dec.Candidates) != 1 {
		t.Fatalf("Candidates len=%d, want 1", len(dec.Candidates))
	}
	cand := dec.Candidates[0]
	if !cand.Eligible {
		t.Fatalf("candidate ineligible: %#v", cand)
	}
	// subscription-first provider preference adds +30 to quota_health. With
	// MaxPower set this must still apply to in-bounds subscription candidates.
	quotaHealth := cand.ScoreComponents["quota_health"]
	if quotaHealth < 30 {
		t.Fatalf("quota_health=%.1f, want >= 30 (subscription-first +30 must apply with MaxPower set)", quotaHealth)
	}
}

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
	if codex.Eligible {
		t.Fatalf("codex candidate must be excluded once an in-bounds route exists: %#v", codex)
	}
	if codex.FilterReason != FilterReasonAboveMaxPower {
		t.Fatalf("codex filter reason=%q, want %q", codex.FilterReason, FilterReasonAboveMaxPower)
	}
	if got := codex.ScoreComponents["power"]; got != -2-aboveMaxPowerExclusionPenalty {
		t.Fatalf("codex power component=%v, want %v with exclusion-strength max_power penalty", got, -2-aboveMaxPowerExclusionPenalty)
	}
	if !strings.Contains(codex.Reason, "max_power=8") {
		t.Fatalf("codex reason=%q, want max_power evidence", codex.Reason)
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

func TestScorePolicy_MaxPowerExcludesOverpoweredCandidates(t *testing.T) {
	in := maxPowerExclusionInputs()

	dec, err := Resolve(Request{Policy: "default", MaxPower: 8}, in)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if dec.Provider != "routine" || dec.Model != "routine-8" {
		t.Fatalf("winner=%s/%s, want routine/routine-8", dec.Provider, dec.Model)
	}

	var routine, frontier Candidate
	for _, c := range dec.Candidates {
		switch c.Provider {
		case "routine":
			routine = c
		case "frontier":
			frontier = c
		}
	}
	if !routine.Eligible || routine.Power != 8 {
		t.Fatalf("routine candidate=%#v, want eligible power-8 route", routine)
	}
	if frontier.Eligible {
		t.Fatalf("frontier candidate must be excluded when an in-bounds route exists: %#v", frontier)
	}
	if frontier.FilterReason != FilterReasonAboveMaxPower {
		t.Fatalf("frontier filter reason=%q, want %q", frontier.FilterReason, FilterReasonAboveMaxPower)
	}
	if got := frontier.ScoreComponents["power"]; got != -2-aboveMaxPowerExclusionPenalty {
		t.Fatalf("frontier power component=%v, want %v", got, -2-aboveMaxPowerExclusionPenalty)
	}
	if !strings.Contains(frontier.Reason, "max_power=8") {
		t.Fatalf("frontier reason=%q, want max_power evidence", frontier.Reason)
	}
}

func TestScorePolicy_MaxPowerPinsRemainConstraints(t *testing.T) {
	in := maxPowerExclusionInputs()

	dec, err := Resolve(Request{Provider: "frontier", MaxPower: 8}, in)
	if err != nil {
		t.Fatalf("Resolve provider pin: %v", err)
	}
	if dec.Provider != "frontier" || dec.Model != "frontier-10" {
		t.Fatalf("provider-pin winner=%s/%s, want frontier/frontier-10", dec.Provider, dec.Model)
	}

	dec, err = Resolve(Request{Model: "frontier-10", MaxPower: 8}, in)
	if err != nil {
		t.Fatalf("Resolve model pin: %v", err)
	}
	if dec.Provider != "frontier" || dec.Model != "frontier-10" {
		t.Fatalf("model-pin winner=%s/%s, want frontier/frontier-10", dec.Provider, dec.Model)
	}
	for _, c := range dec.Candidates {
		if c.Provider == "frontier" && !c.Eligible {
			t.Fatalf("frontier exact model pin must remain eligible under MaxPower: %#v", c)
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

func maxPowerExclusionInputs() Inputs {
	return Inputs{
		Harnesses: []HarnessEntry{
			{
				Name:                "routine",
				Surface:             "embedded-openai",
				CostClass:           "medium",
				AutoRoutingEligible: true,
				ExactPinSupport:     true,
				Available:           true,
				QuotaOK:             true,
				SubscriptionOK:      true,
				SupportsTools:       true,
				Providers: []ProviderEntry{{
					Name:               "routine",
					DefaultModel:       "routine-8",
					DiscoveredIDs:      []string{"routine-8"},
					DiscoveryAttempted: true,
					CostUSDPer1kTokens: 0.04,
					CostSource:         CostSourceCatalog,
					SupportsTools:      true,
				}},
			},
			{
				Name:                "frontier",
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
				DefaultModel:        "frontier-10",
				SupportedModels:     []string{"frontier-10"},
				SupportsTools:       true,
				Providers: []ProviderEntry{{
					Name:               "frontier",
					DefaultModel:       "frontier-10",
					DiscoveredIDs:      []string{"frontier-10"},
					DiscoveryAttempted: true,
					CostSource:         CostSourceSubscription,
					SupportsTools:      true,
				}},
			},
		},
		ModelEligibility: testPowerLookup(map[string]int{
			"routine-8":   8,
			"frontier-10": 10,
		}),
		Now: time.Date(2026, 5, 14, 0, 0, 0, 0, time.UTC),
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
