package fizeau

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/easel/fizeau/internal/harnesses"
)

type claudeTestQuotaSnapshot struct {
	CapturedAt        time.Time               `json:"captured_at"`
	FiveHourRemaining int                     `json:"five_hour_remaining"`
	FiveHourLimit     int                     `json:"five_hour_limit"`
	WeeklyRemaining   int                     `json:"weekly_remaining"`
	WeeklyLimit       int                     `json:"weekly_limit"`
	Windows           []harnesses.QuotaWindow `json:"windows,omitempty"`
	Source            string                  `json:"source"`
	Account           *harnesses.AccountInfo  `json:"account,omitempty"`
}

func writeClaudeQuotaCacheFile(t *testing.T, path string, snap claudeTestQuotaSnapshot) {
	t.Helper()
	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		t.Fatalf("marshal claude quota cache: %v", err)
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir claude quota cache dir: %v", err)
	}
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o600); err != nil {
		t.Fatalf("write claude quota cache: %v", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		t.Fatalf("rename claude quota cache: %v", err)
	}
}

func readClaudeQuotaCacheFile(t *testing.T, path string) (*claudeTestQuotaSnapshot, bool) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	var snap claudeTestQuotaSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		t.Fatalf("decode claude quota cache: %v", err)
	}
	return &snap, true
}
