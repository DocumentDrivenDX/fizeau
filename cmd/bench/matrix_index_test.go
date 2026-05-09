package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMatrixIndexAssignsRunIndexesAndCopiesCanonicalCells(t *testing.T) {
	root := t.TempDir()
	writeMatrixIndexFixtureReport(t, root, "run-a", "fix-git", 1, 1)
	writeMatrixIndexFixtureReport(t, root, "run-b", "fix-git", 1, 0)

	canonical := filepath.Join(root, "canonical")
	rows, err := collectMatrixIndexRows(root, canonical, true, 1, "terminal-bench-2-1", map[string]profileProviderInfo{
		"fiz-openai-gpt-5-5": {Provider: "openai", Model: "gpt-5.5"},
	})
	if err != nil {
		t.Fatalf("collectMatrixIndexRows: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(rows))
	}
	if rows[0].RunIndex != 1 || rows[1].RunIndex != 2 {
		t.Fatalf("run indexes = %d,%d; want 1,2", rows[0].RunIndex, rows[1].RunIndex)
	}
	for _, row := range rows {
		if row.Provider != "openai" || row.Model != "gpt-5.5" {
			t.Fatalf("provider/model = %s/%s", row.Provider, row.Model)
		}
		if row.CanonicalPath == "" {
			t.Fatal("canonical path was not populated")
		}
		if _, err := os.Stat(row.CanonicalPath); err != nil {
			t.Fatalf("canonical report missing %s: %v", row.CanonicalPath, err)
		}
	}
}

func TestMatrixIndexRecordsWrappedHarness(t *testing.T) {
	report := matrixRunReport{
		Harness:   "fiz",
		ProfileID: "fiz-harness-codex-gpt-5-4-mini",
		TaskID:    "hello-world",
	}
	row := matrixIndexRowFromReport("report.json", report, 1, "terminal-bench-2-1", map[string]profileProviderInfo{})
	if row.Harness != "codex" {
		t.Fatalf("harness = %q, want codex", row.Harness)
	}
}

func TestMatrixIndexRunIndexesUseCanonicalIdentity(t *testing.T) {
	rows := []matrixIndexRow{
		{Dataset: "terminal-bench-2-1", TaskID: "hello-world", Provider: "openrouter", Model: "anthropic/claude-sonnet-4.6", Harness: "fiz", ProfileID: "claude-sonnet-4-6"},
		{Dataset: "terminal-bench-2-1", TaskID: "hello-world", Provider: "openrouter", Model: "anthropic/claude-sonnet-4.6", Harness: "fiz", ProfileID: "fiz-openrouter-claude-sonnet-4-6"},
	}
	assignMatrixIndexRunIndexes(rows)
	if rows[0].RunIndex != 1 || rows[1].RunIndex != 2 {
		t.Fatalf("run indexes = %d,%d; want 1,2", rows[0].RunIndex, rows[1].RunIndex)
	}
}

func TestSummarizeMatrixIndexRows(t *testing.T) {
	pass := 1
	fail := 0
	rows := []matrixIndexRow{
		{Dataset: "terminal-bench-2-1", Provider: "openai", Model: "gpt-5.5", Harness: "fiz", ProfileID: "fiz-openai-gpt-5-5", Reward: &pass, FinalStatus: "graded_pass", CostUSD: 0.2, InputTokens: 10, OutputTokens: 2},
		{Dataset: "terminal-bench-2-1", Provider: "openai", Model: "gpt-5.5", Harness: "fiz", ProfileID: "fiz-openai-gpt-5-5", Reward: &fail, FinalStatus: "graded_fail", CostUSD: 0.3, InputTokens: 20, OutputTokens: 3},
		{Dataset: "terminal-bench-2-1", Provider: "openai", Model: "gpt-5.5", Harness: "fiz", ProfileID: "fiz-openai-gpt-5-5", FinalStatus: "harness_crash"},
	}
	got := summarizeMatrixIndexRows(rows)
	if len(got) != 1 {
		t.Fatalf("summary rows = %d, want 1", len(got))
	}
	row := got[0]
	if row.Reports != 3 || row.Pass != 1 || row.GradedFail != 1 || row.Crash != 1 {
		t.Fatalf("summary = %+v", row)
	}
	if row.InputTokens != 30 || row.OutputTokens != 5 || row.CostUSD != 0.5 {
		t.Fatalf("token/cost summary = %+v", row)
	}
}

func writeMatrixIndexFixtureReport(t *testing.T, root, run, task string, rep int, reward int) {
	t.Helper()
	dir := filepath.Join(root, run, "cells", "fiz", "fiz-openai-gpt-5-5", "rep-001", task)
	if err := os.MkdirAll(filepath.Join(dir, "logs", "agent"), 0o750); err != nil {
		t.Fatalf("mkdir fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "logs", "agent", "session.log.jsonl"), []byte("{}\n"), 0o600); err != nil {
		t.Fatalf("write log fixture: %v", err)
	}
	report := matrixRunReport{
		Harness:        "fiz",
		ProfileID:      "fiz-openai-gpt-5-5",
		ProfilePath:    "scripts/benchmark/profiles/fiz-openai-gpt-5-5.yaml",
		Rep:            rep,
		TaskID:         task,
		ProcessOutcome: "completed",
		GradingOutcome: "graded",
		Reward:         &reward,
		FinalStatus:    "graded_fail",
		StartedAt:      time.Date(2026, 5, 8, 1, 0, rep, 0, time.UTC),
		FinishedAt:     time.Date(2026, 5, 8, 1, 1, rep, 0, time.UTC),
		InputTokens:    matrixIndexIntPtr(10),
		OutputTokens:   matrixIndexIntPtr(2),
		WallSeconds:    matrixIndexFloatPtr(3),
		CostUSD:        0.2,
		PricingSource:  "test",
		AdapterModule:  "scripts.benchmark.harness_adapters.fiz",
		HarborAgent:    "scripts/benchmark/harness_adapters/fiz.py:Agent",
	}
	if reward == 1 {
		report.FinalStatus = "graded_pass"
	}
	raw, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, matrixReportName), raw, 0o600); err != nil {
		t.Fatalf("write fixture report: %v", err)
	}
}

func matrixIndexIntPtr(v int) *int { return &v }

func matrixIndexFloatPtr(v float64) *float64 { return &v }
