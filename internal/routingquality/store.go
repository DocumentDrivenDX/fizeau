package routingquality

import (
	"sort"
	"strings"
	"sync"
	"time"
)

// Metrics is the internal routing-quality aggregate.
type Metrics struct {
	AutoAcceptanceRate       float64
	OverrideDisagreementRate float64
	OverrideClassBreakdown   []Bucket
	TotalRequests            int
	TotalOverrides           int
}

// Bucket is one cell in the override-class pivot.
type Bucket struct {
	PromptFeatureBucket string
	Axis                string
	Match               bool
	Count               int

	SuccessOutcomes   int
	StalledOutcomes   int
	FailedOutcomes    int
	CancelledOutcomes int
	UnknownOutcomes   int
}

// OverrideData is the internal representation of a service override event.
type OverrideData struct {
	AxesOverridden []string
	MatchPerAxis   map[string]bool
	PromptFeatures PromptFeatures
	Outcome        *Outcome
}

// PromptFeatures is the internal prompt-feature slice used for buckets.
type PromptFeatures struct {
	EstimatedTokens *int
	RequiresTools   bool
	Reasoning       string
}

// Outcome is the internal terminal outcome for an override.
type Outcome struct {
	Status string
}

// Record is one entry in the in-process routing-quality store.
type Record struct {
	at       time.Time
	override *OverrideData
}

// Store is a bounded in-memory ring of routing-quality records. It is safe for
// concurrent use.
type Store struct {
	mu      sync.RWMutex
	records []*Record
	cap     int
}

// DefaultStoreCap is the service-default bounded ring size.
const DefaultStoreCap = 1024

// NewStore returns an empty store with the given record cap.
func NewStore(cap int) *Store {
	return &Store{cap: cap}
}

// RecordRequest appends a request to the store and returns the freshly
// allocated record. override may be nil for the no-override case.
func (s *Store) RecordRequest(at time.Time, override *OverrideData) *Record {
	if s == nil {
		return nil
	}
	if at.IsZero() {
		at = time.Now().UTC()
	}
	rec := &Record{at: at.UTC()}
	if override != nil {
		rec.override = cloneOverride(*override)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records = append(s.records, rec)
	if s.cap > 0 && len(s.records) > s.cap {
		drop := len(s.records) - s.cap
		s.records = s.records[drop:]
	}
	return rec
}

// SnapshotRecent returns up to maxN of the most recent records, optionally
// filtered by since.
func (s *Store) SnapshotRecent(maxN int, since time.Time) []*Record {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Record, 0, len(s.records))
	for _, r := range s.records {
		if !since.IsZero() && r.at.Before(since) {
			continue
		}
		out = append(out, r)
	}
	if maxN > 0 && len(out) > maxN {
		out = out[len(out)-maxN:]
	}
	return out
}

// SnapshotWindow returns records whose timestamps fall within [start, end).
func (s *Store) SnapshotWindow(start, end time.Time) []*Record {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Record, 0, len(s.records))
	for _, r := range s.records {
		if !start.IsZero() && r.at.Before(start) {
			continue
		}
		if !end.IsZero() && !r.at.Before(end) {
			continue
		}
		out = append(out, r)
	}
	return out
}

// MetricsRecent aggregates metrics over up to maxN of the most recent records,
// optionally filtered by since.
func (s *Store) MetricsRecent(maxN int, since time.Time) Metrics {
	return ComputeMetricsFromRecords(s.SnapshotRecent(maxN, since))
}

// MetricsWindow aggregates metrics for records whose timestamps fall within
// [start, end).
func (s *Store) MetricsWindow(start, end time.Time) Metrics {
	return ComputeMetricsFromRecords(s.SnapshotWindow(start, end))
}

// StampOutcome copies outcome into rec's override payload.
func StampOutcome(rec *Record, outcome *Outcome) {
	if rec == nil || outcome == nil || rec.override == nil {
		return
	}
	clone := *outcome
	rec.override.Outcome = &clone
}

// ComputeMetrics aggregates routing-quality metrics from request and override
// counts.
func ComputeMetrics(totalRequests int, overrides []OverrideData) Metrics {
	m := Metrics{
		TotalRequests:  totalRequests,
		TotalOverrides: len(overrides),
	}
	if totalRequests > 0 {
		noOverride := totalRequests - len(overrides)
		if noOverride < 0 {
			noOverride = 0
		}
		m.AutoAcceptanceRate = float64(noOverride) / float64(totalRequests)
	}
	if len(overrides) > 0 {
		disagree := 0
		for _, ov := range overrides {
			if overrideDisagreesOnAnyAxis(ov) {
				disagree++
			}
		}
		m.OverrideDisagreementRate = float64(disagree) / float64(len(overrides))
	}
	m.OverrideClassBreakdown = buildOverrideClassBreakdown(overrides)
	return m
}

