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

// geminiQuotaSnapshot captures parsed Gemini CLI /model manage quota evidence
// in a durable cache so foreground routing consumers do not invoke PTY
// capture inline.
//
// Auth/account freshness on its own is NOT quota evidence. Only a parsed
// geminiQuotaSnapshot with one or more tier windows promotes Gemini beyond
// the "quota unknown" state.
type geminiQuotaSnapshot struct {
	CapturedAt time.Time               `json:"captured_at"`
	Windows    []harnesses.QuotaWindow `json:"windows"`
	Source     string                  `json:"source"` // e.g. "pty", "cassette"
	Account    *harnesses.AccountInfo  `json:"account,omitempty"`
	Detail     string                  `json:"detail,omitempty"`
}

const defaultGeminiQuotaStaleAfter = 15 * time.Minute

const geminiQuotaCacheEnv = "FIZEAU_GEMINI_QUOTA_CACHE"

func geminiQuotaCachePath() (string, error) {
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

func writeGeminiQuota(path string, snapshot geminiQuotaSnapshot) error {
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

func readGeminiQuotaFrom(path string) (*geminiQuotaSnapshot, bool) {
	data, err := safefs.ReadFile(path)
	if err != nil {
		return nil, false
	}
	var snap geminiQuotaSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return nil, false
	}
	return &snap, true
}

func readGeminiQuota() (*geminiQuotaSnapshot, bool) {
	path, err := geminiQuotaCachePath()
	if err != nil {
		return nil, false
	}
	return readGeminiQuotaFrom(path)
}

func geminiQuotaSnapshotAge(snapshot *geminiQuotaSnapshot, now time.Time) time.Duration {
	if snapshot == nil || snapshot.CapturedAt.IsZero() {
		return 0
	}
	age := now.UTC().Sub(snapshot.CapturedAt.UTC())
	if age < 0 {
		return 0
	}
	return age
}

func isGeminiQuotaFresh(snapshot *geminiQuotaSnapshot, now time.Time, staleAfter time.Duration) bool {
	if snapshot == nil || snapshot.CapturedAt.IsZero() {
		return false
	}
	if staleAfter <= 0 {
		staleAfter = defaultGeminiQuotaStaleAfter
	}
	return geminiQuotaSnapshotAge(snapshot, now) <= staleAfter
}

type geminiQuotaRoutingDecision struct {
	preferGemini    bool
	snapshotPresent bool
	fresh           bool
	age             time.Duration
	snapshot        *geminiQuotaSnapshot
	availableTiers  []string
	exhaustedTiers  []string
	reason          string
}

func decideGeminiQuotaRouting(snapshot *geminiQuotaSnapshot, now time.Time, staleAfter time.Duration) geminiQuotaRoutingDecision {
	decision := geminiQuotaRoutingDecision{snapshot: snapshot}
	if snapshot == nil {
		decision.reason = "no cached gemini quota snapshot; assume limited"
		return decision
	}
	decision.snapshotPresent = true
	decision.age = geminiQuotaSnapshotAge(snapshot, now)
	if !isGeminiQuotaFresh(snapshot, now, staleAfter) {
		decision.reason = "cached gemini quota snapshot is stale; assume limited"
		return decision
	}
	decision.fresh = true
	if len(snapshot.Windows) == 0 {
		decision.reason = "fresh gemini quota snapshot has no tier windows; assume limited"
		return decision
	}
	for _, w := range snapshot.Windows {
		if isExhaustedWindow(w) {
			decision.exhaustedTiers = append(decision.exhaustedTiers, w.LimitID)
			continue
		}
		decision.availableTiers = append(decision.availableTiers, w.LimitID)
	}
	if len(decision.availableTiers) == 0 {
		decision.reason = "fresh gemini quota snapshot reports all tiers exhausted; assume limited"
		return decision
	}
	decision.preferGemini = true
	decision.reason = "fresh gemini quota snapshot has at least one tier with headroom"
	return decision
}

func (d geminiQuotaRoutingDecision) isGeminiTierAvailable(model string) bool {
	if !d.preferGemini {
		return false
	}
	limitID, ok := TierLimitIDForModel(model)
	if !ok {
		return false
	}
	for _, tier := range d.availableTiers {
		if tier == limitID {
			return true
		}
	}
	return false
}

func (s *geminiQuotaSnapshot) maxUsedPercent() float64 {
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
