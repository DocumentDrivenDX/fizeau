package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/fizeau/internal/benchmark/profile"
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
