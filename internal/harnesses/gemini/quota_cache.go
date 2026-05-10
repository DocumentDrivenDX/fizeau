package gemini

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/easel/fizeau/internal/harnesses"
	"github.com/easel/fizeau/internal/productinfo"
	"github.com/easel/fizeau/internal/safefs"
)

// GeminiQuotaSnapshot captures parsed Gemini CLI /model manage quota evidence
// in a durable cache so foreground routing consumers do not invoke PTY
// capture inline.
//
// Auth/account freshness on its own is NOT quota evidence. Only a parsed
// GeminiQuotaSnapshot with one or more tier windows promotes Gemini beyond
// the "quota unknown" state.
type GeminiQuotaSnapshot struct {
	CapturedAt time.Time               `json:"captured_at"`
	Windows    []harnesses.QuotaWindow `json:"windows"`
	Source     string                  `json:"source"` // e.g. "pty", "cassette"
	Account    *harnesses.AccountInfo  `json:"account,omitempty"`
	Detail     string                  `json:"detail,omitempty"`
}

const DefaultGeminiQuotaStaleAfter = 15 * time.Minute

const geminiQuotaCacheEnv = "FIZEAU_GEMINI_QUOTA_CACHE"

// GeminiQuotaCachePath returns the durable Gemini quota cache path.
func GeminiQuotaCachePath() (string, error) {
	if path := os.Getenv(geminiQuotaCacheEnv); path != "" {
		return path, nil
	}
	if xdg := os.Getenv("XDG_STATE_HOME"); xdg != "" {
		return filepath.Join(xdg, productinfo.ConfigDir, "gemini-quota.json"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "state", productinfo.ConfigDir, "gemini-quota.json"), nil
}

// WriteGeminiQuota atomically persists a GeminiQuotaSnapshot to path.
func WriteGeminiQuota(path string, snapshot GeminiQuotaSnapshot) error {
	if path == "" {
		return fmt.Errorf("gemini quota cache path is empty")
	}
	if snapshot.CapturedAt.IsZero() {
		snapshot.CapturedAt = time.Now().UTC()
	} else {
		snapshot.CapturedAt = snapshot.CapturedAt.UTC()
	}
	if snapshot.Source == "" {
		snapshot.Source = "pty"
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("create gemini quota cache dir: %w", err)
	}
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal gemini quota snapshot: %w", err)
	}
	data = append(data, '\n')
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o600); err != nil {
		return fmt.Errorf("write gemini quota cache tmp: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("rename gemini quota cache: %w", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("chmod gemini quota cache: %w", err)
	}
	return nil
}

// ReadGeminiQuotaFrom reads a Gemini quota snapshot from a specific path.
func ReadGeminiQuotaFrom(path string) (*GeminiQuotaSnapshot, bool) {
	data, err := safefs.ReadFile(path)
	if err != nil {
		return nil, false
	}
	var snap GeminiQuotaSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return nil, false
	}
	return &snap, true
}

// ReadGeminiQuota reads the default Gemini quota cache.
func ReadGeminiQuota() (*GeminiQuotaSnapshot, bool) {
	path, err := GeminiQuotaCachePath()
	if err != nil {
		return nil, false
	}
	return ReadGeminiQuotaFrom(path)
}

// GeminiQuotaSnapshotAge reports snapshot age relative to now.
func GeminiQuotaSnapshotAge(snapshot *GeminiQuotaSnapshot, now time.Time) time.Duration {
	if snapshot == nil || snapshot.CapturedAt.IsZero() {
		return 0
	}
	age := now.UTC().Sub(snapshot.CapturedAt.UTC())
	if age < 0 {
		return 0
	}
	return age
}

// IsGeminiQuotaFresh reports whether a snapshot is present and not stale.
func IsGeminiQuotaFresh(snapshot *GeminiQuotaSnapshot, now time.Time, staleAfter time.Duration) bool {
	if snapshot == nil || snapshot.CapturedAt.IsZero() {
		return false
	}
	if staleAfter <= 0 {
		staleAfter = DefaultGeminiQuotaStaleAfter
	}
	return GeminiQuotaSnapshotAge(snapshot, now) <= staleAfter
}

