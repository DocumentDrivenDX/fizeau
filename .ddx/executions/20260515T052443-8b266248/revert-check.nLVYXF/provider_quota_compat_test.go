package fizeau

import (
	"testing"
	"time"
)

func TestProviderQuotaPublicWrappersRemainSourceCompatible(t *testing.T) {
	now := time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC)
	retryAt := now.Add(time.Hour)

	store := NewProviderQuotaStateStore()
	store.MarkQuotaExhausted("openai", retryAt)
	state, gotRetryAt := store.State("openai", now)
	if state != ProviderQuotaStateQuotaExhausted {
		t.Fatalf("state = %q, want quota_exhausted", state)
	}
	if !gotRetryAt.Equal(retryAt) {
		t.Fatalf("retry_at = %v, want %v", gotRetryAt, retryAt)
	}
	if got := store.ExhaustedAt(now); len(got) != 1 {
		t.Fatalf("ExhaustedAt size = %d, want 1: %v", len(got), got)
	}

	store.MarkAvailable("openai")
	state, _ = store.State("openai", now)
	if state != ProviderQuotaStateAvailable {
		t.Fatalf("state after MarkAvailable = %q, want available", state)
	}
}

func TestProviderBurnRatePublicWrapperCascadesToQuotaState(t *testing.T) {
	now := time.Date(2026, 5, 2, 12, 0, 0, 0, time.UTC)
	svc := &service{
		providerQuota:    NewProviderQuotaStateStore(),
		providerBurnRate: NewProviderBurnRateTracker(),
	}
	svc.providerBurnRate.SetBudget("openai", 100)

	svc.observeTokenUsage("openai", 200, now)

	state, retryAt := svc.providerQuota.State("openai", now)
	if state != ProviderQuotaStateQuotaExhausted {
		t.Fatalf("state = %q, want quota_exhausted", state)
	}
	if retryAt.IsZero() || !retryAt.After(now) {
		t.Fatalf("retryAt = %v, want a future reset time", retryAt)
	}
}
