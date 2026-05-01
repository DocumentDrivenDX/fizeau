package agentcli

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestMountCLI_ReturnsFreshCommandAndInjectedOutput(t *testing.T) {
	oldVersion, oldBuildTime, oldGitCommit := Version, BuildTime, GitCommit
	t.Cleanup(func() {
		Version, BuildTime, GitCommit = oldVersion, oldBuildTime, oldGitCommit
	})

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
	if got := stdout.String(); !strings.Contains(got, "fiz v-mounted") || !strings.Contains(got, "abc123") {
		t.Fatalf("stdout = %q, want injected version output", got)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	if Version != "v-mounted" || BuildTime != "2026-04-30T00:00:00Z" || GitCommit != "abc123" {
		t.Fatalf("package version metadata = %q %q %q, want mounted metadata", Version, BuildTime, GitCommit)
	}
}

func TestMountCLI_ReturnsExitErrorForNonZeroRunnerExit(t *testing.T) {
	var stdout, stderr bytes.Buffer
	cmd := MountCLI(WithStdout(&stdout), WithStderr(&stderr))
	cmd.SetArgs([]string{"--", "--definitely-not-a-real-flag"})
	err := cmd.Execute()
	var exitErr *ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("Execute error = %T %v, want ExitError", err, err)
	}
	if exitErr.Code == 0 {
		t.Fatalf("ExitError.Code = 0, want non-zero")
	}
}

func TestMountCLI_RegistersExistingTopLevelCommands(t *testing.T) {
	cmd := MountCLI()
	for _, name := range mountedSubcommands() {
		if child, _, err := cmd.Find([]string{name}); err != nil || child == cmd || child.Name() != name {
			t.Fatalf("Find(%q) = child %q err %v, want registered child", name, child.Name(), err)
		}
	}
}

func TestMountCLI_MigratedCommandsUseNativeFlagParsing(t *testing.T) {
	cmd := MountCLI()
	for _, path := range [][]string{
		{"log"},
		{"replay"},
		{"usage"},
		{"route-status"},
		{"corpus"},
		{"corpus", "list"},
		{"corpus", "promote"},
		{"corpus", "validate"},
		{"import"},
		{"update"},
		{"catalog", "update"},
		{"catalog", "update-pricing"},
	} {
		child, _, err := cmd.Find(path)
		if err != nil {
			t.Fatalf("Find(%v): %v", path, err)
		}
		if child.DisableFlagParsing {
			t.Fatalf("Find(%v) has DisableFlagParsing=true, want native pflag parsing", path)
		}
	}
}

func TestMountCLI_ChildCommandsDelegateWithoutExiting(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "version", args: []string{"version", "--check-only"}},
		{name: "replay", args: []string{"replay"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			cmd := MountCLI(
				WithStdout(&stdout),
				WithStderr(&stderr),
				WithVersion("dev", "", ""),
			)
			cmd.SetArgs(tt.args)
			err := cmd.Execute()
			if err == nil {
				return
			}
			var exitErr *ExitError
			if !errors.As(err, &exitErr) {
				t.Fatalf("Execute error = %T %v, want nil or ExitError", err, err)
			}
		})
	}
}

func TestNeedsLegacyPassthrough(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want bool
	}{
		{name: "root version", args: []string{"--version"}, want: false},
		{name: "root prompt", args: []string{"--work-dir", "/tmp/work", "-p", "hello"}, want: false},
		{name: "explicit run", args: []string{"--work-dir", "/tmp/work", "run", "hello"}, want: false},
		{name: "native session command", args: []string{"--work-dir", "/tmp/work", "usage", "--since", "7d"}, want: false},
		{name: "native route status command", args: []string{"--work-dir", "/tmp/work", "route-status", "--profile", "smart"}, want: false},
		{name: "native read-only catalog subcommand", args: []string{"--work-dir", "/tmp/work", "catalog", "show"}, want: false},
		{name: "native mutating catalog subcommand", args: []string{"--work-dir", "/tmp/work", "catalog", "update"}, want: false},
		{name: "native import command", args: []string{"--work-dir", "/tmp/work", "import", "pi", "--diff"}, want: false},
		{name: "unknown positional", args: []string{"unknown-subcommand"}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NeedsLegacyPassthrough(tt.args); got != tt.want {
				t.Fatalf("NeedsLegacyPassthrough(%v) = %t, want %t", tt.args, got, tt.want)
			}
		})
	}
}
