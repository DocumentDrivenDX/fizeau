package renamecheck

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScanReportsForbiddenActiveHits(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "README.md", "Run ddx-agent with DDX_AGENT_DEBUG=1.\n")
	writeFile(t, root, "agent.go", "package agent\n")
	writeFile(t, root, "internal/provider/provider.go", "package provider\n")

	findings, err := Scan(Options{Root: root})
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}

	assertFinding(t, findings, "README.md", "ddx-agent")
	assertFinding(t, findings, "README.md", "AGENT_*/DDX_AGENT_*")
	assertFinding(t, findings, "agent.go", "root package agent")
	if got := findSurface(findings, "internal/provider/provider.go", "root package agent"); got {
		t.Fatalf("non-root package was reported as root package agent: %#v", findings)
	}
}

func TestScanSkipsAllowlistedHistoricalAndExternalPaths(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "docs/research/archive.md", "Run ddx-agent from ~/.config/agent.\n")
	writeFile(t, root, "docs/helix/plan.md", "DDx Agent used .agent state.\n")
	writeFile(t, root, ".ddx/beads.jsonl", `{"title":"ddx-agent"}`+"\n")
	writeFile(t, root, ".agents/skills/ddx/SKILL.md", "The .agents directory is external.\n")
	writeFile(t, root, ".claude/skills/ddx/SKILL.md", "The old ddx-agent skill copy is historical.\n")

	findings, err := Scan(Options{Root: root})
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("Scan() findings = %#v, want none", findings)
	}
}

func TestRunReportOnlyReturnsFindingsAndNoError(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "config.yaml", "path: .agent/session.jsonl\n")
	var out bytes.Buffer

	findings, err := Run(Options{Root: root, Out: &out})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("Run() findings = %d, want 1", len(findings))
	}
	if got := out.String(); !strings.Contains(got, "rename-noise: 1 unallowlisted old-name hit") || !strings.Contains(got, "config.yaml:1") {
		t.Fatalf("Run() output = %q", got)
	}
}

func writeFile(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}

func assertFinding(t *testing.T, findings []Finding, path, surface string) {
	t.Helper()
	if !findSurface(findings, path, surface) {
		t.Fatalf("missing finding for %s %s in %#v", path, surface, findings)
	}
}

func findSurface(findings []Finding, path, surface string) bool {
	for _, f := range findings {
		if f.Path == path && f.Surface == surface {
			return true
		}
	}
	return false
}
