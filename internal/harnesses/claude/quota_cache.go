package claude

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/easel/fizeau/internal/harnesses"
	"github.com/easel/fizeau/internal/productinfo"
)

// claudeQuotaSnapshot captures Claude's current-quota headroom as absolute
// token/message counts. It is written to a durable per-user cache by an
// asynchronous capture path and read by foreground routing consumers.
//
// The snapshot is intentionally distinct from the percentage-based
// QuotaSignal: foreground routing needs concrete numbers to reason about
// 5-hour / weekly headroom without invoking PTY capture inline.
type claudeQuotaSnapshot struct {
	CapturedAt        time.Time               `json:"captured_at"`
	FiveHourRemaining int                     `json:"five_hour_remaining"`
	FiveHourLimit     int                     `json:"five_hour_limit"`
	WeeklyRemaining   int                     `json:"weekly_remaining"`
	WeeklyLimit       int                     `json:"weekly_limit"`
	Windows           []harnesses.QuotaWindow `json:"windows,omitempty"`
	Source            string                  `json:"source"` // e.g. "pty", "heuristic"
	Account           *harnesses.AccountInfo  `json:"account,omitempty"`
}

// defaultClaudeQuotaStaleAfter is the default maximum age before a cached
// snapshot is considered stale and foreground routing should fall back to
// the safe default.
const defaultClaudeQuotaStaleAfter = 15 * time.Minute

// claudeQuotaCacheEnv lets tests override the cache file path.
const claudeQuotaCacheEnv = "FIZEAU_CLAUDE_QUOTA_CACHE"

// claudeQuotaCachePath returns the durable location for the Claude quota
// cache. It resolves to $XDG_STATE_HOME/<config-dir>/claude-quota.json, or
// ~/.local/state/<config-dir>/claude-quota.json when XDG_STATE_HOME is unset.
// The FIZEAU_CLAUDE_QUOTA_CACHE env var takes precedence (primarily for
// tests).
func claudeQuotaCachePath() (string, error) {
	if path := os.Getenv(claudeQuotaCacheEnv); path != "" {
		return path, nil
	}
	if xdg := os.Getenv("XDG_STATE_HOME"); xdg != "" {
		return filepath.Join(xdg, productinfo.ConfigDir, "claude-quota.json"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "state", productinfo.ConfigDir, "claude-quota.json"), nil
}

// writeClaudeQuota atomically persists a claudeQuotaSnapshot to the given
// path. The parent directory is created if necessary. The file is written
// to a sibling .tmp file and renamed into place so readers never observe a
// partially-written snapshot. The final file mode is 0600.
func writeClaudeQuota(path string, snapshot claudeQuotaSnapshot) error {
	if path == "" {
		return fmt.Errorf("claude quota cache path is empty")
	}
	if snapshot.CapturedAt.IsZero() {
		snapshot.CapturedAt = time.Now().UTC()
	} else {
		snapshot.CapturedAt = snapshot.CapturedAt.UTC()
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create claude quota cache dir: %w", err)
	}

	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal claude quota snapshot: %w", err)
	}
	data = append(data, '\n')

	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o600); err != nil {
		return fmt.Errorf("write claude quota cache tmp: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("rename claude quota cache: %w", err)
	}
	// Ensure final mode is 0600 in case an older file had a different mode.
	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("chmod claude quota cache: %w", err)
	}
	return nil
}

// readClaudeQuotaFrom reads the snapshot at the given path. Returns
// (nil, false) if the file does not exist or cannot be decoded. Non-
// existence is NOT an error: foreground callers are expected to fall back
// to a safe default when no snapshot is present.
func readClaudeQuotaFrom(path string) (*claudeQuotaSnapshot, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	var snap claudeQuotaSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return nil, false
	}
	return &snap, true
}

// The second return value is false if no snapshot is present or cannot be
// decoded.
//
// Callers SHOULD check snapshot age via claudeQuotaSnapshotAge (or
// isClaudeQuotaFresh) before trusting the values; this function does not
// itself enforce a TTL so that callers can report stale snapshots in
// diagnostic surfaces like `ddx agent doctor --routing`.
func readClaudeQuota() (*claudeQuotaSnapshot, bool) {
	path, err := claudeQuotaCachePath()
	if err != nil {
		return nil, false
	}
	return readClaudeQuotaFrom(path)
}

// claudeQuotaSnapshotAge reports the age of a snapshot relative to now.
// A zero or future CapturedAt yields a zero age.
func claudeQuotaSnapshotAge(snapshot *claudeQuotaSnapshot, now time.Time) time.Duration {
	if snapshot == nil || snapshot.CapturedAt.IsZero() {
		return 0
	}
	age := now.UTC().Sub(snapshot.CapturedAt.UTC())
	if age < 0 {
		return 0
	}
	return age
}

// isClaudeQuotaFresh reports whether a snapshot exists and is newer than
// staleAfter relative to now. A nil snapshot is never fresh. A zero
// staleAfter falls back to defaultClaudeQuotaStaleAfter.
func isClaudeQuotaFresh(snapshot *claudeQuotaSnapshot, now time.Time, staleAfter time.Duration) bool {
	if snapshot == nil || snapshot.CapturedAt.IsZero() {
		return false
	}
	if staleAfter <= 0 {
		staleAfter = defaultClaudeQuotaStaleAfter
	}
	return claudeQuotaSnapshotAge(snapshot, now) <= staleAfter
}

