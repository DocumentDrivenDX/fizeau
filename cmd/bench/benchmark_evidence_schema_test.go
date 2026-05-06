package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v5"
)

func TestBenchmarkEvidenceFixturesValidate(t *testing.T) {
	schema := compileBenchmarkEvidenceSchema(t)

	validate := func(name string) map[string]any {
		t.Helper()
		doc := loadBenchmarkEvidenceFixture(t, name)
		if err := schema.Validate(doc); err != nil {
			t.Fatalf("%s failed schema validation: %v", name, err)
		}
		return doc
	}

	frontier := validate("managed-frontier-opus.json")
	assertString(t, frontier, "subject.model_raw", "Opus 4.7")
	assertString(t, frontier, "subject.harness", "claude-code")
	assertString(t, frontier, "subject.provider", "anthropic")
	assertString(t, frontier, "benchmark.subset_id", "tb2-wide")
	assertString(t, frontier, "benchmark.subset_version", "v2")
	assertString(t, frontier, "provenance.harness_wrapper_name", "claude-code")
	assertString(t, frontier, "provenance.harness_wrapper_version", "1.0.0")
	assertString(t, frontier, "provenance.provider_capture_at", "2026-05-06T20:00:00Z")
	assertString(t, frontier, "provenance.model_snapshot", "opus-4.7-20260505")
	assertString(t, frontier, "coverage.formula_version", "fhi/v1")

	local := validate("local-omlx-qwen.json")
	assertString(t, local, "subject.model_raw", "Qwen3.6-27B-MLX-8bit")
	assertString(t, local, "subject.provider", "omlx")
	assertString(t, local, "subject.endpoint", "http://vidar:1235/v1")
	assertString(t, local, "runtime.deployment_class", "local")
	assertString(t, local, "runtime.quantization", "8-bit")
	assertString(t, local, "runtime.local_runtime_name", "omlx")
	assertString(t, local, "runtime.local_runtime_version", "0.8.10")
	assertString(t, local, "runtime.hardware_class", "Mac Studio")
	assertString(t, local, "provenance.fizeau_version", "0.1.0")
	assertString(t, local, "provenance.fizeau_git_commit", "fa48595c7262b1522ab41897c3f60e128014f598")
	assertString(t, local, "provenance.provider_version", "0.8.10")
	assertString(t, local, "provenance.session_log_path", "benchmark-results/evidence/local-omlx-qwen/session.log.jsonl")
	assertString(t, local, "provenance.session_log_sha256", "3333333333333333333333333333333333333333333333333333333333333333")
	assertString(t, local, "benchmark.subset_id", "tb2-wide")
	assertString(t, local, "benchmark.subset_version", "v2")

	invalid := validate("invalid-run.json")
	assertString(t, invalid, "invalid_class", "invalid_setup")
	assertBool(t, invalid, "denominator.included", false)
	assertString(t, invalid, "denominator.policy", "exclude_invalid_runs")
	assertString(t, invalid, "denominator.reason", "setup failure before first benchmark task")
	assertString(t, invalid, "scope.denominator_rule", "exclude_invalid_runs")
}

func compileBenchmarkEvidenceSchema(t *testing.T) *jsonschema.Schema {
	t.Helper()

	repoRoot := benchRepoRoot(t)
	rawSchema, err := os.ReadFile(filepath.Join(repoRoot, "scripts", "benchmark", "benchmark-evidence.schema.json"))
	if err != nil {
		t.Fatalf("read benchmark evidence schema: %v", err)
	}

	compiler := jsonschema.NewCompiler()
	compiler.AssertFormat = true
	if err := compiler.AddResource("benchmark-evidence.schema.json", bytes.NewReader(rawSchema)); err != nil {
		t.Fatalf("add benchmark evidence schema: %v", err)
	}
	schema, err := compiler.Compile("benchmark-evidence.schema.json")
	if err != nil {
		t.Fatalf("compile benchmark evidence schema: %v", err)
	}
	return schema
}

func loadBenchmarkEvidenceFixture(t *testing.T, name string) map[string]any {
	t.Helper()

	repoRoot := benchRepoRoot(t)
	raw, err := os.ReadFile(filepath.Join(repoRoot, "cmd", "bench", "testdata", "benchmark-evidence", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}

	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var doc map[string]any
	if err := dec.Decode(&doc); err != nil {
		t.Fatalf("parse fixture %s: %v", name, err)
	}
	return doc
}

func assertString(t *testing.T, doc map[string]any, path, want string) {
	t.Helper()
	got, ok := lookupPath(doc, path)
	if !ok {
		t.Fatalf("missing %s", path)
	}
	str, ok := got.(string)
	if !ok {
		t.Fatalf("%s = %T, want string", path, got)
	}
	if str != want {
		t.Fatalf("%s = %q, want %q", path, str, want)
	}
}

func assertBool(t *testing.T, doc map[string]any, path string, want bool) {
	t.Helper()
	got, ok := lookupPath(doc, path)
	if !ok {
		t.Fatalf("missing %s", path)
	}
	b, ok := got.(bool)
	if !ok {
		t.Fatalf("%s = %T, want bool", path, got)
	}
	if b != want {
		t.Fatalf("%s = %t, want %t", path, b, want)
	}
}

func lookupPath(doc map[string]any, path string) (any, bool) {
	var cur any = doc
	for _, segment := range bytes.Split([]byte(path), []byte(".")) {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		next, ok := m[string(segment)]
		if !ok {
			return nil, false
		}
		cur = next
	}
	return cur, true
}
