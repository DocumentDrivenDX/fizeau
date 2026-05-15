package routehealth

import (
	"testing"
	"time"

	"github.com/easel/fizeau/internal/provider/utilization"
)

func TestStickyStateApplyStickyLeaseReusesLiveLease(t *testing.T) {
	state := NewStickyState()
	now := time.Date(2026, 5, 15, 14, 0, 0, 0, time.UTC)

	first := state.ApplyStickyLease(now, DefaultLeaseTTL, 250, StickyRequest{
		StickyKey:      "corr-id",
		Harness:        "fiz",
		Provider:       "local@desk-a",
		Endpoint:       "desk-a",
		ServerInstance: "desk-a",
		Model:          "model-a",
	})
	if first.Assignment != "acquired" {
		t.Fatalf("first assignment=%q want acquired", first.Assignment)
	}

	second := state.ApplyStickyLease(now.Add(time.Second), DefaultLeaseTTL, 250, StickyRequest{
		StickyKey:      "corr-id",
		Harness:        "fiz",
		Provider:       "local@desk-a",
		Endpoint:       "desk-a",
		ServerInstance: "desk-a",
		Model:          "model-a",
	})
	if second.Assignment != "reused" {
		t.Fatalf("second assignment=%q want reused", second.Assignment)
	}
	if second.Bonus != 250 {
		t.Fatalf("second bonus=%v want 250", second.Bonus)
	}
}

func TestStickyStateEndpointLoadResolverFallsBackToLeaseCounts(t *testing.T) {
	state := NewStickyState()
	now := time.Date(2026, 5, 15, 14, 0, 0, 0, time.UTC)

	state.LeaseStore().Acquire(now, DefaultLeaseTTL, NormalizeLeaseKey("a"), "local", "desk-a", "model-a")
	state.LeaseStore().Acquire(now, DefaultLeaseTTL, NormalizeLeaseKey("b"), "local", "desk-a", "model-a")
	state.UtilizationStore().Record("local", "desk-a", "model-a", utilization.EndpointUtilization{
		ActiveRequests: utilization.Int(0),
		MaxConcurrency: utilization.Int(2),
		Source:         utilization.SourceLlamaSlots,
		Freshness:      utilization.FreshnessStale,
	})

	resolve := state.EndpointLoadResolver(now.Add(time.Second))
	load, ok := resolve("local", "desk-a", "model-a")
	if !ok {
		t.Fatal("expected load resolver hit")
	}
	if load.LeaseCount != 2 {
		t.Fatalf("lease count=%d want 2", load.LeaseCount)
	}
	if load.NormalizedLoad != 2 {
		t.Fatalf("normalized load=%v want 2", load.NormalizedLoad)
	}
	if load.UtilizationFresh {
		t.Fatal("stale utilization should not be marked fresh")
	}
}
