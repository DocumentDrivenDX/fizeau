package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/easel/fizeau/internal/benchmark/profile"
)

func TestMatrixNoopDumbScriptProducesValidMatrix(t *testing.T) {
	repoRoot := benchRepoRoot(t)
	outDir := t.TempDir()

	code := cmdMatrix([]string{
		"--work-dir", repoRoot,
		"--harnesses", "noop,dumb_script",
		"--profiles", "noop",
		"--reps", "2",
		"--out", outDir,
	})
	if code != 0 {
		t.Fatalf("cmdMatrix exit = %d, want 0", code)
	}

	matrix := readMatrixOutput(t, filepath.Join(outDir, "matrix.json"))
	if got, want := len(matrix.Runs), 12; got != want {
		t.Fatalf("runs = %d, want %d", got, want)
	}
	for _, run := range matrix.Runs {
		if run.Harness == "" || run.ProfileID == "" || run.TaskID == "" {
			t.Fatalf("incomplete run identity: %+v", run)
		}
		if run.AdapterModule == "" || run.HarborAgent == "" {
			t.Fatalf("missing adapter path metadata: %+v", run)
		}
		if run.FinalStatus == "" {
			t.Fatalf("missing final_status: %+v", run)
		}
		if _, err := os.Stat(filepath.Join(run.OutputDir, matrixReportName)); err != nil {
			t.Fatalf("report missing for %s/%s/%s: %v", run.Harness, run.ProfileID, run.TaskID, err)
		}
	}
}

func TestResolveMatrixTaskPathHarborDownloadLayout(t *testing.T) {
	root := t.TempDir()
	taskDir := filepath.Join(root, "terminal-bench", "fix-git", "abcdef")
	if err := os.MkdirAll(taskDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(taskDir, "task.toml"), []byte("schema_version = \"1.1\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := resolveMatrixTaskPath(root, "fix-git")
	if err != nil {
		t.Fatalf("resolveMatrixTaskPath returned error: %v", err)
	}
	if got != taskDir {
		t.Fatalf("resolveMatrixTaskPath = %q, want %q", got, taskDir)
	}
}

func TestMatrixResumeSkipsTerminalReport(t *testing.T) {
	repoRoot := benchRepoRoot(t)
	outDir := t.TempDir()

	args := []string{
		"--work-dir", repoRoot,
		"--harnesses", "dumb_script",
		"--profiles", "noop",
		"--reps", "1",
		"--out", outDir,
	}
	if code := cmdMatrix(args); code != 0 {
		t.Fatalf("initial cmdMatrix exit = %d, want 0", code)
	}

	reportPath := filepath.Join(outDir, "cells", "dumb_script", "noop", "rep-001", "fix-git", matrixReportName)
	before, err := os.Stat(reportPath)
	if err != nil {
		t.Fatalf("stat report: %v", err)
	}
	time.Sleep(20 * time.Millisecond)
	if code := cmdMatrix(append(args, "--resume")); code != 0 {
		t.Fatalf("resume cmdMatrix exit = %d, want 0", code)
	}
	after, err := os.Stat(reportPath)
	if err != nil {
		t.Fatalf("stat report after resume: %v", err)
	}
	if !after.ModTime().Equal(before.ModTime()) {
		t.Fatalf("terminal report was rewritten on resume: before=%s after=%s", before.ModTime(), after.ModTime())
	}
}

func TestMatrixTupleDirForCanonicalCellsRoot(t *testing.T) {
	cellsRoot := t.TempDir()
	prof := &profile.Profile{
		ID: "fiz-openrouter-qwen3-6-27b",
		Provider: profile.Provider{
			Type:  profile.ProviderOpenRouter,
			Model: "qwen/qwen3.6-27b",
		},
	}
	got := matrixTupleDirFor("/unused/out", cellsRoot, "fiz", prof, 2, "break-filter-js-from-html", "terminal-bench/terminal-bench-2-1")
	want := filepath.Join(cellsRoot, "terminal-bench-2-1", "break-filter-js-from-html", "fiz-openrouter-qwen3-6-27b", "rep-002")
	if got != want {
		t.Fatalf("canonical cell dir = %q, want %q", got, want)
	}

	got = matrixTupleDirFor("/unused/out", cellsRoot, "fiz", prof, 1, "legacy-task", "terminal-bench@2.0")
	want = filepath.Join(cellsRoot, "terminal-bench-2-0", "legacy-task", "fiz-openrouter-qwen3-6-27b", "rep-001")
	if got != want {
		t.Fatalf("canonical legacy cell dir = %q, want %q", got, want)
	}
}

func TestMatrixDatasetVersion(t *testing.T) {
	tests := map[string]string{
		"terminal-bench/terminal-bench-2-1": "2.1",
		"terminal-bench@2.0":                "2.0",
		"terminal-bench/2.0":                "2.0",
	}
	for dataset, want := range tests {
		if got := matrixDatasetVersion(dataset); got != want {
			t.Fatalf("matrixDatasetVersion(%q) = %q, want %q", dataset, got, want)
		}
	}
}

func TestMatrixLockPreventsDoubleSpend(t *testing.T) {
	repoRoot := benchRepoRoot(t)
	outDir := t.TempDir()
	lockPath := filepath.Join(outDir, "cells", "noop", "noop", "rep-001", "fix-git", matrixLockName)
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o750); err != nil {
		t.Fatal(err)
	}
	lock := matrixLock{PID: os.Getpid(), StartedAt: time.Now().UTC()}
	raw, _ := json.Marshal(lock)
	if err := os.WriteFile(lockPath, raw, 0o600); err != nil {
		t.Fatal(err)
	}

	code := cmdMatrix([]string{
		"--work-dir", repoRoot,
		"--harnesses", "noop",
		"--profiles", "noop",
		"--reps", "1",
		"--out", outDir,
	})
	if code == 0 {
		t.Fatal("cmdMatrix succeeded despite live tuple lock")
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(lockPath), matrixReportName)); !os.IsNotExist(err) {
		t.Fatalf("locked tuple should not write report, stat err=%v", err)
	}
}

