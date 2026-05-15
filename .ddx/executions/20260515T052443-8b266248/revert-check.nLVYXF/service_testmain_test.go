package fizeau_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestMain isolates the agent_test package from the developer's live
// $HOME/.config/fizeau/config.yaml. Without this, calls like
// agent.New(agent.ServiceOptions{}) auto-load the user's real config —
// which has caused verify-worktree gates at older base revisions to fail
// when the live config contains provider types that revision didn't yet
// know about (agent-27806ad5).
//
// Per-test t.Setenv("HOME", ...) calls still take precedence for tests
// that need to plant a specific config on disk.
func TestMain(m *testing.M) {
	os.Exit(runTests(m))
}

func runTests(m *testing.M) int {
	tmp, err := os.MkdirTemp("", "agent-test-home-")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(tmp)

	os.Setenv("HOME", tmp)
	os.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, ".config"))
	os.Setenv("FIZEAU_CACHE_DIR", filepath.Join(tmp, "cache"))
	installFreshQuotaFixtures(tmp)

	return m.Run()
}

func installFreshQuotaFixtures(root string) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	window := map[string]any{
		"name":           "test",
		"limit_id":       "codex",
		"window_minutes": 300,
		"used_percent":   10,
		"state":          "ok",
	}
	geminiWindow := map[string]any{
		"name":           "Flash",
		"limit_id":       "gemini-flash",
		"window_minutes": 1440,
		"used_percent":   10,
		"state":          "ok",
	}
	claudePath := filepath.Join(root, "claude-quota.json")
	codexPath := filepath.Join(root, "codex-quota.json")
	geminiPath := filepath.Join(root, "gemini-quota.json")
	os.Setenv("FIZEAU_CLAUDE_QUOTA_CACHE", claudePath)
	os.Setenv("FIZEAU_CODEX_QUOTA_CACHE", codexPath)
	os.Setenv("FIZEAU_GEMINI_QUOTA_CACHE", geminiPath)
	writeTestJSON(claudePath, map[string]any{
		"captured_at":         now,
		"five_hour_remaining": 90,
		"five_hour_limit":     100,
		"weekly_remaining":    900,
		"weekly_limit":        1000,
		"windows":             []map[string]any{window},
		"source":              "test",
		"account":             map[string]any{"plan_type": "Claude Max"},
	})
	writeTestJSON(codexPath, map[string]any{
		"captured_at": now,
		"windows":     []map[string]any{window},
		"source":      "test",
		"account":     map[string]any{"plan_type": "ChatGPT Pro"},
	})
	writeTestJSON(geminiPath, map[string]any{
		"captured_at": now,
		"windows":     []map[string]any{geminiWindow},
		"source":      "test",
		"account":     map[string]any{"plan_type": "Gemini OAuth"},
	})
}

func writeTestJSON(path string, payload any) {
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		panic(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		panic(err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		panic(err)
	}
}
