package fizeau_test

import (
	"errors"
	"fmt"
	"strings"

	fizeau "github.com/DocumentDrivenDX/fizeau"
)

func ExampleErrHarnessModelIncompatible() {
	err := fmt.Errorf("ddx preflight: %w", &fizeau.ErrHarnessModelIncompatible{
		Harness:         "gemini",
		Model:           "minimax/minimax-m2.7",
		SupportedModels: []string{"gemini-2.5-pro", "gemini-2.5-flash"},
	})

	var routeErr *fizeau.ErrHarnessModelIncompatible
	if errors.As(err, &routeErr) {
		fmt.Printf("ddx failed bead: harness=%s model=%s supported=%s\n",
			routeErr.Harness,
			routeErr.Model,
			strings.Join(routeErr.SupportedModels, ","))
	}
	fmt.Println(errors.Is(err, fizeau.ErrHarnessModelIncompatible{}))

	// Output:
	// ddx failed bead: harness=gemini model=minimax/minimax-m2.7 supported=gemini-2.5-pro,gemini-2.5-flash
	// true
}

func ExampleErrProfilePinConflict() {
	err := fmt.Errorf("ddx preflight: %w", &fizeau.ErrProfilePinConflict{
		Profile:           "local",
		ConflictingPin:    "Harness=claude",
		ProfileConstraint: "local-only",
	})

	var routeErr *fizeau.ErrProfilePinConflict
	if errors.As(err, &routeErr) {
		fmt.Printf("ddx failed bead: profile=%s conflict=%s constraint=%s\n",
			routeErr.Profile,
			routeErr.ConflictingPin,
			routeErr.ProfileConstraint)
	}
	fmt.Println(errors.Is(err, fizeau.ErrProfilePinConflict{}))

	// Output:
	// ddx failed bead: profile=local conflict=Harness=claude constraint=local-only
	// true
}

func ExampleRouteDecision_candidates() {
	decision := &fizeau.RouteDecision{
		Candidates: []fizeau.RouteCandidate{
			{Harness: "fiz", Provider: "local", Model: "qwen", Eligible: false, Reason: "provider is in cooldown"},
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
	// fiz/qwen eligible=false reason=provider is in cooldown
	// codex/gpt-5.4 eligible=true reason=score=71.2
}