func TestMatrixResumeAfterStaleLockMatchesCleanRun(t *testing.T) {
	repoRoot := benchRepoRoot(t)
	cleanOut := t.TempDir()
	resumeOut := t.TempDir()

	args := []string{
		"--work-dir", repoRoot,
		"--harnesses", "dumb_script",
		"--profiles", "noop",
		"--reps", "1",
	}
	if code := cmdMatrix(append(args, "--out", cleanOut)); code != 0 {
		t.Fatalf("clean cmdMatrix exit = %d, want 0", code)
	}

	lockPath := filepath.Join(resumeOut, "cells", "dumb_script", "noop", "rep-001", "fix-git", matrixLockName)
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o750); err != nil {
		t.Fatal(err)
	}
	lock := matrixLock{PID: -1, StartedAt: time.Now().UTC().Add(-time.Minute)}
	raw, _ := json.Marshal(lock)
	if err := os.WriteFile(lockPath, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if code := cmdMatrix(append(args, "--out", resumeOut, "--resume")); code != 0 {
		t.Fatalf("resume cmdMatrix exit = %d, want 0", code)
	}

	clean := readMatrixOutput(t, filepath.Join(cleanOut, "matrix.json"))
	resumed := readMatrixOutput(t, filepath.Join(resumeOut, "matrix.json"))
	if len(clean.Runs) != len(resumed.Runs) {
		t.Fatalf("resumed run count = %d, want %d", len(resumed.Runs), len(clean.Runs))
	}
	for i := range clean.Runs {
		if clean.Runs[i].Harness != resumed.Runs[i].Harness ||
			clean.Runs[i].ProfileID != resumed.Runs[i].ProfileID ||
			clean.Runs[i].Rep != resumed.Runs[i].Rep ||
			clean.Runs[i].TaskID != resumed.Runs[i].TaskID ||
			clean.Runs[i].FinalStatus != resumed.Runs[i].FinalStatus {
			t.Fatalf("resumed run[%d] mismatch:\nclean=%+v\nresumed=%+v", i, clean.Runs[i], resumed.Runs[i])
		}
		if clean.Runs[i].Reward == nil && resumed.Runs[i].Reward != nil ||
			clean.Runs[i].Reward != nil && resumed.Runs[i].Reward == nil ||
			clean.Runs[i].Reward != nil && resumed.Runs[i].Reward != nil && *clean.Runs[i].Reward != *resumed.Runs[i].Reward {
			t.Fatalf("resumed reward[%d] mismatch", i)
		}
	}
}

