package fizeau

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/DocumentDrivenDX/fizeau/internal/harnesses"
	"github.com/DocumentDrivenDX/fizeau/internal/modelcatalog"
	"github.com/DocumentDrivenDX/fizeau/internal/provider/utilization"
	"github.com/DocumentDrivenDX/fizeau/internal/routing"
)

var loadRoutingCatalog = modelcatalog.Default

// ResolveRoute resolves an under-specified RouteRequest to a concrete
// (Harness, Provider, Model) decision per CONTRACT-003.
//
// The implementation delegates to internal/routing.Resolve — the single
// routing engine that consolidates DDx-side harness-tier ranking and
// fiz-side provider failover ordering.
func (s *service) ResolveRoute(ctx context.Context, req RouteRequest) (*RouteDecision, error) {
	if err := ValidatePowerBounds(req.MinPower, req.MaxPower); err != nil {
		return nil, err
	}
	if err := ValidateRole(req.Role); err != nil {
		return nil, err
	}
	if err := ValidateCorrelationID(req.CorrelationID); err != nil {
		return nil, err
	}
	if req.Harness != "" && req.Model != "" {
		canonical := harnesses.ResolveHarnessAlias(req.Harness)
		if !s.registry.Has(canonical) {
			return nil, fmt.Errorf("unknown harness %q", req.Harness)
		}
		cfg, _ := s.registry.Get(canonical)
		if err := validateExplicitHarnessModel(canonical, cfg, req.Model, req.Provider); err != nil {
			return nil, err
		}
	}
	s.ensurePrimaryQuotaRefresh(ctx, quotaRefreshAsync)
	cat := serviceRoutingCatalog()
	profile := req.Profile
	modelRef := req.ModelRef
	providerPreference, err := providerPreferenceForProfile(cat, profile)
	if err != nil {
		return &RouteDecision{}, err
	}
	powerPolicy := RoutePowerPolicy{
		Profile:  profile,
		MinPower: req.MinPower,
		MaxPower: req.MaxPower,
	}
	if req.Model == "" && req.Provider == "" && req.Harness == "" {
		powerPolicy, err = effectiveRoutePowerPolicy(cat, profile, req.MinPower, req.MaxPower)
		if err != nil {
			return &RouteDecision{}, err
		}
	}
	in := s.buildRoutingInputsWithCatalog(ctx, cat)

	resolvedModel, modelCandidates, modelErr := s.resolveModelConstraint(req.Harness, req.Provider, req.Model, in, cat)
	if modelErr != nil {
		result := &RouteDecision{Candidates: modelCandidates}
		s.annotateRouteDecisionEvidence(result)
		return result, publicRoutingError(modelErr, result.Candidates)
	}

	rReq := routing.Request{
		Profile:               profile,
		ModelRef:              modelRef,
		Model:                 resolvedModel,
		Provider:              req.Provider,
		Harness:               req.Harness,
		Reasoning:             effectiveReasoningString(req.Reasoning),
		Permissions:           req.Permissions,
		ProviderPreference:    providerPreference,
		EstimatedPromptTokens: req.EstimatedPromptTokens,
		RequiresTools:         req.RequiresTools,
		MinPower:              powerPolicy.MinPower,
		MaxPower:              powerPolicy.MaxPower,
	}
	s.applyRouteAttemptCooldowns(&in)
	dec, err := routing.Resolve(rReq, in)
	if err != nil {
		if escalated, edec, eerr := escalateProfileLadder(rReq, in, err); escalated {
			dec = edec
			err = eerr
		}
	}
	result := routeDecisionFromInternal(dec)
	if err != nil {
		if result == nil {
			result = &RouteDecision{}
		}
		s.annotateRouteDecisionEvidence(result)
		return result, publicRoutingError(err, result.Candidates)
	}
	s.applyStickyRouteLease(req.CorrelationID, result)
	if result != nil && result.Endpoint == "" {
		_, endpoint, _ := splitEndpointProviderRef(result.Provider)
		result.Endpoint = endpoint
	}
	s.annotateRouteDecisionEvidence(result)
	// Cache the decision so RouteStatus can surface LastDecision.
	if result != nil {
		result.RequestedProfile = req.Profile
		result.PowerPolicy = powerPolicy
		result.Model = resolveSubprocessModelAlias(result.Harness, result.Model)
		result.Power = catalogPowerForModel(cat, result.Model)
	}
	s.cacheRouteDecision(req.Model, result)
	return result, nil
}

func routeDecisionFromInternal(dec *routing.Decision) *RouteDecision {
	if dec == nil {
		return nil
	}
	return &RouteDecision{
		Harness:        dec.Harness,
		Provider:       dec.Provider,
		Endpoint:       dec.Endpoint,
		ServerInstance: dec.ServerInstance,
		Model:          dec.Model,
		Reason:         dec.Reason,
		Candidates:     routeCandidatesFromInternal(dec.Candidates),
	}
}

