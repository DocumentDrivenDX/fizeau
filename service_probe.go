package fizeau

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/easel/fizeau/internal/provider/openai"
	"github.com/easel/fizeau/internal/sdk/openaicompat"
)

// QuotaRecoveryProber reports whether a quota_exhausted provider has recovered.
// A nil error indicates the provider is back online; a non-nil error keeps it
// in quota_exhausted with an extended retry_after.
type QuotaRecoveryProber func(ctx context.Context, provider string) error

const (
	// defaultQuotaRecoveryFallbackInterval is the wake interval used when no
	// quota_exhausted provider has a known retry_after to drive the next probe.
	defaultQuotaRecoveryFallbackInterval = 5 * time.Minute
	// quotaRecoveryBackoffInitial is the first retry_after extension applied
	// after a probe failure when no prior backoff is recorded.
	quotaRecoveryBackoffInitial = 5 * time.Minute
	// quotaRecoveryBackoffMax caps the exponential backoff between failed
	// probes so a persistently-down provider is still re-probed at least
	// once an hour.
	quotaRecoveryBackoffMax = 1 * time.Hour
)

// runQuotaRecoveryProbeLoop periodically probes providers that the store
// reports as quota_exhausted. When a provider's retry_after has elapsed (or
// is unknown) the loop calls probe; on success the provider is moved back to
// available, on failure its retry_after is extended with bounded exponential
// backoff. The loop exits when ctx is cancelled.
//
// fallback bounds the maximum sleep between passes. now/sleep are seams for
// deterministic tests; pass nil for production defaults.
func runQuotaRecoveryProbeLoop(
	ctx context.Context,
	store *ProviderQuotaStateStore,
	probe QuotaRecoveryProber,
	fallback time.Duration,
	now func() time.Time,
	sleep func(ctx context.Context, d time.Duration) bool,
) {
	if ctx == nil || store == nil || probe == nil {
		return
	}
	if fallback <= 0 {
		fallback = defaultQuotaRecoveryFallbackInterval
	}
	if now == nil {
		now = time.Now
	}
	if sleep == nil {
		sleep = quotaRecoverySleep
	}
	backoffs := make(map[string]time.Duration)
	for {
		next := runQuotaRecoveryProbePass(ctx, store, probe, fallback, now, backoffs)
		if !sleep(ctx, next) {
			return
		}
	}
}

// runQuotaRecoveryProbePass executes a single sweep over the quota_exhausted
// set and returns the duration to sleep before the next sweep.
func runQuotaRecoveryProbePass(
	ctx context.Context,
	store *ProviderQuotaStateStore,
	probe QuotaRecoveryProber,
	fallback time.Duration,
	now func() time.Time,
	backoffs map[string]time.Duration,
) time.Duration {
	entries := store.AllExhausted()
	t := now()
	// Drop backoff bookkeeping for providers no longer exhausted (e.g.
	// MarkAvailable was called externally) so a future re-exhaustion starts
	// with a fresh initial backoff.
	for name := range backoffs {
		if _, ok := entries[name]; !ok {
			delete(backoffs, name)
		}
	}
	if len(entries) == 0 {
		return fallback
	}
	nextWake := fallback
	for name, retry := range entries {
		if ctx.Err() != nil {
			return fallback
		}
		if !retry.IsZero() && retry.After(t) {
			if d := retry.Sub(t); d < nextWake {
				nextWake = d
			}
			continue
		}
		if err := probe(ctx, name); err == nil {
			store.MarkAvailable(name)
			delete(backoffs, name)
			continue
		}
		next := nextQuotaRecoveryBackoff(backoffs[name])
		backoffs[name] = next
		newRetry := t.Add(next)
		store.MarkQuotaExhausted(name, newRetry)
		if next < nextWake {
			nextWake = next
		}
	}
	if nextWake <= 0 {
		nextWake = fallback
	}
	return nextWake
}

