package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func writeExecutable(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o755); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
}

func writeSweepWrapperFakeTools(t *testing.T, binDir string) {
	t.Helper()

	writeExecutable(t, filepath.Join(binDir, "docker"), `#!/usr/bin/env bash
set -euo pipefail
case "${1:-}" in
  info)
    if [[ "${2:-}" == "--format" ]]; then
      echo x86_64
    fi
    exit 0
    ;;
  create)
    echo fake-container
    exit 0
    ;;
  cp)
    dest="${@: -1}"
    mkdir -p "${dest}"
    echo fake > "${dest}/tool"
    exit 0
    ;;
  rm|build)
    exit 0
    ;;
  *)
    exit 0
    ;;
esac
`)

	writeExecutable(t, filepath.Join(binDir, "harbor"), `#!/usr/bin/env bash
exit 0
`)

	writeExecutable(t, filepath.Join(binDir, "go"), `#!/usr/bin/env bash
set -euo pipefail
out=""
while [[ $# -gt 0 ]]; do
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
[[ -n "${out}" ]] || exit 0
mkdir -p "$(dirname "${out}")"
if [[ "$(basename "${out}")" == "fiz-bench" ]]; then
  cat > "${out}" <<'BENCH'
#!/usr/bin/env bash
set -euo pipefail
printf '%q ' "$@" >> "${SWEEP_WRAPPER_BENCH_LOG:?}"
printf '\n' >> "${SWEEP_WRAPPER_BENCH_LOG:?}"
cmd="${1:-}"
if [[ $# -gt 0 ]]; then
  shift
fi
case "${cmd}" in
  sweep)
    subset=""
    recipe=""
    phase=""
    lanes=""
    dry_run=0
    while [[ $# -gt 0 ]]; do
      case "$1" in
        --subset)
          subset="$2"
          shift 2
          ;;
        --subset=*)
          subset="${1#*=}"
          shift
          ;;
        --recipe)
          recipe="$2"
          shift 2
          ;;
        --recipe=*)
          recipe="${1#*=}"
          shift
          ;;
        --phase)
          phase="$2"
          shift 2
          ;;
        --phase=*)
          phase="${1#*=}"
          shift
          ;;
        --lanes)
          lanes="$2"
          shift 2
          ;;
        --lanes=*)
          lanes="${1#*=}"
          shift
          ;;
        --dry-run)
          dry_run=1
          shift
          ;;
        *)
          shift
          ;;
      esac
    done
    if [[ "${dry_run}" = "1" ]]; then
      echo "=== Sweep selection ==="
      if [[ -n "${subset}" ]]; then
        echo "  Subset: ${subset}"
      elif [[ -n "${recipe}" ]]; then
        echo "  Recipe: ${recipe}"
      else
        echo "  Phase: ${phase}"
      fi
      if [[ -n "${lanes}" ]]; then
        IFS=',' read -ra lane_items <<< "${lanes}"
        for lane in "${lane_items[@]}"; do
          echo "  Lane: ${lane}"
        done
      else
        echo "  Lane: all"
      fi
    fi
    ;;
  matrix-index)
    ;;
esac
BENCH
else
  cat > "${out}" <<'FIZ'
#!/usr/bin/env bash
exit 0
FIZ
fi
chmod +x "${out}"
`)
}

func writeSweepWrapperTasks(t *testing.T, tasksDir string) {
	t.Helper()

	repoRoot := benchRepoRoot(t)
	paths, err := filepath.Glob(filepath.Join(repoRoot, "scripts", "benchmark", "task-subset-tb21-*.yaml"))
	if err != nil {
		t.Fatalf("Glob(task-subset-tb21-*.yaml): %v", err)
	}
	idPattern := regexp.MustCompile(`^\s*-\s*id:\s*([A-Za-z0-9_.-]+)\s*$`)
	ids := map[string]struct{}{}
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile(%s): %v", path, err)
		}
		for _, line := range strings.Split(string(raw), "\n") {
			match := idPattern.FindStringSubmatch(line)
			if len(match) == 2 {
				ids[match[1]] = struct{}{}
			}
		}
	}
	for id := range ids {
		taskDir := filepath.Join(tasksDir, id)
		writeTestFile(t, filepath.Join(taskDir, "task.toml"), "docker_image = \"example/test:latest\"\n")
		writeTestFile(t, filepath.Join(taskDir, "environment", "Dockerfile"), "FROM scratch\n")
	}
}