func routeCandidatesFromInternal(candidates []routing.Candidate) []RouteCandidate {
	if len(candidates) == 0 {
		return nil
	}
	out := make([]RouteCandidate, len(candidates))
	for i, candidate := range candidates {
		out[i] = routeCandidateFromInternal(candidate)
	}
	return out
}

func routeCandidateFromInternal(candidate routing.Candidate) RouteCandidate {
	return RouteCandidate{
		Harness:            candidate.Harness,
		Provider:           candidate.Provider,
		Endpoint:           candidate.Endpoint,
		ServerInstance:     candidate.ServerInstance,
		Model:              candidate.Model,
		Score:              candidate.Score,
		CostUSDPer1kTokens: candidate.CostUSDPer1kTokens,
		CostSource:         candidate.CostSource,
		Eligible:           candidate.Eligible,
		Reason:             candidate.Reason,
		FilterReason:       publicFilterReason(candidate),
		Components: RouteCandidateComponents{
			Power:            candidate.Power,
			Cost:             candidate.CostUSDPer1kTokens,
			CostClass:        candidate.CostClass,
			LatencyMS:        candidate.LatencyMS,
			SpeedTPS:         candidate.SpeedTPS,
			SuccessRate:      candidate.SuccessRate,
			QuotaOK:          candidate.QuotaOK,
			QuotaPercentUsed: candidate.QuotaPercentUsed,
			QuotaTrend:       candidate.QuotaTrend,
			Capability:       capabilityScoreForCostClass(candidate.CostClass),
		},
	}
}

func (s *service) annotateRouteDecisionEvidence(decision *RouteDecision) {
	if s == nil || decision == nil {
		return
	}
	decision.Utilization = s.routeUtilizationEvidence(decision.Provider, decision.ServerInstance, decision.Endpoint, decision.Model)
	for i := range decision.Candidates {
		decision.Candidates[i].Utilization = s.routeUtilizationEvidence(
			decision.Candidates[i].Provider,
			decision.Candidates[i].ServerInstance,
			decision.Candidates[i].Endpoint,
			decision.Candidates[i].Model,
		)
	}
}

func (s *service) routeUtilizationEvidence(provider, serverInstance, endpoint, model string) RouteUtilizationState {
	if s == nil || s.routeUtilization == nil {
		return RouteUtilizationState{}
	}
	keyProvider := strings.TrimSpace(provider)
	keyServerInstance := strings.TrimSpace(serverInstance)
	keyEndpoint := strings.TrimSpace(endpoint)
	if base, ep, ok := splitEndpointProviderRef(keyProvider); ok {
		keyProvider = base
		if keyEndpoint == "" {
			keyEndpoint = ep
		}
	}
	if keyServerInstance == "" {
		keyServerInstance = keyEndpoint
	}
	sample, ok := s.routeUtilization.Sample(keyProvider, keyServerInstance, model)
	if !ok && keyEndpoint != "" && keyEndpoint != keyServerInstance {
		sample, ok = s.routeUtilization.Sample(keyProvider, keyEndpoint, model)
	}
	if !ok {
		return RouteUtilizationState{}
	}
	return routeUtilizationStateFromSample(sample)
}

func routeUtilizationStateFromSample(sample utilization.EndpointUtilization) RouteUtilizationState {
	out := RouteUtilizationState{
		Source:     string(sample.Source),
		Freshness:  string(sample.Freshness),
		ObservedAt: sample.ObservedAt,
	}
	if sample.ActiveRequests != nil {
		out.ActiveRequests = utilization.Int(*sample.ActiveRequests)
	}
	if sample.QueuedRequests != nil {
		out.QueuedRequests = utilization.Int(*sample.QueuedRequests)
	}
	if sample.MaxConcurrency != nil {
		out.MaxConcurrency = utilization.Int(*sample.MaxConcurrency)
	}
	if sample.CacheUsage != nil {
		v := *sample.CacheUsage
		out.CachePressure = &v
	}
	if out.CachePressure == nil && sample.MaxConcurrency != nil && *sample.MaxConcurrency > 0 {
		total := 0
		if sample.ActiveRequests != nil {
			total += *sample.ActiveRequests
		}
		if sample.QueuedRequests != nil {
			total += *sample.QueuedRequests
		}
		pressure := float64(total) / float64(*sample.MaxConcurrency)
		out.CachePressure = &pressure
	}
	return out
}

// publicFilterReason maps the typed FilterReason emitted by the internal
// routing engine to the public FilterReason* string constant. The internal
// constants are defined to share string values with the public surface, so
// this is a one-line passthrough — there is no string parsing.
func publicFilterReason(c routing.Candidate) string {
	if c.Eligible {
		return ""
	}
	return string(c.FilterReason)
}

