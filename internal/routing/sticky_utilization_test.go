package routing

import "testing"

func TestResolveUsesEndpointLoadToRankEquivalentLocalEndpoints(t *testing.T) {
	in := Inputs{
		Harnesses: []HarnessEntry{
			{
				Name:                "agent",
				CostClass:           "local",
				AutoRoutingEligible: true,
				Available:           true,
				ExactPinSupport:     true,
				SupportsTools:       true,
				Providers: []ProviderEntry{
					{Name: "local@desk-a", EndpointName: "desk-a", DefaultModel: "model-a", CostClass: "local"},
					{Name: "local@desk-b", EndpointName: "desk-b", DefaultModel: "model-a", CostClass: "local"},
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
			if provider != "local" || model != "model-a" {
				return EndpointLoad{}, false
			}
			switch endpoint {
			case "desk-a":
				return EndpointLoad{LeaseCount: 2, NormalizedLoad: 2.5}, true
			case "desk-b":
				return EndpointLoad{LeaseCount: 0, NormalizedLoad: 0.5}, true
			default:
				return EndpointLoad{}, false
			}
		},
	}

	dec, err := Resolve(Request{Harness: "agent", Model: "model-a"}, in)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if dec.Provider != "local@desk-b" || dec.Endpoint != "desk-b" {
		t.Fatalf("decision=%#v, want desk-b as least-loaded equivalent local endpoint", dec)
	}
	if len(dec.Candidates) < 2 {
		t.Fatalf("candidates=%#v, want both endpoints", dec.Candidates)
	}
	if dec.Candidates[0].Provider != "local@desk-b" {
		t.Fatalf("top candidate=%#v, want desk-b", dec.Candidates[0])
	}
}
