package fizeau

// This file implements ListProviders and HealthCheck for the FizeauService service.
// It lives in the root package to avoid import cycles; provider config data is
// injected via the ServiceConfig interface defined in service.go.
//
// Note: We cannot import agent/config or provider/openai here because both
// packages import the root agent package, creating a cycle. Provider probing
// is done inline using net/http.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/easel/fizeau/internal/harnesses"
	"github.com/easel/fizeau/internal/serverinstance"
)

const (
	defaultQuotaRefreshDebounce     = 15 * time.Minute
	defaultQuotaRefreshStartupWait  = 2 * time.Second
	defaultQuotaRefreshProbeTimeout = 30 * time.Second
)

// Claude PTY-probe injection used to live here as a package-level
// healthCheckClaudeQuotaRefresher var. Under CONTRACT-004 the Runner
// owns the probe seam (claude.SetCaptureForTest) and service_providers
// drives refreshes through QuotaHarness.RefreshQuota — see
// requestPrimaryQuotaRefresh and HealthCheck below.

var healthCheckQuotaProbeMu sync.RWMutex

var primaryQuotaRefresh = &quotaRefreshCoordinator{
	lastAttempt: make(map[string]time.Time),
	inFlight:    make(map[string]bool),
}

type quotaRefreshCoordinator struct {
	mu          sync.Mutex
	lastAttempt map[string]time.Time
	inFlight    map[string]bool
}

type quotaRefreshMode int

const (
	quotaRefreshAsync quotaRefreshMode = iota
	quotaRefreshStartup
)

type quotaRefreshPolicy struct {
	debounce     time.Duration
	startupWait  time.Duration
	probeTimeout time.Duration
}

type quotaCacheStatus struct {
	needsRefresh bool
	usable       bool
}

type providerProbeResult struct {
	status           string
	modelCount       int
	caps             []string
	detail           string
	endpointName     string
	baseURL          string
	serverInstance   string
	endpointStatuses []EndpointStatus
}

func (s *service) ensurePrimaryQuotaRefresh(ctx context.Context, mode quotaRefreshMode) {
	policy := s.quotaRefreshPolicy()
	var waits []<-chan struct{}
	for _, name := range []string{"claude", "codex"} {
		status := s.primaryQuotaCacheStatus(ctx, name, policy.debounce)
		if !status.needsRefresh {
			continue
		}
		done := s.requestPrimaryQuotaRefresh(ctx, name, policy)
		if mode == quotaRefreshStartup && !status.usable && done != nil {
			waits = append(waits, done)
		}
	}
	if mode == quotaRefreshStartup && len(waits) > 0 && policy.startupWait > 0 {
		waitForPrimaryQuotaRefreshes(waits, policy.startupWait)
	}
}

func (s *service) quotaRefreshPolicy() quotaRefreshPolicy {
	policy := quotaRefreshPolicy{
		debounce:     defaultQuotaRefreshDebounce,
		startupWait:  defaultQuotaRefreshStartupWait,
		probeTimeout: defaultQuotaRefreshProbeTimeout,
	}
	if s != nil {
		if s.opts.QuotaRefreshDebounce > 0 {
			policy.debounce = s.opts.QuotaRefreshDebounce
		}
		if s.opts.QuotaRefreshStartupWait > 0 {
			policy.startupWait = s.opts.QuotaRefreshStartupWait
		}
	}
	return policy
}

func (s *service) startPrimaryQuotaRefreshWorker() {
	if s == nil || s.opts.QuotaRefreshInterval <= 0 {
		return
	}
	ctx := s.opts.QuotaRefreshContext
	if ctx == nil {
		ctx = context.Background()
	}
	interval := s.opts.QuotaRefreshInterval
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.ensurePrimaryQuotaRefresh(ctx, quotaRefreshAsync)
			}
		}
	}()
}

func (s *service) requestPrimaryQuotaRefresh(ctx context.Context, harnessName string, policy quotaRefreshPolicy) <-chan struct{} {
	return requestPrimaryQuotaRefresh(ctx, harnessName, policy, s.harnessByName)
}

