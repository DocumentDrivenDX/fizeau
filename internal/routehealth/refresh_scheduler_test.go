package routehealth

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/easel/fizeau/internal/harnesses"
	"github.com/easel/fizeau/internal/harnesses/harnesstest"
)

// fakeClock is a controllable clock for RefreshScheduler tests. Time only
// advances when the test calls Advance; tickers fire synchronously from the
// caller's goroutine when their next-fire instant is reached.
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

func TestRefreshScheduler_Debounces(t *testing.T) {
	start := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	clock := newFakeClock(start)

	freshness := 30 * time.Minute
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

	clock.Advance(freshness / 2)
	waitForTick(t, notify, time.Second)
	if got := qh.ProbeCount(); got != 0 {
		t.Fatalf("after first tick: ProbeCount=%d want 0 (still fresh)", got)
	}

	clock.Advance(freshness / 2)
	waitForTick(t, notify, time.Second)
	if got := qh.ProbeCount(); got != 1 {
		t.Fatalf("after second tick: ProbeCount=%d want 1", got)
	}

	qh.SetQuotaStatus(harnesses.QuotaStatus{
		State:      harnesses.QuotaOK,
		CapturedAt: clock.Now(),
	})

	clock.Advance(freshness / 2)
	waitForTick(t, notify, time.Second)
	if got := qh.ProbeCount(); got != 1 {
		t.Fatalf("after third tick: ProbeCount=%d want 1 (still fresh after refresh)", got)
	}
}

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

	coldAccount := harnesstest.NewSyntheticAccountHarness("coldacct", harnesses.AccountSnapshot{}, 7*24*time.Hour)

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

func TestRefreshScheduler_SingleFlight(t *testing.T) {
	start := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	clock := newFakeClock(start)

	freshness := 30 * time.Minute
	captured := start.Add(-2 * freshness)
	qh := harnesstest.NewSyntheticQuotaHarness("synth", harnesses.QuotaStatus{
		State:      harnesses.QuotaOK,
		CapturedAt: captured,
	}, nil)
	qh.SetQuotaFreshness(freshness)

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

	clock.Advance(freshness / 2)

	externalDone := make(chan struct{})
	go func() {
		_, _ = qh.RefreshQuota(context.Background())
		close(externalDone)
	}()

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

	before := qh.ProbeCount()
	clock.Advance(freshness * 4)
	time.Sleep(20 * time.Millisecond)
	if got := qh.ProbeCount(); got != before {
		t.Fatalf("post-Stop ProbeCount=%d want %d (no refresh after Stop)", got, before)
	}

	sched.Stop()
}

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

// comboHarness implements both QuotaHarness and AccountHarness by delegating to
// underlying synthetic instances. Used to exercise the
// account-ticker-suppression branch.
type comboHarness struct {
	quota   *harnesstest.SyntheticQuotaHarness
	account *harnesstest.SyntheticAccountHarness
}

func (h *comboHarness) Info() harnesses.HarnessInfo { return h.quota.Info() }

func (h *comboHarness) HealthCheck(ctx context.Context) error { return h.quota.HealthCheck(ctx) }

func (h *comboHarness) Execute(ctx context.Context, req harnesses.ExecuteRequest) (<-chan harnesses.Event, error) {
	return h.quota.Execute(ctx, req)
}

func (h *comboHarness) QuotaStatus(ctx context.Context, now time.Time) (harnesses.QuotaStatus, error) {
	return h.quota.QuotaStatus(ctx, now)
}

func (h *comboHarness) RefreshQuota(ctx context.Context) (harnesses.QuotaStatus, error) {
	return h.quota.RefreshQuota(ctx)
}

func (h *comboHarness) QuotaFreshness() time.Duration {
	return h.quota.QuotaFreshness()
}

func (h *comboHarness) SupportedLimitIDs() []string { return h.quota.SupportedLimitIDs() }

func (h *comboHarness) AccountStatus(ctx context.Context, now time.Time) (harnesses.AccountSnapshot, error) {
	return h.account.AccountStatus(ctx, now)
}

func (h *comboHarness) RefreshAccount(ctx context.Context) (harnesses.AccountSnapshot, error) {
	return h.account.RefreshAccount(ctx)
}

func (h *comboHarness) AccountFreshness() time.Duration {
	return h.account.AccountFreshness()
}
