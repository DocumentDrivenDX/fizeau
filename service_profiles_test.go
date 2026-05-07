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
	if byName["cheap"].CompatibilityTarget != "code-economy" || byName["cheap"].MinPower != 5 || byName["cheap"].MaxPower != 5 {
		t.Fatalf("cheap profile policy: %#v", byName["cheap"])
	}
	if byName["standard"].CatalogVersion == "" {
		t.Fatal("CatalogVersion should be populated")
	}
	if byName["default"].CompatibilityTarget != "standard" || byName["default"].ProviderPreference != "local-first" {
		t.Fatalf("default profile: %#v, want compatibility target standard/local-first", byName["default"])
	}
	if byName["local"].CompatibilityTarget != "code-economy" || byName["local"].ProviderPreference != "local-only" {
		t.Fatalf("local profile: %#v, want compatibility target code-economy/local-only", byName["local"])
	}
	if byName["code-smart"].AliasOf != "smart" || byName["code-smart"].CompatibilityTarget != "smart" {
		t.Fatalf("code-smart profile: %#v", byName["code-smart"])
	}
	if byName["code-fast"].AliasOf != "standard" || byName["code-fast"].CompatibilityTarget != "standard" {
		t.Fatalf("code-fast profile: %#v", byName["code-fast"])
	}
	if _, ok := byName["code-high"]; ok {
		t.Fatal("code-high should not be listed as a primary profile")
	}
	if _, ok := byName["code-medium"]; ok {
		t.Fatal("code-medium should not be listed as a primary profile")
	}
	if !byName["claude-sonnet"].Deprecated {
		t.Fatal("claude-sonnet should be listed as a deprecated alias")
	}
	if byName["claude-sonnet"].Replacement != "standard" {
		t.Fatalf("claude-sonnet Replacement: got %q, want standard", byName["claude-sonnet"].Replacement)
	}

	aliases, err := svc.ProfileAliases(context.Background())
	if err != nil {
		t.Fatalf("ProfileAliases: %v", err)
	}
	if aliases["code-smart"] != "smart" {
		t.Fatalf("code-smart alias: got %q, want smart", aliases["code-smart"])
	}
	if aliases["code-fast"] != "standard" {
		t.Fatalf("code-fast alias: got %q, want standard", aliases["code-fast"])
	}
	if aliases["claude-sonnet"] != "standard" {
		t.Fatalf("deprecated claude-sonnet alias: got %q, want standard", aliases["claude-sonnet"])
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
	if deprecated.Replacement != "standard" {
		t.Fatalf("Replacement: got %q, want standard", deprecated.Replacement)
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
