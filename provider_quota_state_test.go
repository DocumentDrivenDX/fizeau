package fizeau

import (
	"testing"
	"time"
)

func TestProviderQuotaStateStoreInitialState(t *testing.T) {
	store := NewProviderQuotaStateStore()
	state, retry := store.State("openai", time.Now())
	if state != ProviderQuotaStateAvailable {
		t.Fatalf("default state = %q, want available", state)
	}
	if !retry.IsZero() {
		t.Fatalf("default retry = %v, want zero", retry)
	}
	if got := store.ExhaustedAt(time.Now()); got != nil {
		t.Fatalf("default ExhaustedAt = %v, want nil", got)
	}
}

func TestProviderQuotaStateStoreMarkAndRecover(t *testing.T) {
	store := NewProviderQuotaStateStore()
	now := time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC)
	retry := now.Add(15 * time.Minute)

	store.MarkQuotaExhausted("openai", retry)

	state, gotRetry := store.State("openai", now)
	if state != ProviderQuotaStateQuotaExhausted {
		t.Fatalf("state = %q, want quota_exhausted", state)
	}
	if !gotRetry.Equal(retry) {
		t.Fatalf("retry = %v, want %v", gotRetry, retry)
	}

	// Auto-decay once now passes retry_after.
	state2, gotRetry2 := store.State("openai", retry.Add(time.Second))
	if state2 != ProviderQuotaStateAvailable {
		t.Fatalf("post-retry state = %q, want available (auto-decay)", state2)
	}
	if !gotRetry2.IsZero() {
		t.Fatalf("post-retry retry = %v, want zero", gotRetry2)
	}

	// Explicit MarkAvailable also clears.
	store.MarkQuotaExhausted("openai", retry)
	store.MarkAvailable("openai")
	state3, _ := store.State("openai", now)
	if state3 != ProviderQuotaStateAvailable {
		t.Fatalf("after MarkAvailable state = %q, want available", state3)
	}
}

func TestProviderQuotaStateStoreExhaustedAt(t *testing.T) {
	store := NewProviderQuotaStateStore()
	now := time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC)
	store.MarkQuotaExhausted("openai", now.Add(5*time.Minute))
	store.MarkQuotaExhausted("openrouter", now.Add(20*time.Minute))
	// Past retry — should be filtered out.
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

	// Mutating returned map must not affect store.
	got["openai"] = time.Time{}
	state, _ := store.State("openai", now)
	if state != ProviderQuotaStateQuotaExhausted {
		t.Fatalf("store mutated by external map write")
	}
}

func TestProviderQuotaStateStoreZeroRetryNormalizes(t *testing.T) {
	store := NewProviderQuotaStateStore()
	store.MarkQuotaExhausted("openai", time.Time{})
	state, _ := store.State("openai", time.Now())
	if state != ProviderQuotaStateAvailable {
		t.Fatalf("zero retry_after should normalize to available, got %q", state)
	}
}

func TestProviderQuotaStateStoreNilReceiverSafe(t *testing.T) {
	var store *ProviderQuotaStateStore
	store.MarkQuotaExhausted("x", time.Now())
	store.MarkAvailable("x")
	if state, _ := store.State("x", time.Now()); state != ProviderQuotaStateAvailable {
		t.Fatalf("nil receiver state = %q, want available", state)
	}
	if got := store.ExhaustedAt(time.Now()); got != nil {
		t.Fatalf("nil receiver ExhaustedAt = %v, want nil", got)
	}
}
