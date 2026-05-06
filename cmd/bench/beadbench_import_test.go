package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestBeadBenchReportImport(t *testing.T) {
	repoRoot := benchRepoRoot(t)
	fixture := filepath.Join(repoRoot, "cmd", "bench", "testdata", "beadbench-report", "report.json")
	outPath := filepath.Join(t.TempDir(), "beadbench-evidence.jsonl")

	if code, out := runBenchCLI(t, repoRoot, "evidence", "import-beadbench", "--work-dir", repoRoot, "--report", fixture, "--out", outPath); code != 0 {
		t.Fatalf("import-beadbench exit=%d output=%s", code, out)
	}

	raw, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	lines := bytes.Split(bytes.TrimSpace(raw), []byte("\n"))
	if got, want := len(lines), 4; got != want {
		t.Fatalf("record count = %d, want %d\n%s", got, want, string(raw))
	}

	schema := compileBenchmarkEvidenceSchema(t)
	records := map[string]map[string]any{}
	for _, line := range lines {
		doc := decodeJSONMap(t, line)
		if err := schema.Validate(doc); err != nil {
			t.Fatalf("schema validation failed: %v\n%s", err, string(line))
		}
		recordID := mustStringField(t, doc, "record_id")
		if _, exists := records[recordID]; exists {
			t.Fatalf("duplicate record_id %s", recordID)
		}
		records[recordID] = doc
	}

	success := findRecordByField(t, records, "final_status", "success")
	assertString(t, success, "benchmark.name", "beadbench")
	assertString(t, success, "benchmark.version", "1")
	assertString(t, success, "source.type", "imported_report")
	assertString(t, success, "source.artifact_path", fixture)
	assertString(t, success, "subject.harness", "agent")
	assertString(t, success, "subject.provider", "openrouter")
	assertString(t, success, "subject.reasoning", "medium")
	assertString(t, success, "components.bead_id", "agent-37aeb88e")
	assertString(t, success, "components.task_capability", "beadbench-harness-instrumentation")
	assertBool(t, success, "denominator.included", true)
	assertString(t, success, "scope.denominator_rule", "count_valid_runs_only")
	assertString(t, success, "components.review_outcome", "pass")
	assertString(t, success, "runtime.outcome", "success")
	assertString(t, success, "provenance.session_log_path", "benchmark-results/beadbench/run-20260506T210000Z-1234/agent-beadbench-preflight__agent-openrouter-gpt54__r1/session.log.jsonl")
	assertString(t, success, "provenance.trajectory_path", "benchmark-results/beadbench/run-20260506T210000Z-1234/agent-beadbench-preflight__agent-openrouter-gpt54__r1/trajectory.json")
	assertFloat(t, success, "runtime.tool_calls", 11)
	assertFloat(t, success, "cost.usd", 0.0425)

	invalidAuth := findRecordByField(t, records, "final_status", "invalid_auth")
	assertString(t, invalidAuth, "components.raw_status", "auth_fail")
	assertString(t, invalidAuth, "components.task_capability", "sampling-resolver-cli-wiring")
	assertBool(t, invalidAuth, "denominator.included", false)
	assertString(t, invalidAuth, "denominator.policy", "exclude_invalid_runs")
	assertString(t, invalidAuth, "denominator.reason", "authentication failure before benchmark task")
	assertContainsString(t, invalidAuth, "denominator.excluded_classes", "invalid_auth")

	invalidProvider := findRecordByField(t, records, "final_status", "invalid_provider")
	assertString(t, invalidProvider, "components.raw_status", "harness_crash")
	assertString(t, invalidProvider, "components.task_failure_mode", "openai-flavor-thinking-leak")
	assertContainsString(t, invalidProvider, "denominator.excluded_classes", "invalid_provider")

	invalidSetup := findRecordByField(t, records, "final_status", "invalid_setup")
	assertString(t, invalidSetup, "components.raw_status", "install_fail_permanent")
	assertBool(t, invalidSetup, "denominator.included", false)
	assertString(t, invalidSetup, "denominator.reason", "setup failure before first benchmark task")
}

func findRecordByField(t *testing.T, records map[string]map[string]any, path, want string) map[string]any {
	t.Helper()

	for _, doc := range records {
		got, ok := lookupPath(doc, path)
		if !ok {
			continue
		}
		str, ok := got.(string)
		if ok && str == want {
			return doc
		}
	}
	t.Fatalf("no record with %s=%q", path, want)
	return nil
}

func assertFloat(t *testing.T, doc map[string]any, path string, want float64) {
	t.Helper()

	got, ok := lookupPath(doc, path)
	if !ok {
		t.Fatalf("missing %s", path)
	}
	switch v := got.(type) {
	case float64:
		if v != want {
			t.Fatalf("%s = %v, want %v", path, v, want)
		}
	case int:
		if float64(v) != want {
			t.Fatalf("%s = %v, want %v", path, v, want)
		}
	default:
		t.Fatalf("%s = %T, want number", path, got)
	}
}