func TestMatrixOverBudgetRunBecomesBudgetHaltedAndContinues(t *testing.T) {
	repoRoot := benchRepoRoot(t)
	outDir := t.TempDir()

	code := cmdMatrix([]string{
		"--work-dir", repoRoot,
		"--harnesses", "cost_probe",
		"--profiles", "smoke",
		"--reps", "2",
		"--per-run-budget-usd", "0.000001",
		"--out", outDir,
	})
	if code != 0 {
		t.Fatalf("cmdMatrix exit = %d, want 0", code)
	}
	matrix := readMatrixOutput(t, filepath.Join(outDir, "matrix.json"))
	if got, want := len(matrix.Runs), 6; got != want {
		t.Fatalf("runs = %d, want %d", got, want)
	}
	for _, run := range matrix.Runs {
		if run.FinalStatus != "budget_halted" {
			t.Fatalf("run %s/%d final_status = %s, want budget_halted", run.TaskID, run.Rep, run.FinalStatus)
		}
		if run.CostUSD <= 0 {
			t.Fatalf("run %s/%d cost = %f, want > 0", run.TaskID, run.Rep, run.CostUSD)
		}
	}
}

func TestSweepShutdownTermGraceful(t *testing.T) {
	tmp := t.TempDir()
	harborPath := filepath.Join(tmp, "harbor")
	logPath := filepath.Join(tmp, "harbor.log")
	script := `#!/usr/bin/env bash
set -euo pipefail
log="${HARBOR_SIGNAL_LOG:?}"
jobs_dir=""
job_name=""
while [[ $# -gt 0 ]]; do
  printf 'arg:%s\n' "$1" >> "${log}"
  case "$1" in
    --jobs-dir)
      jobs_dir="$2"
      shift 2
      ;;
    --job-name)
      job_name="$2"
      shift 2
      ;;
    *)
      shift
      ;;
  esac
done
trap 'printf "term\n" >> "${log}"; mkdir -p "${jobs_dir}/${job_name}/trial/verifier"; printf "0\n" > "${jobs_dir}/${job_name}/trial/verifier/reward.txt"; printf "teardown\n" >> "${log}"; exit 143' TERM
printf "started\n" >> "${log}"
while true; do
  sleep 1
done
`
	if err := os.WriteFile(harborPath, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HARBOR_SIGNAL_LOG", logPath)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	resultCh := make(chan struct {
		result harborRunResult
		err    error
	}, 1)
	go func() {
		result, err := runMatrixHarbor(harborRunOpts{
			harborBin: harborPath,
			taskPath:  filepath.Join(tmp, "task"),
			harness:   "fiz",
			profile: &profile.Profile{
				ID: "noop",
				Provider: profile.Provider{
					Type:    profile.ProviderOpenAICompat,
					BaseURL: "http://127.0.0.1:1/v1",
					Model:   "test-model",
				},
			},
			jobsDir:   filepath.Join(tmp, "jobs"),
			jobName:   "fiz-task-rep1",
			repoRoot:  benchRepoRoot(t),
			parentCtx: ctx,
		})
		resultCh <- struct {
			result harborRunResult
			err    error
		}{result: result, err: err}
	}()

	waitForLogContains(t, logPath, "started", 2*time.Second)
	cancel()

	select {
	case got := <-resultCh:
		if got.err != nil {
			t.Fatalf("runMatrixHarbor returned error: %v", got.err)
		}
		if got.result.exitCode != 143 {
			t.Fatalf("harbor exit code = %d, want 143", got.result.exitCode)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("runMatrixHarbor did not return after parent context cancellation")
	}

	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	log := string(raw)
	for _, want := range []string{"arg:--delete", "term", "teardown"} {
		if !strings.Contains(log, want) {
			t.Fatalf("harbor log missing %q:\n%s", want, log)
		}
	}
}

