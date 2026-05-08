package fizeau

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/DocumentDrivenDX/fizeau/internal/modelcatalog"
	"github.com/DocumentDrivenDX/fizeau/internal/routing"
)

func (s *service) ListProfiles(_ context.Context) ([]ProfileInfo, error) {
	cat, err := modelcatalog.Default()
	if err != nil {
		return nil, err
	}
	meta := cat.Metadata()
	policies := cat.Policies()
	out := make([]ProfileInfo, 0, len(policies)+len(profileCompatibilityAliases()))

	for _, policy := range policies {
		out = append(out, profileInfoFromPolicy(policy, policy.Name, "", meta))
	}
	for alias, target := range profileCompatibilityAliases() {
		policy, ok := cat.Policy(target)
		if !ok {
			continue
		}
		out = append(out, profileInfoFromPolicy(policy, alias, target, meta))
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out, nil
}

func profileInfoFromPolicy(policy modelcatalog.Policy, name, aliasOf string, meta modelcatalog.Metadata) ProfileInfo {
	target := policy.Name
	return ProfileInfo{
		Name:                name,
		Target:              target,
		CompatibilityTarget: target,
		AliasOf:             aliasOf,
		MinPower:            policy.MinPower,
		MaxPower:            policy.MaxPower,
		ProviderPreference:  providerPreferenceForPolicyName(name),
		CatalogVersion:      meta.CatalogVersion,
		ManifestSource:      meta.ManifestSource,
		ManifestVersion:     meta.ManifestVersion,
	}
}

func (s *service) ProfileAliases(_ context.Context) (map[string]string, error) {
	if _, err := modelcatalog.Default(); err != nil {
		return nil, err
	}
	return profileCompatibilityAliases(), nil
}

func (s *service) ResolveProfile(_ context.Context, name string) (*ResolvedProfile, error) {
	cat, err := modelcatalog.Default()
	if err != nil {
		return nil, err
	}
	meta := cat.Metadata()
	policy, target, ok := policyForProfileName(cat, name)
	if !ok {
		return nil, fmt.Errorf("profile %q is not defined in the catalog", name)
	}

	resolved := &ResolvedProfile{
		Name:                name,
		Target:              target,
		CompatibilityTarget: target,
		MinPower:            policy.MinPower,
		MaxPower:            policy.MaxPower,
		CatalogVersion:      meta.CatalogVersion,
		ManifestSource:      meta.ManifestSource,
		ManifestVersion:     meta.ManifestVersion,
	}
	for _, surface := range serviceProfileSurfaces() {
		targetModel, err := cat.Resolve(target, modelcatalog.ResolveOptions{
			Surface:         surface.catalogSurface,
			AllowDeprecated: true,
		})
		if err != nil {
			if _, ok := err.(*modelcatalog.MissingSurfaceError); ok {
				continue
			}
			return nil, err
		}
		resolved.Surfaces = append(resolved.Surfaces, ProfileSurface{
			Name:             surface.name,
			Harness:          surface.harness,
			ProviderSystem:   surface.providerSystem,
			Model:            targetModel.ConcreteModel,
			ReasoningDefault: Reasoning(targetModel.SurfacePolicy.ReasoningDefault),
		})
	}
	if len(resolved.Surfaces) == 0 {
		return nil, fmt.Errorf("profile %q has no service-supported surface", name)
	}
	sort.Slice(resolved.Surfaces, func(i, j int) bool {
		return resolved.Surfaces[i].Name < resolved.Surfaces[j].Name
	})
	return resolved, nil
}

func policyForProfileName(cat *modelcatalog.Catalog, name string) (modelcatalog.Policy, string, bool) {
	if cat == nil {
		return modelcatalog.Policy{}, "", false
	}
	name = strings.TrimSpace(name)
	if policy, ok := cat.Policy(name); ok {
		return policy, policy.Name, true
	}
	if target, ok := profileCompatibilityAliases()[name]; ok {
		policy, ok := cat.Policy(target)
		return policy, target, ok
	}
	return modelcatalog.Policy{}, "", false
}

func profileCompatibilityAliases() map[string]string {
	return map[string]string{
		"standard":     "default",
		"code-fast":    "default",
		"fast":         "default",
		"code-smart":   "smart",
		"code-economy": "cheap",
		"local":        "cheap",
		"offline":      "cheap",
	}
}

func providerPreferenceForPolicyName(name string) string {
	switch strings.TrimSpace(name) {
	case "local", "offline", "air-gapped":
		return routing.ProviderPreferenceLocalOnly
	case "smart", "code-smart":
		return routing.ProviderPreferenceSubscriptionFirst
	default:
		return routing.ProviderPreferenceLocalFirst
	}
}

type serviceProfileSurface struct {
	name           string
	harness        string
	providerSystem string
	catalogSurface modelcatalog.Surface
}

func serviceProfileSurfaces() []serviceProfileSurface {
	return []serviceProfileSurface{
		{name: "native-anthropic", harness: "fiz", providerSystem: "anthropic", catalogSurface: modelcatalog.SurfaceAgentAnthropic},
		{name: "native-openai", harness: "fiz", providerSystem: "openai-compatible", catalogSurface: modelcatalog.SurfaceAgentOpenAI},
		{name: "codex", harness: "codex", providerSystem: "openai", catalogSurface: modelcatalog.SurfaceCodex},
		{name: "claude", harness: "claude", providerSystem: "anthropic", catalogSurface: modelcatalog.SurfaceClaudeCode},
		{name: "gemini", harness: "gemini", providerSystem: "google", catalogSurface: modelcatalog.SurfaceGemini},
	}
}

func cloneFloat64(v *float64) *float64 {
	if v == nil {
		return nil
	}
	cp := *v
	return &cp
}