func requestPrimaryQuotaRefresh(ctx context.Context, harnessName string, policy quotaRefreshPolicy, harnessByName func(string) harnesses.Harness) <-chan struct{} {
	done := make(chan struct{})
	if ctx == nil {
		ctx = context.Background()
	}
	if ctx.Err() != nil {
		close(done)
		return nil
	}

	now := time.Now()
	primaryQuotaRefresh.mu.Lock()
	if primaryQuotaRefresh.inFlight[harnessName] {
		primaryQuotaRefresh.mu.Unlock()
		close(done)
		return nil
	}
	if last := primaryQuotaRefresh.lastAttempt[harnessName]; !last.IsZero() && now.Sub(last) < policy.debounce {
		primaryQuotaRefresh.mu.Unlock()
		close(done)
		return nil
	}
	primaryQuotaRefresh.lastAttempt[harnessName] = now
	primaryQuotaRefresh.inFlight[harnessName] = true
	primaryQuotaRefresh.mu.Unlock()

	go func() {
		defer close(done)
		defer func() {
			primaryQuotaRefresh.mu.Lock()
			primaryQuotaRefresh.inFlight[harnessName] = false
			primaryQuotaRefresh.mu.Unlock()
		}()

		switch harnessName {
		case "claude":
			if qh, ok := harnessByName("claude").(harnesses.QuotaHarness); ok {
				probeCtx, probeCancel := context.WithTimeout(ctx, policy.probeTimeout)
				_, _ = qh.RefreshQuota(probeCtx)
				probeCancel()
			}
		case "codex":
			if qh, ok := harnessByName("codex").(harnesses.QuotaHarness); ok {
				_, _ = qh.RefreshQuota(ctx)
			}
		}
	}()
	return done
}

func waitForPrimaryQuotaRefreshes(waits []<-chan struct{}, timeout time.Duration) {
	deadline := time.After(timeout)
	for _, done := range waits {
		select {
		case <-done:
		case <-deadline:
			return
		}
	}
}

func (s *service) primaryQuotaCacheStatus(ctx context.Context, harnessName string, debounce time.Duration) quotaCacheStatus {
	now := time.Now()
	switch harnessName {
	case "claude":
		qh, ok := s.harnessByName("claude").(harnesses.QuotaHarness)
		if !ok {
			return quotaCacheStatus{}
		}
		status, err := qh.QuotaStatus(ctx, now)
		if err != nil {
			return quotaCacheStatus{}
		}
		if status.State == harnesses.QuotaUnavailable {
			return quotaCacheStatus{needsRefresh: true}
		}
		stale := !status.CapturedAt.IsZero() && now.Sub(status.CapturedAt) >= debounce
		return quotaCacheStatus{
			needsRefresh: stale,
			usable:       !stale,
		}
	case "codex":
		qh, ok := s.harnessByName("codex").(harnesses.QuotaHarness)
		if !ok {
			return quotaCacheStatus{}
		}
		status, err := qh.QuotaStatus(ctx, now)
		if err != nil {
			return quotaCacheStatus{}
		}
		if status.State == harnesses.QuotaUnavailable {
			return quotaCacheStatus{needsRefresh: true}
		}
		usable := status.Fresh && status.RoutingPreference == harnesses.RoutingPreferenceAvailable
		return quotaCacheStatus{
			needsRefresh: !usable,
			usable:       usable,
		}
	default:
		return quotaCacheStatus{}
	}
}

