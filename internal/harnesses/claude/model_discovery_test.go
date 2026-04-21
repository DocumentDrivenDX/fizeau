package claude

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/agent/internal/harnesses/ptyquota"
	"github.com/DocumentDrivenDX/agent/internal/pty/cassette"
	"github.com/stretchr/testify/require"
)

func TestParseClaudeModelsAndReasoning(t *testing.T) {
	text := "Model: 'sonnet' or 'opus' or 'claude-sonnet-4-6'\n--effort <level> (low, medium, high, xhigh, max)"
	require.Equal(t, []string{"claude-sonnet-4-6", "sonnet", "opus"}, parseClaudeModels(text))
	require.Equal(t, []string{"low", "medium", "high", "xhigh", "max"}, parseClaudeReasoningLevels(text))
}

func TestReadClaudeReasoningFromHelp(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell-backed helper requires Unix script")
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "fake-claude")
	require.NoError(t, os.WriteFile(script, []byte(`#!/bin/sh
cat <<'EOF'
Usage: claude [options]
  --effort <level>  Effort level for the current session (low, medium, high, xhigh, max)
EOF
`), 0o700))

	levels, err := ReadClaudeReasoningFromHelp(context.Background(), script)
	require.NoError(t, err)
	require.Equal(t, []string{"low", "medium", "high", "xhigh", "max"}, levels)
}

func TestReadClaudeModelDiscoveryViaPTYRecordsDiscovery(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell-backed PTY probes require Unix PTY support")
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "fake-claude")
	require.NoError(t, os.WriteFile(script, []byte(`#!/bin/sh
printf '❯ '
IFS= read line
printf '/model\r\nSelect model\r\n> sonnet\r\n  opus\r\n  claude-sonnet-4-6\r\n❯ '
sleep 1
`), 0o700))
	cassetteDir := filepath.Join(dir, "cassette")

	snapshot, err := ReadClaudeModelDiscoveryViaPTY(2*time.Second, WithQuotaPTYCommand(script), WithQuotaPTYCassetteDir(cassetteDir))
	require.NoError(t, err)
	require.Equal(t, []string{"claude-sonnet-4-6", "sonnet", "opus"}, snapshot.Models)
	require.Contains(t, snapshot.ReasoningLevels, "xhigh")

	replayed, err := ReadClaudeModelDiscoveryFromCassette(cassetteDir)
	require.NoError(t, err)
	require.Equal(t, snapshot.Models, replayed.Models)
	reader, err := cassette.Open(cassetteDir)
	require.NoError(t, err)
	require.NotNil(t, reader.Discovery())
}

func TestReadClaudeModelDiscoveryViaPTYRejectsEmptyMenu(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell-backed PTY probes require Unix PTY support")
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "fake-claude")
	require.NoError(t, os.WriteFile(script, []byte(`#!/bin/sh
printf '❯ '
IFS= read line
printf '/model\r\nNo model picker\r\n'
sleep 5
`), 0o700))
	cassetteDir := filepath.Join(dir, "cassette")

	_, err := ReadClaudeModelDiscoveryViaPTY(200*time.Millisecond, WithQuotaPTYCommand(script), WithQuotaPTYCassetteDir(cassetteDir))
	require.Error(t, err)
	require.Equal(t, ptyquota.StatusError, ptyquota.ErrorStatus(err))
	_, statErr := os.Stat(filepath.Join(cassetteDir, cassette.ManifestFile))
	require.True(t, errors.Is(statErr, os.ErrNotExist), "empty model output should not promote a cassette")
}
