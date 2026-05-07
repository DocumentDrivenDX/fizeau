package routing

import "testing"

func TestResolvePenalizesUnknownUtilizationAndPerformance(t *testing.T) {
	in := Inputs{
		Harnesses: []HarnessEntry{
			{
				Name:                "fiz",
				CostClass:           "local",
				AutoRoutingEligible: true,
				Available:           true,
				ExactPinSupport:     true,
				SupportsTools:       true,
				Providers: []ProviderEntry{
					{Name: "known", DefaultModel: "model-a", CostClass: "local", SupportsTools: true},
					{Name: "unknown", DefaultModel: "model-a", CostClass: "local", SupportsTools: true},
				},
			},
		},
		ModelEligibility: func(model string) (ModelEligibility, bool) {
			if model != "model-a" {
				return ModelEligibility{}, false
			}
			return ModelEligibility{Power: 7, AutoRoutable: true}, true
		},
		EndpointLoadResolver: func(provider, endpoint, model string) (EndpointLoad, bool) {
			if provider != "known" || model != "model-a" {
				return EndpointLoad{}, false
			}
			return EndpointLoad{NormalizedLoad: 0.1, UtilizationFresh: true}, true
		},
		ObservedSpeedTPS: map[string]float64{
			ProviderModelKey("known", "", "model-a"): 90,
		},
		ObservedLatencyMS: map[string]float64{
			ProviderModelKey("known", "", "model-a"): 100,
		},
	}

	dec, err := Resolve(Request{Harness: "fiz", Model: "model-a"}, in)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(dec.Candidates) != 2 {
		t.Fatalf("Candidates=%d, want 2", len(dec.Candidates))
	}
	var known, unknown Candidate
	for _, c := range dec.Candidates {
		switch c.Provider {
		case "known":
			known = c
		case "unknown":
			unknown = c
		}
	}
	if known.Provider != "known" || unknown.Provider != "unknown" {
		t.Fatalf("candidates=%+v, want known and unknown providers", dec.Candidates)
	}
	if known.Score <= unknown.Score {
		t.Fatalf("known score %.1f should beat unknown score %.1f", known.Score, unknown.Score)
	}
	if known.ScoreComponents == nil || unknown.ScoreComponents == nil {
		t.Fatalf("score components must be populated: known=%v unknown=%v", known.ScoreComponents, unknown.ScoreComponents)
	}
	for _, key := range []string{"base", "cost", "deployment_locality", "quota_health", "utilization", "performance", "power", "context_headroom", "sticky_affinity"} {
		if _, ok := known.ScoreComponents[key]; !ok {
			t.Fatalf("known score components missing %q: %+v", key, known.ScoreComponents)
		}
		if _, ok := unknown.ScoreComponents[key]; !ok {
			t.Fatalf("unknown score components missing %q: %+v", key, unknown.ScoreComponents)
		}
	}
	if known.Utilization <= 0 {
		t.Fatalf("known utilization component=%v, want positive known load", known.Utilization)
	}
	if unknown.Utilization != 0 {
		t.Fatalf("unknown utilization component=%v, want 0 for unknown load", unknown.Utilization)
	}
	if known.ScoreComponents["utilization"] == 0 {
		t.Fatalf("known utilization score component=%v, want explicit known-load adjustment", known.ScoreComponents["utilization"])
	}
	if unknown.ScoreComponents["utilization"] >= 0 {
		t.Fatalf("unknown utilization score component=%v, want explicit penalty", unknown.ScoreComponents["utilization"])
	}
	if known.ScoreComponents["performance"] <= 0 {
		t.Fatalf("known performance score component=%v, want positive known performance", known.ScoreComponents["performance"])
	}
	if unknown.ScoreComponents["performance"] >= 0 {
		t.Fatalf("unknown performance score component=%v, want explicit penalty", unknown.ScoreComponents["performance"])
	}
	if known.StickyAffinity != 0 || unknown.StickyAffinity != 0 {
		t.Fatalf("unexpected sticky affinity components: known=%v unknown=%v", known.StickyAffinity, unknown.StickyAffinity)
	}
	if dec.Provider != "known" {
		t.Fatalf("winner provider=%q, want known", dec.Provider)
	}
}

func TestResolveReportsStickyAffinityComponentWhenStickyKeyMatches(t *testing.T) {
	in := Inputs{
		Harnesses: []HarnessEntry{
			{
				Name:                "fiz",
				CostClass:           "local",
				AutoRoutingEligible: true,
				Available:           true,
				ExactPinSupport:     true,
				SupportsTools:       true,
				Providers: []ProviderEntry{
					{Name: "server-a", ServerInstance: "desk-a", DefaultModel: "model-a", CostClass: "local", SupportsTools: true},
					{Name: "server-b", ServerInstance: "desk-b", DefaultModel: "model-a", CostClass: "local", SupportsTools: true},
				},
			},
		},
		ModelEligibility: func(model string) (ModelEligibility, bool) {
			if model != "model-a" {
				return ModelEligibility{}, false
			}
			return ModelEligibility{Power: 7, AutoRoutable: true}, true
		},
		EndpointLoadResolver: func(provider, endpoint, model string) (EndpointLoad, bool) {
			if provider != "server-a" && provider != "server-b" {
				return EndpointLoad{}, false
			}
			if endpoint == "desk-a" {
				return EndpointLoad{NormalizedLoad: 0.2, UtilizationFresh: true}, true
			}
			return EndpointLoad{NormalizedLoad: 0.4, UtilizationFresh: true}, true
		},
		StickyServerInstanceResolver: func(stickyKey string) (string, bool) {
			if stickyKey == "session-1" {
				return "desk-a", true
			}
			return "", false
		},
	}

	dec, err := Resolve(Request{Harness: "fiz", Model: "model-a", CorrelationID: "session-1"}, in)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if dec.ServerInstance != "desk-a" {
		t.Fatalf("server_instance=%q, want desk-a", dec.ServerInstance)
	}
	var sticky Candidate
	for _, c := range dec.Candidates {
		if c.ServerInstance == "desk-a" {
			sticky = c
			break
		}
	}
	if sticky.ServerInstance != "desk-a" {
		t.Fatalf("sticky candidate not found: %+v", dec.Candidates)
	}
	if sticky.StickyAffinity != StickyAffinityBonus {
		t.Fatalf("sticky affinity component=%v, want %v", sticky.StickyAffinity, StickyAffinityBonus)
	}
	if sticky.ScoreComponents["sticky_affinity"] != StickyAffinityBonus {
		t.Fatalf("sticky score component=%v, want %v", sticky.ScoreComponents["sticky_affinity"], StickyAffinityBonus)
	}
	if sticky.Utilization <= 0 {
		t.Fatalf("sticky utilization component=%v, want positive load", sticky.Utilization)
	}
}
