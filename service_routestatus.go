package agent

import (
	"context"
	"time"
)

// RouteStatus returns global routing state across all configured routes.
// It is the operator dashboard view: cooldowns, recent decisions, and
// per-candidate health. Distinct from per-request ResolveRoute.
func (s *service) RouteStatus(_ context.Context) (*RouteStatusReport, error) {
	sc := s.opts.ServiceConfig
	report := &RouteStatusReport{
		GeneratedAt: time.Now(),
	}
	if sc == nil {
		return report, nil
	}

	cooldown := sc.HealthCooldown()
	if cooldown <= 0 {
		cooldown = 30 * time.Second
	}

	routeNames := sc.ModelRouteNames()
	report.Routes = make([]RouteStatusEntry, 0, len(routeNames))

	for _, routeName := range routeNames {
		rc := sc.ModelRouteConfig(routeName)
		entry := RouteStatusEntry{
			Model:    routeName,
			Strategy: rc.Strategy,
		}

		// Populate LastDecision from cache.
		if cached, ok := s.lookupRouteDecision(routeName); ok {
			entry.LastDecision = cached.decision
			entry.LastDecisionAt = cached.at
		}

		// Build per-candidate status.
		for _, cand := range rc.Candidates {
			cs := RouteCandidateStatus{
				Provider: cand.Provider,
				Model:    cand.Model,
				Priority: cand.Priority,
			}
			// Check cooldown state for this candidate.
			cs.Cooldown = routeCandidateCooldown(sc, routeName, cand.Provider, cooldown)
			cs.Healthy = cs.Cooldown == nil
			// RecentLatencyMS and RecentSuccessRate: zero — observation store not yet wired.
			entry.Candidates = append(entry.Candidates, cs)
		}

		report.Routes = append(report.Routes, entry)
	}

	return report, nil
}

// routeCandidateCooldown returns the active CooldownState for a specific
// (route, provider) pair, or nil if not in cooldown.
func routeCandidateCooldown(sc ServiceConfig, routeName, providerName string, cooldown time.Duration) *CooldownState {
	workDir := sc.WorkDir()
	if workDir == "" {
		return nil
	}
	now := time.Now().UTC()
	failures := serviceLoadRouteFailures(workDir, routeName)
	failedAt, hasFail := failures[providerName]
	if !hasFail {
		return nil
	}
	until := failedAt.Add(cooldown)
	if until.Before(now) {
		return nil
	}
	return &CooldownState{
		Reason:    "consecutive_failures",
		Until:     until,
		FailCount: 1,
	}
}

// cacheRouteDecision stores a ResolveRoute result keyed by routeKey.
// Called by ResolveRoute after a successful resolution.
func (s *service) cacheRouteDecision(routeKey string, dec *RouteDecision) {
	if routeKey == "" || dec == nil {
		return
	}
	s.lastDecisionMu.Lock()
	defer s.lastDecisionMu.Unlock()
	if s.lastDecisionCache == nil {
		s.lastDecisionCache = make(map[string]lastDecisionEntry)
	}
	s.lastDecisionCache[routeKey] = lastDecisionEntry{
		decision: dec,
		at:       time.Now(),
	}
}

// lookupRouteDecision retrieves a cached decision for routeKey.
func (s *service) lookupRouteDecision(routeKey string) (lastDecisionEntry, bool) {
	s.lastDecisionMu.RLock()
	defer s.lastDecisionMu.RUnlock()
	if s.lastDecisionCache == nil {
		return lastDecisionEntry{}, false
	}
	e, ok := s.lastDecisionCache[routeKey]
	return e, ok
}
