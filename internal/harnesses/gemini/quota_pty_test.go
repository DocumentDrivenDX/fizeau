package gemini

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/easel/fizeau/internal/pty/cassette"
)

func TestReadGeminiQuotaViaPTY_CapturesTierUsage(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell-backed PTY probes require Unix PTY support")
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "fake-gemini")
	// Emits a ready prompt, reads the slash-command, then prints a
	// fixture resembling Gemini CLI 0.38.2 /model manage output. The
	// sleep keeps the script alive long enough for the probe to harvest
	// the screen before we send SIGTERM via the probe's stop path.
	body := `#!/bin/sh
printf 'Gemini CLI 0.38.2\r\n> '
IFS= read line
cat <<'EOF'
Model management

  Flash         4% used      Resets 9:13 PM (23h 46m)
  Flash Lite    0% used      Resets 9:27 PM (24h)
  Pro         100% used

EOF
sleep 2
`
	if err := os.WriteFile(script, []byte(body), 0o700); err != nil {
		t.Fatalf("write script: %v", err)
	}
	cassetteDir := filepath.Join(dir, "cassette")

	windows, err := ReadGeminiQuotaViaPTY(3*time.Second, WithQuotaPTYCommand(script), WithQuotaPTYCassetteDir(cassetteDir))
	if err != nil {
		t.Fatalf("ReadGeminiQuotaViaPTY: %v", err)
	}
	if len(windows) != 3 {
		t.Fatalf("want 3 tier windows, got %d: %#v", len(windows), windows)
	}
	flash := FindGeminiQuotaWindow(windows, "gemini-flash")
	if flash == nil || flash.UsedPercent != 4 {
		t.Fatalf("flash parsed from PTY: %#v", flash)
	}
	lite := FindGeminiQuotaWindow(windows, "gemini-flash-lite")
	if lite == nil || lite.UsedPercent != 0 {
		t.Fatalf("flash-lite parsed from PTY: %#v", lite)
	}
	pro := FindGeminiQuotaWindow(windows, "gemini-pro")
	if pro == nil || pro.UsedPercent != 100 || pro.State != "exhausted" {
		t.Fatalf("pro parsed from PTY: %#v", pro)
	}

	reader, err := cassette.Open(cassetteDir)
	if err != nil {
		t.Fatalf("cassette.Open: %v", err)
	}
	quota := reader.Quota()
	if quota.Source != "pty" {
		t.Fatalf("cassette quota source: %q", quota.Source)
	}
	if quota.CapturedAt == "" {
		t.Fatal("cassette quota must record captured_at")
	}
	if quota.FreshnessWindow != DefaultGeminiQuotaStaleAfter.String() {
		t.Fatalf("cassette freshness window: %q", quota.FreshnessWindow)
	}
	if len(quota.Windows) != 3 {
		t.Fatalf("cassette windows: %d", len(quota.Windows))
	}
}

func TestReadGeminiQuotaFromCassette_RoundTrip(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell-backed PTY probes require Unix PTY support")
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "fake-gemini")
	body := `#!/bin/sh
printf '> '
IFS= read line
cat <<'EOF'
  Flash    4% used      Resets 9:13 PM
  Pro    100% used
EOF
sleep 2
`
	if err := os.WriteFile(script, []byte(body), 0o700); err != nil {
		t.Fatalf("write: %v", err)
	}
	cassetteDir := filepath.Join(dir, "cassette")
	if _, err := ReadGeminiQuotaViaPTY(3*time.Second, WithQuotaPTYCommand(script), WithQuotaPTYCassetteDir(cassetteDir)); err != nil {
		t.Fatalf("ReadGeminiQuotaViaPTY: %v", err)
	}
	windows, err := ReadGeminiQuotaFromCassette(cassetteDir)
	if err != nil {
		t.Fatalf("ReadGeminiQuotaFromCassette: %v", err)
	}
	if len(windows) != 2 {
		t.Fatalf("cassette replay lost a tier: %#v", windows)
	}
}
