package routehealth

import (
	"strings"
	"sync"
	"time"
)

// CachedDecision is one stored route-status decision plus the timestamp when
// it was recorded.
type CachedDecision[T any] struct {
	Decision T
	At       time.Time
}

// DecisionStore retains the latest decision for each route key.
type DecisionStore[T any] struct {
	mu    sync.RWMutex
	items map[string]CachedDecision[T]
}

// NewDecisionStore returns an empty decision store.
func NewDecisionStore[T any]() *DecisionStore[T] {
	return &DecisionStore[T]{
		items: make(map[string]CachedDecision[T]),
	}
}

// Store records the latest decision for routeKey.
func (s *DecisionStore[T]) Store(routeKey string, decision T, at time.Time) {
	if s == nil {
		return
	}
	routeKey = strings.TrimSpace(routeKey)
	if routeKey == "" {
		return
	}
	if at.IsZero() {
		at = time.Now()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.items == nil {
		s.items = make(map[string]CachedDecision[T])
	}
	s.items[routeKey] = CachedDecision[T]{
		Decision: decision,
		At:       at,
	}
}

// Lookup returns the cached decision for routeKey when present.
func (s *DecisionStore[T]) Lookup(routeKey string) (CachedDecision[T], bool) {
	if s == nil {
		return CachedDecision[T]{}, false
	}
	routeKey = strings.TrimSpace(routeKey)
	if routeKey == "" {
		return CachedDecision[T]{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.items == nil {
		return CachedDecision[T]{}, false
	}
	decision, ok := s.items[routeKey]
	return decision, ok
}
