package routehealth

import (
	"encoding/json"
	"os"
	"sync"
	"time"

	"github.com/easel/fizeau/internal/safefs"
)

// ProbeRecord records the most recent aliveness probe result for a provider endpoint.
type ProbeRecord struct {
	Provider         string    `json:"provider"`
	Endpoint         string    `json:"endpoint,omitempty"`
	LastProbeAt      time.Time `json:"last_probe_at"`
	LastProbeSuccess bool      `json:"last_probe_success"`
}

type probeKey struct {
	Provider string
	Endpoint string
}

// ProbeStore records per-provider aliveness probe results. It is safe for concurrent use.
type ProbeStore struct {
	mu      sync.RWMutex
	records map[probeKey]ProbeRecord
}

// NewProbeStore returns an empty probe store.
func NewProbeStore() *ProbeStore {
	return &ProbeStore{records: make(map[probeKey]ProbeRecord)}
}

// RecordProbe records an aliveness probe result for a provider/endpoint pair.
// Empty endpoint matches the provider's primary endpoint.
func (ps *ProbeStore) RecordProbe(provider, endpoint string, success bool, probeAt time.Time) {
	if provider == "" {
		return
	}
	if probeAt.IsZero() {
		probeAt = time.Now()
	}
	probeAt = probeAt.UTC()
	key := probeKey{Provider: provider, Endpoint: endpoint}
	ps.mu.Lock()
	defer ps.mu.Unlock()
	ps.records[key] = ProbeRecord{
		Provider:         provider,
		Endpoint:         endpoint,
		LastProbeAt:      probeAt,
		LastProbeSuccess: success,
	}
}

// LastProbe returns the most recent probe record for a provider/endpoint pair.
func (ps *ProbeStore) LastProbe(provider, endpoint string) (ProbeRecord, bool) {
	if ps == nil {
		return ProbeRecord{}, false
	}
	key := probeKey{Provider: provider, Endpoint: endpoint}
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	r, ok := ps.records[key]
	return r, ok
}

// ProbeNeeded reports whether a provider/endpoint needs a new probe because its
// last probe is older than interval or has never been probed.
func (ps *ProbeStore) ProbeNeeded(provider, endpoint string, now time.Time, interval time.Duration) bool {
	if ps == nil {
		return false
	}
	r, ok := ps.LastProbe(provider, endpoint)
	if !ok {
		return true
	}
	return now.Sub(r.LastProbeAt) >= interval
}

// UnreachableProviders returns a map of provider name → probe time for providers
// whose most recent probe failed within ttl of now. Used to populate
// routing.Inputs.ProbeUnreachable.
func (ps *ProbeStore) UnreachableProviders(now time.Time, ttl time.Duration) map[string]time.Time {
	if ps == nil {
		return nil
	}
	if now.IsZero() {
		now = time.Now()
	}
	now = now.UTC()
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	var out map[string]time.Time
	for _, r := range ps.records {
		if r.LastProbeSuccess {
			continue
		}
		if ttl > 0 && now.Sub(r.LastProbeAt) > ttl {
			continue
		}
		if out == nil {
			out = make(map[string]time.Time)
		}
		existing, ok := out[r.Provider]
		if !ok || r.LastProbeAt.After(existing) {
			out[r.Provider] = r.LastProbeAt
		}
	}
	return out
}

type persistedProbeStore struct {
	Records []ProbeRecord `json:"records"`
}

// Save persists probe records to a JSON file at path.
func (ps *ProbeStore) Save(path string) error {
	if ps == nil || path == "" {
		return nil
	}
	ps.mu.RLock()
	records := make([]ProbeRecord, 0, len(ps.records))
	for _, r := range ps.records {
		records = append(records, r)
	}
	ps.mu.RUnlock()
	data, err := json.MarshalIndent(persistedProbeStore{Records: records}, "", "  ")
	if err != nil {
		return err
	}
	return safefs.WriteFile(path, data, 0o600)
}

// Load reads probe records from a JSON file at path. Non-existent files are silently ignored.
func (ps *ProbeStore) Load(path string) error {
	if ps == nil || path == "" {
		return nil
	}
	data, err := safefs.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var stored persistedProbeStore
	if err := json.Unmarshal(data, &stored); err != nil {
		return err
	}
	ps.mu.Lock()
	defer ps.mu.Unlock()
	for _, r := range stored.Records {
		if r.Provider == "" {
			continue
		}
		key := probeKey{Provider: r.Provider, Endpoint: r.Endpoint}
		existing, ok := ps.records[key]
		if !ok || r.LastProbeAt.After(existing.LastProbeAt) {
			ps.records[key] = r
		}
	}
	return nil
}