// ListProviders returns providers known to the native fiz harness with live
// status, configured-default markers, and cooldown state.
func (s *service) ListProviders(ctx context.Context) ([]ProviderInfo, error) {
	sc := s.opts.ServiceConfig
	if sc == nil {
		return nil, fmt.Errorf("service: no ServiceConfig provided; pass ServiceOptions.ServiceConfig")
	}

	names := sc.ProviderNames()
	defaultName := sc.DefaultProviderName()
	cooldown := sc.HealthCooldown()
	if cooldown <= 0 {
		cooldown = 30 * time.Second
	}

	type indexedInfo struct {
		idx  int
		info ProviderInfo
	}
	results := make([]indexedInfo, len(names))
	var wg sync.WaitGroup

	for i, name := range names {
		wg.Add(1)
		go func(idx int, name string) {
			defer wg.Done()

			entry, ok := sc.Provider(name)
			if !ok {
				results[idx] = indexedInfo{idx: idx, info: ProviderInfo{
					Name:   name,
					Status: "error: provider not found in config",
				}}
				return
			}

			info := ProviderInfo{
				Name:             name,
				Type:             normalizeServiceProviderType(entry.Type),
				BaseURL:          entry.BaseURL,
				Endpoints:        append([]ServiceProviderEndpoint(nil), entry.Endpoints...),
				Billing:          serviceProviderBilling(entry),
				IncludeByDefault: serviceProviderDefaultInclusion(entry),
				IsDefault:        name == defaultName,
				DefaultModel:     entry.Model,
			}

			if entry.ConfigError != "" {
				capturedAt := time.Now().UTC()
				info.Status = "error: invalid provider config"
				info.EndpointStatus = providerEndpointStatusesFromProbe(entry, providerProbeResult{
					status: "error: invalid provider config",
					detail: entry.ConfigError,
				}, capturedAt)
				info.Auth = providerAuthStatus(entry, info.Status, capturedAt)
				info.Quota = providerQuotaState(entry, capturedAt)
				info.CooldownState = serviceProviderCooldown(sc, name, cooldown)
				info.LastError = statusErrorDetail(info.Status, entry.ConfigError, "service provider config", capturedAt)
				results[idx] = indexedInfo{idx: idx, info: info}
				return
			}

			probeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			capturedAt := time.Now().UTC()
			probe := probeProviderStatus(probeCtx, entry, capturedAt)
			info.Status = probe.status
			info.ModelCount = probe.modelCount
			info.Capabilities = probe.caps
			info.CooldownState = serviceProviderCooldown(sc, name, cooldown)
			info.Auth = providerAuthStatus(entry, info.Status, capturedAt)
			info.EndpointStatus = probe.endpointStatuses
			info.Quota = providerQuotaState(entry, capturedAt)
			lastErrorSource := "service provider config"
			if len(info.EndpointStatus) > 0 {
				lastErrorSource = info.EndpointStatus[0].Source
			}
			info.LastError = statusErrorDetail(info.Status, probe.detail, lastErrorSource, capturedAt)

			results[idx] = indexedInfo{idx: idx, info: info}
		}(i, name)
	}
	wg.Wait()

	out := make([]ProviderInfo, len(names))
	for _, r := range results {
		out[r.idx] = r.info
	}
	return out, nil
}

// HealthCheck triggers a fresh probe for the named health-check subject and updates internal state.
// health.Type is "harness" or "provider".
func (s *service) HealthCheck(ctx context.Context, health HealthTarget) error {
	switch health.Type {
	case "provider":
		sc := s.opts.ServiceConfig
		if sc == nil {
			return fmt.Errorf("service: no ServiceConfig provided; pass ServiceOptions.ServiceConfig")
		}
		entry, ok := sc.Provider(health.Name)
		if !ok {
			return fmt.Errorf("service: provider %q not found", health.Name)
		}
		probeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		probe := probeProviderStatus(probeCtx, entry, time.Now().UTC())
		if probe.status == "connected" {
			return nil
		}
		msg := probe.status
		if probe.detail != "" {
			msg = probe.detail
		}
		return fmt.Errorf("service: provider %q: %s", health.Name, msg)

	case "harness":
		statuses := s.registry.Discover()
		for _, st := range statuses {
			if st.Name != health.Name {
				continue
			}
			if !st.Available {
				return fmt.Errorf("service: harness %q unavailable: %s", health.Name, st.Error)
			}
			// For subscription harnesses, refresh the quota cache when stale.
			if health.Name == "claude" {
				s.healthCheckRefreshClaudeQuota(ctx)
			}
			if health.Name == "codex" {
				if qh, ok := s.harnessByName("codex").(harnesses.QuotaHarness); ok {
					_, _ = qh.RefreshQuota(ctx)
				}
			}
			return nil
		}
		return fmt.Errorf("service: harness %q not registered", health.Name)

	default:
		return fmt.Errorf("service: unknown HealthTarget.Type %q (want \"harness\" or \"provider\")", health.Type)
	}
}

