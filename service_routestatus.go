package fizeau

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/easel/fizeau/internal/modelsnapshot"
	"github.com/easel/fizeau/internal/routehealth"
	"github.com/easel/fizeau/internal/serverinstance"
)

// RouteStatus returns live routing state for configured providers/models.
// It is the operator dashboard view: cooldowns, recent decisions, and
// per-candidate health. Distinct from per-request ResolveRoute.
func (s *service) RouteStatus(ctx context.Context) (*RouteStatusReport, error) {
	s.ensurePrimaryQuotaRefresh(ctx, quotaRefreshAsync)
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
	if s == nil || s.opts.ServiceConfig == nil {
		return report, nil
	}

	cat := serviceRoutingCatalog()
	_, snapshot := s.buildRoutingInputsWithCatalog(ctx, cat)
	cooldown := s.routeAttemptTTL()
	activeAttempts := s.activeRouteAttempts(report.GeneratedAt, cooldown)
	successRate, latencyMS := s.routeMetricSignals(report.GeneratedAt, cooldown)
	report.Routes = s.routeStatusEntriesFromSnapshot(snapshot, activeAttempts, successRate, latencyMS, cooldown)
	return report, nil
}

func (s *service) routeStatusEntriesFromSnapshot(snapshot modelsnapshot.ModelSnapshot, activeAttempts []routehealth.Record, successRate, latencyMS map[string]float64, cooldown time.Duration) []RouteStatusEntry {
	if len(snapshot.Models) == 0 {
		return nil
	}
	grouped := make(map[string][]modelsnapshot.KnownModel)
	for _, row := range snapshot.Models {
		if harness := strings.TrimSpace(row.Harness); harness != "" && harness != "fiz" {
			continue
		}
		if s.opts.ServiceConfig != nil {
			if _, ok := s.opts.ServiceConfig.Provider(row.Provider); !ok {
				continue
			}
		}
		model := strings.TrimSpace(row.ID)
		if model == "" {
			continue
		}
		grouped[model] = append(grouped[model], row)
	}
	if len(grouped) == 0 {
		return nil
	}
	keys := make([]string, 0, len(grouped))
	for model := range grouped {
		keys = append(keys, model)
	}
	sort.Strings(keys)

	entries := make([]RouteStatusEntry, 0, len(keys))
	for _, model := range keys {
		rows := grouped[model]
		sort.Slice(rows, func(i, j int) bool {
			if rows[i].Provider != rows[j].Provider {
				return rows[i].Provider < rows[j].Provider
			}
			if rows[i].EndpointName != rows[j].EndpointName {
				return rows[i].EndpointName < rows[j].EndpointName
			}
			if rows[i].EndpointBaseURL != rows[j].EndpointBaseURL {
				return rows[i].EndpointBaseURL < rows[j].EndpointBaseURL
			}
			if rows[i].ServerInstance != rows[j].ServerInstance {
				return rows[i].ServerInstance < rows[j].ServerInstance
			}
			return rows[i].ID < rows[j].ID
		})
		entry := RouteStatusEntry{Model: model, Strategy: "auto"}
		if cached, ok := s.lookupRouteDecision(model); ok {
			entry.LastDecision = cached.decision
			entry.LastDecisionAt = cached.at
			entry.SelectedEndpoint = cached.decision.Endpoint
			entry.SelectedServerInstance = cached.decision.ServerInstance
			entry.Sticky = cached.decision.Sticky
		}
		entry.Candidates = make([]RouteCandidateStatus, 0, len(rows))
		for i, row := range rows {
			provider := strings.TrimSpace(row.Provider)
			endpoint := strings.TrimSpace(row.EndpointName)
			serverID := serverinstance.Normalize(row.EndpointBaseURL, row.ServerInstance)
			candidate := RouteCandidateStatus{
				Provider:                provider,
				Endpoint:                endpoint,
				Model:                   model,
				ServerInstance:          serverID,
				Billing:                 row.Billing,
				Priority:                len(rows) - i,
				Healthy:                 true,
				SourceStatus:            string(row.Status),
				AutoRoutable:            row.AutoRoutable,
				ExactPinOnly:            row.ExactPinOnly,
				ExclusionReason:         row.ExclusionReason,
				Power:                   row.Power,
				ContextLength:           row.ContextWindow,
				CostInputPerMTok:        row.CostInputPerM,
				CostOutputPerMTok:       row.CostOutputPerM,
				RecentLatencyMS:         float64(row.RecentP50Latency.Milliseconds()),
				ProviderReliabilityRate: routeStatusMetricValue(successRate, provider, endpoint, model),
				QuotaRemaining:          row.QuotaRemaining,
			}
			if attemptCooldown := routeAttemptCandidateCooldown(activeAttempts, provider, endpoint, model, cooldown); attemptCooldown != nil {
				candidate.Cooldown = attemptCooldown
			}
			if candidate.Cooldown != nil {
				candidate.Healthy = false
			}
			if candidate.RecentLatencyMS == 0 {
				candidate.RecentLatencyMS = routeStatusMetricValue(latencyMS, provider, endpoint, model)
			}
			entry.Candidates = append(entry.Candidates, candidate)
		}
		entries = append(entries, entry)
	}
	return entries
}

func routeStatusMetricValue(values map[string]float64, provider, endpoint, model string) float64 {
	if len(values) == 0 {
		return 0
	}
	key := routeStatusMetricKey(provider, endpoint, model)
	if value, ok := values[key]; ok {
		return value
	}
	if endpoint != "" {
		if value, ok := values[routeStatusMetricKey(provider, "", model)]; ok {
			return value
		}
	}
	return 0
}

func routeStatusMetricKey(provider, endpoint, model string) string {
	return routehealth.ProviderModelKey(routehealth.Key{
		Provider: provider,
		Endpoint: endpoint,
		Model:    model,
	})
}

func routeAttemptCandidateCooldown(records []routehealth.Record, providerName, endpointName, model string, cooldown time.Duration) *CooldownState {
	var newest *routehealth.Record
	for i := range records {
		record := &records[i]
		if record.Key.Provider == "" {
			continue
		}
		recordProvider := record.Key.Provider
		recordEndpoint := record.Key.Endpoint
		if base, ep, ok := splitEndpointProviderRef(recordProvider); ok {
			recordProvider = base
			if recordEndpoint == "" {
				recordEndpoint = ep
			}
		}
		if providerName != "" && recordProvider != providerName {
			continue
		}
		if endpointName != "" && recordEndpoint != "" && recordEndpoint != endpointName {
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
