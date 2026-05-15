package fizeau

import (
	"testing"
	"time"

	"github.com/easel/fizeau/internal/provider/quotaheaders"
)

func TestQuotaSignalObserver_MarksProviderExhaustedOnZeroRemaining(t *testing.T) {
	svc := &service{providerQuota: NewProviderQuotaStateStore()}
	observer := svc.quotaSignalObserver("openai")
	if observer == nil {
		t.Fatal("expected non-nil observer when service has a quota store")
	}

	now := time.Now()
	signal := quotaheaders.Signal{
		Present:           true,
		RemainingRequests: 0,
		RemainingTokens:   1000,
		ResetTime:         now.Add(15 * time.Minute),
	}
	observer(signal)

	state, retryAt := svc.providerQuota.State("openai", now)
	if state != ProviderQuotaStateQuotaExhausted {
		t.Fatalf("provider state = %q, want quota_exhausted", state)
	}
	if !retryAt.Equal(signal.ResetTime) {
		t.Errorf("retryAt = %v, want %v", retryAt, signal.ResetTime)
	}
}

func TestQuotaSignalObserver_RetryAfterDrivesExhaustion(t *testing.T) {
	svc := &service{providerQuota: NewProviderQuotaStateStore()}
	observer := svc.quotaSignalObserver("anthropic")

	signal := quotaheaders.Signal{
		Present:           true,
		RemainingRequests: 50,
		RemainingTokens:   50000,
		RetryAfter:        90 * time.Second,
	}
	observer(signal)

	now := time.Now()
	state, retryAt := svc.providerQuota.State("anthropic", now)
	if state != ProviderQuotaStateQuotaExhausted {
		t.Fatalf("state = %q, want quota_exhausted (Retry-After should override remaining)", state)
	}
	// retryAt should be roughly now + 90s; allow generous slack to avoid flakes.
	if retryAt.Before(now.Add(60*time.Second)) || retryAt.After(now.Add(120*time.Second)) {
		t.Errorf("retryAt = %v, expected ~now+90s", retryAt)
	}
}

func TestQuotaSignalObserver_PlentyRemainingNoOp(t *testing.T) {
	svc := &service{providerQuota: NewProviderQuotaStateStore()}
	observer := svc.quotaSignalObserver("openrouter")

	signal := quotaheaders.Signal{
		Present:           true,
		RemainingRequests: 800,
		RemainingTokens:   -1,
		ResetTime:         time.Now().Add(time.Hour),
	}
	observer(signal)

	state, _ := svc.providerQuota.State("openrouter", time.Now())
	if state != ProviderQuotaStateAvailable {
		t.Errorf("plenty-remaining response should leave state available, got %q", state)
	}
}

func TestQuotaSignalObserver_NilStoreReturnsNil(t *testing.T) {
	svc := &service{}
	if svc.quotaSignalObserver("openai") != nil {
		t.Error("observer must be nil when service has no quota store")
	}
}
