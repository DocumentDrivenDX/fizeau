package routehealth

import (
	"sort"
	"strings"
	"sync"
	"time"
)

// DefaultLeaseTTL bounds how long an in-process sticky route lease may live
// without being refreshed.
const DefaultLeaseTTL = 5 * time.Minute

// LeaseKey identifies one sticky lease scope.
type LeaseKey struct {
	StickyKey string
}

// Lease describes one sticky endpoint assignment.
type Lease struct {
	Key         LeaseKey
	Provider    string
	Endpoint    string
	Model       string
	AcquiredAt  time.Time
	RefreshedAt time.Time
	ExpiresAt   time.Time
}

// LeaseInvalidation records why a lease was removed.
type LeaseInvalidation struct {
	Key        LeaseKey
	Provider   string
	Endpoint   string
	Model      string
	Reason     string
	RecordedAt time.Time
}

// LeaseStore owns in-process sticky route assignments. It is safe for
// concurrent use.
type LeaseStore struct {
	mu            sync.Mutex
	leases        map[LeaseKey]Lease
	invalidations map[LeaseKey]LeaseInvalidation
}

// NewLeaseStore returns an empty lease store.
func NewLeaseStore() *LeaseStore {
	return &LeaseStore{
		leases:        make(map[LeaseKey]Lease),
		invalidations: make(map[LeaseKey]LeaseInvalidation),
	}
}

// NormalizeLeaseKey trims whitespace from the sticky key.
func NormalizeLeaseKey(stickyKey string) LeaseKey {
	return LeaseKey{StickyKey: strings.TrimSpace(stickyKey)}
}

// Live returns the current lease for key when it has not expired.
func (s *LeaseStore) Live(now time.Time, key LeaseKey) (Lease, bool) {
	if s == nil {
		return Lease{}, false
	}
	now = normalizeLeaseNow(now)

	s.mu.Lock()
	defer s.mu.Unlock()
	s.expireLocked(now)
	lease, ok := s.leases[key]
	if !ok {
		return Lease{}, false
	}
	return lease, true
}

// Acquire stores or refreshes a lease for key.
func (s *LeaseStore) Acquire(now time.Time, ttl time.Duration, key LeaseKey, provider, endpoint, model string) Lease {
	if s == nil {
		return Lease{}
	}
	now = normalizeLeaseNow(now)
	if ttl <= 0 {
		ttl = DefaultLeaseTTL
	}
	key = NormalizeLeaseKey(key.StickyKey)
	provider = strings.TrimSpace(provider)
	endpoint = strings.TrimSpace(endpoint)
	model = strings.TrimSpace(model)

	s.mu.Lock()
	defer s.mu.Unlock()
	s.expireLocked(now)

	lease := Lease{
		Key:         key,
		Provider:    provider,
		Endpoint:    endpoint,
		Model:       model,
		AcquiredAt:  now,
		RefreshedAt: now,
		ExpiresAt:   now.Add(ttl),
	}
	if existing, ok := s.leases[key]; ok {
		if existing.Provider == provider && existing.Endpoint == endpoint && existing.Model == model {
			lease.AcquiredAt = existing.AcquiredAt
		}
	}
	s.leases[key] = lease
	delete(s.invalidations, key)
	return lease
}

// Invalidate removes the lease for key and records the reason.
func (s *LeaseStore) Invalidate(now time.Time, key LeaseKey, reason string) (LeaseInvalidation, bool) {
	if s == nil {
		return LeaseInvalidation{}, false
	}
	now = normalizeLeaseNow(now)
	key = NormalizeLeaseKey(key.StickyKey)

	s.mu.Lock()
	defer s.mu.Unlock()
	s.expireLocked(now)
	lease, ok := s.leases[key]
	if !ok {
		return LeaseInvalidation{}, false
	}
	delete(s.leases, key)
	invalidation := LeaseInvalidation{
		Key:        key,
		Provider:   lease.Provider,
		Endpoint:   lease.Endpoint,
		Model:      lease.Model,
		Reason:     strings.TrimSpace(reason),
		RecordedAt: now,
	}
	s.invalidations[key] = invalidation
	return invalidation, true
}

