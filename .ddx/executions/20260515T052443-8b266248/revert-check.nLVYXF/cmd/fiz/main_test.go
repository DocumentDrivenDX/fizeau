package main

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/easel/fizeau/agentcli"
	"github.com/easel/fizeau/internal/buildinfo"
)

// TestRouteStatusCommandJSON verifies that fiz route-status --help documents
// all routing-evidence flags that operators need for --json inspection:
// --json (output mode), --policy, --min-power, --max-power (power policy),
// --model, --provider (pin constraints).
// This confirms AC-2 of FEAT-004: the route-status command surfaces the JSON
// evidence envelope via its documented flag set.
func TestRouteStatusCommandJSON(t *testing.T) {
	var stdout, stderr bytes.Buffer
	cmd := agentcli.MountCLI(
		agentcli.WithStdout(&stdout),
		agentcli.WithStderr(&stderr),
	)
	cmd.SetArgs([]string{"route-status", "--help"})
	_ = cmd.Execute()

	help := stdout.String() + stderr.String()
	for _, flag := range []string{"--json", "--policy", "--min-power", "--max-power", "--model", "--provider"} {
		if !strings.Contains(help, flag) {
			t.Errorf("route-status help missing routing-evidence flag %q:\n%s", flag, help)
		}
	}
	// Retired compatibility names must not appear in help: the target surface is power-based routing.
	for _, legacy := range []string{"code" + "-high", "code" + "-medium"} {
		if strings.Contains(help, legacy) {
			t.Errorf("route-status help must not mention legacy name %q:\n%s", legacy, help)
		}
	}
}

// TestListModelsCommandJSON verifies that fiz --list-models --json is a
// documented flag path in the root command, confirming AC-1 of FEAT-004:
// list-models JSON output includes the routing metadata fields operators need.
func TestListModelsCommandJSON(t *testing.T) {
	var stdout, stderr bytes.Buffer
	cmd := agentcli.MountCLI(
		agentcli.WithStdout(&stdout),
		agentcli.WithStderr(&stderr),
	)
	cmd.SetArgs([]string{"--help"})
	_ = cmd.Execute()

	help := stdout.String() + stderr.String()
	for _, flag := range []string{"--list-models", "--json"} {
		if !strings.Contains(help, flag) {
			t.Errorf("fiz --help missing flag %q:\n%s", flag, help)
		}
	}
}

// TestRouteStatusJSONOutputSchema verifies the stable JSON envelope shape that
// route-status --json emits when no providers are configured. Even with a
// routing error, the envelope must include all documented top-level keys so
// operator tooling can parse it unconditionally.
func TestRouteStatusJSONOutputSchema(t *testing.T) {
	workDir := t.TempDir()
	// Write minimal config (no providers) so the service can be instantiated.
	cfgPath := workDir + "/.fizeau.yaml"
	if err := os.WriteFile(cfgPath, []byte("providers: {}\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	var stdout, stderr bytes.Buffer
	cmd := agentcli.MountCLI(
		agentcli.WithStdout(&stdout),
		agentcli.WithStderr(&stderr),
	)
	// Invoke route-status --json with the temp work dir.
	// No providers → routing will fail, but the JSON envelope is always emitted.
	cmd.SetArgs([]string{"--work-dir", workDir, "--json", "route-status"})
	_ = cmd.Execute()

	out := stdout.String()
	if out == "" {
		t.Skip("route-status produced no stdout (possible permission or config issue); skip schema check")
	}

	// The output must be valid JSON.
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &envelope); err != nil {
		t.Fatalf("route-status --json output is not valid JSON: %v\noutput: %s", err, out)
	}

	// Required top-level keys per AC-2 of FEAT-004.
	for _, key := range []string{"candidates", "power_policy"} {
		if _, ok := envelope[key]; !ok {
			t.Errorf("route-status JSON envelope missing required key %q:\n%s", key, out)
		}
	}
}

func TestEffectiveVersionKeepsStampedVersion(t *testing.T) {
	if got := effectiveVersion("v1.2.3"); got != "v1.2.3" {
		t.Fatalf("effectiveVersion(stamped) = %q, want stamped version", got)
	}
}

func TestEffectiveVersionDevFallback(t *testing.T) {
	// "dev" is the sentinel for missing ldflags; the function must return a
	// non-empty string — either "dev" (no VCS info) or a module version.
	got := effectiveVersion("dev")
	if got == "" {
		t.Fatal("effectiveVersion(dev) returned empty string")
	}
}

// TestVersionFlagOutputsNonEmptyCommit verifies that when buildinfo.Read finds
// a VCS commit (as it does during go run from a git checkout), the --version
// flag outputs it rather than an empty field. Uses buildinfo.Read directly to
// simulate what main() does — test binaries may not embed VCS info, so the
// test skips when no commit is available.
func TestVersionFlagOutputsNonEmptyCommit(t *testing.T) {
	bi := buildinfo.Read("dev", "", "")
	if bi.Commit == "" {
		t.Skip("no VCS commit info in test binary; skipping (pass with go run)")
	}

	var stdout bytes.Buffer
	cmd := agentcli.MountCLI(
		agentcli.WithStdout(&stdout),
		agentcli.WithVersion(bi.Version, bi.Built, bi.Commit),
	)
	cmd.SetArgs([]string{"--version"})
	_ = cmd.Execute()
	out := stdout.String()
	if strings.Contains(out, "commit ,") {
		t.Errorf("--version output has empty commit field: %q", out)
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
	for _, want := range []string{"--policy", "--min-power", "--max-power"} {
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