// capabilityScoreForCostClass maps the harness cost class to a coarse
// numeric capability proxy. Mirrors the engine's costClassRank ordering
// (more expensive ≈ more capable) for reporting purposes only.
func capabilityScoreForCostClass(class string) float64 {
	switch class {
	case "local":
		return 0
	case "cheap":
		return 1
	case "medium", "":
		return 2
	case "expensive":
		return 3
	case "experimental":
		return -1
	default:
		return 0
	}
}

// escalateProfileLadder walks routing.ProfileEscalationLadder when Resolve
// returns a "no eligible candidate" error and the request's profile is in
// the ladder. Returns (true, decision, nil) when a higher tier resolves to
// an eligible candidate, or (true, nil, *routing.ErrNoLiveProvider) when
// the entire remaining ladder is also empty. Returns (false, _, _) when
// escalation does not apply (hard pin error, profile not in ladder, etc.).
func escalateProfileLadder(req routing.Request, in routing.Inputs, origErr error) (bool, *routing.Decision, error) {
	if origErr == nil || req.Profile == "" {
		return false, nil, nil
	}
	if !shouldEscalateOnError(origErr) {
		return false, nil, nil
	}
	startIdx := -1
	for i, p := range routing.ProfileEscalationLadder {
		if p == req.Profile {
			startIdx = i
			break
		}
	}
	if startIdx < 0 {
		return false, nil, nil
	}
	for i := startIdx + 1; i < len(routing.ProfileEscalationLadder); i++ {
		probe := req
		probe.Profile = routing.ProfileEscalationLadder[i]
		dec, err := routing.Resolve(probe, in)
		if err == nil && dec != nil && dec.Harness != "" {
			return true, dec, nil
		}
	}
	return true, nil, &routing.ErrNoLiveProvider{
		PromptTokens:  req.EstimatedPromptTokens,
		RequiresTools: req.RequiresTools,
		StartingTier:  req.Profile,
	}
}

// shouldEscalateOnError gates ladder escalation to "no eligible candidate"
// errors. Hard caller-pin conflicts (ErrHarnessModelIncompatible,
// ErrProfilePinConflict) are surfaced as-is — escalating past an explicit
// pin would silently change the caller's intent.
func shouldEscalateOnError(err error) bool {
	var modelConstraintAmbiguous *ErrModelConstraintAmbiguous
	if errors.As(err, &modelConstraintAmbiguous) {
		return false
	}
	var modelConstraintNoMatch *ErrModelConstraintNoMatch
	if errors.As(err, &modelConstraintNoMatch) {
		return false
	}
	var modelErr *routing.ErrHarnessModelIncompatible
	if errors.As(err, &modelErr) {
		return false
	}
	var pinErr *routing.ErrProfilePinConflict
	if errors.As(err, &pinErr) {
		return false
	}
	return true
}

func publicRoutingError(err error, candidates []RouteCandidate) error {
	var modelErr *routing.ErrHarnessModelIncompatible
	if errors.As(err, &modelErr) {
		return withRouteCandidates(&ErrHarnessModelIncompatible{
			Harness:         modelErr.Harness,
			Model:           modelErr.Model,
			SupportedModels: append([]string(nil), modelErr.SupportedModels...),
		}, candidates)
	}
	var profileErr *routing.ErrProfilePinConflict
	if errors.As(err, &profileErr) {
		return withRouteCandidates(&ErrProfilePinConflict{
			Profile:           profileErr.Profile,
			ConflictingPin:    profileErr.ConflictingPin,
			ProfileConstraint: profileErr.ProfileConstraint,
		}, candidates)
	}
	var noProfileErr *routing.ErrNoProfileCandidate
	if errors.As(err, &noProfileErr) {
		return withRouteCandidates(&ErrNoProfileCandidate{
			Profile:           noProfileErr.Profile,
			MissingCapability: noProfileErr.MissingCapability,
			Rejected:          noProfileErr.Rejected,
		}, candidates)
	}
	var noLiveErr *routing.ErrNoLiveProvider
	if errors.As(err, &noLiveErr) {
		return withRouteCandidates(&ErrNoLiveProvider{
			PromptTokens:  noLiveErr.PromptTokens,
			RequiresTools: noLiveErr.RequiresTools,
			StartingTier:  noLiveErr.StartingTier,
		}, candidates)
	}
	var quotaErr *routing.ErrAllProvidersQuotaExhausted
	if errors.As(err, &quotaErr) {
		return withRouteCandidates(&NoViableProviderForNow{
			RetryAfter:         quotaErr.RetryAfter,
			ExhaustedProviders: append([]string(nil), quotaErr.ExhaustedProviders...),
		}, candidates)
	}
	return withRouteCandidates(err, candidates)
}