// ComputeMetricsFromRecords aggregates the store-side record shape directly.
func ComputeMetricsFromRecords(records []*Record) Metrics {
	overrides := make([]OverrideData, 0, len(records))
	for _, r := range records {
		if r == nil || r.override == nil {
			continue
		}
		overrides = append(overrides, *cloneOverride(*r.override))
	}
	return ComputeMetrics(len(records), overrides)
}

func cloneOverride(ov OverrideData) *OverrideData {
	clone := ov
	clone.AxesOverridden = append([]string(nil), ov.AxesOverridden...)
	if ov.MatchPerAxis != nil {
		clone.MatchPerAxis = make(map[string]bool, len(ov.MatchPerAxis))
		for k, v := range ov.MatchPerAxis {
			clone.MatchPerAxis[k] = v
		}
	}
	if ov.PromptFeatures.EstimatedTokens != nil {
		tokens := *ov.PromptFeatures.EstimatedTokens
		clone.PromptFeatures.EstimatedTokens = &tokens
	}
	if ov.Outcome != nil {
		outcome := *ov.Outcome
		clone.Outcome = &outcome
	}
	return &clone
}

func overrideDisagreesOnAnyAxis(ov OverrideData) bool {
	if len(ov.AxesOverridden) == 0 {
		return false
	}
	for _, axis := range ov.AxesOverridden {
		match, ok := ov.MatchPerAxis[axis]
		if !ok || !match {
			return true
		}
	}
	return false
}

func buildOverrideClassBreakdown(overrides []OverrideData) []Bucket {
	if len(overrides) == 0 {
		return nil
	}
	type key struct {
		bucket string
		axis   string
		match  bool
	}
	tally := make(map[key]*Bucket)
	for _, ov := range overrides {
		bucket := promptFeatureBucket(ov.PromptFeatures)
		for _, axis := range ov.AxesOverridden {
			match := ov.MatchPerAxis[axis]
			k := key{bucket: bucket, axis: axis, match: match}
			b, ok := tally[k]
			if !ok {
				b = &Bucket{
					PromptFeatureBucket: bucket,
					Axis:                axis,
					Match:               match,
				}
				tally[k] = b
			}
			b.Count++
			switch outcomeStatus(ov.Outcome) {
			case "success":
				b.SuccessOutcomes++
			case "stalled":
				b.StalledOutcomes++
			case "failed":
				b.FailedOutcomes++
			case "cancelled":
				b.CancelledOutcomes++
			default:
				b.UnknownOutcomes++
			}
		}
	}
	out := make([]Bucket, 0, len(tally))
	for _, b := range tally {
		out = append(out, *b)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].PromptFeatureBucket != out[j].PromptFeatureBucket {
			return out[i].PromptFeatureBucket < out[j].PromptFeatureBucket
		}
		if out[i].Axis != out[j].Axis {
			return out[i].Axis < out[j].Axis
		}
		return !out[i].Match && out[j].Match
	})
	return out
}

func outcomeStatus(o *Outcome) string {
	if o == nil {
		return ""
	}
	return o.Status
}

func promptFeatureBucket(pf PromptFeatures) string {
	parts := make([]string, 0, 3)
	parts = append(parts, "tokens="+tokenSizeBucket(pf.EstimatedTokens))
	if pf.RequiresTools {
		parts = append(parts, "tools=yes")
	} else {
		parts = append(parts, "tools=no")
	}
	if pf.Reasoning != "" {
		parts = append(parts, "reasoning="+pf.Reasoning)
	} else {
		parts = append(parts, "reasoning=none")
	}
	return strings.Join(parts, ",")
}

func tokenSizeBucket(tokens *int) string {
	if tokens == nil {
		return "unknown"
	}
	t := *tokens
	switch {
	case t <= 0:
		return "unknown"
	case t < 1000:
		return "tiny"
	case t < 4000:
		return "small"
	case t < 16000:
		return "medium"
	case t < 64000:
		return "large"
	default:
		return "xlarge"
	}
}
