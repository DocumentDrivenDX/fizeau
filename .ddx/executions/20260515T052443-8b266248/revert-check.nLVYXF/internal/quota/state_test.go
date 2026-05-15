package quota

import (
	"testing"
	"time"
)

func TestStateStoreInitialState(t *testing.T) {
	store := NewStateStore()
	state, retry := store.State("openai", time.Now())
	if state != StateAvailable {
		t.Fatalf("default state = %q, want available", state)
	}
	if !retry.IsZero() {
		t.Fatalf("default retry = %v, want zero", retry)
	}
	if got := store.ExhaustedAt(time.Now()); got != nil {
		t.Fatalf("default ExhaustedAt = %v, want nil", got)
	}
}

func TestStateStoreMarkAndRecover(t *testing.T) {
	store := NewStateStore()
	now := time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC)
	retry := now.Add(15 * time.Minute)

	store.MarkQuotaExhausted("openai", retry)

	state, gotRetry := store.State("openai", now)
	if state != StateQuotaExhausted {
		t.Fatalf("state = %q, want quota_exhausted", state)
	}
	if !gotRetry.Equal(retry) {
		t.Fatalf("retry = %v, want %v", gotRetry, retry)
	}

	state2, gotRetry2 := store.State("openai", retry.Add(time.Second))
	if state2 != StateAvailable {
		t.Fatalf("post-retry state = %q, want available (auto-decay)", state2)
	}
	if !gotRetry2.IsZero() {
		t.Fatalf("post-retry retry = %v, want zero", gotRetry2)
	}

	store.MarkQuotaExhausted("openai", retry)
	store.MarkAvailable("openai")
	state3, _ := store.State("openai", now)
	if state3 != StateAvailable {
		t.Fatalf("after MarkAvailable state = %q, want available", state3)
	}
}

func TestStateStoreExhaustedAt(t *testing.T) {
	store := NewStateStore()
	now := time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC)
	store.MarkQuotaExhausted("openai", now.Add(5*time.Minute))
	store.MarkQuotaExhausted("openrouter", now.Add(20*time.Minute))
	store.MarkQuotaExhausted("anthropic", now.Add(-time.Minute))

	got := store.ExhaustedAt(now)
	if len(got) != 2 {
		t.Fatalf("ExhaustedAt size = %d, want 2 (anthropic auto-decayed): %v", len(got), got)
	}
	if _, ok := got["openai"]; !ok {
		t.Fatalf("missing openai in exhausted set: %v", got)
	}
	if _, ok := got["openrouter"]; !ok {
		t.Fatalf("missing openrouter in exhausted set: %v", got)
	}
	if _, ok := got["anthropic"]; ok {
		t.Fatalf("anthropic should have auto-decayed: %v", got)
	}

	got["openai"] = time.Time{}
	state, _ := store.State("openai", now)
	if state != StateQuotaExhausted {
		t.Fatalf("store mutated by external map write")
	}
}

func TestStateStoreZeroRetryNormalizes(t *testing.T) {
	store := NewStateStore()
	store.MarkQuotaExhausted("openai", time.Time{})
	state, _ := store.State("openai", time.Now())
	if state != StateAvailable {
		t.Fatalf("zero retry_after should normalize to available, got %q", state)
	}
}

func TestStateStoreAllExhaustedIncludesElapsed(t *testing.T) {
	store := NewStateStore()
	now := time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC)
	store.MarkQuotaExhausted("openai", now.Add(5*time.Minute))
	store.MarkQuotaExhausted("openrouter", now.Add(-time.Minute))

	got := store.AllExhausted()
	if len(got) != 2 {
		t.Fatalf("AllExhausted size = %d, want 2 (elapsed entry must remain visible): %v", len(got), got)
	}
	if _, ok := got["openrouter"]; !ok {
		t.Fatalf("AllExhausted must include providers whose retry_after has elapsed: %v", got)
	}

	got["openai"] = time.Time{}
	state, _ := store.State("openai", now)
	if state != StateQuotaExhausted {
		t.Fatalf("store mutated by external map write")
	}
}

func TestStateStoreNilReceiverSafe(t *testing.T) {
	var store *StateStore
	store.MarkQuotaExhausted("x", time.Now())
	store.MarkAvailable("x")
	if state, _ := store.State("x", time.Now()); state != StateAvailable {
		t.Fatalf("nil receiver state = %q, want available", state)
	}
	if got := store.ExhaustedAt(time.Now()); got != nil {
		t.Fatalf("nil receiver ExhaustedAt = %v, want nil", got)
	}
}
