package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestTerminalBenchMatrixImport(t *testing.T) {
	repoRoot := benchRepoRoot(t)
	fixtureDir := filepath.Join(repoRoot, "cmd", "bench", "testdata", "terminalbench-matrix")
	outPath := filepath.Join(t.TempDir(), "terminalbench-evidence.jsonl")

	if code, out := runBenchCLI(t, repoRoot, "evidence", "import-terminalbench", "--work-dir", repoRoot, "--matrix", fixtureDir, "--out", outPath); code != 0 {
		t.Fatalf("import-terminalbench exit=%d output=%s", code, out)
	}

	raw, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	lines := bytes.Split(bytes.TrimSpace(raw), []byte("\n"))
	if got, want := len(lines), 5; got != want {
		t.Fatalf("record count = %d, want %d\n%s", got, want, string(raw))
	}

	schema := compileBenchmarkEvidenceSchema(t)
	seenIDs := map[string]bool{}
	records := map[string]map[string]any{}
	for _, line := range lines {
		doc := decodeJSONMap(t, line)
		if err := schema.Validate(doc); err != nil {
			t.Fatalf("schema validation failed: %v\n%s", err, string(line))
		}
		recordID := mustStringField(t, doc, "record_id")
		if seenIDs[recordID] {
			t.Fatalf("duplicate record_id %s", recordID)
		}
		seenIDs[recordID] = true

		status := mustStringField(t, doc, "final_status")
		records[status] = doc
	}

	valid := records["graded_pass"]
	if valid == nil {
		t.Fatal("missing graded_pass record")
	}
	assertString(t, valid, "benchmark.name", "terminal-bench")
	assertString(t, valid, "benchmark.version", "2026.05.06")
	assertString(t, valid, "benchmark.dataset", "terminal-bench@2.0")
	assertString(t, valid, "benchmark.subset_id", "tb2-canary")
	assertString(t, valid, "subject.harness", "fiz")
	assertString(t, valid, "subject.provider", "omlx")
	assertString(t, valid, "provenance.fizeau_git_commit", "fa48595c7262b1522ab41897c3f60e128014f598")
	assertString(t, valid, "provenance.harness_wrapper_name", "fiz-native")
	assertString(t, valid, "provenance.provider_version", "0.8.10")
	assertString(t, valid, "provenance.session_log_path", "cells/fiz/vidar-qwen/rep-001/fix-git/logs/agent/session.log.jsonl")
	assertString(t, valid, "provenance.trajectory_path", "cells/fiz/vidar-qwen/rep-001/fix-git/logs/agent/trajectory.json")
	assertString(t, valid, "runtime.deployment_class", "local")
	assertString(t, valid, "runtime.local_runtime_name", "omlx")
	assertString(t, valid, "runtime.local_runtime_version", "0.8.10")
	assertBool(t, valid, "denominator.included", true)
	assertString(t, valid, "scope.denominator_rule", "count_valid_tasks")
	assertString(t, valid, "source.artifact_path", "matrix.json")

	invalidQuota := records["invalid_quota"]
	if invalidQuota == nil {
		t.Fatal("missing invalid_quota record")
	}
	assertString(t, invalidQuota, "subject.harness", "claude")
	assertString(t, invalidQuota, "provenance.harness_wrapper_name", "claude-code")
	assertBool(t, invalidQuota, "denominator.included", false)
	assertString(t, invalidQuota, "denominator.policy", "exclude_invalid_runs")
	assertContainsString(t, invalidQuota, "denominator.excluded_classes", "invalid_quota")
	assertString(t, invalidQuota, "scope.denominator_rule", "exclude_invalid_runs")
	assertString(t, invalidQuota, "final_status", "invalid_quota")
	assertString(t, invalidQuota, "invalid_class", "invalid_quota")
	assertHex64(t, invalidQuota, "provenance.session_log_sha256")
	assertHex64(t, invalidQuota, "provenance.trajectory_sha256")

	invalidProvider := records["invalid_provider"]
	if invalidProvider == nil {
		t.Fatal("missing invalid_provider record")
	}
	assertString(t, invalidProvider, "subject.harness", "opencode")
	assertString(t, invalidProvider, "invalid_class", "invalid_provider")
	assertContainsString(t, invalidProvider, "denominator.excluded_classes", "invalid_provider")
}