func withRouteCandidates(err error, candidates []RouteCandidate) error {
	if err == nil || len(candidates) == 0 {
		return err
	}
	return &routeDecisionError{
		err:        err,
		candidates: append([]RouteCandidate(nil), candidates...),
	}
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
		if record.Key.Provider != "" {
			existing, ok := in.ProviderCooldowns[record.Key.Provider]
			if !ok || record.RecordedAt.After(existing) {
				in.ProviderCooldowns[record.Key.Provider] = record.RecordedAt
			}
		}
		if record.Key.Provider == "" && record.Key.Harness != "" {
			for i := range in.Harnesses {
				if in.Harnesses[i].Name == record.Key.Harness {
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
	return s.buildRoutingInputsWithCatalog(ctx, serviceRoutingCatalog())
}

func (s *service) buildRoutingInputsWithCatalog(ctx context.Context, cat *modelcatalog.Catalog) routing.Inputs {
	statuses := s.registry.Discover()
	statusByName := make(map[string]harnesses.HarnessStatus, len(statuses))
	for _, st := range statuses {
		statusByName[st.Name] = st
	}
	now := time.Now().UTC()

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
			SupportedModels:     subprocessHarnessModelIDs(name, cfg),
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
		if name == "fiz" && s.opts.ServiceConfig == nil {
			entry.AutoRoutingEligible = false
		}

		if qs, ok := subscriptionQuotaForHarness(name, time.Now()); ok {
			entry.QuotaOK = qs.OK
			entry.QuotaStale = qs.Present && !qs.Fresh
			entry.SubscriptionOK = qs.OK
			entry.QuotaPercentUsed = qs.PercentUsed
			entry.QuotaTrend = qs.Trend
			entry.QuotaReason = qs.Reason
		}

		// Native "fiz" harness: enumerate live configured provider endpoints.
		if name == "fiz" && s.opts.ServiceConfig != nil {
			for _, pname := range s.opts.ServiceConfig.ProviderNames() {
				pcfg, ok := s.opts.ServiceConfig.Provider(pname)
				if !ok {
					continue
				}
				entry.Providers = append(entry.Providers, s.liveProviderEntries(ctx, pname, pcfg, cat)...)
			}
			// Tool support for the agent harness is per-(provider, model);
			// the harness-level baseline is whether ANY provider supports
			// tools. Engine OR-combines harness and provider SupportsTools
			// so this lets a per-model no_tools catalog flag actually fire
			// the RequiresTools gate when every provider's resolved model
			// is no-tools.
			if len(entry.Providers) > 0 {
				entry.SupportsTools = anyProviderSupportsTools(entry.Providers)
			} else {
				entry.Available = false
			}
		}
		s.applySubscriptionRoutingCost(&entry, cat)
		entries = append(entries, entry)
	}
	successRate, latencyMS := s.routeMetricSignals(now, s.routeAttemptTTL())
	return routing.Inputs{
		Harnesses:                   entries,
		ProviderSuccessRate:         successRate,
		ObservedLatencyMS:           latencyMS,
		ProviderQuotaExhaustedUntil: s.providerQuotaExhaustedUntil(now),
		CatalogResolver:             serviceRoutingCatalogResolver(cat),
		CatalogCandidatesResolver:   serviceRoutingCatalogCandidatesResolver(cat),
		ModelEligibility:            serviceRoutingModelEligibility(cat),
		ReasoningResolver:           serviceRoutingReasoningResolver(cat),
		EndpointLoadResolver:        s.routeEndpointLoadsResolver(now),
	}
}

// providerQuotaExhaustedUntil snapshots the per-provider quota state machine
// at the given instant for the routing engine. Returns nil when no provider
// is currently in quota_exhausted state, which keeps the routing path
// allocation-free in the common case.
func (s *service) providerQuotaExhaustedUntil(now time.Time) map[string]time.Time {
	if s == nil || s.providerQuota == nil {
		return nil
	}
	return s.providerQuota.ExhaustedAt(now)
}

// startQuotaRecoveryProbeLoop spawns the goroutine that periodically probes
// quota_exhausted providers and either restores them to available or extends
// their retry_after with bounded backoff. The goroutine is tied to
// QuotaRefreshContext (or context.Background()) so server callers can cancel
// it on shutdown.
func (s *service) startQuotaRecoveryProbeLoop() {
	if s == nil || s.providerQuota == nil {
		return
	}
	ctx := s.opts.QuotaRefreshContext
	if ctx == nil {
		ctx = context.Background()
	}
	probe := s.quotaRecoveryProber()
	if probe == nil {
		return
	}
	go runQuotaRecoveryProbeLoop(ctx, s.providerQuota, probe, defaultQuotaRecoveryFallbackInterval, nil, nil)
}

// quotaRecoveryProber returns the QuotaRecoveryProber used by the recovery
// loop. It looks up the provider entry in ServiceConfig and reuses the same
// probeProviderStatus the HealthCheck endpoint uses; a "connected" status
// counts as recovery, anything else is reported as a probe failure so the
// retry_after gets extended with backoff.
func (s *service) quotaRecoveryProber() QuotaRecoveryProber {
	sc := s.opts.ServiceConfig
	if sc == nil {
		return nil
	}
	return func(ctx context.Context, name string) error {
		entry, ok := sc.Provider(name)
		if !ok {
			return fmt.Errorf("provider %q not found", name)
		}
		probeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		probe := probeProviderStatus(probeCtx, entry, time.Now().UTC())
		if probe.status == "connected" {
			return nil
		}
		if probe.detail != "" {
			return fmt.Errorf("%s", probe.detail)
		}
		return fmt.Errorf("%s", probe.status)
	}
}

// ProviderQuotaState returns the per-provider quota state machine for this
// service. Callers (notably the quota-signal ingest path defined in sibling
// beads) drive transitions via MarkQuotaExhausted / MarkAvailable.
func (s *service) ProviderQuotaState() *ProviderQuotaStateStore {
	if s == nil {
		return nil
	}
	return s.providerQuota
}

func serviceRoutingCatalog() *modelcatalog.Catalog {
	cat, err := loadRoutingCatalog()
	if err != nil || cat == nil {
		return nil
	}
	return cat
}

func serviceRoutingCatalogResolver(cat *modelcatalog.Catalog) func(ref, surface string) (string, bool) {
	if cat == nil {
		return nil
	}
	return func(ref, surface string) (string, bool) {
		catalogSurface, ok := serviceRoutingCatalogSurface(surface)
		if !ok {
			return "", false
		}
		resolved, err := cat.Resolve(ref, modelcatalog.ResolveOptions{
			Surface:         catalogSurface,
			AllowDeprecated: true,
		})
		if err != nil || resolved.ConcreteModel == "" {
			return "", false
		}
		return resolved.ConcreteModel, true
	}
}

func serviceRoutingCatalogCandidatesResolver(cat *modelcatalog.Catalog) func(ref, surface string) ([]string, bool) {
	if cat == nil {
		return nil
	}
	return func(ref, surface string) ([]string, bool) {
		catalogSurface, ok := serviceRoutingCatalogSurface(surface)
		if !ok {
			return nil, false
		}
		resolved, err := cat.Resolve(ref, modelcatalog.ResolveOptions{
			Surface:         catalogSurface,
			AllowDeprecated: true,
		})
		if err != nil || resolved.CanonicalID == "" {
			return nil, false
		}
		candidates := cat.CandidatesFor(catalogSurface, resolved.CanonicalID)
		if len(candidates) == 0 {
			if resolved.ConcreteModel == "" {
				return nil, false
			}
			return []string{resolved.ConcreteModel}, true
		}
		return candidates, true
	}
}

func serviceRoutingModelEligibility(cat *modelcatalog.Catalog) func(model string) (routing.ModelEligibility, bool) {
	if cat == nil {
		return nil
	}
	return func(model string) (routing.ModelEligibility, bool) {
		eligibility, ok := cat.ModelEligibility(model)
		if !ok {
			return routing.ModelEligibility{}, false
		}
		return routing.ModelEligibility{
			Power:        eligibility.Power,
			ExactPinOnly: eligibility.ExactPinOnly,
			AutoRoutable: eligibility.AutoRoutable,
		}, true
	}
}

// serviceRoutingReasoningResolver returns the catalog's surface_policy
// reasoning_default for a (profile, surface) pair. Used by the routing engine
// to resolve Reasoning=auto to a concrete level before the capability gate.
func serviceRoutingReasoningResolver(cat *modelcatalog.Catalog) func(profile, surface string) (string, bool) {
	if cat == nil {
		return nil
	}
	return func(profile, surface string) (string, bool) {
		if profile == "" {
			return "", false
		}
		catalogSurface, ok := serviceRoutingCatalogSurface(surface)
		if !ok {
			return "", false
		}
		resolved, err := cat.Resolve(profile, modelcatalog.ResolveOptions{
			Surface:         catalogSurface,
			AllowDeprecated: true,
		})
		if err != nil {
			return "", false
		}
		def := string(resolved.SurfacePolicy.ReasoningDefault)
		if def == "" {
			return "", false
		}
		return def, true
	}
}

func serviceRoutingCatalogSurface(surface string) (modelcatalog.Surface, bool) {
	switch surface {
	case "embedded-openai":
		return modelcatalog.SurfaceAgentOpenAI, true
	case "embedded-anthropic":
		return modelcatalog.SurfaceAgentAnthropic, true
	case "codex":
		return modelcatalog.SurfaceCodex, true
	case "claude":
		return modelcatalog.SurfaceClaudeCode, true
	case "gemini":
		return modelcatalog.SurfaceGemini, true
	default:
		return "", false
	}
}

func (s *service) liveProviderEntries(ctx context.Context, providerName string, pcfg ServiceProviderEntry, cat *modelcatalog.Catalog) []routing.ProviderEntry {
	if pcfg.ConfigError != "" {
		return nil
	}
	if providerUsesLiveDiscovery(pcfg.Type) && s.catalog != nil {
		endpoints := modelDiscoveryEndpoints(pcfg)
		out := make([]routing.ProviderEntry, 0, len(endpoints))
		for _, endpoint := range endpoints {
			ids, ok := s.probeEndpointDiscoveredIDs(ctx, pcfg, endpoint.BaseURL)
			if !ok || len(ids) == 0 {
				continue
			}
			routeName := providerName
			if len(endpoints) > 1 {
				routeName = endpointProviderRef(providerName, endpoint.Name)
			}
			entry := routing.ProviderEntry{
				Name:               routeName,
				BaseURL:            endpoint.BaseURL,
				ServerInstance:     endpoint.ServerInstance,
				EndpointName:       endpoint.Name,
				EndpointBaseURL:    endpoint.BaseURL,
				DefaultModel:       pcfg.Model,
				CostClass:          providerRoutingCostClass(pcfg.Type),
				DiscoveredIDs:      ids,
				DiscoveryAttempted: true,
				ContextWindows:     buildProviderContextWindows(cat, pcfg.Model, ids),
				SupportsTools:      providerSupportsTools(cat, pcfg.Model, ids),
			}
			s.applyEndpointRoutingCost(&entry, pcfg, cat)
			out = append(out, entry)
		}
		return out
	}
	entry := routing.ProviderEntry{
		Name:           providerName,
		BaseURL:        pcfg.BaseURL,
		ServerInstance: pcfg.ServerInstance,
		DefaultModel:   pcfg.Model,
		CostClass:      providerRoutingCostClass(pcfg.Type),
		ContextWindows: buildProviderContextWindows(cat, pcfg.Model, nil),
		SupportsTools:  providerSupportsTools(cat, pcfg.Model, nil),
	}
	s.applyEndpointRoutingCost(&entry, pcfg, cat)
	return []routing.ProviderEntry{entry}
}

// buildProviderContextWindows assembles the ContextWindows map for a
// ProviderEntry from the model catalog. Entries are added for the provider's
// configured DefaultModel and every DiscoveredID that has a non-zero
// context_window declared in the catalog. Models the catalog does not know
// about are omitted (engine treats missing entries as unknown context).
func buildProviderContextWindows(cat *modelcatalog.Catalog, defaultModel string, discoveredIDs []string) map[string]int {
	if cat == nil {
		return nil
	}
	out := make(map[string]int)
	if defaultModel != "" {
		if n := cat.ContextWindowForModel(defaultModel); n > 0 {
			out[defaultModel] = n
		}
	}
	for _, id := range discoveredIDs {
		if id == "" {
			continue
		}
		if _, exists := out[id]; exists {
			continue
		}
		if n := cat.ContextWindowForModel(id); n > 0 {
			out[id] = n
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// providerSupportsTools returns whether the provider should be advertised as
// supporting tools to the routing engine. Defaults to true; only flips to
// false when the catalog explicitly marks every relevant model (the
// DefaultModel and any DiscoveredIDs) with no_tools=true.
func providerSupportsTools(cat *modelcatalog.Catalog, defaultModel string, discoveredIDs []string) bool {
	if cat == nil {
		return true
	}
	checked := false
	if defaultModel != "" {
		if cat.SupportsToolsForModel(defaultModel) {
			return true
		}
		checked = true
	}
	for _, id := range discoveredIDs {
		if id == "" {
			continue
		}
		if cat.SupportsToolsForModel(id) {
			return true
		}
		checked = true
	}
	if !checked {
		return true
	}
	return false
}

func anyProviderSupportsTools(providers []routing.ProviderEntry) bool {
	for _, p := range providers {
		if p.SupportsTools {
			return true
		}
	}
	return false
}

func providerUsesLiveDiscovery(providerType string) bool {
	switch normalizeServiceProviderType(providerType) {
	case "openai", "openrouter", "lmstudio", "llama-server", "omlx", "rapid-mlx", "ollama", "lucebox", "vllm", "minimax", "qwen", "zai":
		return true
	default:
		return false
	}
}

func (s *service) applyEndpointRoutingCost(entry *routing.ProviderEntry, pcfg ServiceProviderEntry, cat *modelcatalog.Catalog) {
	if entry == nil {
		return
	}
	if providerTypeIsLocalEndpoint(pcfg.Type) {
		if s.opts.LocalCostUSDPer1kTokens > 0 {
			entry.CostUSDPer1kTokens = s.opts.LocalCostUSDPer1kTokens
			entry.CostSource = routing.CostSourceUserConfig
		} else {
			entry.CostUSDPer1kTokens = 0
			entry.CostSource = routing.CostSourceUnknown
		}
		return
	}
	if cost, ok := catalogCostUSDPer1kTokens(cat, entry.DefaultModel); ok {
		entry.CostUSDPer1kTokens = cost
		entry.CostSource = routing.CostSourceCatalog
		return
	}
	entry.CostUSDPer1kTokens = 0
	entry.CostSource = routing.CostSourceUnknown
}

func (s *service) applySubscriptionRoutingCost(entry *routing.HarnessEntry, cat *modelcatalog.Catalog) {
	if entry == nil || !entry.IsSubscription {
		return
	}
	baseCost, ok := catalogCostUSDPer1kTokens(cat, entry.DefaultModel)
	if !ok {
		baseCost, ok = catalogCostUSDPer1kTokens(cat, subscriptionFallbackProfile(entry.Name))
		if !ok {
			baseCost = 0
		}
	}
	cost := subscriptionEffectiveCostUSDPer1kTokens(baseCost, entry.QuotaPercentUsed, s.subscriptionCostCurve())
	entry.Providers = []routing.ProviderEntry{{
		CostUSDPer1kTokens: cost,
		CostSource:         routing.CostSourceSubscription,
	}}
}

func providerTypeIsLocalEndpoint(providerType string) bool {
	switch normalizeServiceProviderType(providerType) {
	case "lmstudio", "llama-server", "omlx", "rapid-mlx", "ollama", "lucebox", "vllm":
		return true
	default:
		return false
	}
}

func providerRoutingCostClass(providerType string) string {
	if providerTypeIsLocalEndpoint(providerType) {
		return "local"
	}
	return "medium"
}

func subscriptionFallbackProfile(harnessName string) string {
	switch harnessName {
	case "claude", "codex", "gemini":
		return "standard"
	default:
		return ""
	}
}

func catalogCostUSDPer1kTokens(cat *modelcatalog.Catalog, modelID string) (float64, bool) {
	if cat == nil || strings.TrimSpace(modelID) == "" {
		return 0, false
	}
	entry, ok := cat.LookupModel(modelID)
	if !ok {
		resolved := resolveCatalogCostModel(cat, modelID)
		if resolved == "" {
			return 0, false
		}
		entry, ok = cat.LookupModel(resolved)
		if !ok {
			return 0, false
		}
	}
	input := entry.CostInputPerM
	if input == 0 {
		input = entry.CostInputPerMTok
	}
	output := entry.CostOutputPerM
	if output == 0 {
		output = entry.CostOutputPerMTok
	}
	switch {
	case input > 0 && output > 0:
		return ((input + output) / 2) / 1000, true
	case input > 0:
		return input / 1000, true
	case output > 0:
		return output / 1000, true
	default:
		return 0, false
	}
}

func resolveCatalogCostModel(cat *modelcatalog.Catalog, ref string) string {
	for _, surface := range []modelcatalog.Surface{
		modelcatalog.SurfaceAgentOpenAI,
		modelcatalog.SurfaceAgentAnthropic,
		modelcatalog.SurfaceCodex,
		modelcatalog.SurfaceClaudeCode,
		modelcatalog.SurfaceGemini,
	} {
		resolved, err := cat.Resolve(ref, modelcatalog.ResolveOptions{
			Surface:         surface,
			AllowDeprecated: true,
		})
		if err == nil && resolved.ConcreteModel != "" {
			return resolved.ConcreteModel
		}
	}
	return ""
}

func (s *service) subscriptionCostCurve() SubscriptionCostCurve {
	if s.opts.SubscriptionCostCurve == nil {
		return defaultSubscriptionCostCurve()
	}
	curve := *s.opts.SubscriptionCostCurve
	def := defaultSubscriptionCostCurve()
	if curve.FreeUntilPercent == 0 {
		curve.FreeUntilPercent = def.FreeUntilPercent
	}
	if curve.LowUntilPercent == 0 {
		curve.LowUntilPercent = def.LowUntilPercent
	}
	if curve.MediumUntilPercent == 0 {
		curve.MediumUntilPercent = def.MediumUntilPercent
	}
	if curve.LowMultiplier == 0 {
		curve.LowMultiplier = def.LowMultiplier
	}
	if curve.MediumMultiplier == 0 {
		curve.MediumMultiplier = def.MediumMultiplier
	}
	if curve.HighMultiplier == 0 {
		curve.HighMultiplier = def.HighMultiplier
	}
	return curve
}

func defaultSubscriptionCostCurve() SubscriptionCostCurve {
	return SubscriptionCostCurve{
		FreeUntilPercent:   70,
		LowUntilPercent:    80,
		MediumUntilPercent: 90,
		LowMultiplier:      0.1,
		MediumMultiplier:   0.3,
		HighMultiplier:     1.2,
	}
}

func subscriptionEffectiveCostUSDPer1kTokens(baseCost float64, quotaPercentUsed int, curve SubscriptionCostCurve) float64 {
	switch {
	case quotaPercentUsed <= curve.FreeUntilPercent:
		return 0
	case quotaPercentUsed <= curve.LowUntilPercent:
		return baseCost * curve.LowMultiplier
	case quotaPercentUsed <= curve.MediumUntilPercent:
		return baseCost * curve.MediumMultiplier
	default:
		return baseCost * curve.HighMultiplier
	}
}

var probeOpenAIModelsForDiscovery = probeOpenAIModels

// probeEndpointDiscoveredIDs returns the foreground /v1/models catalog for one
// endpoint. Dispatch routing must not use stale cache hits: a recently dead
// endpoint has to be skipped before any chat request is attempted.
func (s *service) probeEndpointDiscoveredIDs(ctx context.Context, pcfg ServiceProviderEntry, baseURL string) ([]string, bool) {
	if baseURL == "" {
		return nil, false
	}
	probeCtx, cancel := context.WithTimeout(ctx, s.opts.catalogProbeTimeout())
	defer cancel()
	ids, err := probeOpenAIModelsForDiscovery(probeCtx, baseURL, pcfg.APIKey)
	if err != nil || len(ids) == 0 {
		return nil, false
	}
	return ids, true
}

// resolveExecuteRouteWithEngine is the post-engine variant of resolveExecuteRoute.
// It is invoked by Execute when the request is under-specified
// (no PreResolved, no fully-specified Harness). Returns nil when the request
// is already specific enough that the legacy resolveExecuteRoute path applies.
func (s *service) resolveExecuteRouteWithEngine(req ServiceExecuteRequest) (*RouteDecision, error) {
	rr := RouteRequest{
		Profile:       req.Profile,
		Model:         req.Model,
		Provider:      req.Provider,
		Harness:       req.Harness,
		ModelRef:      req.ModelRef,
		Reasoning:     req.Reasoning,
		Permissions:   req.Permissions,
		CachePolicy:   req.CachePolicy,
		MinPower:      req.MinPower,
		MaxPower:      req.MaxPower,
		Role:          req.Role,
		CorrelationID: req.CorrelationID,
	}
	dec, err := s.ResolveRoute(context.Background(), rr)
	if err != nil {
		if isExplicitPinError(err) {
			return nil, err
		}
		return nil, fmt.Errorf("ResolveRoute: %w", err)
	}
	return dec, nil
}

func providerPreferenceForProfile(cat *modelcatalog.Catalog, profile string) (string, error) {
	if profile == "" {
		return routing.ProviderPreferenceLocalFirst, nil
	}
	if cat == nil {
		return "", &ErrUnknownProfile{Profile: profile}
	}
	info, ok := cat.Profile(profile)
	if !ok {
		return "", &ErrUnknownProfile{Profile: profile}
	}
	switch info.ProviderPreference {
	case routing.ProviderPreferenceLocalOnly, routing.ProviderPreferenceSubscriptionOnly,
		routing.ProviderPreferenceLocalFirst, routing.ProviderPreferenceSubscriptionFirst:
		return info.ProviderPreference, nil
	default:
		return "", fmt.Errorf("profile %q has unsupported provider preference %q", profile, info.ProviderPreference)
	}
}

func effectiveRoutePowerPolicy(cat *modelcatalog.Catalog, profile string, minPower, maxPower int) (RoutePowerPolicy, error) {
	policy := RoutePowerPolicy{
		Profile:  profile,
		MinPower: minPower,
		MaxPower: maxPower,
	}
	if profile == "" {
		return policy, nil
	}
	if cat == nil {
		return policy, &ErrUnknownProfile{Profile: profile}
	}
	info, ok := cat.Profile(profile)
	if !ok {
		return policy, &ErrUnknownProfile{Profile: profile}
	}
	policy.MinPower = info.MinPower
	policy.MaxPower = info.MaxPower
	if minPower > policy.MinPower {
		policy.MinPower = minPower
	}
	if maxPower > 0 && (policy.MaxPower == 0 || maxPower < policy.MaxPower) {
		policy.MaxPower = maxPower
	}
	return policy, nil
}
