package fizeau

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/easel/fizeau/internal/harnesses"
	"github.com/easel/fizeau/internal/harnesses/harnesstest"
)

// fakeClock is a controllable clock for refreshScheduler tests. Time
// only advances when the test calls Advance; tickers fire synchronously
// from the caller's goroutine when their next-fire instant is reached.
type fakeClock struct {
	mu      sync.Mutex
	now     time.Time
	tickers []*fakeTicker
}

func newFakeClock(start time.Time) *fakeClock {
	return &fakeClock{now: start}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) NewTicker(d time.Duration) schedulerTicker {
	c.mu.Lock()
	defer c.mu.Unlock()
	t := &fakeTicker{
		c:        make(chan time.Time, 1),
		interval: d,
		next:     c.now.Add(d),
	}
	c.tickers = append(c.tickers, t)
	return t
}

// Advance moves the fake clock forward by d and fires every ticker
// whose next-fire instant is at or before the new now. Each ticker
// fires at most once per Advance — chained ticks within a single
// Advance window are coalesced (matching time.Ticker's drop-on-full
// behavior).
func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	target := c.now
	tickers := append([]*fakeTicker(nil), c.tickers...)
	c.mu.Unlock()
	for _, t := range tickers {
		t.fireUpTo(target)
	}
}

type fakeTicker struct {
	c        chan time.Time
	interval time.Duration

	mu      sync.Mutex
	next    time.Time
	stopped bool
}

func (t *fakeTicker) C() <-chan time.Time { return t.c }

func (t *fakeTicker) Stop() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.stopped = true
}

func (t *fakeTicker) fireUpTo(now time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.stopped {
		return
	}
	for !now.Before(t.next) {
		select {
		case t.c <- t.next:
		default:
		}
		t.next = t.next.Add(t.interval)
	}
}

// waitForTick waits for the scheduler to publish a tick-completed
// notification for any harness, or fails the test on timeout.
func waitForTick(t *testing.T, ch <-chan string, timeout time.Duration) string {
	t.Helper()
	select {
	case name := <-ch:
		return name
	case <-time.After(timeout):
		t.Fatalf("timed out waiting for tick notification after %s", timeout)
		return ""
	}
}

// TestRefreshScheduler_Debounces verifies that the scheduler fires
// RefreshQuota at QuotaFreshness()/2 cadence, and skips ticks when the
// cached CapturedAt is still within QuotaFreshness() of now.
func TestRefreshScheduler_Debounces(t *testing.T) {
	start := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	clock := newFakeClock(start)

	freshness := 30 * time.Minute
	// Seed CapturedAt at start time; State=QuotaOK so it isn't
	// treated as cache-cold.
	qh := harnesstest.NewSyntheticQuotaHarness("synth", harnesses.QuotaStatus{
		State:      harnesses.QuotaOK,
		CapturedAt: start,
	}, nil)
	qh.SetQuotaFreshness(freshness)

	notify := make(chan string, 8)
	sched := newRefreshScheduler(func(name string) harnesses.Harness {
		if name == "synth" {
			return qh
		}
		return nil
	}, []string{"synth"}, clock)
	sched.tickNotify = notify

	sched.Start(context.Background())
	t.Cleanup(sched.Stop)

	// Tick 1 at freshness/2 (15m): CapturedAt is 15m old, still
	// within freshness → skip.
	clock.Advance(freshness / 2)
	waitForTick(t, notify, time.Second)
	if got := qh.ProbeCount(); got != 0 {
		t.Fatalf("after first tick: ProbeCount=%d want 0 (still fresh)", got)
	}

	// Tick 2 at freshness (30m total): CapturedAt is 30m old,
	// outside freshness → refresh.
	clock.Advance(freshness / 2)
	waitForTick(t, notify, time.Second)
	if got := qh.ProbeCount(); got != 1 {
		t.Fatalf("after second tick: ProbeCount=%d want 1", got)
	}

	// The refresh stored the same QuotaStatus as before (with the
	// CapturedAt from the original status, which is still `start`).
	// Update synthetic status to reflect "just refreshed" so the
	// next tick is correctly classified as still-fresh.
	qh.SetQuotaStatus(harnesses.QuotaStatus{
		State:      harnesses.QuotaOK,
		CapturedAt: clock.Now(),
	})

	// Tick 3 at freshness*1.5 (45m): CapturedAt is 15m old → skip.
	clock.Advance(freshness / 2)
	waitForTick(t, notify, time.Second)
	if got := qh.ProbeCount(); got != 1 {
		t.Fatalf("after third tick: ProbeCount=%d want 1 (still fresh after refresh)", got)
	}
}

