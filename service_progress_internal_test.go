//go:build testseam

package fizeau

import (
	"strings"
	"testing"
)

func TestRouteProgressData_IncludesEconomicsWhenPresent(t *testing.T) {
	payload := routeProgressData(RouteDecision{
		Harness:  "agent",
		Provider: "alpha",
		Model:    "model-a",
		Power:    7,
		Candidates: []RouteCandidate{{
			Harness:            "agent",
			Provider:           "alpha",
			Model:              "model-a",
			CostUSDPer1kTokens: 0.012,
			CostSource:         "catalog",
			Components: RouteCandidateComponents{
				Power:     7,
				SpeedTPS:  55,
				CostClass: "local",
			},
		}},
	})
	if payload.Phase != "route" || payload.State != "start" {
		t.Fatalf("payload=%#v, want route start", payload)
	}
	if payload.Message == "" {
		t.Fatal("route progress message is empty")
	}
	for _, want := range []string{"agent/alpha/model-a", "power=", "speed=", "cost=", "cost_source="} {
		if !strings.Contains(payload.Message, want) {
			t.Fatalf("route progress message %q missing %q", payload.Message, want)
		}
	}
	if len(payload.Message) > 80 {
		t.Fatalf("route progress message too long: %d", len(payload.Message))
	}
	if payload.SessionSummary != payload.Message {
		t.Fatalf("session summary=%q, want same as message %q", payload.SessionSummary, payload.Message)
	}
}
