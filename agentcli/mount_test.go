package agentcli

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestMountCLI_ReturnsFreshCommandAndInjectedOutput(t *testing.T) {
	var stdout, stderr bytes.Buffer
	cmd := MountCLI(
		WithUse("ddx agent"),
		WithShort("mounted agent"),
		WithStdout(&stdout),
		WithStderr(&stderr),
		WithVersion("v-mounted", "2026-04-30T00:00:00Z", "abc123"),
	)
	if cmd.Use != "ddx agent" {
		t.Fatalf("Use = %q, want override", cmd.Use)
	}
	if cmd.Short != "mounted agent" {
		t.Fatalf("Short = %q, want override", cmd.Short)
	}
	other := MountCLI()
	if other == cmd {
		t.Fatal("MountCLI returned the same command instance twice")
	}

	cmd.SetArgs([]string{"--version"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := stdout.String(); !strings.Contains(got, "ddx-agent v-mounted") || !strings.Contains(got, "abc123") {
		t.Fatalf("stdout = %q, want injected version output", got)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestMountCLI_ReturnsExitErrorForNonZeroRunnerExit(t *testing.T) {
	var stdout, stderr bytes.Buffer
	cmd := MountCLI(WithStdout(&stdout), WithStderr(&stderr))
	cmd.SetArgs([]string{"--definitely-not-a-real-flag"})
	err := cmd.Execute()
	var exitErr *ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("Execute error = %T %v, want ExitError", err, err)
	}
	if exitErr.Code == 0 {
		t.Fatalf("ExitError.Code = 0, want non-zero")
	}
}
