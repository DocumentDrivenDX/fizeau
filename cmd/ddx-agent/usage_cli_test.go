package main_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/agent"
	"github.com/DocumentDrivenDX/agent/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func seedMixedUsageLogs(t *testing.T, logDir string) {
	t.Helper()

	writeUsageLog := func(t *testing.T, sessionID string, startAt, endAt time.Time, start session.SessionStartData, end session.SessionEndData) {
		t.Helper()

		logger := session.NewLogger(logDir, sessionID)
		startEvent := session.NewEvent(sessionID, 0, agent.EventSessionStart, start)
		startEvent.Timestamp = startAt
		logger.Write(startEvent)

		endEvent := session.NewEvent(sessionID, 1, agent.EventSessionEnd, end)
		endEvent.Timestamp = endAt
		logger.Write(endEvent)

		require.NoError(t, logger.Close())
	}

	writeUsageLog(t, "recent-known", time.Date(2026, 4, 8, 10, 0, 0, 0, time.UTC), time.Date(2026, 4, 8, 10, 0, 1, 0, time.UTC), session.SessionStartData{
		Provider: "openai-compat",
		Model:    "qwen3.5-7b",
		Prompt:   "recent known",
	}, session.SessionEndData{
		Status:     agent.StatusSuccess,
		Output:     "ok",
		Tokens:     agent.TokenUsage{Input: 10, Output: 5, Total: 15},
		CostUSD:    usageFloat64Ptr(0.25),
		DurationMs: 1000,
		Model:      "qwen3.5-7b",
	})

	writeUsageLog(t, "recent-unknown", time.Date(2026, 4, 8, 11, 0, 0, 0, time.UTC), time.Date(2026, 4, 8, 11, 0, 2, 0, time.UTC), session.SessionStartData{
		Provider: "openai-compat",
		Model:    "qwen3.5-7b",
		Prompt:   "recent unknown",
	}, session.SessionEndData{
		Status:     agent.StatusSuccess,
		Output:     "ok",
		Tokens:     agent.TokenUsage{Input: 20, Output: 10, Total: 30},
		CostUSD:    usageFloat64Ptr(-1),
		DurationMs: 2000,
		Model:      "qwen3.5-7b",
	})

	writeUsageLog(t, "old-session", time.Date(2026, 3, 25, 9, 0, 0, 0, time.UTC), time.Date(2026, 3, 25, 9, 0, 3, 0, time.UTC), session.SessionStartData{
		Provider: "anthropic",
		Model:    "claude-sonnet-4-20250514",
		Prompt:   "old",
	}, session.SessionEndData{
		Status:     agent.StatusSuccess,
		Output:     "ok",
		Tokens:     agent.TokenUsage{Input: 100, Output: 50, Total: 150},
		CostUSD:    usageFloat64Ptr(0.5),
		DurationMs: 3000,
		Model:      "claude-sonnet-4-20250514",
	})
}

func TestCLI_Usage(t *testing.T) {
	workDir := t.TempDir()
	logDir := filepath.Join(workDir, ".agent", "sessions")
	require.NoError(t, os.MkdirAll(logDir, 0o755))
	seedMixedUsageLogs(t, logDir)

	out, err := runAgentCLI(t, "--work-dir", workDir, "usage", "--since=7d")
	require.NoError(t, err, string(out))

	output := string(out)
	assert.Contains(t, output, "PROVIDER")
	assert.Contains(t, output, "TOTAL")
	assert.Contains(t, output, "openai-compat")
	assert.Contains(t, output, "qwen3.5-7b")
	assert.Contains(t, output, "Window:")
	assert.Contains(t, output, "unknown")
	assert.NotContains(t, output, "$0.2500")
}

func TestCLI_Usage_JSON_MixedCost(t *testing.T) {
	workDir := t.TempDir()
	logDir := filepath.Join(workDir, ".agent", "sessions")
	require.NoError(t, os.MkdirAll(logDir, 0o755))
	seedMixedUsageLogs(t, logDir)

	out, err := runAgentCLI(t, "--work-dir", workDir, "usage", "--since=7d", "--json")
	require.NoError(t, err, string(out))

	var report struct {
		Rows []struct {
			Provider            string   `json:"provider"`
			Model               string   `json:"model"`
			KnownCostUSD        *float64 `json:"known_cost_usd"`
			UnknownCostSessions int      `json:"unknown_cost_sessions"`
		} `json:"rows"`
		Totals struct {
			KnownCostUSD        *float64 `json:"known_cost_usd"`
			UnknownCostSessions int      `json:"unknown_cost_sessions"`
		} `json:"totals"`
	}
	require.NoError(t, json.Unmarshal(out, &report))

	require.Len(t, report.Rows, 1)
	assert.Equal(t, "openai-compat", report.Rows[0].Provider)
	assert.Equal(t, "qwen3.5-7b", report.Rows[0].Model)
	assert.Nil(t, report.Rows[0].KnownCostUSD)
	assert.Equal(t, 1, report.Rows[0].UnknownCostSessions)
	assert.Nil(t, report.Totals.KnownCostUSD)
	assert.Equal(t, 1, report.Totals.UnknownCostSessions)
}

func TestCLI_Usage_InvalidSince_ExitCode(t *testing.T) {
	exe := buildAgentCLI(t)
	workDir := t.TempDir()
	cmd := exec.Command(exe, "--work-dir", workDir, "usage", "--since=bad-window")
	home := t.TempDir()
	cmd.Env = append(os.Environ(),
		"HOME="+home,
		"XDG_CONFIG_HOME="+filepath.Join(home, ".config"),
	)
	out, err := cmd.CombinedOutput()
	require.Error(t, err, string(out))

	exitErr, ok := err.(*exec.ExitError)
	require.True(t, ok, "expected process exit error, got %T: %v", err, err)
	assert.Equal(t, 2, exitErr.ExitCode())
	assert.Contains(t, string(out), "invalid time window")
}

func buildAgentCLI(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	exe := filepath.Join(dir, "ddx-agent")
	wd, err := os.Getwd()
	require.NoError(t, err)
	cmd := exec.Command("go", "build", "-o", exe, "./cmd/ddx-agent")
	cmd.Dir = filepath.Clean(filepath.Join(wd, "..", ".."))
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
	return exe
}

func usageFloat64Ptr(v float64) *float64 {
	return &v
}
