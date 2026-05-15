package fizeau

import (
	"context"
	"sync"
	"time"

	"github.com/easel/fizeau/internal/harnesses"
)

// refreshScheduler owns the single service-wide async refresh cadence
// across every registered QuotaHarness and AccountHarness. It is the
// replacement for the per-harness Refresh*Async helpers described in
// CONTRACT-004 / plan-2026-05-14-harness-interface-refactor.md Step 4.
//
// One debounced ticker is scheduled per registered QuotaHarness at
// QuotaFreshness()/2 cadence. An independent ticker is scheduled per
// registered AccountHarness only when its AccountFreshness() differs
// from the harness's QuotaFreshness() — when the cadences match, the
// QuotaHarness ticker is sufficient for callers that want to drive
// account refreshes alongside quota.
//
// The scheduler does not enforce single-flight itself. RefreshQuota /
// RefreshAccount are required by CONTRACT-004 to be single-flight per
// harness instance (via the harness's cache lock); the scheduler relies
// on that contract. Cache-freshness deduplication, however, is the
// scheduler's responsibility: it reads QuotaStatus before each refresh
// and skips ticks where CapturedAt is within QuotaFreshness() of now,
// so that RefreshQuota itself can probe unconditionally per
// CONTRACT-004 invariant #4.
//
// Pre-migration (Steps 5–8 of the refactor not yet landed), the
// concrete Runner types do not yet implement QuotaHarness or
// AccountHarness. Type assertions fail and the scheduler registers
// nothing for those harnesses; they continue to use their existing
// Refresh*Async helpers until each harness's e-step deletes them.
type refreshScheduler struct {
	lookup func(name string) harnesses.Harness
	names  []string
	clock  schedulerClock

	// tickNotify, when non-nil, receives the harness name after each
	// tick is fully processed (refresh attempted or skipped). Tests
	// use this to synchronize on tick completion; production leaves
	// it nil.
	tickNotify chan<- string

	mu     sync.Mutex
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// schedulerClock is the time abstraction the refresh scheduler uses.
// The production implementation wraps time.Now / time.NewTicker; tests
// inject a controllable fake clock to drive cadence deterministically.
type schedulerClock interface {
	Now() time.Time
	NewTicker(d time.Duration) schedulerTicker
}

// schedulerTicker is a minimal time.Ticker abstraction.
type schedulerTicker interface {
	C() <-chan time.Time
	Stop()
}

// realClock is the wall-clock implementation of schedulerClock.
type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

func (realClock) NewTicker(d time.Duration) schedulerTicker {
	return &realTicker{t: time.NewTicker(d)}
}

type realTicker struct{ t *time.Ticker }

func (r *realTicker) C() <-chan time.Time { return r.t.C }
func (r *realTicker) Stop()               { r.t.Stop() }

// newRefreshScheduler constructs (but does not start) a scheduler over
// the harness names supplied. lookup returns the registered Harness
// instance for each name; a nil result is tolerated and means the name
// is registered but has no live instance, in which case the scheduler
// registers nothing for that name. clock defaults to realClock when
// nil.
func newRefreshScheduler(lookup func(string) harnesses.Harness, names []string, clock schedulerClock) *refreshScheduler {
	if clock == nil {
		clock = realClock{}
	}
	return &refreshScheduler{
		lookup: lookup,
		names:  append([]string(nil), names...),
		clock:  clock,
	}
}

// Start launches the scheduler. For every registered harness, it
// type-asserts to QuotaHarness and AccountHarness independently. For
// each capability the harness satisfies, it:
//
//  1. Reads the current cached status. When the status reports
//     QuotaUnavailable (cache-cold), the scheduler fires one
//     immediate refresh.
//  2. Starts a debounced ticker at Freshness/2 cadence that drives
//     subsequent refreshes.
//
// Start is not safe to call concurrently; concurrent Start panics.
// Call Stop before re-starting.
func (s *refreshScheduler) Start(parent context.Context) {
	s.mu.Lock()
	if s.cancel != nil {
		s.mu.Unlock()
		panic("refreshScheduler: Start called twice without intervening Stop")
	}
	s.ctx, s.cancel = context.WithCancel(parent)
	s.mu.Unlock()

	now := s.clock.Now()
	for _, name := range s.names {
		h := s.lookup(name)
		if h == nil {
			continue
		}
		qh, isQuota := h.(harnesses.QuotaHarness)
		ah, isAccount := h.(harnesses.AccountHarness)

		if isQuota {
			s.startQuotaTicker(name, qh, now)
		}
		if isAccount {
			// Only start a separate account ticker when account
			// freshness differs from quota freshness. When they
			// match, the quota cadence already covers the cache
			// lifetime account refresh would observe; harnesses
			// that want account-aligned refresh can implement it
			// inside RefreshQuota. See CONTRACT-004 / Step 4.
			if !isQuota || ah.AccountFreshness() != qh.QuotaFreshness() {
				s.startAccountTicker(name, ah, now)
			}
		}
	}
}

// Stop cancels the scheduler context and waits for every ticker
// goroutine to exit. Safe to call on a never-started scheduler (no-op).
func (s *refreshScheduler) Stop() {
	s.mu.Lock()
	cancel := s.cancel
	s.cancel = nil
	s.mu.Unlock()
	if cancel == nil {
		return
	}
	cancel()
	s.wg.Wait()
}

// startQuotaTicker fires the cache-cold kickoff (if applicable) and
// launches the per-harness debounced ticker.
func (s *refreshScheduler) startQuotaTicker(name string, qh harnesses.QuotaHarness, now time.Time) {
	interval := qh.QuotaFreshness() / 2
	if interval <= 0 {
		// Misconfigured harness: a non-positive freshness disables
		// scheduling — the scheduler refuses to busy-loop. Surfaced
		// as a CONTRACT-004 violation by the conformance suite.
		return
	}

	// Cache-cold startup kickoff. Per
	// primary-harness-capability-baseline.md and AC #6, the
	// scheduler fires one RefreshQuota immediately when the
	// cached state is QuotaUnavailable.
	if status, err := qh.QuotaStatus(s.ctx, now); err == nil && status.State == harnesses.QuotaUnavailable {
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			_, _ = qh.RefreshQuota(s.ctx)
		}()
	}

	ticker := s.clock.NewTicker(interval)
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer ticker.Stop()
		for {
			select {
			case <-s.ctx.Done():
				return
			case <-ticker.C():
				s.maybeRefreshQuota(name, qh)
			}
		}
	}()
}

