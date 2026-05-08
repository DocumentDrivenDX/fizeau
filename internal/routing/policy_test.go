package routing

import (
	"errors"
	"testing"
)

func TestPolicyRequireNoRemoteFiltersRemoteCandidates(t *testing.T) {
	in := policyFilterInputs()

	dec, err := Resolve(Request{Policy: "air-gapped", Require: []string{"no_remote"}}, in)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if dec.Provider != "local" {
		t.Fatalf("Provider=%q, want local", dec.Provider)
	}
	remote, ok := candidateByProvider(dec.Candidates, "openrouter")
	if !ok {
		t.Fatal("openrouter candidate not found")
	}
	if remote.Eligible || remote.FilterReason != FilterReasonPolicyFiltered {
		t.Fatalf("openrouter candidate=%#v, want policy-filtered", remote)
	}
}

func TestPolicyRequireBlocksRemotePinConflict(t *testing.T) {
	in := policyFilterInputs()

	_, err := Resolve(Request{Policy: "air-gapped", Require: []string{"no_remote"}, Provider: "openrouter"}, in)
	if err == nil {
		t.Fatal("expected no_remote policy to reject openrouter pin")
	}
	var typed *ErrPolicyRequirementUnsatisfied
	if !errors.As(err, &typed) {
		t.Fatalf("errors.As ErrPolicyRequirementUnsatisfied: %T %v", err, err)
	}
	if typed.Policy != "air-gapped" || typed.Requirement != "no_remote" || typed.AttemptedPin != "openrouter" {
		t.Fatalf("policy requirement error=%#v, want air-gapped/no_remote/openrouter", typed)
	}
}

func TestAllowLocalFalseExcludesLocalCandidates(t *testing.T) {
	in := Inputs{Harnesses: []HarnessEntry{
		{
			Name:                "fiz",
			Surface:             "embedded-openai",
			CostClass:           "local",
			IsLocal:             true,
			AutoRoutingEligible: true,
			Available:           true,
			ExactPinSupport:     true,
			SupportsTools:       true,
			Providers: []ProviderEntry{{
				Name:          "local",
				CostClass:     "local",
				DefaultModel:  "local-good",
				SupportsTools: true,
			}},
		},
		{
			Name:                "codex",
			Surface:             "codex",
			CostClass:           "medium",
			IsSubscription:      true,
			AutoRoutingEligible: true,
			Available:           true,
			QuotaOK:             true,
			SubscriptionOK:      true,
			ExactPinSupport:     true,
			DefaultModel:        "frontier",
			SupportsTools:       true,
		},
	}}

	dec, err := Resolve(Request{Policy: "smart", AllowLocal: false}, in)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if dec.Harness != "codex" {
		t.Fatalf("Harness=%q, want codex", dec.Harness)
	}
	local, ok := candidateByProvider(dec.Candidates, "local")
	if !ok {
		t.Fatal("local candidate not found")
	}
	if local.Eligible || local.FilterReason != FilterReasonPolicyFiltered {
		t.Fatalf("local candidate=%#v, want policy-filtered", local)
	}
}

func policyFilterInputs() Inputs {
	return Inputs{Harnesses: []HarnessEntry{{
		Name:                "fiz",
		Surface:             "embedded-openai",
		CostClass:           "local",
		IsLocal:             true,
		AutoRoutingEligible: true,
		Available:           true,
		ExactPinSupport:     true,
		SupportsTools:       true,
		Providers: []ProviderEntry{
			{Name: "local", CostClass: "local", DefaultModel: "local-good", SupportsTools: true},
			{Name: "openrouter", CostClass: "medium", DefaultModel: "remote-good", SupportsTools: true},
		},
	}}}
}

func candidateByProvider(candidates []Candidate, provider string) (Candidate, bool) {
	for _, candidate := range candidates {
		if candidate.Provider == provider {
			return candidate, true
		}
	}
	return Candidate{}, false
}
