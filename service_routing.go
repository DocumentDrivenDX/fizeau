package agent

import (
	"context"
	"fmt"
	"time"

	"github.com/DocumentDrivenDX/agent/internal/harnesses"
	claudeharness "github.com/DocumentDrivenDX/agent/internal/harnesses/claude"
	codexharness "github.com/DocumentDrivenDX/agent/internal/harnesses/codex"
	"github.com/DocumentDrivenDX/agent/internal/routing"
)

// ResolveRoute resolves an under-specified RouteRequest to a concrete
// (Harness, Provider, Model) decision per CONTRACT-003.
//
// The implementation delegates to internal/routing.Resolve — the single
// routing engine that consolidates DDx-side harness-tier ranking and
// agent-side provider failover ordering.
func (s *service) ResolveRoute(ctx context.Context, req RouteRequest) (*RouteDecision, error) {
	s.ensurePrimaryQuotaRefresh(ctx, quotaRefreshAsync)
	in := s.buildRoutingInputs(ctx)
	profile := req.Profile
	if profile == "" {
		profile = reqProfileFromModelRef(req.ModelRef)
	}

	rReq := routing.Request{
		Profile:            profile,
		ModelRef:           reqModelRefStripProfile(req.ModelRef),
		Model:              req.Model,
		Provider:           req.Provider,
		Harness:            req.Harness,
		Reasoning:          effectiveReasoningString(req.Reasoning),
		Permissions:        req.Permissions,
		ProviderPreference: providerPreferenceForProfile(profile),
	}
	s.applyRouteAttemptCooldowns(&in)
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

func (s *service) applyRouteAttemptCooldowns(in *routing.Inputs) {
	if in == nil {
		return
	}
	ttl := s.routeAttemptTTL()
	records := s.activeRouteAttempts(time.Now(), ttl)
	if len(records) == 0 {
		return
	}
	if in.ProviderCooldowns == nil {
		in.ProviderCooldowns = make(map[string]time.Time)
	}
	if in.CooldownDuration <= 0 {
		in.CooldownDuration = ttl
	}
	for _, record := range records {
		if record.key.Provider != "" {
			existing, ok := in.ProviderCooldowns[record.key.Provider]
			if !ok || record.recordedAt.After(existing) {
				in.ProviderCooldowns[record.key.Provider] = record.recordedAt
			}
		}
		if record.key.Provider == "" && record.key.Harness != "" {
			for i := range in.Harnesses {
				if in.Harnesses[i].Name == record.key.Harness {
					in.Harnesses[i].InCooldown = true
				}
			}
		}
	}
}

func (s *service) routeAttemptTTL() time.Duration {
	if s.opts.ServiceConfig == nil {
		return defaultRouteAttemptCooldown
	}
	ttl := s.opts.ServiceConfig.HealthCooldown()
	if ttl <= 0 {
		return defaultRouteAttemptCooldown
	}
	return ttl
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
// and ServiceConfig. When the service has a catalog cache attached (v0.9.2+),
// each configured provider's ProviderEntry is populated with DiscoveredIDs
// from the cache's live /v1/models probe, so routing.FuzzyMatch matches the
// request against IDs the server actually serves rather than the configured
// default-model string.
//
// ctx is used for cache probes with a short deadline; the cache's
// stale-while-revalidate flow makes most calls non-blocking.
func (s *service) buildRoutingInputs(ctx context.Context) routing.Inputs {
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
			Name:                name,
			Surface:             cfg.Surface,
			CostClass:           cfg.CostClass,
			IsLocal:             cfg.IsLocal,
			IsSubscription:      cfg.IsSubscription,
			IsHTTPProvider:      cfg.IsHTTPProvider,
			AutoRoutingEligible: cfg.AutoRoutingEligible,
			TestOnly:            cfg.TestOnly,
			ExactPinSupport:     cfg.ExactPinSupport,
			DefaultModel:        cfg.DefaultModel,
			SupportedReasoning:  supportedReasoning(cfg),
			MaxReasoningTokens:  cfg.MaxReasoningTokens,
			SupportedPerms:      supportedPermissions(cfg),
			SupportsTools:       true, // all builtin harnesses support tools today
			Available:           st.Available,
			QuotaOK:             true,
			QuotaTrend:          routing.QuotaTrendUnknown,
			// SubscriptionOK defaults to true and is refined by subscription
			// harness quota caches below.
			SubscriptionOK: true,
		}

		if name == "claude" {
			dec := claudeharness.ReadClaudeQuotaRoutingDecision(time.Now(), 0)
			entry.QuotaOK = dec.PreferClaude
			entry.QuotaStale = !dec.Fresh && dec.SnapshotPresent
			entry.SubscriptionOK = dec.PreferClaude
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

		if name == "codex" {
			dec := codexharness.ReadCodexQuotaRoutingDecision(time.Now(), 0)
			entry.QuotaOK = dec.PreferCodex
			entry.QuotaStale = !dec.Fresh && dec.SnapshotPresent
			entry.SubscriptionOK = dec.PreferCodex
			if dec.Snapshot != nil {
				maxUsed := 0.0
				for _, window := range dec.Snapshot.Windows {
					if window.UsedPercent > maxUsed {
						maxUsed = window.UsedPercent
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
				pe := routing.ProviderEntry{
					Name:          pname,
					BaseURL:       pcfg.BaseURL,
					DefaultModel:  pcfg.Model,
					SupportsTools: true,
				}
				// Populate DiscoveredIDs from the live /v1/models cache so
				// FuzzyMatch matches against what the server actually
				// serves — not the statically-configured default model
				// string. Silent-fails: if the probe errors or the endpoint
				// doesn't support discovery, DiscoveredIDs stays empty
				// and routing falls back to DefaultModel behaviour.
				if s.catalog != nil {
					ids := s.probeProviderDiscoveredIDs(ctx, pcfg)
					if len(ids) > 0 {
						pe.DiscoveredIDs = ids
					}
				}
				entry.Providers = append(entry.Providers, pe)
			}
		}
		entries = append(entries, entry)
	}
	return routing.Inputs{
		Harnesses: entries,
	}
}

// probeProviderDiscoveredIDs returns the live /v1/models catalog for the
// given provider via the service catalog cache. Returns nil on any error
// or when discovery is unsupported; callers then fall back to the
// configured DefaultModel behaviour.
//
// Probes use a 2-second deadline so a slow or partially-degraded endpoint
// can't block route resolution. The cache's stale-while-revalidate flow
// means this is usually sub-millisecond (fresh or stale hit).
func (s *service) probeProviderDiscoveredIDs(ctx context.Context, pcfg ServiceProviderEntry) []string {
	if pcfg.BaseURL == "" {
		return nil
	}
	key := newCatalogCacheKey(pcfg.BaseURL, pcfg.APIKey, nil)
	probeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	probe := func(ctx context.Context) ([]string, error) {
		return probeOpenAIModels(ctx, pcfg.BaseURL, pcfg.APIKey)
	}
	result, err := s.catalog.Get(probeCtx, key, probe)
	if err != nil {
		return nil
	}
	if !result.DiscoverySupported {
		return nil
	}
	return result.IDs
}

// resolveExecuteRouteWithEngine is the post-engine variant of resolveExecuteRoute.
// It is invoked by Execute when the request is under-specified
// (no PreResolved, no fully-specified Harness). Returns nil when the request
// is already specific enough that the legacy resolveExecuteRoute path applies.
func (s *service) resolveExecuteRouteWithEngine(req ServiceExecuteRequest) (*RouteDecision, error) {
	rr := RouteRequest{
		Profile:     req.Profile,
		Model:       req.Model,
		Provider:    req.Provider,
		Harness:     req.Harness,
		ModelRef:    req.ModelRef,
		Reasoning:   req.Reasoning,
		Permissions: req.Permissions,
	}
	dec, err := s.ResolveRoute(context.Background(), rr)
	if err != nil {
		return nil, fmt.Errorf("ResolveRoute: %w", err)
	}
	return dec, nil
}

func providerPreferenceForProfile(profile string) string {
	switch profile {
	case "offline", "air-gapped":
		return routing.ProviderPreferenceLocalOnly
	case "smart", "code-high":
		return routing.ProviderPreferenceSubscriptionFirst
	default:
		return routing.ProviderPreferenceLocalFirst
	}
}