// maybeRefreshQuota implements the per-tick decision. On cache-cold
// (QuotaUnavailable) it always refreshes. Otherwise it skips when
// CapturedAt is within QuotaFreshness() of now (cache-freshness dedup
// is the scheduler's job, not RefreshQuota's). All other tick
// outcomes result in a RefreshQuota call.
func (s *refreshScheduler) maybeRefreshQuota(name string, qh harnesses.QuotaHarness) {
	defer s.notifyTick(name)

	now := s.clock.Now()
	status, err := qh.QuotaStatus(s.ctx, now)
	if err != nil {
		return
	}

	if status.State == harnesses.QuotaUnavailable {
		_, _ = qh.RefreshQuota(s.ctx)
		return
	}

	if !status.CapturedAt.IsZero() && now.Sub(status.CapturedAt) < qh.QuotaFreshness() {
		return
	}

	_, _ = qh.RefreshQuota(s.ctx)
}

// startAccountTicker is the AccountHarness analogue of
// startQuotaTicker. Cache-cold is detected via a zero CapturedAt
// because AccountSnapshot has no explicit "unavailable" state value.
func (s *refreshScheduler) startAccountTicker(name string, ah harnesses.AccountHarness, now time.Time) {
	interval := ah.AccountFreshness() / 2
	if interval <= 0 {
		return
	}

	if snap, err := ah.AccountStatus(s.ctx, now); err == nil && snap.CapturedAt.IsZero() {
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			_, _ = ah.RefreshAccount(s.ctx)
		}()
	}

	ticker := s.clock.NewTicker(interval)
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer ticker.Stop()
		for {
			select {
			case <-s.ctx.Done():
				return
			case <-ticker.C():
				s.maybeRefreshAccount(name, ah)
			}
		}
	}()
}

// maybeRefreshAccount mirrors maybeRefreshQuota for AccountHarness.
func (s *refreshScheduler) maybeRefreshAccount(name string, ah harnesses.AccountHarness) {
	defer s.notifyTick(name)

	now := s.clock.Now()
	snap, err := ah.AccountStatus(s.ctx, now)
	if err != nil {
		return
	}

	if snap.CapturedAt.IsZero() {
		_, _ = ah.RefreshAccount(s.ctx)
		return
	}

	if now.Sub(snap.CapturedAt) < ah.AccountFreshness() {
		return
	}

	_, _ = ah.RefreshAccount(s.ctx)
}

// notifyTick fans out a tick-processed notification when tickNotify is
// set. Never blocks the scheduler goroutine on the receiver.
func (s *refreshScheduler) notifyTick(name string) {
	if s.tickNotify == nil {
		return
	}
	select {
	case s.tickNotify <- name:
	case <-s.ctx.Done():
	}
}