// nextQuotaRecoveryBackoff returns the next bounded backoff value: doubles the
// previous extension, starting at quotaRecoveryBackoffInitial, capped at
// quotaRecoveryBackoffMax.
func nextQuotaRecoveryBackoff(prev time.Duration) time.Duration {
	if prev <= 0 {
		return quotaRecoveryBackoffInitial
	}
	next := prev * 2
	if next > quotaRecoveryBackoffMax {
		return quotaRecoveryBackoffMax
	}
	return next
}

// quotaRecoverySleep blocks for d or until ctx is cancelled. Returns true if
// the sleep completed normally and false if ctx was cancelled.
func quotaRecoverySleep(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return ctx.Err() == nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

// probeOpenAIModels calls GET /v1/models against baseURL and classifies
// failures into the three shapes the catalog cache understands:
//
//   - *openai.ReachabilityError for 5xx / transport failures (endpoint
//     unreachable); cache sets an UnreachableCooldown.
//   - errDiscoveryUnsupported for 404 / endpoints that don't expose
//     /v1/models; cache marks DiscoverySupported=false and callers
//     fall back to passthrough model naming.
//   - other errors (401/403 auth, unexpected body) are returned as-is;
//     the cache records them but doesn't mark the endpoint unreachable.
//
// The returned IDs are whatever the server returned in its `data[].id`
// list, preserving server-provided order.
func probeOpenAIModels(ctx context.Context, baseURL, apiKey string) ([]string, error) {
	ids, err := openaicompat.DiscoverModels(ctx, baseURL, apiKey)
	if err == nil {
		return ids, nil
	}
	msg := err.Error()
	// openaicompat.DiscoverModels returns errors prefixed with
	// "HTTP <code>: <body>" for non-2xx responses.
	if strings.Contains(msg, "HTTP 404") {
		return nil, ErrDiscoveryUnsupported()
	}
	if isServerError(msg) || isNetworkFailure(err) {
		return nil, &openai.ReachabilityError{
			Endpoint:   baseURL,
			Operation:  "probe_models",
			StatusCode: extractStatusCode(msg),
			Cause:      err,
		}
	}
	return nil, err
}

// isServerError returns true when the error message indicates a 5xx
// response from the discovery endpoint.
func isServerError(msg string) bool {
	for _, prefix := range []string{"HTTP 500", "HTTP 501", "HTTP 502", "HTTP 503", "HTTP 504", "HTTP 505"} {
		if strings.Contains(msg, prefix) {
			return true
		}
	}
	return false
}

// isNetworkFailure returns true when err looks like a transport-level
// failure (dial, timeout, TLS, connection reset) — the ReachabilityError
// classification applies.
func isNetworkFailure(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	markers := []string{
		"connection refused",
		"connection reset",
		"no such host",
		"tls: handshake",
		"TLS handshake",
		"i/o timeout",
		"dial tcp",
		"broken pipe",
	}
	for _, m := range markers {
		if strings.Contains(msg, m) {
			return true
		}
	}
	// Explicitly NOT classifying context.DeadlineExceeded as network —
	// that's the caller's own deadline.
	return errors.Is(err, errDialishNetworkError)
}

// extractStatusCode pulls the status code out of the "HTTP NNN:" prefix
// used by openaicompat.DiscoverModels. Returns 0 when no code is found.
func extractStatusCode(msg string) int {
	const prefix = "HTTP "
	idx := strings.Index(msg, prefix)
	if idx < 0 {
		return 0
	}
	tail := msg[idx+len(prefix):]
	if len(tail) < 3 {
		return 0
	}
	code := 0
	for i := 0; i < 3 && i < len(tail); i++ {
		c := tail[i]
		if c < '0' || c > '9' {
			return 0
		}
		code = code*10 + int(c-'0')
	}
	return code
}

// errDialishNetworkError is a sentinel for errors.Is tests. Callers may
// wrap their own network errors with this to ensure isNetworkFailure
// picks them up even when the message doesn't match one of the literal
// markers.
var errDialishNetworkError = errors.New("agent: network failure")
