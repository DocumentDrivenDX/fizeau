package codex

import (
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

func TestParseCodexModels(t *testing.T) {
	models := parseCodexModels("Select model\r\n> gpt-5.4\r\n  gpt-5.4-mini\r\n  gpt-5.4\r\n")
	require.Equal(t, []string{"gpt-5.4", "gpt-5.4-mini"}, models)
}

func TestReadCodexModelDiscoveryViaPTYRecordsDiscovery(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell-backed PTY probes require Unix PTY support")
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "fake-codex")
	require.NoError(t, os.WriteFile(script, []byte(`#!/bin/sh
printf '› '
IFS= read line
printf '/model\r\nSelect model\r\n> gpt-5.4\r\n  gpt-5.4-mini\r\n› '
sleep 1
`), 0o700))
	cassetteDir := filepath.Join(dir, "cassette")

	snapshot, err := ReadCodexModelDiscoveryViaPTY(2*time.Second, WithQuotaPTYCommand(script), WithQuotaPTYCassetteDir(cassetteDir))
	require.NoError(t, err)
	require.Equal(t, []string{"gpt-5.4", "gpt-5.4-mini"}, snapshot.Models)
	require.Contains(t, snapshot.ReasoningLevels, "high")

	replayed, err := ReadCodexModelDiscoveryFromCassette(cassetteDir)
	require.NoError(t, err)
	require.Equal(t, snapshot.Models, replayed.Models)
	reader, err := cassette.Open(cassetteDir)
	require.NoError(t, err)
	require.NotNil(t, reader.Discovery())
}

func TestReadCodexModelDiscoveryViaPTYRejectsEmptyMenu(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell-backed PTY probes require Unix PTY support")
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "fake-codex")
	require.NoError(t, os.WriteFile(script, []byte(`#!/bin/sh
printf '› '
IFS= read line
printf '/model\r\nNo models available\r\n'
sleep 5
`), 0o700))
	cassetteDir := filepath.Join(dir, "cassette")

	_, err := ReadCodexModelDiscoveryViaPTY(200*time.Millisecond, WithQuotaPTYCommand(script), WithQuotaPTYCassetteDir(cassetteDir))
	require.Error(t, err)
	require.Equal(t, ptyquota.StatusError, ptyquota.ErrorStatus(err))
	_, statErr := os.Stat(filepath.Join(cassetteDir, cassette.ManifestFile))
	require.True(t, errors.Is(statErr, os.ErrNotExist), "empty model output should not promote a cassette")
}
