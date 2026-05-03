package fizeau

import (
	"sync"
	"time"
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
	mu      sync.RWMutex
	entries map[string]providerQuotaEntry
}

type providerQuotaEntry struct {
	state      ProviderQuotaState
	retryAfter time.Time
}

// NewProviderQuotaStateStore returns an empty store. Every provider is
// implicitly available until MarkQuotaExhausted is called.
func NewProviderQuotaStateStore() *ProviderQuotaStateStore {
	return &ProviderQuotaStateStore{entries: make(map[string]providerQuotaEntry)}
}

// MarkQuotaExhausted transitions provider into quota_exhausted with the given
// retry_after. A zero or past retryAfter is normalized to "available" since
// there is no future window to exclude.
func (s *ProviderQuotaStateStore) MarkQuotaExhausted(provider string, retryAfter time.Time) {
	if s == nil || provider == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.entries == nil {
		s.entries = make(map[string]providerQuotaEntry)
	}
	if retryAfter.IsZero() {
		delete(s.entries, provider)
		return
	}
	s.entries[provider] = providerQuotaEntry{
		state:      ProviderQuotaStateQuotaExhausted,
		retryAfter: retryAfter,
	}
}

// MarkAvailable forces provider back to available, dropping any pending
// retry_after. Use when an explicit recovery signal arrives (probe success,
// fresh quota window, operator override).
func (s *ProviderQuotaStateStore) MarkAvailable(provider string) {
	if s == nil || provider == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.entries, provider)
}

// State returns the effective state of provider at the given instant. The
// quota_exhausted state auto-decays to available once now >= retry_after.
func (s *ProviderQuotaStateStore) State(provider string, now time.Time) (ProviderQuotaState, time.Time) {
	if s == nil || provider == "" {
		return ProviderQuotaStateAvailable, time.Time{}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	entry, ok := s.entries[provider]
	if !ok {
		return ProviderQuotaStateAvailable, time.Time{}
	}
	if !entry.retryAfter.After(now) {
		return ProviderQuotaStateAvailable, time.Time{}
	}
	return entry.state, entry.retryAfter
}

// ExhaustedAt returns provider→retry_after for every provider currently in
// quota_exhausted state with retry_after > now. The returned map is a copy
// safe for the caller to mutate.
func (s *ProviderQuotaStateStore) ExhaustedAt(now time.Time) map[string]time.Time {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.entries) == 0 {
		return nil
	}
	out := make(map[string]time.Time, len(s.entries))
	for name, entry := range s.entries {
		if entry.state != ProviderQuotaStateQuotaExhausted {
			continue
		}
		if !entry.retryAfter.After(now) {
			continue
		}
		out[name] = entry.retryAfter
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