// TestRefreshScheduler_StartupKickoff verifies that scheduler
// construction fires a single RefreshQuota for every registered
// QuotaHarness whose cached state is QuotaUnavailable (cache-cold).
func TestRefreshScheduler_StartupKickoff(t *testing.T) {
	start := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	clock := newFakeClock(start)

	coldQuota := harnesstest.NewSyntheticQuotaHarness("cold", harnesses.QuotaStatus{
		State: harnesses.QuotaUnavailable,
	}, nil)
	coldQuota.SetQuotaFreshness(30 * time.Minute)

	warmQuota := harnesstest.NewSyntheticQuotaHarness("warm", harnesses.QuotaStatus{
		State:      harnesses.QuotaOK,
		CapturedAt: start,
	}, nil)
	warmQuota.SetQuotaFreshness(30 * time.Minute)

	coldAccount := harnesstest.NewSyntheticAccountHarness("coldacct", harnesses.AccountSnapshot{
		// CapturedAt zero → cache-cold.
	}, 7*24*time.Hour)

	lookup := func(name string) harnesses.Harness {
		switch name {
		case "cold":
			return coldQuota
		case "warm":
			return warmQuota
		case "coldacct":
			return coldAccount
		}
		return nil
	}

	sched := newRefreshScheduler(lookup, []string{"cold", "warm", "coldacct"}, clock)
	sched.Start(context.Background())
	t.Cleanup(sched.Stop)

	// Wait for the startup goroutines to complete by polling.
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if coldQuota.ProbeCount() == 1 && coldAccount.ProbeCount() == 1 {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}

	if got := coldQuota.ProbeCount(); got != 1 {
		t.Fatalf("cold QuotaHarness kickoff: ProbeCount=%d want 1", got)
	}
	if got := warmQuota.ProbeCount(); got != 0 {
		t.Fatalf("warm QuotaHarness kickoff: ProbeCount=%d want 0", got)
	}
	if got := coldAccount.ProbeCount(); got != 1 {
		t.Fatalf("cold AccountHarness kickoff: ProbeCount=%d want 1", got)
	}
}

// TestRefreshScheduler_SingleFlight verifies that concurrent
// RefreshQuota callers driven by the scheduler share one underlying
// probe through the harness's cache lock — the scheduler does not
// double-probe under simultaneous tick + external refresh.
func TestRefreshScheduler_SingleFlight(t *testing.T) {
	start := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	clock := newFakeClock(start)

	freshness := 30 * time.Minute
	captured := start.Add(-2 * freshness) // stale so the tick refreshes
	qh := harnesstest.NewSyntheticQuotaHarness("synth", harnesses.QuotaStatus{
		State:      harnesses.QuotaOK,
		CapturedAt: captured,
	}, nil)
	qh.SetQuotaFreshness(freshness)

	// Latch: the scheduler's RefreshQuota and an external
	// RefreshQuota must collide in the same single-flight cohort.
	latch := make(chan struct{})
	qh.SetProbeLatch(latch)

	notify := make(chan string, 1)
	sched := newRefreshScheduler(func(name string) harnesses.Harness {
		if name == "synth" {
			return qh
		}
		return nil
	}, []string{"synth"}, clock)
	sched.tickNotify = notify
	sched.Start(context.Background())
	t.Cleanup(sched.Stop)

	// Drive the tick.
	clock.Advance(freshness / 2)

	// External caller races the scheduler. Both should resolve to
	// a single probe.
	externalDone := make(chan struct{})
	go func() {
		_, _ = qh.RefreshQuota(context.Background())
		close(externalDone)
	}()

	// Wait long enough for both callers to enter the probe.
	time.Sleep(20 * time.Millisecond)
	close(latch)

	waitForTick(t, notify, time.Second)
	select {
	case <-externalDone:
	case <-time.After(time.Second):
		t.Fatal("external RefreshQuota did not complete")
	}

	if got := qh.ProbeCount(); got != 1 {
		t.Fatalf("ProbeCount=%d; want exactly one probe under single-flight", got)
	}
}

// TestRefreshScheduler_StopCancelsTickers verifies that Stop cancels
// the scheduler context, all ticker goroutines exit, and no further
// refreshes fire after Stop returns.
func TestRefreshScheduler_StopCancelsTickers(t *testing.T) {
	start := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	clock := newFakeClock(start)

	freshness := 30 * time.Minute
	qh := harnesstest.NewSyntheticQuotaHarness("synth", harnesses.QuotaStatus{
		State:      harnesses.QuotaOK,
		CapturedAt: start.Add(-2 * freshness),
	}, nil)
	qh.SetQuotaFreshness(freshness)

	sched := newRefreshScheduler(func(name string) harnesses.Harness {
		if name == "synth" {
			return qh
		}
		return nil
	}, []string{"synth"}, clock)

	sched.Start(context.Background())
	sched.Stop()

	// After Stop returns, the ticker goroutine has exited (wg
	// drained). Subsequent Advance must not race with the goroutine
	// or cause further refreshes.
	before := qh.ProbeCount()
	clock.Advance(freshness * 4)
	// Give any (incorrectly) leaked goroutine time to misbehave.
	time.Sleep(20 * time.Millisecond)
	if got := qh.ProbeCount(); got != before {
		t.Fatalf("post-Stop ProbeCount=%d want %d (no refresh after Stop)", got, before)
	}

	// Stop is idempotent.
	sched.Stop()
}

