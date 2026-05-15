package main

import (
	"strings"
	"testing"
)

func TestMediumTerminalbenchComparisonDryRunPrintsOfficialFizWrapperLanes(t *testing.T) {
	repoRoot := benchRepoRoot(t)
	outDir := t.TempDir()

	code, output := captureStdout(t, func() int {
		return cmdSweep([]string{
			"--work-dir", repoRoot,
			"--recipe", "medium-model",
			"--dry-run",
			"--out", outDir,
		})
	})

	if code != 0 {
		t.Fatalf("cmdSweep dry-run medium-model exit = %d\noutput:\n%s", code, output)
	}
	required := []string{
		"Recipe: medium-model",
		"Lane: fiz-harness-claude-sonnet-4-6",
		"Lane: fiz-harness-codex-gpt-5-4-mini",
		"Lane: fiz-harness-pi-gpt-5-4-mini",
		"Lane: fiz-harness-opencode-gpt-5-4-mini",
		"Lane: fiz-openrouter-claude-sonnet-4-6",
		"Lane: fiz-openrouter-gpt-5-4-mini",
	}
	for _, want := range required {
		if !strings.Contains(output, want) {
			t.Fatalf("dry-run output missing %q\nfull output:\n%s", want, output)
		}
	}
}
