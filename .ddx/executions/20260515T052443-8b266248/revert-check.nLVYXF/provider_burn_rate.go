package fizeau

import (
	"time"

	"github.com/easel/fizeau/internal/quota"
)

// ProviderBurnRateTracker maintains a per-provider rolling window of token
// usage and compares observed burn-rate against an operator-configured daily
// token budget. When the projected end-of-window usage exceeds the budget,
// Record signals predictive exhaustion so the caller can pre-emptively
// transition the provider into quota_exhausted (without waiting for the
// real provider quota error).
//
// Window semantics (default):
//   - One window = one calendar day in UTC, starting at 00:00 UTC.
//   - When now crosses into the next UTC day, the prior window's usage is
//     dropped and counting starts fresh against the same daily budget.
//   - retry_after on a predicted-exhaustion transition is the start of the
//     NEXT UTC daily window (i.e. windowStart + 24h).
//
// Providers without a configured budget are inert: Record still tallies
// observed tokens (so the same tracker can report usage later) but never
// returns an exhausted=true signal.
//
// The tracker is safe for concurrent use.
type ProviderBurnRateTracker struct {
	inner *quota.BurnRateTracker
}

// NewProviderBurnRateTracker returns an empty tracker with no configured
// budgets.
func NewProviderBurnRateTracker() *ProviderBurnRateTracker {
	return &ProviderBurnRateTracker{inner: quota.NewBurnRateTracker()}
}

// SetBudget installs (or replaces) the daily token budget for provider.
// budget <= 0 disables predictive exhaustion for that provider (and removes
// any prior budget).
func (t *ProviderBurnRateTracker) SetBudget(provider string, budget int) {
	t.innerTracker().SetBudget(provider, budget)
}

// Budget returns the currently-configured daily budget for provider, or 0
// when none is set.
func (t *ProviderBurnRateTracker) Budget(provider string) int {
	return t.innerTracker().Budget(provider)
}

// Used returns the tokens recorded for provider in the current window at
// instant now. Crossing a window boundary resets the count to zero; the
// returned value reflects that reset.
func (t *ProviderBurnRateTracker) Used(provider string, now time.Time) int {
	return t.innerTracker().Used(provider, now)
}

// Record adds tokens to provider's rolling-window usage at instant now and
// evaluates whether predictive exhaustion has been triggered.
//
// Predictive rule (matches AC fizeau-f2661619 §3):
//
//	burn_rate        = used / elapsed
//	predicted_more   = burn_rate * remaining_window
//	exhausted        = predicted_more > (budget - used)
//
// In the equivalent "predicted total at window end > budget" form:
//
//	predicted_total  = used * (window_total / elapsed)
//
// If used >= budget already, exhausted is true with no projection needed.
//
// When exhausted, retryAfter is set to the start of the next daily window
// (windowStart + 24h, UTC). Otherwise the zero time.
//
// Providers with no configured budget always return exhausted=false.
func (t *ProviderBurnRateTracker) Record(provider string, tokens int, now time.Time) (exhausted bool, retryAfter time.Time) {
	return t.innerTracker().Record(provider, tokens, now)
}

// Reset drops all per-provider windows (budgets are preserved). Primarily
// useful in tests.
func (t *ProviderBurnRateTracker) Reset() {
	t.innerTracker().Reset()
}

// observeTokenUsage funnels post-execution token counts into the burn-rate
// tracker and cascades a predictive transition into the quota state store
// when the daily_token_budget is projected to be exceeded.
//
// Either argument may be nil; this is a no-op in that case. Provider names
// or non-positive token counts are likewise no-ops (apart from window
// initialization).
func (s *service) observeTokenUsage(provider string, tokens int, now time.Time) {
	if s == nil {
		return
	}
	quota.ObserveTokenUsage(s.providerBurnRate.innerTracker(), s.providerQuota.innerStore(), provider, tokens, now)
}

func (t *ProviderBurnRateTracker) innerTracker() *quota.BurnRateTracker {
	if t == nil {
		return nil
	}
	return t.inner
}
