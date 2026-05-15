package fizeau

import (
	"time"

	"github.com/easel/fizeau/internal/routehealth"
	"github.com/easel/fizeau/internal/routing"
)

const stickyRouteLeaseTTL = routehealth.DefaultLeaseTTL
const stickyRouteAffinityBonus = 250.0

func (s *service) routeStickyState() *routehealth.StickyState {
	if s == nil {
		return nil
	}
	if s.routeSticky == nil {
		s.routeSticky = routehealth.NewStickyState()
	}
	return s.routeSticky
}

func (s *service) routeLeaseStore() *routehealth.LeaseStore {
	state := s.routeStickyState()
	if state == nil {
		return nil
	}
	return state.LeaseStore()
}

func (s *service) routeUtilizationStore() *routehealth.UtilizationStore {
	state := s.routeStickyState()
	if state == nil {
		return nil
	}
	return state.UtilizationStore()
}

func (s *service) applyStickyRouteLease(stickyKey string, decision *RouteDecision) {
	if s == nil || decision == nil {
		return
	}
	sticky := s.routeStickyState().ApplyStickyLease(time.Now().UTC(), stickyRouteLeaseTTL, stickyRouteAffinityBonus, routehealth.StickyRequest{
		StickyKey:      stickyKey,
		Harness:        decision.Harness,
		Provider:       decision.Provider,
		Endpoint:       decision.Endpoint,
		ServerInstance: decision.ServerInstance,
		Model:          decision.Model,
	})
	decision.Sticky = RouteStickyState{
		KeyPresent:     sticky.KeyPresent,
		Assignment:     sticky.Assignment,
		ServerInstance: sticky.ServerInstance,
		Reason:         sticky.Reason,
		Bonus:          sticky.Bonus,
	}
}

func (s *service) routeEndpointLoadsResolver(now time.Time) func(provider, endpoint, model string) (routing.EndpointLoad, bool) {
	state := s.routeStickyState()
	if state == nil {
		return nil
	}
	return state.EndpointLoadResolver(now)
}

func (s *service) routeStickyServerInstanceResolver(now time.Time) func(stickyKey string) (string, bool) {
	state := s.routeStickyState()
	if state == nil {
		return nil
	}
	return state.StickyServerInstanceResolver(now)
}
