package fizeau

import (
	"sync"
	"time"
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
	mu      sync.Mutex
	budgets map[string]int
	windows map[string]*burnRateWindow
}

type burnRateWindow struct {
	start time.Time // UTC window start (00:00 UTC)
	used  int       // tokens accumulated within [start, start+24h)
}

// NewProviderBurnRateTracker returns an empty tracker with no configured
// budgets.
func NewProviderBurnRateTracker() *ProviderBurnRateTracker {
	return &ProviderBurnRateTracker{
		budgets: make(map[string]int),
		windows: make(map[string]*burnRateWindow),
	}
}

// SetBudget installs (or replaces) the daily token budget for provider.
// budget <= 0 disables predictive exhaustion for that provider (and removes
// any prior budget).
func (t *ProviderBurnRateTracker) SetBudget(provider string, budget int) {
	if t == nil || provider == "" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if budget <= 0 {
		delete(t.budgets, provider)
		return
	}
	t.budgets[provider] = budget
}

// Budget returns the currently-configured daily budget for provider, or 0
// when none is set.
func (t *ProviderBurnRateTracker) Budget(provider string) int {
	if t == nil {
		return 0
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.budgets[provider]
}

// Used returns the tokens recorded for provider in the current window at
// instant now. Crossing a window boundary resets the count to zero; the
// returned value reflects that reset.
func (t *ProviderBurnRateTracker) Used(provider string, now time.Time) int {
	if t == nil || provider == "" {
		return 0
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	w := t.currentWindowLocked(provider, now)
	if w == nil {
		return 0
	}
	return w.used
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
	if t == nil || provider == "" {
		return false, time.Time{}
	}
	if tokens < 0 {
		tokens = 0
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	w := t.currentWindowLocked(provider, now)
	w.used += tokens

	budget, ok := t.budgets[provider]
	if !ok || budget <= 0 {
		return false, time.Time{}
	}

	windowEnd := w.start.Add(24 * time.Hour)
	if w.used >= budget {
		return true, windowEnd
	}

	elapsed := now.Sub(w.start)
	if elapsed <= 0 {
		// Just past midnight (or clock skew before window start). No
		// meaningful burn-rate yet; defer prediction until at least one
		// nanosecond has elapsed in the window.
		return false, time.Time{}
	}
	remaining := windowEnd.Sub(now)
	if remaining <= 0 {
		// Past window end; the next currentWindowLocked call will reset.
		return false, time.Time{}
	}

	// burn_rate * remaining_window > budget - used
	// ⇔ used * remaining_seconds > (budget - used) * elapsed_seconds
	// Use float64 (seconds) to avoid int64 overflow on full-day windows.
	lhs := float64(w.used) * remaining.Seconds()
	rhs := float64(budget-w.used) * elapsed.Seconds()
	if lhs > rhs {
		return true, windowEnd
	}
	return false, time.Time{}
}

// Reset drops all per-provider windows (budgets are preserved). Primarily
// useful in tests.
func (t *ProviderBurnRateTracker) Reset() {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.windows = make(map[string]*burnRateWindow)
}

// currentWindowLocked returns the window for provider that contains now,
// resetting it if now has crossed into a new UTC day. Caller must hold
// t.mu.
func (t *ProviderBurnRateTracker) currentWindowLocked(provider string, now time.Time) *burnRateWindow {
	if t.windows == nil {
		t.windows = make(map[string]*burnRateWindow)
	}
	start := utcDayStart(now)
	w, ok := t.windows[provider]
	if !ok || !w.start.Equal(start) {
		w = &burnRateWindow{start: start}
		t.windows[provider] = w
	}
	return w
}

// observeTokenUsage funnels post-execution token counts into the burn-rate
// tracker and cascades a predictive transition into the quota state store
// when the daily_token_budget is projected to be exceeded.
//
// Either argument may be nil; this is a no-op in that case. Provider names
// or non-positive token counts are likewise no-ops (apart from window
// initialization).
func (s *service) observeTokenUsage(provider string, tokens int, now time.Time) {
	if s == nil || s.providerBurnRate == nil || provider == "" {
		return
	}
	exhausted, retryAt := s.providerBurnRate.Record(provider, tokens, now)
	if !exhausted || s.providerQuota == nil {
		return
	}
	s.providerQuota.MarkQuotaExhausted(provider, retryAt)
}

// utcDayStart returns the UTC midnight at-or-before t.
func utcDayStart(t time.Time) time.Time {
	u := t.UTC()
	return time.Date(u.Year(), u.Month(), u.Day(), 0, 0, 0, 0, time.UTC)
}
