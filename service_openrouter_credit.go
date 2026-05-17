package fizeau

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/easel/fizeau/internal/routing"
)

// DefaultOpenrouterCreditBalanceThresholdUSD is the floor below which a
// cached openrouter account balance triggers the credit-balance gate.
// Operators can override per-provider via ServiceProviderEntry.
const DefaultOpenrouterCreditBalanceThresholdUSD = 0.50

// DefaultOpenrouterCreditProbeTTL bounds how long a cached balance reading
// stays fresh before the next routing pass re-probes /api/v1/credits.
// Picked at the middle of the 5–15 minute band specified by the bead so
// the cache amortizes across drains while still detecting top-ups quickly.
const DefaultOpenrouterCreditProbeTTL = 10 * time.Minute

// openrouterCreditProbeTimeout bounds one synchronous credit probe so a
// hung openrouter.ai response cannot stall a routing pass.
const openrouterCreditProbeTimeout = 5 * time.Second

// openrouterCreditFreshnessSource labels candidate.QuotaFreshnessAt overrides
// derived from the credit probe so operators can tell which freshness signal
// fed the row.
const openrouterCreditFreshnessSource = "openrouter_credits_probe"

// openrouterCreditRecord is one cached balance reading. The store keeps the
// last attempt timestamp (regardless of success) so probe failures still
// honor the TTL — see openrouter-probe-failure-modes-and-surfacing for the
// sibling bead that surfaces failure as a distinct filter reason.
type openrouterCreditRecord struct {
	BalanceUSD float64
	ObservedAt time.Time
	// HasBalance is false when the most recent probe attempt did not return
	// a balance reading (transport error, non-200, or empty response). In
	// that case the credit gate stays silent — failure surfacing is owned
	// by a separate bead.
	HasBalance bool
}

// openrouterCreditStore caches openrouter account balance readings with a
// TTL gate so repeated routing passes within the window share one HTTP
// round-trip to /api/v1/credits.
//
// The store is safe for concurrent use; a per-provider single-flight token
// ensures concurrent routing passes coalesce on one probe.
type openrouterCreditStore struct {
	mu        sync.Mutex
	records   map[string]openrouterCreditRecord
	inFlight  map[string]chan struct{}
	transport http.RoundTripper // nil = http.DefaultClient.Transport
}

func newOpenrouterCreditStore() *openrouterCreditStore {
	return &openrouterCreditStore{
		records:  make(map[string]openrouterCreditRecord),
		inFlight: make(map[string]chan struct{}),
	}
}

// Lookup returns the cached balance record for provider, if any.
func (s *openrouterCreditStore) Lookup(provider string) (openrouterCreditRecord, bool) {
	if s == nil {
		return openrouterCreditRecord{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.records[provider]
	return rec, ok
}

// Record forces a balance record for provider. Used by tests; the production
// path goes through EnsureFresh.
func (s *openrouterCreditStore) Record(provider string, rec openrouterCreditRecord) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records[provider] = rec
}

// EnsureFresh issues a synchronous /api/v1/credits probe when the cached
// record for provider is missing or older than ttl. Concurrent callers
// coalesce on a single in-flight probe via a per-provider waiter channel.
func (s *openrouterCreditStore) EnsureFresh(ctx context.Context, provider, baseURL, apiKey string, now time.Time, ttl time.Duration) {
	if s == nil || provider == "" || strings.TrimSpace(apiKey) == "" {
		return
	}
	s.mu.Lock()
	rec, ok := s.records[provider]
	if ok && ttl > 0 && now.Sub(rec.ObservedAt) < ttl {
		s.mu.Unlock()
		return
	}
	if waiter, busy := s.inFlight[provider]; busy {
		s.mu.Unlock()
		select {
		case <-waiter:
		case <-ctx.Done():
		}
		return
	}
	waiter := make(chan struct{})
	s.inFlight[provider] = waiter
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.inFlight, provider)
		close(waiter)
		s.mu.Unlock()
	}()

	balance, ok := s.probe(ctx, baseURL, apiKey)
	next := openrouterCreditRecord{ObservedAt: now}
	if ok {
		next.BalanceUSD = balance
		next.HasBalance = true
	}
	s.mu.Lock()
	s.records[provider] = next
	s.mu.Unlock()
}

// probe issues one /api/v1/credits request and parses the balance. Returns
// (balance, true) on success; (0, false) on transport or decode failure.
// Failure surfacing is intentionally out of scope here — sibling bead
// [[openrouter-probe-failure-modes-and-surfacing]] owns the credential_invalid
// and provider_unreachable filter reasons.
func (s *openrouterCreditStore) probe(ctx context.Context, baseURL, apiKey string) (float64, bool) {
	endpoint := openrouterCreditsEndpoint(baseURL)
	probeCtx, cancel := context.WithTimeout(ctx, openrouterCreditProbeTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(probeCtx, http.MethodGet, endpoint, nil)
	if err != nil {
		return 0, false
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := http.DefaultClient
	if s.transport != nil {
		client = &http.Client{Transport: s.transport, Timeout: openrouterCreditProbeTimeout}
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, false
	}
	defer resp.Body.Close() //nolint:errcheck
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return 0, false
	}
	var parsed openrouterCreditsResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return 0, false
	}
	return parsed.balanceUSD(), true
}

