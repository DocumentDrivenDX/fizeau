package main

import (
	"bytes"
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
