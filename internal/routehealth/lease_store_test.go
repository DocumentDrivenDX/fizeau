package routehealth

import (
	"sync"
	"testing"
	"time"
)

func TestLeaseStoreAcquireRefreshAndExpire(t *testing.T) {
	store := NewLeaseStore()
	now := time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC)
	key := NormalizeLeaseKey("bead-1")

	first := store.Acquire(now, time.Minute, key, "fiz", "desk-a", "model-a")
	if first.AcquiredAt != now || first.RefreshedAt != now {
		t.Fatalf("first lease timestamps=%#v, want acquired/refreshed at now", first)
	}
	if first.ExpiresAt != now.Add(time.Minute) {
		t.Fatalf("first lease expires=%v, want %v", first.ExpiresAt, now.Add(time.Minute))
	}

	refreshed := store.Acquire(now.Add(10*time.Second), time.Minute, key, "fiz", "desk-a", "model-a")
	if refreshed.AcquiredAt != now {
		t.Fatalf("refreshed lease acquired_at=%v, want %v", refreshed.AcquiredAt, now)
	}
	if refreshed.RefreshedAt != now.Add(10*time.Second) {
		t.Fatalf("refreshed lease refreshed_at=%v, want %v", refreshed.RefreshedAt, now.Add(10*time.Second))
	}
	if refreshed.ExpiresAt != now.Add(70*time.Second) {
		t.Fatalf("refreshed lease expires=%v, want %v", refreshed.ExpiresAt, now.Add(70*time.Second))
	}

	if live, ok := store.Live(now.Add(30*time.Second), key); !ok || live.Endpoint != "desk-a" {
		t.Fatalf("Live returned %#v ok=%v, want desk-a", live, ok)
	}
	if live, ok := store.Live(now.Add(2*time.Minute), key); ok || live != (Lease{}) {
		t.Fatalf("expired lease still live: %#v ok=%v", live, ok)
	}
	if inv, ok := store.LastInvalidation(key); !ok || inv.Reason != "expired" {
		t.Fatalf("last invalidation=%#v ok=%v, want expired", inv, ok)
	}
}

func TestLeaseStoreInvalidateRecordsReason(t *testing.T) {
	store := NewLeaseStore()
	now := time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC)
	key := NormalizeLeaseKey("bead-2")

	store.Acquire(now, time.Minute, key, "fiz", "desk-b", "model-a")
	inv, ok := store.Invalidate(now.Add(5*time.Second), key, "endpoint disappeared")
	if !ok {
		t.Fatal("Invalidate returned false")
	}
	if inv.Reason != "endpoint disappeared" {
		t.Fatalf("invalidate reason=%q, want endpoint disappeared", inv.Reason)
	}
	if live, ok := store.Live(now.Add(5*time.Second), key); ok || live != (Lease{}) {
		t.Fatalf("invalidated lease still live: %#v ok=%v", live, ok)
	}
	if got := store.InvalidateEndpoint(now.Add(6*time.Second), "fiz", "desk-b", "model-a", "model unavailable"); len(got) != 0 {
		t.Fatalf("InvalidateEndpoint should not match removed lease, got %#v", got)
	}
	if inv2, ok := store.LastInvalidation(key); !ok || inv2.Reason != "endpoint disappeared" {
		t.Fatalf("last invalidation=%#v ok=%v, want endpoint disappeared", inv2, ok)
	}
}

func TestLeaseStoreDistinctStickyKeysCanCoexistConcurrently(t *testing.T) {
	store := NewLeaseStore()
	now := time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC)
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key := NormalizeLeaseKey(string(rune('a' + i)))
			endpoint := "desk-a"
			if i%2 == 1 {
				endpoint = "desk-b"
			}
			store.Acquire(now.Add(time.Duration(i)*time.Second), time.Minute, key, "fiz", endpoint, "model-a")
		}(i)
	}
	wg.Wait()

	leases := store.LiveByScope(now.Add(30*time.Second), "fiz", "model-a")
	if len(leases) != 32 {
		t.Fatalf("live leases=%d, want 32", len(leases))
	}
	counts := store.LeaseCounts(now.Add(30*time.Second), "fiz", "model-a")
	if counts["desk-a"] == 0 || counts["desk-b"] == 0 {
		t.Fatalf("counts=%#v, want both endpoints represented", counts)
	}
}