// openrouterCreditsResponse decodes /api/v1/credits. OpenRouter returns
// total_credits (lifetime top-ups, USD) and total_usage (lifetime spend, USD)
// nested under a "data" object; the spendable balance is the difference.
type openrouterCreditsResponse struct {
	Data struct {
		TotalCredits float64 `json:"total_credits"`
		TotalUsage   float64 `json:"total_usage"`
	} `json:"data"`
}

func (r openrouterCreditsResponse) balanceUSD() float64 {
	return r.Data.TotalCredits - r.Data.TotalUsage
}

// openrouterCreditsEndpoint resolves the /api/v1/credits URL from a provider's
// configured base URL. An empty base URL falls back to the public OpenRouter
// endpoint.
func openrouterCreditsEndpoint(baseURL string) string {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" {
		base = "https://openrouter.ai/api/v1"
	}
	return base + "/credits"
}

// openrouterCreditThresholdFor returns the configured credit balance threshold
// for the provider, falling back to the package default when unset.
func openrouterCreditThresholdFor(pcfg ServiceProviderEntry) float64 {
	if pcfg.CreditBalanceThresholdUSD > 0 {
		return pcfg.CreditBalanceThresholdUSD
	}
	return DefaultOpenrouterCreditBalanceThresholdUSD
}

// openrouterCreditTTLFor returns the configured credit-probe TTL for the
// provider, falling back to the package default when unset.
func openrouterCreditTTLFor(pcfg ServiceProviderEntry) time.Duration {
	if pcfg.CreditProbeTTL > 0 {
		return pcfg.CreditProbeTTL
	}
	return DefaultOpenrouterCreditProbeTTL
}

// openrouterCreditExhaustedMap refreshes the per-provider credit cache (one
// HTTP call at most, per TTL window, per provider) and returns the routing
// engine's view of which providers are currently below threshold.
//
// Cache misses and TTL-expired entries trigger a synchronous probe; entries
// still inside the TTL are reused. Providers without an api key are skipped
// (the credential gate already surfaces that case). Providers whose probe
// returns no balance reading are skipped as well — sibling bead
// [[openrouter-probe-failure-modes-and-surfacing]] owns surfacing failure.
func (s *service) openrouterCreditExhaustedMap(ctx context.Context, now time.Time) map[string]routing.ProviderCreditExhaustedEvidence {
	if s == nil || s.openrouterCredit == nil || s.opts.ServiceConfig == nil {
		return nil
	}
	cfg := s.opts.ServiceConfig
	names := cfg.ProviderNames()
	if len(names) == 0 {
		return nil
	}
	out := map[string]routing.ProviderCreditExhaustedEvidence{}
	for _, name := range names {
		pcfg, ok := cfg.Provider(name)
		if !ok {
			continue
		}
		if normalizeServiceProviderType(pcfg.Type) != "openrouter" {
			continue
		}
		apiKey := strings.TrimSpace(pcfg.APIKey)
		if apiKey == "" || !openrouterAPIKeyWellFormed(apiKey) {
			// Credential gate is responsible for these cases; do not probe.
			continue
		}
		ttl := openrouterCreditTTLFor(pcfg)
		s.openrouterCredit.EnsureFresh(ctx, name, pcfg.BaseURL, apiKey, now, ttl)
		rec, ok := s.openrouterCredit.Lookup(name)
		if !ok || !rec.HasBalance {
			continue
		}
		threshold := openrouterCreditThresholdFor(pcfg)
		if rec.BalanceUSD >= threshold {
			continue
		}
		out[name] = routing.ProviderCreditExhaustedEvidence{
			BalanceUSD:   rec.BalanceUSD,
			ThresholdUSD: threshold,
			ObservedAt:   rec.ObservedAt,
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// annotateOpenrouterCreditFreshness overlays QuotaFreshnessAt/QuotaFreshnessSource
// on every openrouter candidate so operators can see how stale the cached
// credit reading is. Runs against the post-engine decision so both eligible
// and credit-gated rows surface the same freshness signal.
func (s *service) annotateOpenrouterCreditFreshness(decision *RouteDecision) {
	if s == nil || decision == nil || s.openrouterCredit == nil || s.opts.ServiceConfig == nil {
		return
	}
	for i := range decision.Candidates {
		provider := candidateBaseProviderName(decision.Candidates[i].Provider)
		if provider == "" {
			continue
		}
		pcfg, ok := s.opts.ServiceConfig.Provider(provider)
		if !ok || normalizeServiceProviderType(pcfg.Type) != "openrouter" {
			continue
		}
		rec, ok := s.openrouterCredit.Lookup(provider)
		if !ok || rec.ObservedAt.IsZero() {
			continue
		}
		decision.Candidates[i].QuotaFreshnessAt = rec.ObservedAt.UTC()
		decision.Candidates[i].QuotaFreshnessSource = openrouterCreditFreshnessSource
	}
}

// candidateBaseProviderName strips any "@endpoint" suffix from a candidate
// provider identity to match ServiceConfig.Provider keys.
func candidateBaseProviderName(identity string) string {
	identity = strings.TrimSpace(identity)
	if identity == "" {
		return ""
	}
	if i := strings.IndexByte(identity, '@'); i > 0 {
		return identity[:i]
	}
	return identity
}
