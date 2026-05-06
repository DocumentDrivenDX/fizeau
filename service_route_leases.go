package fizeau

import (
	"strings"
	"time"

	"github.com/DocumentDrivenDX/fizeau/internal/routehealth"
	"github.com/DocumentDrivenDX/fizeau/internal/routing"
)

const stickyRouteLeaseTTL = routehealth.DefaultLeaseTTL

func (s *service) routeLeaseStore() *routehealth.LeaseStore {
	if s.routeLeases == nil {
		s.routeLeases = routehealth.NewLeaseStore()
	}
	return s.routeLeases
}

func (s *service) routeUtilizationStore() *routehealth.UtilizationStore {
	if s.routeUtilization == nil {
		s.routeUtilization = routehealth.NewUtilizationStore()
	}
	return s.routeUtilization
}

func (s *service) applyStickyRouteLease(stickyKey string, decision *RouteDecision) {
	if s == nil || decision == nil || strings.TrimSpace(stickyKey) == "" {
		return
	}
	decision.Sticky.KeyPresent = true
	if decision.Harness != "agent" || decision.Provider == "" {
		decision.Sticky.Assignment = "not_applicable"
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
			decision.Sticky.Assignment = "reused"
			decision.Sticky.Reason = "live sticky lease reused"
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
	if decision.Provider == "" && decision.Endpoint == "" {
		decision.Sticky.Assignment = "none"
		return
	}
	chosenEndpoint := decision.Endpoint
	if chosenEndpoint == "" {
		_, chosenEndpoint, _ = splitEndpointProviderRef(decision.Provider)
	}
	if chosenEndpoint == "" {
		chosenEndpoint = decision.Provider
	}
	if chosenEndpoint == "" {
		return
	}
	store.Acquire(now, stickyRouteLeaseTTL, key, baseProvider, chosenEndpoint, decision.Model)
	decision.Sticky.Assignment = "acquired"
	if decision.Sticky.Reason == "" {
		decision.Sticky.Reason = "new sticky lease acquired"
	}
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

func (s *service) routeEndpointLoadsResolver(now time.Time) func(provider, endpoint, model string) (routing.EndpointLoad, bool) {
	if s == nil {
		return nil
	}
	leaseStore := s.routeLeaseStore()
	utilStore := s.routeUtilizationStore()
	return func(provider, endpoint, model string) (routing.EndpointLoad, bool) {
		leaseCounts := leaseStore.LeaseCounts(now, provider, model)
		loads := utilStore.EndpointLoads(provider, model, leaseCounts)
		load, ok := loads[endpoint]
		if !ok {
			if count, ok := leaseCounts[endpoint]; ok {
				return routing.EndpointLoad{
					LeaseCount:       count,
					NormalizedLoad:   float64(count),
					UtilizationFresh: false,
				}, true
			}
			return routing.EndpointLoad{}, false
		}
		return routing.EndpointLoad{
			LeaseCount:           load.LeaseCount,
			NormalizedLoad:       load.NormalizedLoad,
			UtilizationFresh:     load.UtilizationFresh,
			UtilizationSaturated: load.UtilizationSaturated,
		}, true
	}
}
