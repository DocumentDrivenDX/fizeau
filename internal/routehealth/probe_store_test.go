package routehealth

import (
	"path/filepath"
	"testing"
	"time"
)

func TestProbeStore_RecordAndLastProbe(t *testing.T) {
	store := NewProbeStore()
	now := time.Date(2026, 5, 14, 10, 0, 0, 0, time.UTC)

	store.RecordProbe("bragi", "", false, now)
	r, ok := store.LastProbe("bragi", "")
	if !ok {
		t.Fatal("expected probe record for bragi")
	}
	if r.LastProbeSuccess {
		t.Error("expected probe failure for bragi")
	}
	if !r.LastProbeAt.Equal(now) {
		t.Errorf("LastProbeAt = %v, want %v", r.LastProbeAt, now)
	}

	// Record success — overwrites
	store.RecordProbe("bragi", "", true, now.Add(time.Minute))
	r, ok = store.LastProbe("bragi", "")
	if !ok {
		t.Fatal("expected probe record for bragi after success")
	}
	if !r.LastProbeSuccess {
		t.Error("expected probe success for bragi")
	}
}

func TestProbeStore_UnreachableProviders(t *testing.T) {
	store := NewProbeStore()
	now := time.Date(2026, 5, 14, 10, 0, 0, 0, time.UTC)
	ttl := 10 * time.Minute

	store.RecordProbe("bragi", "", false, now)
	store.RecordProbe("grendel", "", false, now.Add(-15*time.Minute)) // expired
	store.RecordProbe("vidar", "", true, now)                         // reachable

	unreachable := store.UnreachableProviders(now.Add(5*time.Minute), ttl)
	if _, ok := unreachable["bragi"]; !ok {
		t.Error("expected bragi to be unreachable")
	}
	if _, ok := unreachable["grendel"]; ok {
		t.Error("expected grendel to be expired (outside TTL)")
	}
	if _, ok := unreachable["vidar"]; ok {
		t.Error("expected vidar to be reachable, not in unreachable map")
	}
}

func TestProbeStore_ProbeNeeded(t *testing.T) {
	store := NewProbeStore()
	now := time.Date(2026, 5, 14, 10, 0, 0, 0, time.UTC)
	interval := 60 * time.Second

	// Never probed → needed
	if !store.ProbeNeeded("bragi", "", now, interval) {
		t.Error("expected probe needed when never probed")
	}

	store.RecordProbe("bragi", "", true, now)

	// Just probed → not needed
	if store.ProbeNeeded("bragi", "", now.Add(30*time.Second), interval) {
		t.Error("expected probe not needed when probed recently")
	}

	// Interval elapsed → needed
	if !store.ProbeNeeded("bragi", "", now.Add(interval), interval) {
		t.Error("expected probe needed when interval elapsed")
	}
}

// TestRouteHealth_PersistsProbeResults asserts that a fresh process can read
// prior probe results within HealthSignalTTL (AC #5).
func TestRouteHealth_PersistsProbeResults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "probe_health.json")
	now := time.Date(2026, 5, 14, 10, 0, 0, 0, time.UTC)
	ttl := 10 * time.Minute

	// First process: record a failure and save.
	store1 := NewProbeStore()
	store1.RecordProbe("bragi", "", false, now)
	if err := store1.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Second process: load and verify the probe result is present.
	store2 := NewProbeStore()
	if err := store2.Load(path); err != nil {
		t.Fatalf("Load: %v", err)
	}

	unreachable := store2.UnreachableProviders(now.Add(5*time.Minute), ttl)
	if _, ok := unreachable["bragi"]; !ok {
		t.Fatal("expected bragi to be unreachable after loading persisted probe results")
	}

	// Verify probe beyond TTL is not returned.
	expired := store2.UnreachableProviders(now.Add(ttl+time.Second), ttl)
	if _, ok := expired["bragi"]; ok {
		t.Error("expected bragi to be expired beyond TTL")
	}
}

func TestProbeStore_LoadIgnoresMissingFile(t *testing.T) {
	store := NewProbeStore()
	if err := store.Load("/nonexistent/path/probe.json"); err != nil {
		t.Fatalf("Load of missing file returned error: %v", err)
	}
}

func TestProbeStore_NilSafe(t *testing.T) {
	var ps *ProbeStore
	// nil store should not panic
	if _, ok := ps.LastProbe("bragi", ""); ok {
		t.Error("expected no record from nil store")
	}
	unreachable := ps.UnreachableProviders(time.Now(), time.Minute)
	if len(unreachable) != 0 {
		t.Error("expected empty unreachable from nil store")
	}
}