func TestFizeauProviderEnvMapsOpenRouterCompatProfile(t *testing.T) {
	got := fizeauProviderEnv(&profile.Profile{
		Provider: profile.Provider{
			Type:    profile.ProviderOpenAICompat,
			BaseURL: "https://openrouter.ai/api/v1",
		},
	})
	if got != string(profile.ProviderOpenRouter) {
		t.Fatalf("provider env = %q, want %q", got, profile.ProviderOpenRouter)
	}
}

func TestSamplingEnvPairsOmitsDefaultOnlyNativeOpenAIGPT5Fields(t *testing.T) {
	topP := 0.95
	topK := 20
	got := samplingEnvPairs(&profile.Profile{
		Provider: profile.Provider{
			Type:  profile.ProviderOpenAI,
			Model: "gpt-5.5",
		},
		Sampling: profile.Sampling{
			Temperature: 0,
			TopP:        &topP,
			TopK:        &topK,
		},
	})
	want := []string{"FIZEAU_TOP_K=20"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("sampling env pairs = %#v, want %#v", got, want)
	}
}

func TestSamplingEnvPairsKeepsOpenRouterGPT5Fields(t *testing.T) {
	topP := 0.95
	topK := 20
	got := samplingEnvPairs(&profile.Profile{
		Provider: profile.Provider{
			Type:    profile.ProviderOpenAICompat,
			Model:   "openai/gpt-5.5",
			BaseURL: "https://openrouter.ai/api/v1",
		},
		Sampling: profile.Sampling{
			Temperature: 0,
			TopP:        &topP,
			TopK:        &topK,
		},
	})
	want := []string{"FIZEAU_TEMPERATURE=0", "FIZEAU_TOP_P=0.95", "FIZEAU_TOP_K=20"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("sampling env pairs = %#v, want %#v", got, want)
	}
}

func TestHarborAgentArgsIncludesReferenceHarnessAdapters(t *testing.T) {
	cases := map[string]string{
		"claude": "scripts.benchmark.harbor_adapters.claude:ClaudeAgent",
		"codex":  "scripts.benchmark.harbor_adapters.codex:CodexAgent",
	}
	for harness, want := range cases {
		got := harborAgentArgs(harness)
		if len(got) != 2 || got[0] != "--agent-import-path" || got[1] != want {
			t.Fatalf("harborAgentArgs(%q) = %#v, want import path %q", harness, got, want)
		}
	}
}

