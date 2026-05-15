package runtimesignals

import (
	"sync"

	"github.com/easel/fizeau/internal/provider/quotaheaders"
)

// headerStore holds the most recently observed rate-limit signal per provider.
// Signals are sourced from HTTP response headers parsed by the quotaheaders
// package (extends commit 7776890e quota_exhausted path).
type headerStore struct {
	mu      sync.RWMutex
	signals map[string]quotaheaders.Signal
}

func newHeaderStore() *headerStore {
	return &headerStore{signals: make(map[string]quotaheaders.Signal)}
}

// record stores the signal for provider. Signals where Present is false are
// silently dropped so that responses without rate-limit headers do not clear
// a previously recorded signal.
func (s *headerStore) record(provider string, sig quotaheaders.Signal) {
	if !sig.Present {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.signals[provider] = sig
}

// get returns the last recorded signal for provider. The second return value
// is false when no signal has been recorded yet.
func (s *headerStore) get(provider string) (quotaheaders.Signal, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sig, ok := s.signals[provider]
	return sig, ok
}