// TestRefreshScheduler_AccountTickerSkippedWhenSameCadence verifies
// that a harness implementing both QuotaHarness and AccountHarness
// with matching freshness windows runs only the quota ticker — no
// duplicate account ticker is registered.
func TestRefreshScheduler_AccountTickerSkippedWhenSameCadence(t *testing.T) {
	start := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	clock := newFakeClock(start)

	freshness := 30 * time.Minute
	combo := &comboHarness{
		quota: harnesstest.NewSyntheticQuotaHarness("combo", harnesses.QuotaStatus{
			State:      harnesses.QuotaOK,
			CapturedAt: start.Add(-2 * freshness),
		}, nil),
		account: harnesstest.NewSyntheticAccountHarness("combo", harnesses.AccountSnapshot{
			Authenticated: true,
			CapturedAt:    start.Add(-2 * freshness),
		}, freshness),
	}
	combo.quota.SetQuotaFreshness(freshness)

	notify := make(chan string, 8)
	sched := newRefreshScheduler(func(name string) harnesses.Harness {
		if name == "combo" {
			return combo
		}
		return nil
	}, []string{"combo"}, clock)
	sched.tickNotify = notify

	sched.Start(context.Background())
	t.Cleanup(sched.Stop)

	clock.Advance(freshness / 2)
	waitForTick(t, notify, time.Second)

	if got := combo.quota.ProbeCount(); got != 1 {
		t.Fatalf("quota probe count: %d want 1", got)
	}
	if got := combo.account.ProbeCount(); got != 0 {
		t.Fatalf("account probe count: %d want 0 (cadence matched, no separate ticker)", got)
	}
}

// comboHarness implements both QuotaHarness and AccountHarness by
// delegating to underlying synthetic instances. Used to exercise the
// account-ticker-suppression branch.
type comboHarness struct {
	quota   *harnesstest.SyntheticQuotaHarness
	account *harnesstest.SyntheticAccountHarness
}

func (c *comboHarness) Info() harnesses.HarnessInfo { return c.quota.Info() }
func (c *comboHarness) HealthCheck(ctx context.Context) error {
	return c.quota.HealthCheck(ctx)
}
func (c *comboHarness) Execute(ctx context.Context, req harnesses.ExecuteRequest) (<-chan harnesses.Event, error) {
	return c.quota.Execute(ctx, req)
}
func (c *comboHarness) QuotaStatus(ctx context.Context, now time.Time) (harnesses.QuotaStatus, error) {
	return c.quota.QuotaStatus(ctx, now)
}
func (c *comboHarness) RefreshQuota(ctx context.Context) (harnesses.QuotaStatus, error) {
	return c.quota.RefreshQuota(ctx)
}
func (c *comboHarness) QuotaFreshness() time.Duration { return c.quota.QuotaFreshness() }
func (c *comboHarness) SupportedLimitIDs() []string   { return c.quota.SupportedLimitIDs() }
func (c *comboHarness) AccountStatus(ctx context.Context, now time.Time) (harnesses.AccountSnapshot, error) {
	return c.account.AccountStatus(ctx, now)
}
func (c *comboHarness) RefreshAccount(ctx context.Context) (harnesses.AccountSnapshot, error) {
	return c.account.RefreshAccount(ctx)
}
func (c *comboHarness) AccountFreshness() time.Duration { return c.account.AccountFreshness() }

// TestHarnessByName verifies the service.harnessByName helper returns
// the registered Harness instance for each known harness and nil for
// unknown names.
func TestHarnessByName(t *testing.T) {
	svc := &service{harnessInstances: defaultHarnessInstances()}

	for _, name := range []string{"claude", "codex", "gemini", "opencode", "pi"} {
		if h := svc.harnessByName(name); h == nil {
			t.Errorf("harnessByName(%q) = nil; want non-nil instance", name)
		}
	}
	if h := svc.harnessByName("does-not-exist"); h != nil {
		t.Errorf("harnessByName(unknown) = %v; want nil", h)
	}
}
