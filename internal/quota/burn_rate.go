package quota

import (
	"sync"
	"time"
)

// BurnRateTracker maintains a per-provider rolling window of token usage and
// compares observed burn-rate against an operator-configured daily token
// budget. The tracker is safe for concurrent use.
type BurnRateTracker struct {
	mu      sync.Mutex
	budgets map[string]int
	windows map[string]*burnRateWindow
}

type burnRateWindow struct {
	start time.Time
	used  int
}

// NewBurnRateTracker returns an empty tracker with no configured budgets.
func NewBurnRateTracker() *BurnRateTracker {
	return &BurnRateTracker{
		budgets: make(map[string]int),
		windows: make(map[string]*burnRateWindow),
	}
}

// SetBudget installs or replaces the daily token budget for provider. A
// non-positive budget disables predictive exhaustion for that provider.
func (t *BurnRateTracker) SetBudget(provider string, budget int) {
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

// Budget returns the configured daily budget for provider, or 0 when none is
// set.
func (t *BurnRateTracker) Budget(provider string) int {
	if t == nil {
		return 0
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.budgets[provider]
}

// Used returns the tokens recorded for provider in the current window at now.
func (t *BurnRateTracker) Used(provider string, now time.Time) int {
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

// Record adds tokens to provider's rolling-window usage at now and evaluates
// whether predictive exhaustion has been triggered.
func (t *BurnRateTracker) Record(provider string, tokens int, now time.Time) (exhausted bool, retryAfter time.Time) {
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
		return false, time.Time{}
	}
	remaining := windowEnd.Sub(now)
	if remaining <= 0 {
		return false, time.Time{}
	}

	lhs := float64(w.used) * remaining.Seconds()
	rhs := float64(budget-w.used) * elapsed.Seconds()
	if lhs > rhs {
		return true, windowEnd
	}
	return false, time.Time{}
}

// Reset drops all per-provider windows. Budgets are preserved.
func (t *BurnRateTracker) Reset() {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.windows = make(map[string]*burnRateWindow)
}

func (t *BurnRateTracker) currentWindowLocked(provider string, now time.Time) *burnRateWindow {
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

// ObserveTokenUsage funnels post-execution token counts into the burn-rate
// tracker and cascades a predictive transition into the quota state store when
// the daily token budget is projected to be exceeded.
func ObserveTokenUsage(burn *BurnRateTracker, store *StateStore, provider string, tokens int, now time.Time) {
	if burn == nil || provider == "" {
		return
	}
	exhausted, retryAt := burn.Record(provider, tokens, now)
	if !exhausted || store == nil {
		return
	}
	store.MarkQuotaExhausted(provider, retryAt)
}

func utcDayStart(t time.Time) time.Time {
	u := t.UTC()
	return time.Date(u.Year(), u.Month(), u.Day(), 0, 0, 0, 0, time.UTC)
}
