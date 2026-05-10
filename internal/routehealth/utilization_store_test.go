package routehealth

import (
	"testing"

	"github.com/easel/fizeau/internal/provider/utilization"
)

func TestUtilizationStoreEndpointLoadsUsesFreshSamplesAndLeaseFallback(t *testing.T) {
	store := NewUtilizationStore()
	store.Record("local", "desk-a", "model-a", utilization.EndpointUtilization{
		ActiveRequests: utilization.Int(1),
		QueuedRequests: utilization.Int(1),
		MaxConcurrency: utilization.Int(4),
		Source:         utilization.SourceVLLMMetrics,
		Freshness:      utilization.FreshnessFresh,
	})
	store.Record("local", "desk-b", "model-a", utilization.EndpointUtilization{
		ActiveRequests: utilization.Int(9),
		QueuedRequests: utilization.Int(9),
		MaxConcurrency: utilization.Int(10),
		Source:         utilization.SourceLlamaMetrics,
		Freshness:      utilization.FreshnessStale,
	})

	loads := store.EndpointLoads("local", "model-a", map[string]int{
		"desk-a": 2,
		"desk-b": 1,
	})

	if got := loads["desk-a"]; !got.UtilizationFresh {
		t.Fatalf("desk-a fresh=%v, want true", got.UtilizationFresh)
	} else if got.UtilizationSaturated {
		t.Fatalf("desk-a saturated=%v, want false", got.UtilizationSaturated)
	} else if got.NormalizedLoad != 2.5 {
		t.Fatalf("desk-a normalized_load=%v, want 2.5", got.NormalizedLoad)
	}

	if got := loads["desk-b"]; got.UtilizationFresh {
		t.Fatalf("desk-b fresh=%v, want false for stale sample", got.UtilizationFresh)
	} else if got.NormalizedLoad != 1 {
		t.Fatalf("desk-b normalized_load=%v, want lease-count fallback 1", got.NormalizedLoad)
	}
}

func TestUtilizationStoreEndpointLoadsMarksSaturationFromFreshCapacity(t *testing.T) {
	store := NewUtilizationStore()
	store.Record("local", "desk-a", "model-a", utilization.EndpointUtilization{
		ActiveRequests: utilization.Int(2),
		QueuedRequests: utilization.Int(0),
		MaxConcurrency: utilization.Int(2),
		Source:         utilization.SourceVLLMMetrics,
		Freshness:      utilization.FreshnessFresh,
	})

	loads := store.EndpointLoads("local", "model-a", map[string]int{
		"desk-a": 0,
	})
	got := loads["desk-a"]
	if !got.UtilizationSaturated {
		t.Fatalf("desk-a saturated=%v, want true", got.UtilizationSaturated)
	}
	if got.NormalizedLoad != 1 {
		t.Fatalf("desk-a normalized_load=%v, want 1", got.NormalizedLoad)
	}
}
