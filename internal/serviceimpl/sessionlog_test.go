package serviceimpl

import (
	"path/filepath"
	"testing"
)

func TestSessionLogDirPrefersOverride(t *testing.T) {
	if got := SessionLogDir("/override", "/config"); got != "/override" {
		t.Fatalf("SessionLogDir() = %q, want override", got)
	}
}

func TestSessionLogDirFallsBackToConfig(t *testing.T) {
	if got := SessionLogDir("", "/config"); got != "/config" {
		t.Fatalf("SessionLogDir() = %q, want config", got)
	}
}

func TestSessionLogPath(t *testing.T) {
	got, err := SessionLogPath("/logs", "svc-123")
	if err != nil {
		t.Fatalf("SessionLogPath: %v", err)
	}
	if want := filepath.Join("/logs", "svc-123.jsonl"); got != want {
		t.Fatalf("SessionLogPath() = %q, want %q", got, want)
	}
}

func TestSessionLogPathRejectsMissingInputs(t *testing.T) {
	if _, err := SessionLogPath("", "svc-123"); err == nil {
		t.Fatal("SessionLogPath empty dir: got nil error")
	}
	if _, err := SessionLogPath("/logs", ""); err == nil {
		t.Fatal("SessionLogPath empty sessionID: got nil error")
	}
}
