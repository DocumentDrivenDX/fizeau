package fizeau

import (
	"strings"
	"time"

	"github.com/DocumentDrivenDX/fizeau/internal/routehealth"
	"github.com/DocumentDrivenDX/fizeau/internal/routing"
)

const stickyRouteLeaseTTL = routehealth.DefaultLeaseTTL
const stickyRouteAffinityBonus = 250.0

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
	if decision.Harness != "fiz" || decision.Provider == "" {
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

	key := routehealth.NormalizeLeaseKey(stickyKey)
	selectedServerInstance := strings.TrimSpace(decision.ServerInstance)
	if selectedServerInstance == "" {
		selectedServerInstance = strings.TrimSpace(decision.Endpoint)
	}
	if selectedServerInstance == "" {
		_, selectedServerInstance, _ = splitEndpointProviderRef(decision.Provider)
	}
	if selectedServerInstance == "" {
		selectedServerInstance = decision.Provider
	}
	lease, ok := store.Live(now, key)
	if ok && lease.Endpoint == selectedServerInstance {
		decision.Sticky.Assignment = "reused"
		decision.Sticky.ServerInstance = selectedServerInstance
		decision.Sticky.Bonus = stickyRouteAffinityBonus
		decision.Sticky.Reason = "live sticky lease reused"
	} else if ok {
		decision.Sticky.Assignment = "moved"
		decision.Sticky.ServerInstance = selectedServerInstance
		decision.Sticky.Bonus = 0
		decision.Sticky.Reason = "sticky server instance lost to a stronger candidate"
	} else {
		decision.Sticky.Assignment = "acquired"
		decision.Sticky.ServerInstance = selectedServerInstance
		decision.Sticky.Bonus = 0
		if decision.Sticky.Reason == "" {
			decision.Sticky.Reason = "new sticky lease acquired"
		}
	}
	if selectedServerInstance == "" {
		decision.Sticky.Assignment = "none"
		return
	}
	store.Acquire(now, stickyRouteLeaseTTL, key, baseProvider, selectedServerInstance, decision.Model)
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

func (s *service) routeStickyServerInstanceResolver(now time.Time) func(stickyKey string) (string, bool) {
	if s == nil {
		return nil
	}
	store := s.routeLeaseStore()
	return func(stickyKey string) (string, bool) {
		if strings.TrimSpace(stickyKey) == "" {
			return "", false
		}
		lease, ok := store.Live(now, routehealth.NormalizeLeaseKey(stickyKey))
		if !ok {
			return "", false
		}
		if lease.Endpoint == "" {
			return "", false
		}
		return lease.Endpoint, true
	}
}