func runSweepWrapper(t *testing.T, args ...string) (string, string) {
	t.Helper()

	repoRoot := benchRepoRoot(t)
	tmpRoot := t.TempDir()
	binDir := filepath.Join(tmpRoot, "bin")
	writeSweepWrapperFakeTools(t, binDir)
	writeSweepWrapperTasks(t, filepath.Join(tmpRoot, "tasks"))

	benchLog := filepath.Join(tmpRoot, "bench.log")
	outDir := filepath.Join(tmpRoot, "out")
	overlayDir := filepath.Join(repoRoot, "bench", "results", "external", "terminal-bench-2-1-amd64-preflight")
	t.Cleanup(func() {
		_ = os.RemoveAll(overlayDir)
	})

	cmdArgs := []string{filepath.Join(repoRoot, "scripts", "benchmark", "run_terminalbench_2_1_sweep.sh")}
	cmdArgs = append(cmdArgs, "--tasks-dir", filepath.Join(tmpRoot, "tasks"), "--out", outDir)
	cmdArgs = append(cmdArgs, args...)

	cmd := exec.Command("bash", cmdArgs...)
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(),
		"PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"SWEEP_WRAPPER_BENCH_LOG="+benchLog,
		"BENCHMARK_BIN_DIR="+filepath.Join(tmpRoot, ".local", "bin"),
		"BENCHMARK_RUNTIME_DIR="+filepath.Join(tmpRoot, ".local", "share", "fizeau", "benchmark-runtime"),
		"BENCHMARK_CONFIRM_DELAY=0",
		"HARBOR_SKIP_NATIVE_HOME=1",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run_terminalbench_2_1_sweep.sh %v: %v\n%s", args, err, string(output))
	}
	logData, err := os.ReadFile(benchLog)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", benchLog, err)
	}
	return string(output), string(logData)
}

func TestRunTerminalbenchSweepWrapperSupportsSubsetDryRun(t *testing.T) {
	output, benchLog := runSweepWrapper(t,
		"--subset", "terminalbench-2-1-canary",
		"--lanes", "fiz-vidar-ds4-mtp",
		"--dry-run",
	)

	for _, want := range []string{
		"Dry-run plan:",
		"Subset: terminalbench-2-1-canary",
		"Lane: fiz-vidar-ds4-mtp",
		"[preflight] fiz-vidar-ds4-mtp SKIPPED (provider type ds4 is not preflight-enabled)",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("wrapper output missing %q\n%s", want, output)
		}
	}
	for _, forbidden := range []string{
		"unknown flag: --subset",
		"profile-not-found",
		"command not found",
	} {
		if strings.Contains(output, forbidden) {
			t.Fatalf("wrapper output unexpectedly contained %q\n%s", forbidden, output)
		}
	}
	for _, want := range []string{"sweep", "--subset", "terminalbench-2-1-canary", "--lanes", "fiz-vidar-ds4-mtp", "--dry-run"} {
		if !strings.Contains(benchLog, want) {
			t.Fatalf("bench log missing %q\n%s", want, benchLog)
		}
	}
	if strings.Contains(benchLog, "--recipe") {
		t.Fatalf("bench log unexpectedly used --recipe for subset run\n%s", benchLog)
	}
}

func TestRunTerminalbenchSweepWrapperUsageDoesNotExecuteBackticks(t *testing.T) {
	repoRoot := benchRepoRoot(t)
	cmd := exec.Command("bash", filepath.Join(repoRoot, "scripts", "benchmark", "run_terminalbench_2_1_sweep.sh"), "--bogus-flag")
	cmd.Dir = repoRoot
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("run_terminalbench_2_1_sweep.sh --bogus-flag succeeded unexpectedly\n%s", string(output))
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("unexpected error type %T: %v", err, err)
	}
	if exitErr.ExitCode() != 2 {
		t.Fatalf("exit code = %d, want 2\n%s", exitErr.ExitCode(), string(output))
	}
	text := string(output)
	if !strings.Contains(text, "unknown flag: --bogus-flag") {
		t.Fatalf("output missing unknown-flag error\n%s", text)
	}
	if !strings.Contains(text, "`all` means the staged sweep phases; use `full`/`tb21-all` for the 89-task") {
		t.Fatalf("output missing literal backtick usage text\n%s", text)
	}
	for _, forbidden := range []string{
		"command not found",
		"line 136:",
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("usage output unexpectedly contained %q\n%s", forbidden, text)
		}
	}
}
