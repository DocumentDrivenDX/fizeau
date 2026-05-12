package discoverycache

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func newTestCache(t *testing.T) *Cache {
	t.Helper()
	return &Cache{Root: t.TempDir()}
}

func testSource(name string, ttl, deadline time.Duration) Source {
	return Source{Tier: "discovery", Name: name, TTL: ttl, RefreshDeadline: deadline}
}

func TestReadEmpty(t *testing.T) {
	c := newTestCache(t)
	s := testSource("empty", time.Hour, 10*time.Second)
	res, err := c.Read(s)
	if err != nil {
		t.Fatal(err)
	}
	if res.Data != nil {
		t.Errorf("expected nil Data, got %d bytes", len(res.Data))
	}
	if !res.Stale {
		t.Error("expected Stale=true for empty cache")
	}
	if res.Fresh {
		t.Error("expected Fresh=false for empty cache")
	}
}

func TestReadAfterWrite(t *testing.T) {
	c := newTestCache(t)
	s := testSource("after-write", time.Hour, 10*time.Second)
	want := []byte(`{"hello":"world"}`)

	if err := c.Refresh(s, func(_ context.Context) ([]byte, error) { return want, nil }); err != nil {
		t.Fatal(err)
	}

	res, err := c.Read(s)
	if err != nil {
		t.Fatal(err)
	}
	if string(res.Data) != string(want) {
		t.Errorf("Read() = %q, want %q", res.Data, want)
	}
	if !res.Fresh {
		t.Errorf("expected Fresh=true, Age=%v TTL=%v", res.Age, s.TTL)
	}
	if res.Stale {
		t.Error("expected Stale=false after write")
	}
}

func TestReadStaleByMtime(t *testing.T) {
	c := newTestCache(t)
	s := testSource("stale-mtime", time.Hour, 10*time.Second)

	if err := c.Refresh(s, func(_ context.Context) ([]byte, error) { return []byte(`{}`), nil }); err != nil {
		t.Fatal(err)
	}
	// Backdate the file's mtime by 2h to make it stale.
	past := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(c.dataPath(s), past, past); err != nil {
		t.Fatal(err)
	}

	res, err := c.Read(s)
	if err != nil {
		t.Fatal(err)
	}
	if res.Fresh {
		t.Errorf("expected Fresh=false, Age=%v TTL=%v", res.Age, s.TTL)
	}
	if !res.Stale {
		t.Error("expected Stale=true for backdated file")
	}
	if res.Data == nil {
		t.Error("expected stale Data to be non-nil")
	}
}

func TestRefreshIdempotent(t *testing.T) {
	c := newTestCache(t)
	s := testSource("idempotent", time.Hour, 10*time.Second)
	for i := range 3 {
		if err := c.Refresh(s, func(_ context.Context) ([]byte, error) { return []byte(`{}`), nil }); err != nil {
			t.Fatalf("iter %d: %v", i, err)
		}
	}
	res, err := c.Read(s)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Fresh {
		t.Error("expected Fresh=true after repeated Refresh")
	}
}

func TestReadIsSubHundredMs(t *testing.T) {
	// AC: Cache.Read returns within 100ms p99 under no-contention baseline.
	c := newTestCache(t)
	s := testSource("perf", time.Hour, 10*time.Second)
	if err := c.Refresh(s, func(_ context.Context) ([]byte, error) { return []byte(`{}`), nil }); err != nil {
		t.Fatal(err)
	}

	var maxDuration time.Duration
	for range 200 {
		start := time.Now()
		if _, err := c.Read(s); err != nil {
			t.Fatal(err)
		}
		if d := time.Since(start); d > maxDuration {
			maxDuration = d
		}
	}
	if maxDuration > 100*time.Millisecond {
		t.Errorf("Read() max = %v, want < 100ms", maxDuration)
	}
}

func TestPruneRemovesInactive(t *testing.T) {
	c := newTestCache(t)
	active := testSource("active", time.Hour, 10*time.Second)
	inactive := testSource("old", time.Hour, 10*time.Second)

	for _, s := range []Source{active, inactive} {
		if err := c.Refresh(s, func(_ context.Context) ([]byte, error) { return []byte(`{}`), nil }); err != nil {
			t.Fatal(err)
		}
	}

	if err := c.Prune([]Source{active}); err != nil {
		t.Fatal(err)
	}

	res, _ := c.Read(active)
	if res.Data == nil {
		t.Error("active source was pruned")
	}
	res2, _ := c.Read(inactive)
	if res2.Data != nil {
		t.Error("inactive source was not pruned")
	}
}

func TestPruneSkipsActiveMarker(t *testing.T) {
	c := newTestCache(t)
	s := testSource("locked", time.Hour, 30*time.Second)
	dir := filepath.Join(c.Root, s.Tier)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := atomicWrite(c.dataPath(s), []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	m := &refreshMarker{
		PID:       os.Getpid(),
		StartedAt: time.Now().UTC(),
		Deadline:  time.Now().UTC().Add(30 * time.Second),
	}
	if err := writeMarker(c.markerPath(s), m); err != nil {
		t.Fatal(err)
	}

	if err := c.Prune(nil); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(c.dataPath(s)); os.IsNotExist(err) {
		t.Error("Prune removed data file of actively-refreshing source")
	}
}
