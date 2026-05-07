package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

var officialMediumFizProfiles = []string{
	"fiz-harness-claude-sonnet-4-6",
	"fiz-harness-codex-gpt-5-4-mini",
	"fiz-harness-pi-gpt-5-4-mini",
	"fiz-harness-opencode-gpt-5-4-mini",
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

func TestRunMediumModelComparisonScriptUsesOnlyFizHarborAgent(t *testing.T) {
	repoRoot := benchRepoRoot(t)

	t.Run("default_force_build", func(t *testing.T) {
		lines := runMediumComparisonScript(t, repoRoot, map[string]string{})
		matrixLine := findLineContaining(t, lines, "matrix --profiles=")
		normalized := strings.ReplaceAll(matrixLine, `\,`, ",")
		if !strings.Contains(matrixLine, "--harnesses=fiz") {
			t.Fatalf("matrix invocation does not pin Harbor to fiz: %s", matrixLine)
		}
		if !strings.Contains(normalized, officialMediumFizProfilesCSV()) {
			t.Fatalf("matrix invocation missing official profile CSV: %s", matrixLine)
		}
		buildLine := findLineContaining(t, lines, " build ")
		if !strings.Contains(buildLine, "HARBOR_FORCE_BUILD=1") {
			t.Fatalf("default run should set HARBOR_FORCE_BUILD=1: %s", buildLine)
		}
	})

	t.Run("force_build_override", func(t *testing.T) {
		lines := runMediumComparisonScript(t, repoRoot, map[string]string{
			"HARBOR_FORCE_BUILD": "0",
		})
		buildLine := findLineContaining(t, lines, " build ")
		if !strings.Contains(buildLine, "HARBOR_FORCE_BUILD=0") {
			t.Fatalf("override run should preserve HARBOR_FORCE_BUILD=0: %s", buildLine)
		}
		matrixLine := findLineContaining(t, lines, "matrix --profiles=")
		normalized := strings.ReplaceAll(matrixLine, `\,`, ",")
		if !strings.Contains(matrixLine, "--harnesses=fiz") {
			t.Fatalf("matrix invocation does not pin Harbor to fiz: %s", matrixLine)
		}
		if !strings.Contains(normalized, officialMediumFizProfilesCSV()) {
			t.Fatalf("matrix invocation missing official profile CSV: %s", matrixLine)
		}
	})
}

func findLineContaining(t *testing.T, lines []string, needle string) string {
	t.Helper()
	for _, line := range lines {
		if strings.Contains(line, needle) {
			return line
		}
	}
	t.Fatalf("did not find line containing %q in %v", needle, lines)
	return ""
}

func runMediumComparisonScript(t *testing.T, repoRoot string, extraEnv map[string]string) []string {
	t.Helper()

	binDir := t.TempDir()
	goLog := filepath.Join(binDir, "go.log")
	goPath := filepath.Join(binDir, "go")
	harborPath := filepath.Join(binDir, "harbor")
	goWrapper := `#!/usr/bin/env bash
set -euo pipefail
{
  printf 'HARBOR_FORCE_BUILD=%s ' "${HARBOR_FORCE_BUILD:-}"
  printf '%q ' go "$@"
  printf '\n'
} >> "$GO_LOG"
case "${1:-}" in
  build)
    out=""
    while (($#)); do
      case "$1" in
        -o)
          out="$2"
          shift 2
          ;;
        *)
          shift
          ;;
      esac
    done
    if [[ -n "$out" ]]; then
      mkdir -p "$(dirname "$out")"
      printf '#!/usr/bin/env bash\nexit 0\n' > "$out"
      chmod 755 "$out"
    fi
    ;;
  run)
    ;;
esac
exit 0
`
	if err := os.WriteFile(goPath, []byte(goWrapper), 0o755); err != nil {
		t.Fatalf("write fake go: %v", err)
	}

	harborWrapper := `#!/usr/bin/env bash
set -euo pipefail
exit 0
`
	if err := os.WriteFile(harborPath, []byte(harborWrapper), 0o755); err != nil {
		t.Fatalf("write fake harbor: %v", err)
	}

	cmd := exec.Command("bash", filepath.Join(repoRoot, "scripts", "benchmark", "run_medium_model_terminalbench_comparison.sh"), "canary")
	cmd.Dir = repoRoot
	cmd.Env = make([]string, 0, len(os.Environ())+5)
	for _, kv := range os.Environ() {
		switch {
		case strings.HasPrefix(kv, "PATH="):
			continue
		case strings.HasPrefix(kv, "OPENROUTER_API_KEY="):
			continue
		case strings.HasPrefix(kv, "GOFLAGS="):
			continue
		case strings.HasPrefix(kv, "GO_LOG="):
			continue
		}
		cmd.Env = append(cmd.Env, kv)
	}
	cmd.Env = append(cmd.Env,
		"PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"OPENROUTER_API_KEY=dummy",
		"GOFLAGS=-buildvcs=false",
		"GO_LOG="+goLog,
	)
	for k, v := range extraEnv {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("medium comparison script failed: %v\n%s", err, string(out))
	}

	raw, err := os.ReadFile(goLog)
	if err != nil {
		t.Fatalf("read go log: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return nil
	}
	return lines
}
