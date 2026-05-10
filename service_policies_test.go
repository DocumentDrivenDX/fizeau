package fizeau_test

import (
	"context"
	"testing"

	fizeau "github.com/easel/fizeau"
)

func TestListPoliciesReturnsCanonicalEntries(t *testing.T) {
	svc, err := fizeau.New(fizeau.ServiceOptions{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	policies, err := svc.ListPolicies(context.Background())
	if err != nil {
		t.Fatalf("ListPolicies: %v", err)
	}
	byName := make(map[string]fizeau.PolicyInfo)
	for _, policy := range policies {
		byName[policy.Name] = policy
	}

	if len(byName) != 4 {
		t.Fatalf("policies: got %d entries (%v), want 4 canonical policies", len(byName), policyNames(policies))
	}

	smart := byName["smart"]
	if smart.MinPower != 9 || smart.MaxPower != 10 || smart.AllowLocal {
		t.Fatalf("smart policy: %#v", smart)
	}
	if smart.CatalogVersion == "" || smart.ManifestSource == "" || smart.ManifestVersion != 5 {
		t.Fatalf("smart catalog metadata: %#v", smart)
	}
	cheap := byName["cheap"]
	if cheap.MinPower != 5 || cheap.MaxPower != 5 || !cheap.AllowLocal {
		t.Fatalf("cheap policy: %#v", cheap)
	}
	airGapped := byName["air-gapped"]
	if airGapped.MinPower != 5 || airGapped.MaxPower != 5 || !airGapped.AllowLocal {
		t.Fatalf("air-gapped policy: %#v", airGapped)
	}
	if len(airGapped.Require) != 1 || airGapped.Require[0] != "no_remote" {
		t.Fatalf("air-gapped requirements: %#v", airGapped.Require)
	}

	for _, legacy := range []string{"standard", "code-fast", "fast", "code-smart", "code-economy", "local", "offline"} {
		if _, ok := byName[legacy]; ok {
			t.Fatalf("legacy policy alias %q should not be listed", legacy)
		}
	}
}

func policyNames(policies []fizeau.PolicyInfo) []string {
	names := make([]string, 0, len(policies))
	for _, policy := range policies {
		names = append(names, policy.Name)
	}
	return names
}
