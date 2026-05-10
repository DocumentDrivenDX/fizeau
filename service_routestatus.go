package fizeau

import (
	"context"
	"time"

	"github.com/easel/fizeau/internal/routehealth"
	"github.com/easel/fizeau/internal/serverinstance"
)

// RouteStatus returns live routing state for configured providers/models.
// It is the operator dashboard view: cooldowns, recent decisions, and
// per-candidate health. Distinct from per-request ResolveRoute.
func (s *service) RouteStatus(ctx context.Context) (*RouteStatusReport, error) {
	s.ensurePrimaryQuotaRefresh(ctx, quotaRefreshAsync)
	sc := s.opts.ServiceConfig
	report := &RouteStatusReport{
		GeneratedAt: time.Now(),
	}
	// ADR-006 §5: populate routing-quality over a recent window
	// (RouteStatusRoutingQualityWindow) regardless of whether a route
	// catalog is configured — the metric reflects Execute traffic, not
	// configured routes.
	if s != nil && s.routingQuality != nil {
		recent := s.routingQuality.snapshotRecent(RouteStatusRoutingQualityWindow, time.Time{})
		report.RoutingQuality = computeRoutingQualityMetricsFromRecords(recent)
	}
	if sc == nil {
		return report, nil
	}

	cooldown := s.routeAttemptTTL()
	activeAttempts := s.activeRouteAttempts(report.GeneratedAt, cooldown)

	entries := make(map[string]*RouteStatusEntry)
	order := make([]string, 0)
	for i, providerName := range sc.ProviderNames() {
		provider, ok := sc.Provider(providerName)
		if !ok {
			continue
		}
		model := provider.Model
		if model == "" {
			model = providerName
		}
		entry, ok := entries[model]
		if !ok {
			entry = &RouteStatusEntry{Model: model, Strategy: "auto"}
			if cached, ok := s.lookupRouteDecision(model); ok {
				entry.LastDecision = cached.decision
				entry.LastDecisionAt = cached.at
				entry.SelectedEndpoint = cached.decision.Endpoint
				entry.SelectedServerInstance = cached.decision.ServerInstance
				entry.Sticky = cached.decision.Sticky
			}
			entries[model] = entry
			order = append(order, model)
		}
		cs := RouteCandidateStatus{
			Provider:       providerName,
			Model:          model,
			ServerInstance: serverinstance.Normalize(provider.BaseURL, provider.ServerInstance),
			Priority:       len(sc.ProviderNames()) - i,
		}
		if attemptCooldown := routeAttemptCandidateCooldown(activeAttempts, providerName, model, cooldown); attemptCooldown != nil {
			cs.Cooldown = attemptCooldown
		}
		cs.Healthy = cs.Cooldown == nil
		entry.Candidates = append(entry.Candidates, cs)
	}

	report.Routes = make([]RouteStatusEntry, 0, len(order))
	for _, model := range order {
		report.Routes = append(report.Routes, *entries[model])
	}

	return report, nil
}

func routeAttemptCandidateCooldown(records []routehealth.Record, providerName, model string, cooldown time.Duration) *CooldownState {
	var newest *routehealth.Record
	for i := range records {
		record := &records[i]
		if record.Key.Provider == "" {
			continue
		}
		if providerName != "" && record.Key.Provider != providerName {
			continue
		}
		if record.Key.Model != "" && model != "" && record.Key.Model != model {
			continue
		}
		if newest == nil || record.RecordedAt.After(newest.RecordedAt) {
			newest = record
		}
	}
	if newest == nil {
		return nil
	}
	return routeAttemptCooldown(*newest, cooldown)
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