// InvalidateEndpoint removes every lease for the provider/model scope that
// points at the named endpoint and records the supplied reason.
func (s *LeaseStore) InvalidateEndpoint(now time.Time, provider, endpoint, model, reason string) []LeaseInvalidation {
	if s == nil {
		return nil
	}
	now = normalizeLeaseNow(now)
	provider = strings.TrimSpace(provider)
	endpoint = strings.TrimSpace(endpoint)
	model = strings.TrimSpace(model)

	s.mu.Lock()
	defer s.mu.Unlock()
	s.expireLocked(now)
	if len(s.leases) == 0 {
		return nil
	}
	var out []LeaseInvalidation
	for key, lease := range s.leases {
		if provider != "" && lease.Provider != provider {
			continue
		}
		if endpoint != "" && lease.Endpoint != endpoint {
			continue
		}
		if model != "" && lease.Model != model {
			continue
		}
		delete(s.leases, key)
		invalidation := LeaseInvalidation{
			Key:        key,
			Provider:   lease.Provider,
			Endpoint:   lease.Endpoint,
			Model:      lease.Model,
			Reason:     strings.TrimSpace(reason),
			RecordedAt: now,
		}
		s.invalidations[key] = invalidation
		out = append(out, invalidation)
	}
	return out
}

// LiveByScope returns the live leases for provider/model, ordered by sticky
// key for deterministic tests.
func (s *LeaseStore) LiveByScope(now time.Time, provider, model string) []Lease {
	if s == nil {
		return nil
	}
	now = normalizeLeaseNow(now)
	provider = strings.TrimSpace(provider)
	model = strings.TrimSpace(model)

	s.mu.Lock()
	defer s.mu.Unlock()
	s.expireLocked(now)
	if len(s.leases) == 0 {
		return nil
	}
	out := make([]Lease, 0, len(s.leases))
	for _, lease := range s.leases {
		if provider != "" && lease.Provider != provider {
			continue
		}
		if model != "" && lease.Model != model {
			continue
		}
		out = append(out, lease)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Endpoint != out[j].Endpoint {
			return out[i].Endpoint < out[j].Endpoint
		}
		return out[i].Key.StickyKey < out[j].Key.StickyKey
	})
	return out
}

// LeaseCounts returns the live lease count per endpoint for provider/model.
func (s *LeaseStore) LeaseCounts(now time.Time, provider, model string) map[string]int {
	leases := s.LiveByScope(now, provider, model)
	if len(leases) == 0 {
		return nil
	}
	counts := make(map[string]int, len(leases))
	for _, lease := range leases {
		counts[lease.Endpoint]++
	}
	return counts
}

// LastInvalidation returns the most recent invalidation recorded for key.
func (s *LeaseStore) LastInvalidation(key LeaseKey) (LeaseInvalidation, bool) {
	if s == nil {
		return LeaseInvalidation{}, false
	}
	key = NormalizeLeaseKey(key.StickyKey)

	s.mu.Lock()
	defer s.mu.Unlock()
	invalidation, ok := s.invalidations[key]
	return invalidation, ok
}

func (s *LeaseStore) expireLocked(now time.Time) {
	for key, lease := range s.leases {
		if lease.ExpiresAt.IsZero() || lease.ExpiresAt.After(now) {
			continue
		}
		delete(s.leases, key)
		s.invalidations[key] = LeaseInvalidation{
			Key:        key,
			Provider:   lease.Provider,
			Endpoint:   lease.Endpoint,
			Model:      lease.Model,
			Reason:     "expired",
			RecordedAt: now,
		}
	}
}

func normalizeLeaseNow(now time.Time) time.Time {
	if now.IsZero() {
		return time.Now().UTC()
	}
	return now.UTC()
}
