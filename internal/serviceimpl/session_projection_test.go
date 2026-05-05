package serviceimpl

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	agentcore "github.com/DocumentDrivenDX/fizeau/internal/core"
	"github.com/DocumentDrivenDX/fizeau/internal/session"
)

func TestListSessionLogsFiltersAndSortsJSONL(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "b.jsonl"), "{}\n")
	writeFile(t, filepath.Join(dir, "a.jsonl"), "{}\n")
	writeFile(t, filepath.Join(dir, "ignore.txt"), "{}\n")
	if err := os.Mkdir(filepath.Join(dir, "dir.jsonl"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	entries, err := ListSessionLogs(context.Background(), dir)
	if err != nil {
		t.Fatalf("ListSessionLogs: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("len(entries) = %d, want 2", len(entries))
	}
	if entries[0].SessionID != "a" || entries[1].SessionID != "b" {
		t.Fatalf("entries = %+v, want sorted a,b", entries)
	}
	if entries[0].Size == 0 || entries[0].ModTime.IsZero() {
		t.Fatalf("entry metadata not populated: %+v", entries[0])
	}
}

func TestWriteAndReplaySessionLog(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "svc-1.jsonl")
	logger := session.NewLogger(dir, "svc-1")
	logger.Emit(agentcore.EventSessionStart, session.SessionStartData{Prompt: "hello"})
	logger.Emit(agentcore.EventSessionEnd, session.SessionEndData{Status: agentcore.StatusSuccess})
	if err := logger.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	var raw bytes.Buffer
	if err := WriteSessionLog(context.Background(), path, &raw); err != nil {
		t.Fatalf("WriteSessionLog: %v", err)
	}
	if !strings.Contains(raw.String(), "\"type\": \"session.start\"") {
		t.Fatalf("raw log missing session.start: %s", raw.String())
	}

	var replay bytes.Buffer
	if err := ReplaySession(context.Background(), path, &replay); err != nil {
		t.Fatalf("ReplaySession: %v", err)
	}
	if !strings.Contains(replay.String(), "Session svc-1") {
		t.Fatalf("replay missing session header: %s", replay.String())
	}
}

func TestUsageReportDelegatesToSessionAggregation(t *testing.T) {
	dir := t.TempDir()
	logger := session.NewLogger(dir, "usage-1")
	logger.Write(agentcore.Event{
		SessionID: "usage-1",
		Seq:       1,
		Type:      agentcore.EventSessionStart,
		Timestamp: time.Date(2026, 5, 5, 14, 0, 0, 0, time.UTC),
		Data: mustSessionData(t, session.SessionStartData{
			Provider: "openrouter",
			Model:    "gpt-test",
		}),
	})
	logger.Write(agentcore.Event{
		SessionID: "usage-1",
		Seq:       2,
		Type:      agentcore.EventSessionEnd,
		Timestamp: time.Date(2026, 5, 5, 14, 0, 1, 0, time.UTC),
		Data: mustSessionData(t, session.SessionEndData{
			Status: agentcore.StatusSuccess,
			Tokens: agentcore.TokenUsage{
				Input: 10,
				Total: 10,
			},
		}),
	})
	if err := logger.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	report, err := UsageReport(context.Background(), dir, session.UsageOptions{})
	if err != nil {
		t.Fatalf("UsageReport: %v", err)
	}
	if report.Totals.Sessions != 1 || report.Totals.InputTokens != 10 {
		t.Fatalf("totals = %+v, want one 10-token session", report.Totals)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func mustSessionData(t *testing.T, v any) []byte {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("MarshalData: %v", err)
	}
	return data
}
