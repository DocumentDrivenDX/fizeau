package fizeau

import (
	"strings"

	"github.com/easel/fizeau/internal/modelcatalog"
	"github.com/easel/fizeau/internal/modelmatch"
	"github.com/easel/fizeau/internal/routing"
)

// resolveModelConstraint resolves an explicit raw model pin to a single
// concrete model ID using the service's discovered provider inventory and
// catalog aliases.
//
// Matching order:
//  1. discovered/provider concrete models and catalog-resolved concrete IDs
//  2. configured provider or harness default models
//
// A unique match returns the resolved concrete model ID. Multiple matches
// return ErrModelConstraintAmbiguous. No matches return
// ErrModelConstraintNoMatch with nearby candidate evidence.
func (s *service) resolveModelConstraint(reqHarness, reqProvider, reqModel string, in routing.Inputs, cat *modelcatalog.Catalog) (string, []RouteCandidate, error) {
	reqModel = strings.TrimSpace(reqModel)
	if reqModel == "" {
		return "", nil, nil
	}

	concreteCandidates := collectConcreteModelCandidates(reqHarness, reqProvider, reqModel, in, cat)
	if resolved, err := resolveSingleModelMatch(reqModel, concreteCandidates); err != nil {
		return "", modelCandidatesToRouteCandidates(concreteCandidates), err
	} else if resolved != "" {
		return resolved, nil, nil
	}

	defaultCandidates := collectDefaultModelCandidates(reqHarness, reqProvider, in)
	if resolved, err := resolveSingleModelMatch(reqModel, defaultCandidates); err != nil {
		return "", modelCandidatesToRouteCandidates(append(concreteCandidates, defaultCandidates...)), err
	} else if resolved != "" {
		return resolved, nil, nil
	}

	evidence := append([]string(nil), concreteCandidates...)
	evidence = append(evidence, defaultCandidates...)
	return "", modelCandidatesToRouteCandidates(evidence), &ErrModelConstraintNoMatch{
		Model:      reqModel,
		Candidates: evidence,
	}
}

func collectConcreteModelCandidates(reqHarness, reqProvider, reqModel string, in routing.Inputs, cat *modelcatalog.Catalog) []string {
	seen := make(map[string]struct{})
	out := make([]string, 0)
	harnessPin := canonicalHarnessPin(reqHarness)
	providerPin := strings.TrimSpace(reqProvider)
	if base, _, ok := splitEndpointProviderRef(providerPin); ok {
		providerPin = base
	}
	add := func(id string) {
		id = strings.TrimSpace(id)
		if id == "" {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}

	for _, h := range in.Harnesses {
		if harnessPin != "" && harnessPin != canonicalHarnessPin(h.Name) {
			continue
		}
		for _, p := range h.Providers {
			candidateProvider := candidateProviderIdentity(h, p)
			if providerPin != "" && providerPin != candidateProvider {
				continue
			}
			for _, id := range p.DiscoveredIDs {
				add(id)
			}
		}
	}

	if cat != nil {
		seenSurface := make(map[modelcatalog.Surface]struct{})
		for _, h := range in.Harnesses {
			if harnessPin != "" && harnessPin != canonicalHarnessPin(h.Name) {
				continue
			}
			surface := modelcatalog.Surface(h.Surface)
			if surface == "" {
				continue
			}
			if _, ok := seenSurface[surface]; ok {
				continue
			}
			seenSurface[surface] = struct{}{}
			if resolved, err := cat.Resolve(reqModel, modelcatalog.ResolveOptions{
				Surface:         surface,
				AllowDeprecated: true,
			}); err == nil && resolved.ConcreteModel != "" {
				add(resolved.ConcreteModel)
			}
		}
	}

	return out
}

func collectDefaultModelCandidates(reqHarness, reqProvider string, in routing.Inputs) []string {
	seen := make(map[string]struct{})
	out := make([]string, 0)
	harnessPin := canonicalHarnessPin(reqHarness)
	providerPin := strings.TrimSpace(reqProvider)
	if base, _, ok := splitEndpointProviderRef(providerPin); ok {
		providerPin = base
	}
	add := func(id string) {
		id = strings.TrimSpace(id)
		if id == "" {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}

	for _, h := range in.Harnesses {
		if harnessPin != "" && harnessPin != canonicalHarnessPin(h.Name) {
			continue
		}
		if providerPin == "" {
			add(h.DefaultModel)
		}
		for _, p := range h.Providers {
			if providerPin != "" && providerPin != candidateProviderIdentity(h, p) {
				continue
			}
			add(p.DefaultModel)
		}
	}
	return out
}

func resolveSingleModelMatch(reqModel string, candidates []string) (string, error) {
	matches := modelmatch.Match(reqModel, candidates)
	switch len(matches) {
	case 0:
		return "", nil
	case 1:
		return matches[0], nil
	default:
		return "", &ErrModelConstraintAmbiguous{
			Model:      reqModel,
			Candidates: matches,
		}
	}
}

func modelCandidatesToRouteCandidates(ids []string) []RouteCandidate {
	if len(ids) == 0 {
		return nil
	}
	out := make([]RouteCandidate, 0, len(ids))
	seen := make(map[string]struct{})
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, RouteCandidate{Model: id, Reason: "model candidate"})
	}
	return out
}

func canonicalHarnessPin(harness string) string {
	if harness == "local" {
		return "fiz"
	}
	return harness
}

func candidateProviderIdentity(h routing.HarnessEntry, p routing.ProviderEntry) string {
	if p.Name != "" {
		return p.Name
	}
	return h.Name
}
