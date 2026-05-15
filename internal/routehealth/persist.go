package routehealth

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/easel/fizeau/internal/safefs"
)

const persistedRouteHealthVersion = 1

type persistedRouteHealthSnapshot struct {
	Version  int                           `json:"version"`
	Failures []persistedRouteHealthFailure `json:"failures,omitempty"`
	Metrics  []persistedRouteHealthMetric  `json:"metrics,omitempty"`
	Probes   []ProbeRecord                 `json:"probes,omitempty"`
}

type persistedRouteHealthFailure struct {
	Key        Key           `json:"key"`
	Status     string        `json:"status,omitempty"`
	Reason     string        `json:"reason,omitempty"`
	Error      string        `json:"error,omitempty"`
	Duration   time.Duration `json:"duration,omitempty"`
	RecordedAt time.Time     `json:"recorded_at"`
}

type persistedRouteHealthMetric struct {
	Key           Key           `json:"key"`
	Attempts      int           `json:"attempts"`
	Successes     int           `json:"successes"`
	TotalDuration time.Duration `json:"total_duration,omitempty"`
	RecordedAt    time.Time     `json:"recorded_at"`
}

type rawPersistedRouteHealthSnapshot struct {
	Version  int                           `json:"version"`
	Failures []persistedRouteHealthFailure `json:"failures"`
	Metrics  []persistedRouteHealthMetric  `json:"metrics"`
	Probes   []ProbeRecord                 `json:"probes"`
	Records  json.RawMessage               `json:"records"`
}

// LoadPersistedState reads a persisted route-health snapshot and hydrates the
// supplied stores. Signals older than ttl are dropped on load.
func LoadPersistedState(path string, ttl time.Duration, store *Store, probes *ProbeStore) error {
	if path == "" {
		return nil
	}
	snapshot, err := readPersistedRouteHealthSnapshot(path)
	if err != nil {
		return err
	}
	if store != nil {
		store.loadPersistedState(snapshot, ttl, time.Now().UTC())
	}
	if probes != nil {
		probes.loadPersistedState(snapshot)
	}
	return nil
}

// SavePersistedState writes a persisted route-health snapshot for the supplied
// stores. When only one store is supplied, any compatible existing section in
// the file is preserved.
func SavePersistedState(path string, store *Store, probes *ProbeStore) error {
	if path == "" {
		return nil
	}
	snapshot := persistedRouteHealthSnapshot{Version: persistedRouteHealthVersion}
	if existing, err := readPersistedRouteHealthSnapshot(path); err == nil {
		snapshot = existing
	} else if !errors.Is(err, os.ErrNotExist) {
		// Best-effort persistence: replace corrupt/unreadable snapshots with a
		// clean one instead of failing every future save.
		snapshot = persistedRouteHealthSnapshot{Version: persistedRouteHealthVersion}
	}
	if store != nil {
		snapshot.Failures, snapshot.Metrics = store.persistedState()
	}
	if probes != nil {
		snapshot.Probes = probes.persistedState()
	}
	if snapshot.Version == 0 {
		snapshot.Version = persistedRouteHealthVersion
	}
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal route health snapshot: %w", err)
	}
	data = append(data, '\n')
	if err := safefs.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("mkdir route health snapshot dir: %w", err)
	}
	if err := safefs.WriteFileAtomic(path, data, 0o600); err != nil {
		return fmt.Errorf("write route health snapshot: %w", err)
	}
	return nil
}