// claudeQuotaRoutingDecision summarises what foreground routing should do
// given the current cached snapshot.
type claudeQuotaRoutingDecision struct {
	// PreferClaude is true when a fresh snapshot shows headroom in both the
	// 5-hour and weekly windows. When false, routing should prefer a
	// non-claude fallback harness.
	PreferClaude bool
	// SnapshotPresent is true when a snapshot was found in the cache (even
	// if stale).
	SnapshotPresent bool
	// Fresh is true when the snapshot is present and newer than staleAfter.
	Fresh bool
	// Age is the age of the snapshot relative to now (zero when absent).
	Age time.Duration
	// Snapshot is the cached snapshot when present.
	Snapshot *claudeQuotaSnapshot
	// Reason describes why the decision was made (diagnostic surface).
	Reason string
}

// decideClaudeQuotaRouting turns a cached snapshot into a routing decision
// for foreground callers. When the snapshot is missing or stale, the safe
// default is NOT to prefer claude (assume limited).
//
// A snapshot counts as "limited" when either window reports zero or
// negative remaining headroom.
func decideClaudeQuotaRouting(snapshot *claudeQuotaSnapshot, now time.Time, staleAfter time.Duration) claudeQuotaRoutingDecision {
	decision := claudeQuotaRoutingDecision{
		Snapshot: snapshot,
	}
	if snapshot == nil {
		decision.Reason = "no cached snapshot; assume limited"
		return decision
	}
	decision.SnapshotPresent = true
	decision.Age = claudeQuotaSnapshotAge(snapshot, now)
	if !isClaudeQuotaFresh(snapshot, now, staleAfter) {
		decision.Reason = "cached snapshot is stale; assume limited"
		return decision
	}
	if err := validateClaudeQuotaSnapshotForRouting(snapshot); err != nil {
		decision.Reason = "cached snapshot is incomplete: " + err.Error() + "; assume limited"
		return decision
	}
	decision.Fresh = true
	if exhausted := exhaustedClaudeQuotaWindow(snapshot.Windows); exhausted != "" {
		decision.Reason = "fresh snapshot reports exhausted " + exhausted + " window; assume limited"
		return decision
	}
	if snapshot.FiveHourRemaining <= 0 || snapshot.WeeklyRemaining <= 0 {
		decision.Reason = "fresh snapshot reports exhausted window; assume limited"
		return decision
	}
	decision.PreferClaude = true
	decision.Reason = "fresh snapshot has headroom"
	return decision
}

func validateClaudeQuotaSnapshotForRouting(snapshot *claudeQuotaSnapshot) error {
	if snapshot == nil {
		return fmt.Errorf("missing snapshot")
	}
	if strings.TrimSpace(snapshot.Source) == "" {
		return fmt.Errorf("missing source")
	}
	if snapshot.Account == nil || strings.TrimSpace(snapshot.Account.PlanType) == "" {
		return fmt.Errorf("missing account plan")
	}
	if snapshot.FiveHourLimit <= 0 {
		return fmt.Errorf("invalid 5h limit")
	}
	if snapshot.WeeklyLimit <= 0 {
		return fmt.Errorf("invalid weekly limit")
	}
	if snapshot.FiveHourRemaining < 0 || snapshot.FiveHourRemaining > snapshot.FiveHourLimit {
		return fmt.Errorf("invalid 5h remaining")
	}
	if snapshot.WeeklyRemaining < 0 || snapshot.WeeklyRemaining > snapshot.WeeklyLimit {
		return fmt.Errorf("invalid weekly remaining")
	}
	return nil
}

func exhaustedClaudeQuotaWindow(windows []harnesses.QuotaWindow) string {
	for _, window := range windows {
		if !claudeQuotaWindowExhausted(window) {
			continue
		}
		if strings.TrimSpace(window.LimitID) != "" {
			return window.LimitID
		}
		if strings.TrimSpace(window.Name) != "" {
			return window.Name
		}
		return "unknown"
	}
	return ""
}

func claudeQuotaWindowExhausted(window harnesses.QuotaWindow) bool {
	state := strings.ToLower(strings.TrimSpace(window.State))
	return state == "exhausted" || window.UsedPercent >= 100
}

// isClaudeQuotaExhaustedMessage recognizes Claude CLI quota failures that are
// emitted as plain text rather than structured quota data. Claude currently
// reports weekly exhaustion with wording like "out of extra usage", so callers
// must treat these strings as a hard quota signal.
func isClaudeQuotaExhaustedMessage(text string) bool {
	normalized := strings.ToLower(strings.TrimSpace(text))
	return strings.Contains(normalized, "out of extra usage") ||
		strings.Contains(normalized, "usage limit reached") ||
		strings.Contains(normalized, "quota exhausted") ||
		strings.Contains(normalized, "weekly quota") ||
		strings.Contains(normalized, "current week") && strings.Contains(normalized, "exhaust")
}

// markClaudeQuotaExhaustedFromMessage records a runtime Claude quota failure
// in the durable cache so later automatic routing avoids Claude until a fresh
// quota probe proves headroom again.
func markClaudeQuotaExhaustedFromMessage(text string, now time.Time) bool {
	if !isClaudeQuotaExhaustedMessage(text) {
		return false
	}
	path, err := claudeQuotaCachePath()
	if err != nil {
		return true
	}
	if now.IsZero() {
		now = time.Now()
	}
	snap := claudeQuotaSnapshot{
		CapturedAt:        now.UTC(),
		FiveHourLimit:     100,
		FiveHourRemaining: 0,
		WeeklyLimit:       100,
		WeeklyRemaining:   0,
		Windows: []harnesses.QuotaWindow{
			{Name: "Current week (all models)", LimitID: "weekly-all", WindowMinutes: 10080, UsedPercent: 100, State: "exhausted"},
			{Name: "Extra usage", LimitID: "extra", UsedPercent: 100, State: "exhausted"},
		},
		Source:  "runtime_error",
		Account: &harnesses.AccountInfo{PlanType: "unknown"},
	}
	_ = writeClaudeQuota(path, snap)
	return true
}
