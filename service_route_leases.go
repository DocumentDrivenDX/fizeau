package fizeau

import (
	"strings"
	"time"

	"github.com/DocumentDrivenDX/fizeau/internal/routehealth"
)

const stickyRouteLeaseTTL = routehealth.DefaultLeaseTTL

func (s *service) routeLeaseStore() *routehealth.LeaseStore {
	if s.routeLeases == nil {
		s.routeLeases = routehealth.NewLeaseStore()
	}
	return s.routeLeases
}

func (s *service) applyStickyRouteLease(stickyKey string, decision *RouteDecision) {
	if s == nil || decision == nil || strings.TrimSpace(stickyKey) == "" {
		return
	}
	if decision.Harness != "agent" || decision.Provider == "" {
		return
	}

	now := time.Now().UTC()
	store := s.routeLeaseStore()
	baseProvider, _, _ := splitEndpointProviderRef(decision.Provider)
	if baseProvider == "" {
		baseProvider = decision.Provider
	}
	if baseProvider == "" || decision.Model == "" {
		return
	}

	key := routehealth.NormalizeLeaseKey(stickyKey, baseProvider, decision.Model)
	if lease, ok := store.Live(now, key); ok {
		if candidate, found := stickyLeaseCandidate(decision.Candidates, decision.Harness, baseProvider, decision.Model, lease.Endpoint); found && candidate.Eligible {
			store.Acquire(now, stickyRouteLeaseTTL, key, baseProvider, lease.Endpoint, decision.Model)
			decision.Provider = candidate.Provider
			decision.Endpoint = candidate.Endpoint
			return
		}
		reason := "endpoint disappeared"
		if candidate, found := stickyLeaseCandidate(decision.Candidates, decision.Harness, baseProvider, decision.Model, lease.Endpoint); found {
			reason = candidate.Reason
			if reason == "" {
				reason = "sticky endpoint became ineligible"
			}
		} else if candidate, found := stickyLeaseAnyEndpoint(decision.Candidates, decision.Harness, baseProvider, lease.Endpoint); found {
			reason = candidate.Reason
			if reason == "" {
				reason = "sticky endpoint became ineligible"
			}
		}
		store.Invalidate(now, key, reason)
	}

	candidate, found := stickyLeasePick(decision.Candidates, decision.Harness, baseProvider, decision.Model, store.LeaseCounts(now, baseProvider, decision.Model))
	if !found || !candidate.Eligible {
		return
	}
	chosenEndpoint := candidate.Endpoint
	if chosenEndpoint == "" {
		_, chosenEndpoint, _ = splitEndpointProviderRef(candidate.Provider)
	}
	if chosenEndpoint == "" {
		chosenEndpoint = candidate.Provider
	}
	store.Acquire(now, stickyRouteLeaseTTL, key, baseProvider, chosenEndpoint, decision.Model)
	decision.Provider = candidate.Provider
	decision.Endpoint = candidate.Endpoint
}

func stickyLeasePick(candidates []RouteCandidate, harness, provider, model string, counts map[string]int) (RouteCandidate, bool) {
	var chosen RouteCandidate
	found := false
	for _, candidate := range candidates {
		if !candidate.Eligible || candidate.Harness != harness || candidate.Model != model {
			continue
		}
		baseProvider, _, _ := splitEndpointProviderRef(candidate.Provider)
		if baseProvider == "" {
			baseProvider = candidate.Provider
		}
		if baseProvider != provider {
			continue
		}
		if candidate.Endpoint == "" && candidate.Provider == "" {
			continue
		}
		if !found {
			chosen = candidate
			found = true
			continue
		}
		leftCount := counts[endpointOf(candidate)]
		rightCount := counts[endpointOf(chosen)]
		if leftCount != rightCount {
			if leftCount < rightCount {
				chosen = candidate
			}
			continue
		}
		if candidate.Score != chosen.Score {
			if candidate.Score > chosen.Score {
				chosen = candidate
			}
			continue
		}
		if endpointOf(candidate) < endpointOf(chosen) {
			chosen = candidate
		}
	}
	return chosen, found
}

func stickyLeaseCandidate(candidates []RouteCandidate, harness, provider, model, endpoint string) (RouteCandidate, bool) {
	for _, candidate := range candidates {
		if !candidate.Eligible || candidate.Harness != harness || candidate.Model != model {
			continue
		}
		baseProvider, candidateEndpoint, _ := splitEndpointProviderRef(candidate.Provider)
		if baseProvider == "" {
			baseProvider = candidate.Provider
		}
		if baseProvider != provider {
			continue
		}
		if candidateEndpoint == "" {
			candidateEndpoint = candidate.Endpoint
		}
		if candidateEndpoint == endpoint {
			return candidate, true
		}
	}
	return RouteCandidate{}, false
}

func stickyLeaseAnyEndpoint(candidates []RouteCandidate, harness, provider, endpoint string) (RouteCandidate, bool) {
	for _, candidate := range candidates {
		if candidate.Harness != harness {
			continue
		}
		baseProvider, candidateEndpoint, _ := splitEndpointProviderRef(candidate.Provider)
		if baseProvider == "" {
			baseProvider = candidate.Provider
		}
		if baseProvider != provider {
			continue
		}
		if candidateEndpoint == "" {
			candidateEndpoint = candidate.Endpoint
		}
		if candidateEndpoint == endpoint {
			return candidate, true
		}
	}
	return RouteCandidate{}, false
}

func endpointOf(candidate RouteCandidate) string {
	if _, endpoint, ok := splitEndpointProviderRef(candidate.Provider); ok && endpoint != "" {
		return endpoint
	}
	return candidate.Endpoint
}
