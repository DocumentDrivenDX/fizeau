package routehealth

import (
	"testing"
	"time"

	"github.com/easel/fizeau/internal/routing"
)

func TestStoreRecordAttemptSuccessClearsMatchingFailure(t *testing.T) {
	store := NewStore()
	now := time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC)
	if err := store.RecordAttempt(Attempt{
		Harness:   "fiz",
		Provider:  "bragi",
		Model:     "qwen",
		Status:    "failed",
		Timestamp: now,
	}); err != nil {
		t.Fatalf("RecordAttempt failure: %v", err)
	}
	if got := store.ActiveAttempts(now, time.Minute); len(got) != 1 {
		t.Fatalf("active failures = %d, want 1", len(got))
	}

	if err := store.RecordAttempt(Attempt{
		Harness:   "fiz",
		Provider:  "bragi",
		Model:     "qwen",
		Status:    "success",
		Timestamp: now.Add(time.Second),
	}); err != nil {
		t.Fatalf("RecordAttempt success: %v", err)
	}
	if got := store.ActiveAttempts(now.Add(time.Second), time.Minute); len(got) != 0 {
		t.Fatalf("active failures = %d, want 0: %+v", len(got), got)
	}
}

func TestStoreActiveAttemptsExpiresFailures(t *testing.T) {
	store := NewStore()
	now := time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC)
	if err := store.RecordAttempt(Attempt{
		Harness:   "fiz",
		Provider:  "bragi",
		Model:     "qwen",
		Status:    "failed",
		Timestamp: now.Add(-time.Hour),
	}); err != nil {
		t.Fatalf("RecordAttempt: %v", err)
	}
	if got := store.ActiveAttempts(now, time.Minute); len(got) != 0 {
		t.Fatalf("active failures = %d, want 0 after TTL expiry", len(got))
	}
}

func TestStoreMetricSignalsAreProviderModelKeyed(t *testing.T) {
	store := NewStore()
	now := time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC)
	keyX := routing.ProviderModelKey("providerA", "", "modelX")
	keyY := routing.ProviderModelKey("providerA", "", "modelY")

	for i := 0; i < 3; i++ {
		if err := store.RecordAttempt(Attempt{
			Harness:   "fiz",
			Provider:  "providerA",
			Model:     "modelX",
			Status:    "failed",
			Duration:  100 * time.Millisecond,
			Timestamp: now,
		}); err != nil {
			t.Fatalf("RecordAttempt failure %d: %v", i, err)
		}
	}
	if err := store.RecordAttempt(Attempt{
		Harness:   "fiz",
		Provider:  "providerA",
		Model:     "modelY",
		Status:    "success",
		Duration:  50 * time.Millisecond,
		Timestamp: now,
	}); err != nil {
		t.Fatalf("RecordAttempt success: %v", err)
	}

	successRate, latencyMS := store.MetricSignals(now, time.Minute)
	if got, want := successRate[keyX], 0.0; got != want {
		t.Fatalf("successRate[%s] = %v, want %v", keyX, got, want)
	}
	if got, want := successRate[keyY], 1.0; got != want {
		t.Fatalf("successRate[%s] = %v, want %v", keyY, got, want)
	}
	if _, ok := latencyMS[keyY]; !ok {
		t.Fatalf("latencyMS[%s] missing", keyY)
	}
}
