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
	if byName["smart"].AliasOf != "code-high" {
		t.Fatalf("smart AliasOf: got %q, want code-high", byName["smart"].AliasOf)
	}
	if byName["smart"].CompatibilityTarget != "code-high" || byName["smart"].MinPower != 9 || byName["smart"].MaxPower != 10 {
		t.Fatalf("smart profile policy: %#v", byName["smart"])
	}
	if byName["cheap"].CompatibilityTarget != "code-economy" || byName["cheap"].MinPower != 5 || byName["cheap"].MaxPower != 5 {
		t.Fatalf("cheap profile policy: %#v", byName["cheap"])
	}
	if byName["standard"].CatalogVersion == "" {
		t.Fatal("CatalogVersion should be populated")
	}
	if byName["default"].CompatibilityTarget != "code-medium" || byName["default"].ProviderPreference != "local-first" {
		t.Fatalf("default profile: %#v, want compatibility target code-medium/local-first", byName["default"])
	}
	if byName["local"].CompatibilityTarget != "code-economy" || byName["local"].ProviderPreference != "local-only" {
		t.Fatalf("local profile: %#v, want compatibility target code-economy/local-only", byName["local"])
	}
	if !byName["claude-sonnet"].Deprecated {
		t.Fatal("claude-sonnet should be listed as a deprecated alias")
	}
	if byName["claude-sonnet"].Replacement != "code-medium" {
		t.Fatalf("claude-sonnet Replacement: got %q, want code-medium", byName["claude-sonnet"].Replacement)
	}

	aliases, err := svc.ProfileAliases(context.Background())
	if err != nil {
		t.Fatalf("ProfileAliases: %v", err)
	}
	if aliases["smart"] != "code-high" {
		t.Fatalf("smart alias: got %q, want code-high", aliases["smart"])
	}
	if aliases["claude-sonnet"] != "code-medium" {
		t.Fatalf("deprecated claude-sonnet alias: got %q, want code-medium", aliases["claude-sonnet"])
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
	if resolved.Target != "code-high" || resolved.CompatibilityTarget != "code-high" || resolved.MinPower != 9 || resolved.MaxPower != 10 {
		t.Fatalf("profile policy: %#v", resolved)
	}
	if len(resolved.Surfaces) == 0 {
		t.Fatal("expected profile surfaces")
	}
	nativeOpenAI := findProfileSurface(resolved.Surfaces, "native-openai")
	if nativeOpenAI == nil {
		t.Fatalf("native-openai surface missing from %#v", resolved.Surfaces)
	}
	if nativeOpenAI.Harness != "agent" {
		t.Fatalf("native-openai Harness: got %q, want agent", nativeOpenAI.Harness)
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
	if nativeOpenAI.FailurePolicy != "ordered-failover" {
		t.Fatalf("FailurePolicy: got %q, want ordered-failover", nativeOpenAI.FailurePolicy)
	}
	if nativeOpenAI.CostCeilingInputPerMTok == nil || *nativeOpenAI.CostCeilingInputPerMTok != 20 {
		t.Fatalf("CostCeilingInputPerMTok: got %#v, want 20", nativeOpenAI.CostCeilingInputPerMTok)
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

func TestServiceProfiles_ResolveDeprecatedAliasAndUnknown(t *testing.T) {
	svc, err := fizeau.New(fizeau.ServiceOptions{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	deprecated, err := svc.ResolveProfile(context.Background(), "claude-sonnet")
	if err != nil {
		t.Fatalf("ResolveProfile deprecated alias: %v", err)
	}
	if !deprecated.Deprecated {
		t.Fatal("deprecated alias should resolve with Deprecated=true")
	}
	if deprecated.Replacement != "code-medium" {
		t.Fatalf("Replacement: got %q, want code-medium", deprecated.Replacement)
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