func readPersistedRouteHealthSnapshot(path string) (persistedRouteHealthSnapshot, error) {
	data, err := safefs.ReadFile(path)
	if err != nil {
		return persistedRouteHealthSnapshot{}, err
	}
	var raw rawPersistedRouteHealthSnapshot
	if err := json.Unmarshal(data, &raw); err != nil {
		return persistedRouteHealthSnapshot{}, err
	}
	snapshot := persistedRouteHealthSnapshot{
		Version:  raw.Version,
		Failures: raw.Failures,
		Metrics:  raw.Metrics,
		Probes:   raw.Probes,
	}
	if len(snapshot.Probes) == 0 && len(raw.Records) > 0 {
		var legacy []ProbeRecord
		if err := json.Unmarshal(raw.Records, &legacy); err != nil {
			return persistedRouteHealthSnapshot{}, err
		}
		snapshot.Probes = legacy
	}
	if snapshot.Version == 0 {
		snapshot.Version = persistedRouteHealthVersion
	}
	return snapshot, nil
}

func (s *Store) persistedState() ([]persistedRouteHealthFailure, []persistedRouteHealthMetric) {
	if s == nil {
		return nil, nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	failures := make([]persistedRouteHealthFailure, 0, len(s.failures))
	for _, record := range s.failures {
		failures = append(failures, persistedRouteHealthFailure{
			Key:        record.Key,
			Status:     record.Status,
			Reason:     record.Reason,
			Error:      record.Error,
			Duration:   record.Duration,
			RecordedAt: record.RecordedAt,
		})
	}
	metrics := make([]persistedRouteHealthMetric, 0, len(s.metrics))
	for key, record := range s.metrics {
		metrics = append(metrics, persistedRouteHealthMetric{
			Key:           key,
			Attempts:      record.attempts,
			Successes:     record.successes,
			TotalDuration: record.totalDuration,
			RecordedAt:    record.recordedAt,
		})
	}
	return failures, metrics
}

func (s *Store) loadPersistedState(snapshot persistedRouteHealthSnapshot, ttl time.Duration, now time.Time) {
	if s == nil {
		return
	}
	if now.IsZero() {
		now = time.Now()
	}
	now = now.UTC()

	failures := make(map[Key]Record, len(snapshot.Failures))
	for _, stored := range snapshot.Failures {
		recordedAt := stored.RecordedAt.UTC()
		if isPersistedSignalExpired(recordedAt, ttl, now) {
			continue
		}
		failures[stored.Key] = Record{
			Key:        stored.Key,
			Status:     stored.Status,
			Reason:     stored.Reason,
			Error:      stored.Error,
			Duration:   stored.Duration,
			RecordedAt: recordedAt,
		}
	}

	metrics := make(map[Key]metricRecord, len(snapshot.Metrics))
	for _, stored := range snapshot.Metrics {
		recordedAt := stored.RecordedAt.UTC()
		if isPersistedSignalExpired(recordedAt, ttl, now) {
			continue
		}
		metrics[stored.Key] = metricRecord{
			attempts:      stored.Attempts,
			successes:     stored.Successes,
			totalDuration: stored.TotalDuration,
			recordedAt:    recordedAt,
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.failures = failures
	s.metrics = metrics
}

func (ps *ProbeStore) persistedState() []ProbeRecord {
	if ps == nil {
		return nil
	}
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	records := make([]ProbeRecord, 0, len(ps.records))
	for _, record := range ps.records {
		records = append(records, record)
	}
	return records
}

func (ps *ProbeStore) loadPersistedState(snapshot persistedRouteHealthSnapshot) {
	if ps == nil {
		return
	}
	records := make(map[probeKey]ProbeRecord, len(snapshot.Probes))
	for _, record := range snapshot.Probes {
		if record.Provider == "" {
			continue
		}
		key := probeKey{Provider: record.Provider, Endpoint: record.Endpoint}
		existing, ok := records[key]
		if !ok || record.LastProbeAt.After(existing.LastProbeAt) {
			records[key] = record
		}
	}

	ps.mu.Lock()
	defer ps.mu.Unlock()
	ps.records = records
}

func isPersistedSignalExpired(recordedAt time.Time, ttl time.Duration, now time.Time) bool {
	if recordedAt.IsZero() {
		return true
	}
	if ttl <= 0 {
		return false
	}
	return !recordedAt.Add(ttl).After(now)
}