// probeServiceProvider pings a provider and returns (status, modelCount, capabilities).
func probeServiceProvider(ctx context.Context, entry ServiceProviderEntry) (status string, modelCount int, caps []string) {
	probe := probeServiceProviderDetailed(ctx, entry)
	return probe.status, probe.modelCount, probe.caps
}

func probeServiceProviderDetailed(ctx context.Context, entry ServiceProviderEntry) providerProbeResult {
	switch entry.Type {
	case "anthropic":
		if entry.APIKey == "" {
			return providerProbeResult{status: "error: api_key not configured", detail: "api_key not configured"}
		}
		// Anthropic does not expose an unauthenticated /v1/models list endpoint.
		// Treat key presence as the connectivity signal.
		return providerProbeResult{status: "connected", caps: providerCapabilities(entry)}

	case "openai", "openrouter", "lmstudio", "llama-server", "ds4", "omlx", "rapid-mlx", "vllm", "ollama", "minimax", "qwen", "zai", "":
		if entry.BaseURL == "" {
			return providerProbeResult{status: "error: base_url not configured", detail: "base_url not configured"}
		}
		n, err := discoverOpenAIModels(ctx, entry.BaseURL, entry.APIKey)
		if err != nil {
			msg := err.Error()
			if serviceIsUnreachable(msg) {
				return providerProbeResult{status: "unreachable", detail: serviceTrimError(msg)}
			}
			detail := serviceTrimError(msg)
			return providerProbeResult{status: "error: " + detail, detail: detail}
		}
		return providerProbeResult{status: "connected", modelCount: n, caps: providerCapabilities(entry)}

	default:
		detail := "unknown provider type " + entry.Type
		return providerProbeResult{status: "error: " + detail, detail: detail}
	}
}

func probeProviderStatus(ctx context.Context, entry ServiceProviderEntry, capturedAt time.Time) providerProbeResult {
	endpoints := modelDiscoveryEndpoints(entry)
	if len(endpoints) == 0 {
		probe := probeServiceProviderDetailed(ctx, entry)
		probe.endpointStatuses = providerEndpointStatusesFromProbe(entry, probe, capturedAt)
		return probe
	}

	statuses := make([]EndpointStatus, 0, len(endpoints))
	var aggregate providerProbeResult
	aggregate.status = "error: endpoint probe did not run"
	for _, endpoint := range endpoints {
		endpointEntry := entry
		endpointEntry.BaseURL = endpoint.BaseURL
		endpointProbe := probeServiceProviderDetailed(ctx, endpointEntry)
		endpointProbe.endpointName = endpoint.Name
		endpointProbe.baseURL = endpoint.BaseURL
		statuses = append(statuses, endpointStatusFromProbe(endpoint.Name, endpoint.BaseURL, endpointProbe, capturedAt))
		if endpointProbe.status == "connected" {
			if aggregate.status != "connected" {
				aggregate.status = "connected"
				aggregate.caps = append([]string(nil), endpointProbe.caps...)
				aggregate.detail = ""
			}
			aggregate.modelCount += endpointProbe.modelCount
			continue
		}
		if aggregate.status == "connected" {
			continue
		}
		if shouldPreferProviderProbe(endpointProbe, aggregate) {
			aggregate.status = endpointProbe.status
			aggregate.detail = endpointProbe.detail
			aggregate.caps = append([]string(nil), endpointProbe.caps...)
			aggregate.baseURL = endpointProbe.baseURL
			aggregate.endpointName = endpointProbe.endpointName
		}
	}
	aggregate.endpointStatuses = statuses
	return aggregate
}

func shouldPreferProviderProbe(candidate, current providerProbeResult) bool {
	return providerProbePriority(candidate.status) < providerProbePriority(current.status)
}

func providerProbePriority(status string) int {
	switch endpointStatus(status) {
	case "connected":
		return 0
	case "unauthenticated":
		return 1
	case "unreachable":
		return 2
	default:
		return 3
	}
}