// TestTerminalBenchMatrixImportEmbeddedProfile exercises the ADR-016 path
// where the cell record carries its own resolved profile snapshot under
// `runs[].profile`. The importer must read versioning.snapshot and provider
// info from that embedded block instead of loading the on-disk YAML — this
// fixture deliberately omits the profile YAML file so any fallback to
// `profile.Load(profilePath)` fails the test.
func TestTerminalBenchMatrixImportEmbeddedProfile(t *testing.T) {
	repoRoot := benchRepoRoot(t)
	fixtureDir := t.TempDir()

	matrix := map[string]any{
		"generated_at": "2026-05-16T10:30:00Z",
		"subset_path":  "scripts/beadbench/external/termbench-subset-canary.json",
		"profiles":     []string{"vidar-qwen-embedded"},
		"harnesses":    []string{"fiz"},
		"reps":         1,
		"runs": []map[string]any{
			{
				"harness": "fiz",
				"profile": map[string]any{
					"id": "vidar-qwen-embedded",
					"provider": map[string]any{
						"type":        "openai-compat",
						"model":       "Qwen3.6-27B-MLX-8bit",
						"base_url":    "http://vidar:1235/v1",
						"api_key_env": "OMLX_API_KEY",
					},
					"limits": map[string]any{
						"max_output_tokens": 4096,
						"context_tokens":    8192,
					},
					"sampling": map[string]any{
						"temperature": 0.2,
					},
					"versioning": map[string]any{
						"resolved_at": "2026-05-16T10:00:00Z",
						"snapshot":    "qwen3.6-27b-mlx-8bit@embedded-only",
					},
				},
				"profile_id":       "vidar-qwen-embedded",
				"profile_path":     "profiles/does-not-exist.yaml",
				"profile_snapshot": "ignored-legacy-string",
				"rep":              1,
				"task_id":          "fix-git",
				"output_dir":       "cells/fiz/vidar-qwen-embedded/rep-001/fix-git",
				"process_outcome":  "completed",
				"grading_outcome":  "graded",
				"reward":           1,
				"final_status":     "graded_pass",
				"started_at":       "2026-05-16T10:30:00Z",
				"finished_at":      "2026-05-16T10:30:12Z",
			},
		},
		"cells": []any{},
	}
	raw, err := json.Marshal(matrix)
	if err != nil {
		t.Fatalf("marshal matrix: %v", err)
	}
	if err := os.WriteFile(filepath.Join(fixtureDir, "matrix.json"), raw, 0o600); err != nil {
		t.Fatalf("write matrix.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(fixtureDir, "matrix.metadata.json"), []byte(`{"benchmark":{"version":"2026.05.16","dataset":"terminal-bench@2.0","subset_id":"tb2-canary","subset_version":"v1"}}`), 0o600); err != nil {
		t.Fatalf("write matrix.metadata.json: %v", err)
	}
	subsetDir := filepath.Join(fixtureDir, "scripts", "beadbench", "external")
	if err := os.MkdirAll(subsetDir, 0o750); err != nil {
		t.Fatalf("mkdir subsetDir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(subsetDir, "termbench-subset-canary.json"), []byte(`{"dataset":"terminal-bench@2.0","dataset_commit":"deadbeef","version":"v1","tasks":[{"id":"fix-git"}]}`), 0o600); err != nil {
		t.Fatalf("write subset: %v", err)
	}

	outPath := filepath.Join(t.TempDir(), "evidence.jsonl")
	if code, out := runBenchCLI(t, repoRoot, "evidence", "import-terminalbench", "--work-dir", repoRoot, "--matrix", fixtureDir, "--out", outPath); code != 0 {
		t.Fatalf("import-terminalbench exit=%d output=%s", code, out)
	}

	body, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	lines := bytes.Split(bytes.TrimSpace(body), []byte("\n"))
	if got, want := len(lines), 1; got != want {
		t.Fatalf("record count = %d, want %d\n%s", got, want, string(body))
	}
	doc := decodeJSONMap(t, lines[0])
	assertString(t, doc, "provenance.model_snapshot", "qwen3.6-27b-mlx-8bit@embedded-only")
	assertString(t, doc, "subject.model_raw", "Qwen3.6-27B-MLX-8bit")
	assertString(t, doc, "provenance.provider_endpoint", "http://vidar:1235/v1")
}

func assertContainsString(t *testing.T, doc map[string]any, path, want string) {
	t.Helper()

	got, ok := lookupPath(doc, path)
	if !ok {
		t.Fatalf("missing %s", path)
	}
	values, ok := got.([]any)
	if !ok {
		t.Fatalf("%s = %T, want []any", path, got)
	}
	for _, v := range values {
		if s, ok := v.(string); ok && s == want {
			return
		}
	}
	t.Fatalf("%s missing %q in %#v", path, want, values)
}

func assertHex64(t *testing.T, doc map[string]any, path string) {
	t.Helper()

	got, ok := lookupPath(doc, path)
	if !ok {
		t.Fatalf("missing %s", path)
	}
	str, ok := got.(string)
	if !ok {
		t.Fatalf("%s = %T, want string", path, got)
	}
	if len(str) != 64 {
		t.Fatalf("%s length = %d, want 64", path, len(str))
	}
}
