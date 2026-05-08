package fizeau_test

import (
	"context"
	"testing"

	fizeau "github.com/DocumentDrivenDX/fizeau"
)

func TestServiceProfiles_ListResolveAliases(t *testing.T) {
	svc, err := fizeau.New(fizeau.ServiceOptions{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	profiles, err := svc.ListProfiles(context.Background())
	if err != nil {
		t.Fatalf("ListProfiles: %v", err)
	}
	byName := make(map[string]fizeau.ProfileInfo)
	for _, profile := range profiles {
		byName[profile.Name] = profile
	}
	if byName["smart"].AliasOf != "" {
		t.Fatalf("smart AliasOf: got %q, want empty", byName["smart"].AliasOf)
	}
	if byName["smart"].CompatibilityTarget != "smart" || byName["smart"].MinPower != 9 || byName["smart"].MaxPower != 10 {
		t.Fatalf("smart profile policy: %#v", byName["smart"])
	}
	if byName["cheap"].CompatibilityTarget != "cheap" || byName["cheap"].MinPower != 5 || byName["cheap"].MaxPower != 5 {
		t.Fatalf("cheap profile policy: %#v", byName["cheap"])
	}
	if byName["default"].CatalogVersion == "" {
		t.Fatal("CatalogVersion should be populated")
	}
	if byName["default"].CompatibilityTarget != "default" || byName["default"].ProviderPreference != "local-first" {
		t.Fatalf("default profile: %#v, want compatibility target default/local-first", byName["default"])
	}
	if byName["local"].AliasOf != "cheap" || byName["local"].CompatibilityTarget != "cheap" || byName["local"].ProviderPreference != "local-only" {
		t.Fatalf("local profile: %#v, want alias to cheap/local-only", byName["local"])
	}
	if byName["code-smart"].AliasOf != "smart" || byName["code-smart"].CompatibilityTarget != "smart" {
		t.Fatalf("code-smart profile: %#v", byName["code-smart"])
	}
	if byName["code-fast"].AliasOf != "default" || byName["code-fast"].CompatibilityTarget != "default" {
		t.Fatalf("code-fast profile: %#v", byName["code-fast"])
	}
	if _, ok := byName["code-high"]; ok {
		t.Fatal("code-high should not be listed as a primary profile")
	}
	if _, ok := byName["code-medium"]; ok {
		t.Fatal("code-medium should not be listed as a primary profile")
	}
	if _, ok := byName["claude-sonnet"]; ok {
		t.Fatal("target aliases should not be listed after the v5 manifest cutover")
	}

	aliases, err := svc.ProfileAliases(context.Background())
	if err != nil {
		t.Fatalf("ProfileAliases: %v", err)
	}
	if aliases["code-smart"] != "smart" {
		t.Fatalf("code-smart alias: got %q, want smart", aliases["code-smart"])
	}
	if aliases["code-fast"] != "default" {
		t.Fatalf("code-fast alias: got %q, want default", aliases["code-fast"])
	}
	if aliases["local"] != "cheap" {
		t.Fatalf("local alias: got %q, want cheap", aliases["local"])
	}
	if _, ok := aliases["claude-sonnet"]; ok {
		t.Fatal("target aliases should not be exposed after the v5 manifest cutover")
	}
}

func TestServiceProfiles_ResolveProfile(t *testing.T) {
	svc, err := fizeau.New(fizeau.ServiceOptions{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	resolved, err := svc.ResolveProfile(context.Background(), "smart")
	if err != nil {
		t.Fatalf("ResolveProfile: %v", err)
	}
	if resolved.Target != "smart" || resolved.CompatibilityTarget != "smart" || resolved.MinPower != 9 || resolved.MaxPower != 10 {
		t.Fatalf("profile policy: %#v", resolved)
	}
	if len(resolved.Surfaces) == 0 {
		t.Fatal("expected profile surfaces")
	}
	nativeOpenAI := findProfileSurface(resolved.Surfaces, "native-openai")
	if nativeOpenAI == nil {
		t.Fatalf("native-openai surface missing from %#v", resolved.Surfaces)
	}
	if nativeOpenAI.Harness != "fiz" {
		t.Fatalf("native-openai Harness: got %q, want fiz", nativeOpenAI.Harness)
	}
	if nativeOpenAI.Model == "" {
		t.Fatalf("native-openai model missing: %#v", nativeOpenAI)
	}
	if len(nativeOpenAI.Candidates) != 0 {
		t.Fatalf("native-openai should not expose a closed candidate set: %#v", nativeOpenAI.Candidates)
	}
	if nativeOpenAI.ReasoningDefault != fizeau.ReasoningHigh {
		t.Fatalf("ReasoningDefault: got %q, want high", nativeOpenAI.ReasoningDefault)
	}
	if nativeOpenAI.FailurePolicy != "" {
		t.Fatalf("FailurePolicy: got %q, want empty", nativeOpenAI.FailurePolicy)
	}
	if nativeOpenAI.CostCeilingInputPerMTok != nil {
		t.Fatalf("CostCeilingInputPerMTok: got %#v, want nil", nativeOpenAI.CostCeilingInputPerMTok)
	}

	gemini := findProfileSurface(resolved.Surfaces, "gemini")
	if gemini == nil {
		t.Fatalf("gemini surface missing from %#v", resolved.Surfaces)
	}
	if gemini.Harness != "gemini" || gemini.Model != "gemini-2.5-pro" {
		t.Fatalf("gemini smart surface: %#v", gemini)
	}
	if gemini.ReasoningDefault != fizeau.ReasoningOff {
		t.Fatalf("gemini ReasoningDefault: got %q, want off", gemini.ReasoningDefault)
	}
	if len(gemini.Candidates) != 0 {
		t.Fatalf("gemini should not expose a closed candidate set: %#v", gemini.Candidates)
	}
}

func TestServiceProfiles_ResolveCompatibilityAliasAndUnknown(t *testing.T) {
	svc, err := fizeau.New(fizeau.ServiceOptions{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	alias, err := svc.ResolveProfile(context.Background(), "standard")
	if err != nil {
		t.Fatalf("ResolveProfile compatibility alias: %v", err)
	}
	if alias.Target != "default" || alias.CompatibilityTarget != "default" {
		t.Fatalf("standard alias should resolve to default: %#v", alias)
	}

	if _, err := svc.ResolveProfile(context.Background(), "does-not-exist"); err == nil {
		t.Fatal("ResolveProfile unknown should return an error")
	}
}

func findProfileSurface(surfaces []fizeau.ProfileSurface, name string) *fizeau.ProfileSurface {
	for i := range surfaces {
		if surfaces[i].Name == name {
			return &surfaces[i]
		}
	}
	return nil
}