func TestConsecutiveFailureHaltTracker(t *testing.T) {
	t.Run("graded fail provider errors halt", func(t *testing.T) {
		tracker := newMatrixConsecutiveFailureTracker(matrixConsecutiveFailureLimit)
		var aborted bool
		var details matrixFailureFingerprint
		for i := 1; i <= matrixConsecutiveFailureLimit; i++ {
			aborted, details = tracker.Observe(matrixRunReport{
				TaskID:      fmt.Sprintf("task-%d", i),
				FinalStatus: "graded_fail",
				Error:       "agent: provider error: connection refused",
			})
		}
		if !aborted {
			t.Fatal("tracker did not abort after 5 identical graded_fail reports")
		}
		if details.hash == "" || len(details.taskIDs) != matrixConsecutiveFailureLimit {
			t.Fatalf("abort details incomplete: %+v", details)
		}
	})

	t.Run("harness crash context canceled halts", func(t *testing.T) {
		tracker := newMatrixConsecutiveFailureTracker(matrixConsecutiveFailureLimit)
		aborted := false
		for i := 1; i <= matrixConsecutiveFailureLimit; i++ {
			aborted, _ = tracker.Observe(matrixRunReport{
				TaskID:      fmt.Sprintf("task-%d", i),
				FinalStatus: "harness_crash",
				Error:       "harness_crash: context canceled",
			})
		}
		if !aborted {
			t.Fatal("tracker did not abort after 5 identical harness_crash reports")
		}
	})

	t.Run("graded pass resets", func(t *testing.T) {
		tracker := newMatrixConsecutiveFailureTracker(matrixConsecutiveFailureLimit)
		for i := 1; i <= matrixConsecutiveFailureLimit-1; i++ {
			if aborted, _ := tracker.Observe(matrixRunReport{
				TaskID:      fmt.Sprintf("fail-%d", i),
				FinalStatus: "graded_fail",
				Error:       "agent: provider error: stable",
			}); aborted {
				t.Fatalf("unexpected abort before reset at failure %d", i)
			}
		}
		if aborted, _ := tracker.Observe(matrixRunReport{TaskID: "pass", FinalStatus: "graded_pass"}); aborted {
			t.Fatal("graded_pass should not abort")
		}
		for i := 1; i <= matrixConsecutiveFailureLimit-1; i++ {
			if aborted, _ := tracker.Observe(matrixRunReport{
				TaskID:      fmt.Sprintf("after-reset-%d", i),
				FinalStatus: "graded_fail",
				Error:       "agent: provider error: stable",
			}); aborted {
				t.Fatalf("counter was not reset; aborted after %d post-pass failures", i)
			}
		}
	})

	t.Run("one byte different does not halt", func(t *testing.T) {
		tracker := newMatrixConsecutiveFailureTracker(matrixConsecutiveFailureLimit)
		for i := 1; i <= matrixConsecutiveFailureLimit-1; i++ {
			if aborted, _ := tracker.Observe(matrixRunReport{
				TaskID:      fmt.Sprintf("task-%d", i),
				FinalStatus: "graded_fail",
				Error:       "agent: provider error: stable",
			}); aborted {
				t.Fatalf("unexpected abort at failure %d", i)
			}
		}
		if aborted, _ := tracker.Observe(matrixRunReport{
			TaskID:      "task-5",
			FinalStatus: "graded_fail",
			Error:       "agent: provider error: stablE",
		}); aborted {
			t.Fatal("tracker aborted despite a one-byte error difference")
		}
	})
}

func TestConsecutiveFailureHaltMatrixAbort(t *testing.T) {
	repoRoot := benchRepoRoot(t)
	outDir := t.TempDir()
	subsetPath := writeSingleTaskSubset(t, outDir)

	code := cmdMatrix([]string{
		"--work-dir", repoRoot,
		"--subset", subsetPath,
		"--harnesses", "missing_adapter",
		"--profiles", "noop",
		"--reps", "10",
		"--out", outDir,
	})
	if code != matrixLaneAbortCode {
		t.Fatalf("cmdMatrix exit = %d, want %d", code, matrixLaneAbortCode)
	}
	if got := countReportFiles(t, filepath.Join(outDir, "cells")); got != matrixConsecutiveFailureLimit {
		t.Fatalf("cell report count = %d, want %d", got, matrixConsecutiveFailureLimit)
	}
	for rep := matrixConsecutiveFailureLimit + 1; rep <= 10; rep++ {
		reportPath := filepath.Join(outDir, "cells", "missing_adapter", "noop", fmt.Sprintf("rep-%03d", rep), "fix-git", matrixReportName)
		if _, err := os.Stat(reportPath); !os.IsNotExist(err) {
			t.Fatalf("rep %d report should not exist after lane abort, stat err=%v", rep, err)
		}
	}

	abortPath := filepath.Join(outDir, ".lane_aborted", "missing_adapter__noop", "aborted.json")
	abortReport := readMatrixRunReport(t, abortPath)
	if abortReport.FinalStatus != matrixInvalidLaneAbort {
		t.Fatalf("abort final_status = %q, want %q", abortReport.FinalStatus, matrixInvalidLaneAbort)
	}
	if abortReport.FailureFingerprint == "" || len(abortReport.FailureTaskIDs) != matrixConsecutiveFailureLimit {
		t.Fatalf("abort report missing fingerprint details: %+v", abortReport)
	}
	matrix := readMatrixOutput(t, filepath.Join(outDir, "matrix.json"))
	if got := matrix.InvalidByClass[matrixInvalidLaneAbort]; got != 1 {
		t.Fatalf("invalid_by_class[%s] = %d, want 1", matrixInvalidLaneAbort, got)
	}
}

