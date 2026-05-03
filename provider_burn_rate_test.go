package fizeau

import (
	"testing"
	"time"
)

// midUTC returns a deterministic instant within a single UTC day for window
// math.
func midUTC(hour, minute int) time.Time {
	return time.Date(2026, 5, 2, hour, minute, 0, 0, time.UTC)
}

func TestBurnRate_NoTransitionUnderBudget(t *testing.T) {
	tr := NewProviderBurnRateTracker()
	tr.SetBudget("claude-acct-1", 1_000_000)

	// 12:00 UTC: half the day elapsed; 100k tokens used → projected 200k,
	// well below 1M budget.
	now := midUTC(12, 0)
	exhausted, retryAt := tr.Record("claude-acct-1", 100_000, now)
	if exhausted {
		t.Fatalf("expected no transition under budget, got exhausted=true (retryAt=%v)", retryAt)
	}
	if got := tr.Used("claude-acct-1", now); got != 100_000 {
		t.Errorf("Used = %d, want 100000", got)
	}
}

func TestBurnRate_PredictedExhaustionTransitions(t *testing.T) {
	tr := NewProviderBurnRateTracker()
	tr.SetBudget("claude-acct-2", 1_000_000)

	// 06:00 UTC: a quarter of the day elapsed; 500k tokens used → projected
	// 2M total, far above 1M budget. Should transition immediately.
	now := midUTC(6, 0)
	exhausted, retryAt := tr.Record("claude-acct-2", 500_000, now)
	if !exhausted {
		t.Fatalf("expected predictive exhaustion at 6h/quarter-day with 500k of 1M budget")
	}
	wantRetry := utcDayStart(now).Add(24 * time.Hour)
	if !retryAt.Equal(wantRetry) {
		t.Errorf("retryAt = %v, want %v (next UTC day boundary)", retryAt, wantRetry)
	}
}

func TestBurnRate_OverBudgetExhaustsImmediately(t *testing.T) {
	tr := NewProviderBurnRateTracker()
	tr.SetBudget("p", 1_000)
	now := midUTC(23, 30)
	exhausted, retryAt := tr.Record("p", 1_500, now)
	if !exhausted {
		t.Fatal("over-budget usage should exhaust regardless of projection")
	}
	if !retryAt.Equal(utcDayStart(now).Add(24 * time.Hour)) {
		t.Errorf("retryAt = %v, want next UTC midnight", retryAt)
	}
}

func TestBurnRate_NoBudgetIsInert(t *testing.T) {
	tr := NewProviderBurnRateTracker()
	// no SetBudget call
	now := midUTC(1, 0)
	exhausted, _ := tr.Record("unconfigured", 9_999_999, now)
	if exhausted {
		t.Fatal("provider without configured budget must never report exhaustion")
	}
	if got := tr.Used("unconfigured", now); got != 9_999_999 {
		t.Errorf("Used = %d, want 9999999 (tracking still happens, prediction does not)", got)
	}
}

func TestBurnRate_WindowResetClearsUsage(t *testing.T) {
	tr := NewProviderBurnRateTracker()
	tr.SetBudget("p", 1_000_000)

	day1 := midUTC(20, 0)
	exhausted, _ := tr.Record("p", 50_000, day1)
	if exhausted {
		t.Fatalf("setup: should be well under budget on day1")
	}
	if got := tr.Used("p", day1); got != 50_000 {
		t.Fatalf("day1 used = %d, want 50000", got)
	}

	// Cross into the next UTC day. The current window must reset to zero
	// before counting fresh tokens.
	day2 := day1.Add(8 * time.Hour) // 04:00 UTC the next day
	if got := tr.Used("p", day2); got != 0 {
		t.Errorf("after window reset, Used = %d, want 0", got)
	}
	exhausted, _ = tr.Record("p", 100, day2)
	if exhausted {
		t.Errorf("100 tokens at start of fresh day should not exhaust 1M budget")
	}
	if got := tr.Used("p", day2); got != 100 {
		t.Errorf("day2 used = %d, want 100", got)
	}
}

func TestBurnRate_RetryAfterIsNextWindowStart(t *testing.T) {
	tr := NewProviderBurnRateTracker()
	tr.SetBudget("p", 100)
	now := time.Date(2026, 5, 2, 23, 59, 59, 0, time.UTC)
	exhausted, retryAt := tr.Record("p", 1_000, now)
	if !exhausted {
		t.Fatal("over-budget must exhaust")
	}
	want := time.Date(2026, 5, 3, 0, 0, 0, 0, time.UTC)
	if !retryAt.Equal(want) {
		t.Errorf("retryAt = %v, want %v", retryAt, want)
	}
}

func TestBurnRate_SetBudgetZeroDisables(t *testing.T) {
	tr := NewProviderBurnRateTracker()
	tr.SetBudget("p", 1_000)
	tr.SetBudget("p", 0) // disable
	if got := tr.Budget("p"); got != 0 {
		t.Errorf("Budget after zero = %d, want 0", got)
	}
	exhausted, _ := tr.Record("p", 10_000, midUTC(12, 0))
	if exhausted {
		t.Error("disabled provider must not signal exhaustion")
	}
}

func TestBurnRate_ConcurrentRecordSafe(t *testing.T) {
	tr := NewProviderBurnRateTracker()
	tr.SetBudget("p", 1_000_000)
	now := midUTC(12, 0)
	done := make(chan struct{})
	for i := 0; i < 8; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				tr.Record("p", 1, now)
			}
			done <- struct{}{}
		}()
	}
	for i := 0; i < 8; i++ {
		<-done
	}
	if got := tr.Used("p", now); got != 800 {
		t.Errorf("Used after concurrent inserts = %d, want 800", got)
	}
}
