package fizeau

import (
	"time"

	"github.com/DocumentDrivenDX/fizeau/internal/quota"
)

// ProviderQuotaState is the state of one provider in the quota state machine.
type ProviderQuotaState string

const (
	// ProviderQuotaStateAvailable means the provider has no known quota
	// exhaustion and is eligible for routing.
	ProviderQuotaStateAvailable ProviderQuotaState = "available"
	// ProviderQuotaStateQuotaExhausted means the provider returned a quota
	// signal (e.g. a 429 with Retry-After) and should be excluded from
	// routing until RetryAfter has elapsed.
	ProviderQuotaStateQuotaExhausted ProviderQuotaState = "quota_exhausted"
)

// ProviderQuotaStateStore is the per-provider quota state machine.
//
// Transitions:
//
//	available --MarkQuotaExhausted--> quota_exhausted
//	quota_exhausted --MarkAvailable--> available
//	quota_exhausted --(time passes RetryAfter)--> available
//
// The store is safe for concurrent use.
type ProviderQuotaStateStore struct {
	inner *quota.StateStore
}

// NewProviderQuotaStateStore returns an empty store. Every provider is
// implicitly available until MarkQuotaExhausted is called.
func NewProviderQuotaStateStore() *ProviderQuotaStateStore {
	return &ProviderQuotaStateStore{inner: quota.NewStateStore()}
}

// MarkQuotaExhausted transitions provider into quota_exhausted with the given
// retry_after. A zero or past retryAfter is normalized to "available" since
// there is no future window to exclude.
func (s *ProviderQuotaStateStore) MarkQuotaExhausted(provider string, retryAfter time.Time) {
	s.innerStore().MarkQuotaExhausted(provider, retryAfter)
}

// MarkAvailable forces provider back to available, dropping any pending
// retry_after. Use when an explicit recovery signal arrives (probe success,
// fresh quota window, operator override).
func (s *ProviderQuotaStateStore) MarkAvailable(provider string) {
	s.innerStore().MarkAvailable(provider)
}

// State returns the effective state of provider at the given instant. The
// quota_exhausted state auto-decays to available once now >= retry_after.
func (s *ProviderQuotaStateStore) State(provider string, now time.Time) (ProviderQuotaState, time.Time) {
	state, retryAfter := s.innerStore().State(provider, now)
	return ProviderQuotaState(state), retryAfter
}

// AllExhausted returns provider→retry_after for every provider whose entry is
// currently in quota_exhausted state, regardless of whether retry_after has
// elapsed. Unlike ExhaustedAt, entries past their retry_after are still
// reported here; the recovery probe loop uses this to find providers that are
// due for re-probing before the auto-decay in State() reveals them as
// available.
func (s *ProviderQuotaStateStore) AllExhausted() map[string]time.Time {
	return s.innerStore().AllExhausted()
}

// ExhaustedAt returns provider→retry_after for every provider currently in
// quota_exhausted state with retry_after > now. The returned map is a copy
// safe for the caller to mutate.
func (s *ProviderQuotaStateStore) ExhaustedAt(now time.Time) map[string]time.Time {
	return s.innerStore().ExhaustedAt(now)
}

func (s *ProviderQuotaStateStore) innerStore() *quota.StateStore {
	if s == nil {
		return nil
	}
	return s.inner
}
