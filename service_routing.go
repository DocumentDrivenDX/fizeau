package agent

import (
	"context"
	"fmt"
	"time"

	"github.com/DocumentDrivenDX/agent/internal/harnesses"
	claudeharness "github.com/DocumentDrivenDX/agent/internal/harnesses/claude"
	"github.com/DocumentDrivenDX/agent/internal/routing"
)

// ResolveRoute resolves an under-specified RouteRequest to a concrete
// (Harness, Provider, Model) decision per CONTRACT-003.
//
// The implementation delegates to internal/routing.Resolve — the single
// routing engine that consolidates DDx-side harness-tier ranking and
// agent-side provider failover ordering.
func (s *service) ResolveRoute(ctx context.Context, req RouteRequest) (*RouteDecision, error) {
	in := s.buildRoutingInputs()
	pref := req.ProviderPreference
	if pref == "" {
		pref = ProviderPreferenceLocalFirst
	}
	switch pref {
	case ProviderPreferenceLocalFirst, ProviderPreferenceSubscriptionFirst,
		ProviderPreferenceLocalOnly, ProviderPreferenceSubscriptionOnly:
		// Valid.
	default:
		return nil, fmt.Errorf("invalid ProviderPreference: %q", pref)
	}

	rReq := routing.Request{
		Profile:            reqProfileFromModelRef(req.ModelRef),
		ModelRef:           reqModelRefStripProfile(req.ModelRef),
		Model:              req.Model,
		Provider:           req.Provider,
		Harness:            req.Harness,
		Reasoning:          effectiveReasoningString(req.Reasoning),
		Permissions:        req.Permissions,
		ProviderPreference: string(pref),
	}
	dec, err := routing.Resolve(rReq, in)
	if err != nil {
		return nil, err
	}
	result := &RouteDecision{
		Harness:  dec.Harness,
		Provider: dec.Provider,
		Model:    dec.Model,
		Reason:   dec.Reason,
	}
	// Cache the decision so RouteStatus can surface LastDecision.
	s.cacheRouteDecision(req.Model, result)
	return result, nil
}

// reqProfileFromModelRef returns ref when ref is a known profile alias,
// or "" otherwise. The contract puts ModelRef and Profile in the same field.
func reqProfileFromModelRef(ref string) string {
	switch ref {
	case "cheap", "standard", "smart":
		return ref
	}
	return ""
}

// reqModelRefStripProfile returns "" when ref is a known profile alias,
// or ref otherwise.
func reqModelRefStripProfile(ref string) string {
	switch ref {
	case "cheap", "standard", "smart":
		return ""
	}
	return ref
}

// buildRoutingInputs assembles routing.Inputs from the service's registry
// and ServiceConfig. Provider discovery is left as a follow-up; for now the
// inputs reflect harness availability + configured providers.
func (s *service) buildRoutingInputs() routing.Inputs {
	statuses := s.registry.Discover()
	statusByName := make(map[string]harnesses.HarnessStatus, len(statuses))
	for _, st := range statuses {
		statusByName[st.Name] = st
	}

	var entries []routing.HarnessEntry
	for _, name := range s.registry.Names() {
		cfg, ok := s.registry.Get(name)
		if !ok {
			continue
		}
		st := statusByName[name]
		entry := routing.HarnessEntry{
			Name:               name,
			Surface:            cfg.Surface,
			CostClass:          cfg.CostClass,
			IsLocal:            cfg.IsLocal,
			IsSubscription:     cfg.IsSubscription,
			IsHTTPProvider:     cfg.IsHTTPProvider,
			TestOnly:           cfg.TestOnly,
			ExactPinSupport:    cfg.ExactPinSupport,
			DefaultModel:       cfg.DefaultModel,
			SupportedReasoning: supportedReasoning(cfg),
			MaxReasoningTokens: cfg.MaxReasoningTokens,
			SupportedPerms:     supportedPermissions(cfg),
			SupportsTools:      true, // all builtin harnesses support tools today
			Available:          st.Available,
			QuotaOK:            true,
			QuotaTrend:         routing.QuotaTrendUnknown,
		}

		if name == "claude" {
			dec := claudeharness.ReadClaudeQuotaRoutingDecision(time.Now(), 0)
			entry.QuotaOK = dec.PreferClaude
			entry.QuotaStale = !dec.Fresh && dec.SnapshotPresent
			if dec.Snapshot != nil {
				maxUsed := 0.0
				if dec.Snapshot.FiveHourLimit > 0 {
					maxUsed = float64(dec.Snapshot.FiveHourLimit-dec.Snapshot.FiveHourRemaining) / float64(dec.Snapshot.FiveHourLimit) * 100
				}
				if dec.Snapshot.WeeklyLimit > 0 {
					weeklyUsed := float64(dec.Snapshot.WeeklyLimit-dec.Snapshot.WeeklyRemaining) / float64(dec.Snapshot.WeeklyLimit) * 100
					if weeklyUsed > maxUsed {
						maxUsed = weeklyUsed
					}
				}
				entry.QuotaPercentUsed = int(maxUsed)
				if maxUsed >= 90 {
					entry.QuotaTrend = routing.QuotaTrendExhausting
				} else if maxUsed >= 70 {
					entry.QuotaTrend = routing.QuotaTrendBurning
				} else if dec.Fresh {
					entry.QuotaTrend = routing.QuotaTrendHealthy
				}
			}
		}

		// Native "agent" harness: enumerate configured providers.
		if name == "agent" && s.opts.ServiceConfig != nil {
			for _, pname := range s.opts.ServiceConfig.ProviderNames() {
				pcfg, ok := s.opts.ServiceConfig.Provider(pname)
				if !ok {
					continue
				}
				entry.Providers = append(entry.Providers, routing.ProviderEntry{
					Name:          pname,
					BaseURL:       pcfg.BaseURL,
					DefaultModel:  pcfg.Model,
					SupportsTools: true,
				})
			}
		}
		entries = append(entries, entry)
	}
	return routing.Inputs{
		Harnesses: entries,
	}
}

// resolveExecuteRouteWithEngine is the post-engine variant of resolveExecuteRoute.
// It is invoked by Execute when the request is under-specified
// (no PreResolved, no fully-specified Harness). Returns nil when the request
// is already specific enough that the legacy resolveExecuteRoute path applies.
func (s *service) resolveExecuteRouteWithEngine(req ServiceExecuteRequest) (*RouteDecision, error) {
	rr := RouteRequest{
		Model:              req.Model,
		Provider:           req.Provider,
		Harness:            req.Harness,
		ModelRef:           req.ModelRef,
		Reasoning:          req.Reasoning,
		Permissions:        req.Permissions,
		ProviderPreference: req.ProviderPreference,
	}
	dec, err := s.ResolveRoute(context.Background(), rr)
	if err != nil {
		return nil, fmt.Errorf("ResolveRoute: %w", err)
	}
	return dec, nil
}
