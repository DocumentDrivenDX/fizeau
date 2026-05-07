package fizeau

import (
	"context"
	"fmt"
	"sort"

	"github.com/DocumentDrivenDX/fizeau/internal/modelcatalog"
)

func (s *service) ListProfiles(_ context.Context) ([]ProfileInfo, error) {
	cat, err := modelcatalog.Default()
	if err != nil {
		return nil, err
	}
	meta := cat.Metadata()
	profiles := cat.Profiles()
	aliases := cat.Aliases()
	out := make([]ProfileInfo, 0, len(profiles)+len(aliases))
	seen := make(map[string]struct{}, len(profiles)+len(aliases))
	for _, profile := range profiles {
		info := ProfileInfo{
			Name:                profile.Name,
			Target:              profile.Target,
			CompatibilityTarget: profile.CompatibilityTarget,
			MinPower:            profile.MinPower,
			MaxPower:            profile.MaxPower,
			ProviderPreference:  profile.ProviderPreference,
			CatalogVersion:      meta.CatalogVersion,
			ManifestSource:      meta.ManifestSource,
			ManifestVersion:     meta.ManifestVersion,
		}
		if profile.CompatibilityTarget != "" && profile.Name != profile.CompatibilityTarget {
			info.AliasOf = profile.CompatibilityTarget
		}
		out = append(out, info)
		seen[profile.Name] = struct{}{}
	}
	for _, alias := range aliases {
		if _, ok := seen[alias.Name]; ok {
			continue
		}
		out = append(out, ProfileInfo{
			Name:            alias.Name,
			Target:          alias.Target,
			AliasOf:         alias.Target,
			Deprecated:      alias.Deprecated,
			Replacement:     alias.Replacement,
			CatalogVersion:  meta.CatalogVersion,
			ManifestSource:  meta.ManifestSource,
			ManifestVersion: meta.ManifestVersion,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out, nil
}

func (s *service) ProfileAliases(_ context.Context) (map[string]string, error) {
	cat, err := modelcatalog.Default()
	if err != nil {
		return nil, err
	}
	out := make(map[string]string)
	for _, profile := range cat.Profiles() {
		if profile.CompatibilityTarget != "" && profile.Name != profile.CompatibilityTarget {
			out[profile.Name] = profile.CompatibilityTarget
		}
	}
	for _, alias := range cat.Aliases() {
		target := alias.Target
		if alias.Deprecated && alias.Replacement != "" {
			target = alias.Replacement
		}
		out[alias.Name] = target
	}
	return out, nil
}

func (s *service) ResolveProfile(_ context.Context, name string) (*ResolvedProfile, error) {
	cat, err := modelcatalog.Default()
	if err != nil {
		return nil, err
	}
	meta := cat.Metadata()
	profile, ok := cat.Profile(name)
	var compatTarget string
	var deprecated bool
	var replacement string
	if ok {
		compatTarget = profile.CompatibilityTarget
		if compatTarget == "" {
			compatTarget = profile.Target
		}
	}
	if !ok {
		var aliasTarget string
		for _, surface := range serviceProfileSurfaces() {
			target, err := cat.Resolve(name, modelcatalog.ResolveOptions{
				Surface:         surface.catalogSurface,
				AllowDeprecated: true,
			})
			if err != nil {
				if _, ok := err.(*modelcatalog.MissingSurfaceError); ok {
					continue
				}
				return nil, err
			}
			aliasTarget = target.CanonicalID
			if target.Deprecated && target.Replacement != "" {
				deprecated = true
				replacement = target.Replacement
				aliasTarget = target.Replacement
			} else if target.Deprecated {
				deprecated = true
			}
			break
		}
		if aliasTarget == "" {
			return nil, fmt.Errorf("profile %q is not defined in the catalog", name)
		}
		if resolvedProfile, ok := cat.Profile(aliasTarget); ok {
			profile = resolvedProfile
			compatTarget = profile.CompatibilityTarget
			if compatTarget == "" {
				compatTarget = profile.Target
			}
		} else {
			for _, candidate := range cat.Profiles() {
				if candidate.CompatibilityTarget == aliasTarget || candidate.Target == aliasTarget {
					profile = candidate
					compatTarget = candidate.CompatibilityTarget
					if compatTarget == "" {
						compatTarget = candidate.Target
					}
					break
				}
			}
		}
	}
	if compatTarget == "" {
		return nil, fmt.Errorf("profile %q has no compatibility target", name)
	}

	var resolved *ResolvedProfile
	for _, surface := range serviceProfileSurfaces() {
		target, err := cat.Resolve(compatTarget, modelcatalog.ResolveOptions{
			Surface:         surface.catalogSurface,
			AllowDeprecated: true,
		})
		if err != nil {
			if _, ok := err.(*modelcatalog.MissingSurfaceError); ok {
				continue
			}
			return nil, err
		}
		if resolved == nil {
			resolved = &ResolvedProfile{
				Name:                name,
				Target:              compatTarget,
				CompatibilityTarget: compatTarget,
				MinPower:            profile.MinPower,
				MaxPower:            profile.MaxPower,
				Deprecated:          deprecated || target.Deprecated,
				Replacement:         replacement,
				CatalogVersion:      meta.CatalogVersion,
				ManifestSource:      meta.ManifestSource,
				ManifestVersion:     meta.ManifestVersion,
			}
		}
		resolved.Surfaces = append(resolved.Surfaces, ProfileSurface{
			Name:                    surface.name,
			Harness:                 surface.harness,
			ProviderSystem:          surface.providerSystem,
			Model:                   target.ConcreteModel,
			PlacementOrder:          append([]string(nil), target.SurfacePolicy.PlacementOrder...),
			CostCeilingInputPerMTok: cloneFloat64(target.SurfacePolicy.MaxInputCostPerMTokUSD),
			ReasoningDefault:        Reasoning(target.SurfacePolicy.ReasoningDefault),
			FailurePolicy:           target.SurfacePolicy.FailurePolicy,
		})
	}
	if resolved == nil {
		return nil, fmt.Errorf("profile %q has no service-supported surface", name)
	}
	sort.Slice(resolved.Surfaces, func(i, j int) bool {
		return resolved.Surfaces[i].Name < resolved.Surfaces[j].Name
	})
	return resolved, nil
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
