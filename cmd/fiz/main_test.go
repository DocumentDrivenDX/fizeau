package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/DocumentDrivenDX/fizeau/agentcli"
)

func TestEffectiveVersionKeepsStampedVersion(t *testing.T) {
	if got := effectiveVersion("v1.2.3"); got != "v1.2.3" {
		t.Fatalf("effectiveVersion(stamped) = %q, want stamped version", got)
	}
}

func TestProfileHelpAvoidsLegacyCodeHighAndCodeMedium(t *testing.T) {
	var stdout, stderr bytes.Buffer
	cmd := agentcli.MountCLI(
		agentcli.WithStdout(&stdout),
		agentcli.WithStderr(&stderr),
	)
	cmd.SetArgs([]string{"route-status", "--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	help := stdout.String() + stderr.String()
	for _, want := range []string{"--profile", "--min-power", "--max-power"} {
		if !strings.Contains(help, want) {
			t.Fatalf("help output missing %q:\n%s", want, help)
		}
	}
	for _, legacy := range []string{"code-high", "code-medium"} {
		if strings.Contains(help, legacy) {
			t.Fatalf("help output must not mention %q:\n%s", legacy, help)
		}
	}
}