func TestConsecutiveFailureHaltDisabled(t *testing.T) {
	repoRoot := benchRepoRoot(t)
	outDir := t.TempDir()
	subsetPath := writeSingleTaskSubset(t, outDir)

	code := cmdMatrix([]string{
		"--work-dir", repoRoot,
		"--subset", subsetPath,
		"--harnesses", "missing_adapter",
		"--profiles", "noop",
		"--reps", "10",
		"--out", outDir,
		"--no-consecutive-failure-halt",
	})
	if code != 0 {
		t.Fatalf("cmdMatrix exit = %d, want 0", code)
	}
	if got := countReportFiles(t, filepath.Join(outDir, "cells")); got != 10 {
		t.Fatalf("cell report count = %d, want 10", got)
	}
	if _, err := os.Stat(filepath.Join(outDir, ".lane_aborted")); !os.IsNotExist(err) {
		t.Fatalf("lane abort dir should not exist, stat err=%v", err)
	}
	matrix := readMatrixOutput(t, filepath.Join(outDir, "matrix.json"))
	if got := len(matrix.Runs); got != 10 {
		t.Fatalf("matrix runs = %d, want 10", got)
	}
	for _, run := range matrix.Runs {
		if run.FinalStatus != "harness_crash" {
			t.Fatalf("run final_status = %q, want harness_crash", run.FinalStatus)
		}
	}
}

func waitForLogContains(t *testing.T, path, needle string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		raw, _ := os.ReadFile(path)
		if strings.Contains(string(raw), needle) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	raw, _ := os.ReadFile(path)
	t.Fatalf("timed out waiting for %q in %s:\n%s", needle, path, string(raw))
}

func benchRepoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Clean(filepath.Join(wd, "..", ".."))
}

func readMatrixOutput(t *testing.T, path string) matrixOutput {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read matrix output: %v", err)
	}
	var out matrixOutput
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("parse matrix output: %v", err)
	}
	return out
}

func readMatrixRunReport(t *testing.T, path string) matrixRunReport {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read matrix run report: %v", err)
	}
	var report matrixRunReport
	if err := json.Unmarshal(raw, &report); err != nil {
		t.Fatalf("parse matrix run report: %v", err)
	}
	return report
}

func writeSingleTaskSubset(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "single-task-subset.json")
	raw := []byte(`{
  "version": "test",
  "dataset": "terminal-bench@test",
  "tasks": [
    {"id": "fix-git", "category": "software-engineering", "difficulty": "easy"}
  ]
}`)
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatalf("write subset fixture: %v", err)
	}
	return path
}

func countReportFiles(t *testing.T, root string) int {
	t.Helper()
	count := 0
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && d.Name() == matrixReportName {
			count++
		}
		return nil
	})
	if err != nil {
		t.Fatalf("count report files: %v", err)
	}
	return count
}
