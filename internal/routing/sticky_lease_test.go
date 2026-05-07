package routing

import (
	"testing"
	"time"
)

func TestResolveStickyServerInstanceAcrossModelChanges(t *testing.T) {
	now := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
	var stickyServerInstance string

	baseInputs := Inputs{
		Harnesses: []HarnessEntry{
			{
				Name:                "fiz",
				Surface:             "embedded-openai",
				CostClass:           "local",
				AutoRoutingEligible: true,
				Available:           true,
				ExactPinSupport:     true,
				SupportsTools:       true,
				Providers: []ProviderEntry{
					{
						Name:           "alpha",
						ServerInstance: "server-a",
						CostClass:      "local",
						DiscoveredIDs:  []string{"model-a", "model-b"},
						SupportsTools:  true,
					},
					{
						Name:           "beta",
						ServerInstance: "server-b",
						CostClass:      "local",
						DiscoveredIDs:  []string{"model-a", "model-b"},
						SupportsTools:  true,
					},
				},
			},
		},
		Now: now,
		EndpointLoadResolver: func(provider, endpoint, model string) (EndpointLoad, bool) {
			if provider != "alpha" && provider != "beta" {
				return EndpointLoad{}, false
			}
			switch model {
			case "model-a":
				switch endpoint {
				case "server-a":
					return EndpointLoad{NormalizedLoad: 0.1, UtilizationFresh: true}, true
				case "server-b":
					return EndpointLoad{NormalizedLoad: 1.0, UtilizationFresh: true}, true
				}
			case "model-b":
				switch endpoint {
				case "server-a":
					return EndpointLoad{NormalizedLoad: 1.0, UtilizationFresh: true}, true
				case "server-b":
					return EndpointLoad{NormalizedLoad: 0.1, UtilizationFresh: true}, true
				}
			}
			return EndpointLoad{}, false
		},
		StickyServerInstanceResolver: func(stickyKey string) (string, bool) {
			if stickyServerInstance == "" {
				return "", false
			}
			return stickyServerInstance, true
		},
	}

	first, err := Resolve(Request{Harness: "fiz", Model: "model-a", CorrelationID: "bead-lease-1"}, baseInputs)
	if err != nil {
		t.Fatalf("Resolve(model-a): %v", err)
	}
	if first.ServerInstance != "server-a" {
		t.Fatalf("first server_instance=%q, want server-a", first.ServerInstance)
	}
	if !first.Candidates[0].Eligible {
		t.Fatalf("first top candidate must be eligible: %#v", first.Candidates[0])
	}
	stickyServerInstance = first.ServerInstance

	second, err := Resolve(Request{Harness: "fiz", Model: "model-b", CorrelationID: "bead-lease-1"}, baseInputs)
	if err != nil {
		t.Fatalf("Resolve(model-b with sticky lease): %v", err)
	}
	if second.ServerInstance != "server-a" {
		t.Fatalf("sticky decision server_instance=%q, want server-a", second.ServerInstance)
	}
	if second.Candidates[0].ServerInstance != "server-a" {
		t.Fatalf("sticky top candidate=%#v, want server-a", second.Candidates[0])
	}

	controlInputs := baseInputs
	controlInputs.StickyServerInstanceResolver = nil
	control, err := Resolve(Request{Harness: "fiz", Model: "model-b"}, controlInputs)
	if err != nil {
		t.Fatalf("Resolve(model-b without sticky): %v", err)
	}
	if control.ServerInstance != "server-b" {
		t.Fatalf("control server_instance=%q, want server-b without sticky affinity", control.ServerInstance)
	}

	saturatedInputs := baseInputs
	saturatedInputs.StickyServerInstanceResolver = baseInputs.StickyServerInstanceResolver
	saturatedInputs.EndpointLoadResolver = func(provider, endpoint, model string) (EndpointLoad, bool) {
		if provider != "alpha" && provider != "beta" {
			return EndpointLoad{}, false
		}
		switch model {
		case "model-b":
			switch endpoint {
			case "server-a":
				return EndpointLoad{NormalizedLoad: 100, UtilizationFresh: true, UtilizationSaturated: true}, true
			case "server-b":
				return EndpointLoad{NormalizedLoad: 0.1, UtilizationFresh: true}, true
			}
		}
		return EndpointLoad{}, false
	}
	saturated, err := Resolve(Request{Harness: "fiz", Model: "model-b", CorrelationID: "bead-lease-1"}, saturatedInputs)
	if err != nil {
		t.Fatalf("Resolve(model-b with saturation): %v", err)
	}
	if saturated.ServerInstance != "server-b" {
		t.Fatalf("saturated decision server_instance=%q, want server-b because saturation outweighs sticky affinity", saturated.ServerInstance)
	}
	if saturated.Candidates[0].ServerInstance != "server-b" {
		t.Fatalf("saturated top candidate=%#v, want server-b", saturated.Candidates[0])
	}
}
