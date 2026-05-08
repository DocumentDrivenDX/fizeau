package main

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

var officialMediumFizProfiles = []string{
	"fiz-harness-claude-sonnet-4-6",
	"fiz-harness-codex-gpt-5-4-mini",
	"fiz-openrouter-claude-sonnet-4-6",
	"fiz-openrouter-gpt-5-4-mini",
}

func officialMediumFizProfilesCSV() string {
	return strings.Join(officialMediumFizProfiles, ",")
}

func TestMediumTerminalbenchComparisonProfilesAreOfficialFizLanes(t *testing.T) {
	repoRoot := benchRepoRoot(t)
	outDir := t.TempDir()

	code := cmdMatrix([]string{
		"--work-dir", repoRoot,
		"--harnesses", "noop",
		"--profiles", officialMediumFizProfilesCSV(),
		"--reps", "1",
		"--out", outDir,
	})
	if code != 0 {
		t.Fatalf("cmdMatrix exit = %d, want 0", code)
	}

	matrix := readMatrixOutput(t, filepath.Join(outDir, "matrix.json"))
	if !reflect.DeepEqual(matrix.Profiles, officialMediumFizProfiles) {
		t.Fatalf("matrix profiles = %v, want %v", matrix.Profiles, officialMediumFizProfiles)
	}
}
