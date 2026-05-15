package harnesstest_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/easel/fizeau/internal/harnesses"
	"github.com/easel/fizeau/internal/harnesses/harnesstest"
)

// TestSyntheticQuotaHarnessConformance runs the QuotaHarness conformance
// suite against the in-memory synthetic implementation, proving the
// suite is exercisable end-to-end before any real harness consumes it.
func TestSyntheticQuotaHarnessConformance(t *testing.T) {
	status := harnesses.QuotaStatus{
		Source:            "synthetic",
		CapturedAt:        time.Now(),
		Fresh:             true,
		Age:               time.Second,
		State:             harnesses.QuotaOK,
		RoutingPreference: harnesses.RoutingPreferenceAvailable,
		Windows: []harnesses.QuotaWindow{
			{Name: "5h", LimitID: "five_hour", WindowMinutes: 300, UsedPercent: 12, State: "ok"},
			{Name: "weekly", LimitID: "weekly", WindowMinutes: 10080, UsedPercent: 4, State: "ok"},
		},
	}
	h := harnesstest.NewSyntheticQuotaHarness("synthetic-quota", status, []string{"five_hour", "weekly"})
	harnesstest.RunQuotaHarnessConformance(t, h)
}

// TestSyntheticQuotaHarnessProbeCountSingleFlight verifies that
// concurrent RefreshQuota callers share one underlying probe. This is
// the synthetic-specific assertion the shared conformance suite does
// NOT make.
func TestSyntheticQuotaHarnessProbeCountSingleFlight(t *testing.T) {
	status := harnesses.QuotaStatus{
		Source:     "synthetic",
		CapturedAt: time.Now(),
		Fresh:      true,
		State:      harnesses.QuotaOK,
	}
	h := harnesstest.NewSyntheticQuotaHarness("synthetic-quota-probe", status, nil)

	// Install a latch so every caller's probe blocks inside the
	// single-flight cohort until we release it, guaranteeing they
	// observe each other as in-flight.
	latch := make(chan struct{})
	h.SetProbeLatch(latch)

	const N = 16
	var (
		wg    sync.WaitGroup
		start = make(chan struct{})
		ready = make(chan struct{}, N)
	)
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func() {
			defer wg.Done()
			ready <- struct{}{}
			<-start
			if _, err := h.RefreshQuota(context.Background()); err != nil {
				t.Errorf("RefreshQuota: %v", err)
			}
		}()
	}
	for i := 0; i < N; i++ {
		<-ready
	}
	close(start)
	// Give the goroutines a beat to all enter singleflight.Do before
	// releasing the latch. A short sleep is acceptable because the
	// alternative — busy-waiting on ProbeCount — races with the
	// cohort joining the in-flight call.
	time.Sleep(50 * time.Millisecond)
	close(latch)
	wg.Wait()

	probes := h.ProbeCount()
	if probes != 1 {
		t.Errorf("ProbeCount() = %d for %d concurrent RefreshQuota callers; want exactly 1", probes, N)
	}
}

// TestSyntheticQuotaHarnessRespectsEmptySupportedLimitIDs covers the
// CONTRACT-004 surface where a harness emits no limit IDs (e.g. an
// opencode-style placeholder); the conformance suite must accept an
// empty SupportedLimitIDs set as long as Windows[].LimitID values are
// also empty.
func TestSyntheticQuotaHarnessRespectsEmptySupportedLimitIDs(t *testing.T) {
	status := harnesses.QuotaStatus{
		Source: "synthetic",
		State:  harnesses.QuotaUnavailable,
	}
	h := harnesstest.NewSyntheticQuotaHarness("synthetic-empty-limits", status, nil)
	harnesstest.RunQuotaHarnessConformance(t, h)
}

// TestSyntheticAccountHarnessConformance runs the AccountHarness
// suite against the in-memory synthetic implementation.
func TestSyntheticAccountHarnessConformance(t *testing.T) {
	snapshot := harnesses.AccountSnapshot{
		Authenticated: true,
		Email:         "syn@example.test",
		PlanType:      "synthetic",
		Source:        "synthetic",
		CapturedAt:    time.Now(),
		Fresh:         true,
	}
	h := harnesstest.NewSyntheticAccountHarness("synthetic-account", snapshot, 7*24*time.Hour)
	harnesstest.RunAccountHarnessConformance(t, h)
}

// TestSyntheticModelDiscoveryHarnessConformance runs the
// ModelDiscoveryHarness suite against the in-memory synthetic
// implementation with a populated alias set.
func TestSyntheticModelDiscoveryHarnessConformance(t *testing.T) {
	snapshot := harnesses.ModelDiscoverySnapshot{
		CapturedAt: time.Now(),
		Models:     []string{"synthetic-large", "synthetic-small"},
		Source:     "synthetic",
	}
	h := harnesstest.NewSyntheticModelDiscoveryHarness("synthetic-models", snapshot, []string{"large", "small"})
	harnesstest.RunModelDiscoveryHarnessConformance(t, h)
}

// TestSyntheticModelDiscoveryHarnessEmptyAliases covers the
// opencode/pi-style branch where SupportedAliases is empty; the
// positive path is skipped but the negative path still runs.
func TestSyntheticModelDiscoveryHarnessEmptyAliases(t *testing.T) {
	snapshot := harnesses.ModelDiscoverySnapshot{
		CapturedAt: time.Now(),
		Models:     []string{"only-model"},
		Source:     "synthetic",
	}
	h := harnesstest.NewSyntheticModelDiscoveryHarness("synthetic-no-aliases", snapshot, nil)
	harnesstest.RunModelDiscoveryHarnessConformance(t, h)
}

// TestSyntheticModelDiscoveryHarnessNegativePath asserts the
// out-of-set family path independently of the conformance suite.
func TestSyntheticModelDiscoveryHarnessNegativePath(t *testing.T) {
	snapshot := harnesses.ModelDiscoverySnapshot{Models: []string{"synthetic-x"}}
	h := harnesstest.NewSyntheticModelDiscoveryHarness("synthetic-neg", snapshot, []string{"x"})
	_, err := h.ResolveModelAlias("not-in-set", snapshot)
	if !errors.Is(err, harnesses.ErrAliasNotResolvable) {
		t.Fatalf("ResolveModelAlias(out-of-set) = %v, want ErrAliasNotResolvable", err)
	}
}
