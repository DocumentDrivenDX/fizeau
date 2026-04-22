package agent_test

import (
	"errors"
	"fmt"
	"strings"

	agent "github.com/DocumentDrivenDX/agent"
)

func ExampleErrHarnessModelIncompatible() {
	err := fmt.Errorf("ddx preflight: %w", &agent.ErrHarnessModelIncompatible{
		Harness:         "gemini",
		Model:           "minimax/minimax-m2.7",
		SupportedModels: []string{"gemini-2.5-pro", "gemini-2.5-flash"},
	})

	var routeErr *agent.ErrHarnessModelIncompatible
	if errors.As(err, &routeErr) {
		fmt.Printf("ddx failed bead: harness=%s model=%s supported=%s\n",
			routeErr.Harness,
			routeErr.Model,
			strings.Join(routeErr.SupportedModels, ","))
	}
	fmt.Println(errors.Is(err, agent.ErrHarnessModelIncompatible{}))

	// Output:
	// ddx failed bead: harness=gemini model=minimax/minimax-m2.7 supported=gemini-2.5-pro,gemini-2.5-flash
	// true
}

func ExampleErrProfilePinConflict() {
	err := fmt.Errorf("ddx preflight: %w", &agent.ErrProfilePinConflict{
		Profile:           "local",
		ConflictingPin:    "Harness=claude",
		ProfileConstraint: "local-only",
	})

	var routeErr *agent.ErrProfilePinConflict
	if errors.As(err, &routeErr) {
		fmt.Printf("ddx failed bead: profile=%s conflict=%s constraint=%s\n",
			routeErr.Profile,
			routeErr.ConflictingPin,
			routeErr.ProfileConstraint)
	}
	fmt.Println(errors.Is(err, agent.ErrProfilePinConflict{}))

	// Output:
	// ddx failed bead: profile=local conflict=Harness=claude constraint=local-only
	// true
}

func ExampleRouteDecision_candidates() {
	decision := &agent.RouteDecision{
		Candidates: []agent.RouteCandidate{
			{Harness: "agent", Provider: "local", Model: "qwen", Eligible: false, Reason: "provider is in cooldown"},
			{Harness: "codex", Model: "gpt-5.4", Eligible: true, Reason: "score=71.2"},
		},
	}

	for _, candidate := range decision.Candidates {
		fmt.Printf("%s/%s eligible=%t reason=%s\n",
			candidate.Harness,
			candidate.Model,
			candidate.Eligible,
			candidate.Reason)
	}

	// Output:
	// agent/qwen eligible=false reason=provider is in cooldown
	// codex/gpt-5.4 eligible=true reason=score=71.2
}