// discoverOpenAIModels queries the /v1/models endpoint and returns the model count.
// This is a minimal inline version of provider/openai.DiscoverModels that avoids
// the import cycle (provider/openai imports the root agent package).
func discoverOpenAIModels(ctx context.Context, baseURL, apiKey string) (int, error) {
	base := strings.TrimRight(baseURL, "/")
	endpoint := base + "/models"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return 0, fmt.Errorf("discovery: build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return 0, fmt.Errorf("discovery: %s returned HTTP %d: %s", endpoint, resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var mr struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&mr); err != nil {
		return 0, fmt.Errorf("discovery: decode response from %s: %w", endpoint, err)
	}
	return len(mr.Data), nil
}

// serviceProviderCooldown is retained for the provider status field; live
// route-attempt cooldowns are surfaced on RouteStatus per provider/model.
func serviceProviderCooldown(sc ServiceConfig, providerName string, cooldown time.Duration) *CooldownState {
	return nil
}

func normalizeServiceProviderType(t string) string {
	t = strings.ToLower(strings.TrimSpace(t))
	if t == "" {
		return "openai"
	}
	return t
}

func serviceIsUnreachable(msg string) bool {
	lower := strings.ToLower(msg)
	return strings.Contains(lower, "connection refused") ||
		strings.Contains(lower, "no such host") ||
		strings.Contains(lower, "dial tcp") ||
		strings.Contains(lower, "unreachable") ||
		strings.Contains(lower, "connection reset") ||
		strings.Contains(lower, "i/o timeout")
}

func serviceTrimError(msg string) string {
	const maxLen = 120
	if len(msg) > maxLen {
		return msg[:maxLen] + "..."
	}
	return msg
}

func providerEndpointStatusesFromProbe(entry ServiceProviderEntry, probe providerProbeResult, capturedAt time.Time) []EndpointStatus {
	endpoints := modelDiscoveryEndpoints(entry)
	if len(endpoints) == 0 {
		probe.serverInstance = entry.ServerInstance
		return []EndpointStatus{endpointStatusFromProbe("default", entry.BaseURL, probe, capturedAt)}
	}
	out := make([]EndpointStatus, 0, len(endpoints))
	for _, endpoint := range endpoints {
		probe.serverInstance = endpoint.ServerInstance
		out = append(out, endpointStatusFromProbe(endpoint.Name, endpoint.BaseURL, probe, capturedAt))
	}
	return out
}

func endpointStatusFromProbe(name, baseURL string, probe providerProbeResult, capturedAt time.Time) EndpointStatus {
	source := strings.TrimRight(baseURL, "/") + "/models"
	if baseURL == "" {
		source = "service provider config"
	}
	out := EndpointStatus{
		Name:           endpointDisplayName(name, baseURL),
		BaseURL:        baseURL,
		ServerInstance: serverinstance.Normalize(baseURL, probe.serverInstance),
		ProbeURL:       source,
		Status:         endpointStatus(probe.status),
		Source:         source,
		CapturedAt:     capturedAt,
		Fresh:          true,
		ModelCount:     probe.modelCount,
		LastError:      statusErrorDetail(probe.status, probe.detail, source, capturedAt),
	}
	if out.Status == "connected" {
		out.LastSuccessAt = capturedAt
	}
	return out
}

// healthCheckRefreshClaudeQuota refreshes the Claude quota cache when
// the cached snapshot is older than the default refresh debounce. It is
// a best-effort operation: errors are silently discarded so that a
// claude absence does not fail HealthCheck. Under CONTRACT-004 the
// refresh delegates to QuotaHarness.RefreshQuota, which owns the PTY
// probe and cache I/O.
func (s *service) healthCheckRefreshClaudeQuota(ctx context.Context) {
	qh, ok := s.harnessByName("claude").(harnesses.QuotaHarness)
	if !ok {
		return
	}
	now := time.Now()
	if status, err := qh.QuotaStatus(ctx, now); err == nil {
		if status.State != harnesses.QuotaUnavailable && !status.CapturedAt.IsZero() && now.Sub(status.CapturedAt) < defaultQuotaRefreshDebounce {
			// Cache is fresh enough; skip the expensive PTY probe.
			return
		}
	}
	probeCtx, cancel := context.WithTimeout(ctx, defaultQuotaRefreshProbeTimeout)
	defer cancel()
	_, _ = qh.RefreshQuota(probeCtx)
}
