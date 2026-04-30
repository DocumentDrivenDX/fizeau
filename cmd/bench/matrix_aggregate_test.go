package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMatrixAggregateWritesSchemasWithNullsAndBudgetHalted(t *testing.T) {
	outDir := t.TempDir()
	writeFixtureRun(t, outDir, matrixRunReport{
		Harness:        "noop",
		ProfileID:      "noop",
		Rep:            1,
		TaskID:         "hello-world",
		ProcessOutcome: "completed",
		GradingOutcome: "graded",
		Reward:         intPtr(1),
		FinalStatus:    "graded_pass",
		InputTokens:    intPtr(1000),
		OutputTokens:   intPtr(2000),
		CostUSD:        0.003,
		PricingSource:  "profiles/noop.yaml#sha256=abc",
		StartedAt:      time.Now().UTC(),
		FinishedAt:     time.Now().UTC(),
	})
	writeFixtureRun(t, outDir, matrixRunReport{
		Harness:        "noop",
		ProfileID:      "noop",
		Rep:            2,
		TaskID:         "hello-world",
		ProcessOutcome: "completed",
		GradingOutcome: "ungraded",
		Reward:         nil,
		FinalStatus:    "ran",
		StartedAt:      time.Now().UTC(),
		FinishedAt:     time.Now().UTC(),
	})
	writeFixtureRun(t, outDir, matrixRunReport{
		Harness:        "dumb_script",
		ProfileID:      "noop",
		Rep:            1,
		TaskID:         "hello-world",
		ProcessOutcome: "budget_halted",
		GradingOutcome: "ungraded",
		Reward:         nil,
		FinalStatus:    "budget_halted",
		StartedAt:      time.Now().UTC(),
		FinishedAt:     time.Now().UTC(),
	})

	if code := cmdMatrixAggregate([]string{outDir}); code != 0 {
		t.Fatalf("cmdMatrixAggregate exit = %d, want 0", code)
	}

	matrix := readMatrixOutput(t, filepath.Join(outDir, "matrix.json"))
	if got, want := len(matrix.Runs), 3; got != want {
		t.Fatalf("matrix runs = %d, want %d", got, want)
	}
	var noopCell *matrixCell
	for i := range matrix.Cells {
		if matrix.Cells[i].Harness == "noop" && matrix.Cells[i].ProfileID == "noop" {
			noopCell = &matrix.Cells[i]
		}
	}
	if noopCell == nil {
		t.Fatal("noop cell missing")
	}
	if noopCell.NRuns != 2 || noopCell.NReported != 1 {
		t.Fatalf("noop cell counts = %d/%d, want n_runs=2 n_reported=1", noopCell.NRuns, noopCell.NReported)
	}
	if noopCell.MeanReward == nil || *noopCell.MeanReward != 1 {
		t.Fatalf("noop mean reward = %v, want 1", noopCell.MeanReward)
	}

	rawCosts, err := os.ReadFile(filepath.Join(outDir, "costs.json"))
	if err != nil {
		t.Fatal(err)
	}
	var costs matrixCostsOutput
	if err := json.Unmarshal(rawCosts, &costs); err != nil {
		t.Fatal(err)
	}
	if costs.MatrixTotalUSD != 0.003 {
		t.Fatalf("matrix_total_usd = %f, want 0.003", costs.MatrixTotalUSD)
	}

	rawMD, err := os.ReadFile(filepath.Join(outDir, "matrix.md"))
	if err != nil {
		t.Fatal(err)
	}
	md := string(rawMD)
	for _, want := range []string{
		"## Reward (mean +/- SD across N reps)",
		"1.00 +/- 0.00 (n=1/2)",
		"## Per-task pass count",
		"## Costs",
		"## Non-graded runs",
		"budget_halted",
	} {
		if !strings.Contains(md, want) {
			t.Fatalf("matrix.md missing %q:\n%s", want, md)
		}
	}
}

func TestMatrixAggregateFromRealCalibrationRun(t *testing.T) {
	repoRoot := benchRepoRoot(t)
	outDir := t.TempDir()
	if code := cmdMatrix([]string{
		"--work-dir", repoRoot,
		"--harnesses", "noop,dumb_script",
		"--profiles", "noop",
		"--reps", "1",
		"--out", outDir,
	}); code != 0 {
		t.Fatalf("cmdMatrix exit = %d, want 0", code)
	}
	if code := cmdMatrixAggregate([]string{outDir}); code != 0 {
		t.Fatalf("cmdMatrixAggregate exit = %d, want 0", code)
	}
	for _, name := range []string{"matrix.json", "matrix.md", "costs.json"} {
		if info, err := os.Stat(filepath.Join(outDir, name)); err != nil {
			t.Fatalf("%s missing: %v", name, err)
		} else if info.Size() == 0 {
			t.Fatalf("%s is empty", name)
		}
	}
}

func writeFixtureRun(t *testing.T, outDir string, run matrixRunReport) {
	t.Helper()
	dir := matrixTupleDir(outDir, run.Harness, run.ProfileID, run.Rep, run.TaskID)
	run.OutputDir = dir
	if err := writeJSONAtomic(filepath.Join(dir, matrixReportName), run); err != nil {
		t.Fatal(err)
	}
}

func intPtr(v int) *int {
	return &v
}
