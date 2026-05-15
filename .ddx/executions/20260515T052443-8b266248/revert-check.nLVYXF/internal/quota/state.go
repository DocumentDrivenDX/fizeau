package quota

import (
	"sync"
	"time"
)

// State is the state of one provider in the quota state machine.
type State string

const (
	// StateAvailable means the provider has no known quota exhaustion and is
	// eligible for routing.
	StateAvailable State = "available"
	// StateQuotaExhausted means the provider returned a quota signal and
	// should be excluded from routing until RetryAfter has elapsed.
	StateQuotaExhausted State = "quota_exhausted"
)

// StateStore is the per-provider quota state machine.
//
// The store is safe for concurrent use.
type StateStore struct {
	mu      sync.RWMutex
	entries map[string]entry
}

type entry struct {
	state      State
	retryAfter time.Time
}

// NewStateStore returns an empty store. Every provider is implicitly
// available until MarkQuotaExhausted is called.
func NewStateStore() *StateStore {
	return &StateStore{entries: make(map[string]entry)}
}

// MarkQuotaExhausted transitions provider into quota_exhausted with the given
// retry_after. A zero retryAfter is normalized to available.
func (s *StateStore) MarkQuotaExhausted(provider string, retryAfter time.Time) {
	if s == nil || provider == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.entries == nil {
		s.entries = make(map[string]entry)
	}
	if retryAfter.IsZero() {
		delete(s.entries, provider)
		return
	}
	s.entries[provider] = entry{
		state:      StateQuotaExhausted,
		retryAfter: retryAfter,
	}
}

// MarkAvailable forces provider back to available, dropping any pending
// retry_after.
func (s *StateStore) MarkAvailable(provider string) {
	if s == nil || provider == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.entries, provider)
}

// State returns the effective state of provider at the given instant. The
// quota_exhausted state auto-decays to available once now >= retry_after.
func (s *StateStore) State(provider string, now time.Time) (State, time.Time) {
	if s == nil || provider == "" {
		return StateAvailable, time.Time{}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	entry, ok := s.entries[provider]
	if !ok {
		return StateAvailable, time.Time{}
	}
	if !entry.retryAfter.After(now) {
		return StateAvailable, time.Time{}
	}
	return entry.state, entry.retryAfter
}

// AllExhausted returns provider->retry_after for every provider whose entry is
// currently in quota_exhausted state, including entries whose retry_after has
// elapsed. Recovery probing relies on seeing elapsed entries.
func (s *StateStore) AllExhausted() map[string]time.Time {
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
		if entry.state != StateQuotaExhausted {
			continue
		}
		out[name] = entry.retryAfter
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// ExhaustedAt returns provider->retry_after for every provider currently in
// quota_exhausted state with retry_after > now. The returned map is a copy
// safe for the caller to mutate.
func (s *StateStore) ExhaustedAt(now time.Time) map[string]time.Time {
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
		if entry.state != StateQuotaExhausted {
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