// GeminiQuotaRoutingDecision summarises whether foreground routing may
// select Gemini without probing the CLI inline. The decision is made per
// tier: a tier is "available" only when it has a fresh, non-exhausted
// quota window.
type GeminiQuotaRoutingDecision struct {
	// PreferGemini is true when the snapshot is fresh and at least one
	// non-exhausted tier is present.
	PreferGemini bool
	// SnapshotPresent is true when a snapshot was found (even if stale).
	SnapshotPresent bool
	// Fresh is true when the snapshot is present and newer than staleAfter.
	Fresh bool
	// Age is the snapshot age (zero when absent).
	Age time.Duration
	// Snapshot is the cached snapshot when present.
	Snapshot *GeminiQuotaSnapshot
	// AvailableTiers lists tier limit_ids (e.g. "gemini-flash") that are
	// fresh and below the exhaustion threshold.
	AvailableTiers []string
	// ExhaustedTiers lists tier limit_ids currently blocked (e.g. "gemini-pro"
	// at 100% used).
	ExhaustedTiers []string
	// Reason describes the diagnostic outcome.
	Reason string
}

// DecideGeminiQuotaRouting turns a durable quota snapshot into a foreground
// routing decision. Missing, stale, or empty quota evidence keeps Gemini out
// of automatic routing.
func DecideGeminiQuotaRouting(snapshot *GeminiQuotaSnapshot, now time.Time, staleAfter time.Duration) GeminiQuotaRoutingDecision {
	decision := GeminiQuotaRoutingDecision{Snapshot: snapshot}
	if snapshot == nil {
		decision.Reason = "no cached gemini quota snapshot; assume limited"
		return decision
	}
	decision.SnapshotPresent = true
	decision.Age = GeminiQuotaSnapshotAge(snapshot, now)
	if !IsGeminiQuotaFresh(snapshot, now, staleAfter) {
		decision.Reason = "cached gemini quota snapshot is stale; assume limited"
		return decision
	}
	decision.Fresh = true
	if len(snapshot.Windows) == 0 {
		decision.Reason = "fresh gemini quota snapshot has no tier windows; assume limited"
		return decision
	}
	for _, w := range snapshot.Windows {
		if isExhaustedWindow(w) {
			decision.ExhaustedTiers = append(decision.ExhaustedTiers, w.LimitID)
			continue
		}
		decision.AvailableTiers = append(decision.AvailableTiers, w.LimitID)
	}
	if len(decision.AvailableTiers) == 0 {
		decision.Reason = "fresh gemini quota snapshot reports all tiers exhausted; assume limited"
		return decision
	}
	decision.PreferGemini = true
	decision.Reason = "fresh gemini quota snapshot has at least one tier with headroom"
	return decision
}

// ReadGeminiQuotaRoutingDecision reads the default cache and produces a
// routing decision in one call.
func ReadGeminiQuotaRoutingDecision(now time.Time, staleAfter time.Duration) GeminiQuotaRoutingDecision {
	snap, _ := ReadGeminiQuota()
	return DecideGeminiQuotaRouting(snap, now, staleAfter)
}

// IsGeminiTierAvailable reports whether the tier for a concrete Gemini model
// is currently selectable according to the decision. An unknown model returns
// false so routing never silently picks a model outside the parsed tier set.
func (d GeminiQuotaRoutingDecision) IsGeminiTierAvailable(model string) bool {
	if !d.PreferGemini {
		return false
	}
	limitID, ok := TierLimitIDForModel(model)
	if !ok {
		return false
	}
	for _, tier := range d.AvailableTiers {
		if tier == limitID {
			return true
		}
	}
	return false
}

// MaxUsedPercent returns the largest used-percent across all tier windows in
// the snapshot. Returns 0 for nil/empty snapshots.
func (s *GeminiQuotaSnapshot) MaxUsedPercent() float64 {
	if s == nil {
		return 0
	}
	max := 0.0
	for _, w := range s.Windows {
		if w.UsedPercent > max {
			max = w.UsedPercent
		}
	}
	return max
}

func isExhaustedWindow(w harnesses.QuotaWindow) bool {
	if strings.EqualFold(w.State, "exhausted") || strings.EqualFold(w.State, "blocked") {
		return true
	}
	return w.UsedPercent >= 95
}
