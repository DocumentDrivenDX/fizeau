package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestClassifyMatrixInvalidFromFixtures(t *testing.T) {
	cases := []struct {
		name string
		want string
	}{
		{name: "claude-quota.json", want: matrixInvalidQuota},
		{name: "codex-auth.json", want: matrixInvalidAuth},
		{name: "pi-missing-binary.json", want: matrixInvalidSetup},
		{name: "opencode-account.json", want: matrixInvalidAuth},
		{name: "opencode-wrapper-startup.json", want: matrixInvalidSetup},
		{name: "setup-native-arch.json", want: matrixInvalidSetup},
		{name: "provider-transport.json", want: matrixInvalidProvider},
		{name: "verifier-fail-after-attempt.json", want: ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			report := loadMatrixInvalidFixture(t, tc.name)
			if got := classifyMatrixInvalid(report); got != tc.want {
				t.Fatalf("classifyMatrixInvalid(%s) = %q, want %q", tc.name, got, tc.want)
			}
		})
	}

	t.Run("raw install_fail_permanent final status", func(t *testing.T) {
		report := matrixRunReport{FinalStatus: "install_fail_permanent"}
		if got := classifyMatrixInvalid(report); got != matrixInvalidSetup {
			t.Fatalf("classifyMatrixInvalid(raw install_fail_permanent) = %q, want %q", got, matrixInvalidSetup)
		}
	})
}

func TestMatrixAggregateIncludesInvalidCountsAndSkipsInvalidDenominators(t *testing.T) {
	outDir := t.TempDir()

	valid := matrixRunReport{
		Harness:        "claude",
		ProfileID:      "gpt-5-4-mini",
		Rep:            1,
		TaskID:         "fix-git",
		ProcessOutcome: "completed",
		GradingOutcome: "graded",
		Reward:         intPtr(1),
		FinalStatus:    "graded_pass",
		Turns:          intPtr(5),
		ToolCalls:      intPtr(7),
		InputTokens:    intPtr(100),
		OutputTokens:   intPtr(50),
		StartedAt:      time.Now().UTC(),
		FinishedAt:     time.Now().UTC(),
	}
	writeFixtureRun(t, outDir, valid)

	invalidQuota := loadMatrixInvalidFixture(t, "claude-quota.json")
	invalidQuota.Rep = 2
	invalidQuota.TaskID = "fix-git"
	writeFixtureRun(t, outDir, invalidQuota)

	for _, name := range []string{
		"codex-auth.json",
		"pi-missing-binary.json",
		"opencode-account.json",
		"opencode-wrapper-startup.json",
		"setup-native-arch.json",
		"verifier-fail-after-attempt.json",
	} {
		writeFixtureRun(t, outDir, loadMatrixInvalidFixture(t, name))
	}

	providerTransport := loadMatrixInvalidFixture(t, "provider-transport.json")
	providerTransport.Harness = "provider-transport"
	providerTransport.ProfileID = "provider-sim"
	providerTransport.Rep = 1
	providerTransport.TaskID = "git-leak-recovery"
	writeFixtureRun(t, outDir, providerTransport)

	if code := cmdMatrixAggregate([]string{outDir}); code != 0 {
		t.Fatalf("cmdMatrixAggregate exit = %d, want 0", code)
	}

	matrix := readMatrixOutput(t, filepath.Join(outDir, "matrix.json"))
	if got, want := matrix.InvalidRuns, 7; got != want {
		t.Fatalf("invalid_runs = %d, want %d", got, want)
	}
	wantInvalidByClass := map[string]int{
		matrixInvalidQuota:    1,
		matrixInvalidAuth:     2,
		matrixInvalidSetup:    3,
		matrixInvalidProvider: 1,
	}
	if len(matrix.InvalidByClass) != len(wantInvalidByClass) {
		t.Fatalf("invalid_by_class len = %d, want %d", len(matrix.InvalidByClass), len(wantInvalidByClass))
	}
	for class, want := range wantInvalidByClass {
		if got := matrix.InvalidByClass[class]; got != want {
			t.Fatalf("invalid_by_class[%s] = %d, want %d", class, got, want)
		}
	}

	var claudeCell *matrixCell
	for i := range matrix.Cells {
		if matrix.Cells[i].Harness == "claude" && matrix.Cells[i].ProfileID == "gpt-5-4-mini" {
			claudeCell = &matrix.Cells[i]
			break
		}
	}
	if claudeCell == nil {
		t.Fatal("claude cell missing")
	}
	if claudeCell.NRuns != 2 || claudeCell.NValid != 1 || claudeCell.NInvalid != 1 || claudeCell.NReported != 1 {
		t.Fatalf("claude counts = %+v, want NRuns=2 NValid=1 NInvalid=1 NReported=1", *claudeCell)
	}
	if got := claudeCell.InvalidCounts[matrixInvalidQuota]; got != 1 {
		t.Fatalf("claude invalid counts = %#v, want invalid_quota=1", claudeCell.InvalidCounts)
	}
	if claudeCell.MeanReward == nil || *claudeCell.MeanReward != 1 {
		t.Fatalf("claude mean reward = %v, want 1", claudeCell.MeanReward)
	}

	rawMD, err := os.ReadFile(filepath.Join(outDir, "matrix.md"))
	if err != nil {
		t.Fatal(err)
	}
	md := string(rawMD)
	for _, want := range []string{
		"## Invalid runs",
		"invalid_quota",
		"invalid_auth",
		"invalid_setup",
		"invalid_provider",
		"1.00 +/- 0.00 (n=1/1)",
		"1/1 |",
	} {
		if !strings.Contains(md, want) {
			t.Fatalf("matrix.md missing %q:\n%s", want, md)
		}
	}
}

func loadMatrixInvalidFixture(t *testing.T, name string) matrixRunReport {
	t.Helper()
	path := filepath.Join("testdata", "matrix-invalid", name)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	var report matrixRunReport
	if err := json.Unmarshal(raw, &report); err != nil {
		t.Fatalf("parse fixture %s: %v", name, err)
	}
	return report
}
