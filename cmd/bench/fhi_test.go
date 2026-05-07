package main

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestFHIDeltaClaimFromLedgerFixture(t *testing.T) {
	repoRoot := benchRepoRoot(t)
	formula := mustLoadFHIFormula(t, repoRoot)
	records := mustLoadFHILedger(t, repoRoot, filepath.Join(repoRoot, "cmd", "bench", "testdata", "fhi", "terminalbench-delta.jsonl"))

	claim, err := buildFHIDeltaClaim(formula, records, "terminalbench-delta-left-opus-claude-code", "terminalbench-delta-right-opus-claude-code")
	if err != nil {
		t.Fatalf("buildFHIDeltaClaim: %v", err)
	}
	assertStringPath(t, claim, "status", "ok")
	assertStringPath(t, claim, "formula.version", "fhi/v1")
	assertNumberPath(t, claim, "delta", -0.7)
	assertStringPath(t, claim, "benchmark.name", "terminal-bench")
	assertStringPath(t, claim, "left.subject.model_raw", "Opus 4.7")
	assertStringPath(t, claim, "left.subject.provider", "anthropic")
	assertStringPath(t, claim, "left.subject.harness", "fiz-native")
	assertStringPath(t, claim, "left.benchmark.subset_id", "tb2-wide")
	assertStringPath(t, claim, "left.benchmark.scorer", "verifier")
	assertNumberPath(t, claim, "left.scope.rep", 1)
	assertStringPath(t, claim, "left.source.artifact_sha256", "1111111111111111111111111111111111111111111111111111111111111111")
	assertStringPath(t, claim, "right.subject.harness", "claude-code")
	assertStringPath(t, claim, "right.source.artifact_sha256", "4444444444444444444444444444444444444444444444444444444444444444")
	assertStringPath(t, claim, "ledger.denominator_handling.policy", "exclude_invalid_runs")
	assertNumberPath(t, claim, "ledger.invalid_run_count", 1)
	assertNumberPath(t, claim, "ledger.invalid_run_classes.invalid_setup", 1)
	text := mustStringPath(t, claim, "claim_text")
	if !strings.Contains(text, "0.7 points behind") || !strings.Contains(text, "claude-code") {
		t.Fatalf("claim_text = %q, want benchmark delta language", text)
	}
}

func TestFHIRankClaimFromLedgerFixture(t *testing.T) {
	repoRoot := benchRepoRoot(t)
	formula := mustLoadFHIFormula(t, repoRoot)
	records := mustLoadFHILedger(t, repoRoot, filepath.Join(repoRoot, "cmd", "bench", "testdata", "fhi", "fhi-rank.jsonl"))

	claim, err := buildFHIRankClaim(formula, records)
	if err != nil {
		t.Fatalf("buildFHIRankClaim: %v", err)
	}
	assertStringPath(t, claim, "status", "ok")
	assertStringPath(t, claim, "formula.version", "fhi/v1")
	assertStringPath(t, claim, "formula.evidence_window", "2026-Q2")
	assertNumberPath(t, claim, "delta", 6.0)
	assertNumberPath(t, claim, "rankings.0.fhi", 56.0)
	assertNumberPath(t, claim, "rankings.1.fhi", 50.0)
	assertStringPath(t, claim, "rankings.0.subject.subject.model_raw", "Opus 4.7")
	assertStringPath(t, claim, "rankings.0.subject.subject.provider", "anthropic")
	assertStringPath(t, claim, "rankings.0.subject.provenance.provider_endpoint", "https://api.anthropic.com/v1")
	assertStringPath(t, claim, "rankings.1.subject.subject.model_raw", "Qwen3.6-27B-MLX-8bit")
	assertStringPath(t, claim, "rankings.1.subject.runtime.local_runtime_version", "0.8.10")
	assertStringPath(t, claim, "rankings.1.subject.runtime.quantization", "8-bit")
	assertStringPath(t, claim, "rankings.1.subject.runtime.hardware_class", "Mac Studio")
	assertNumberPath(t, claim, "rankings.1.benchmarks.terminal-bench.score", 50)
	assertNumberPath(t, claim, "rankings.1.benchmarks.humaneval.score", 50)
	text := mustStringPath(t, claim, "claim_text")
	if !strings.Contains(text, "6 points behind Opus 4.7") {
		t.Fatalf("claim_text = %q, want FHI delta language", text)
	}
}

func TestFHIRankRefusesOnCoverageMismatch(t *testing.T) {
	repoRoot := benchRepoRoot(t)
	formula := mustLoadFHIFormula(t, repoRoot)
	records := mustLoadFHILedger(t, repoRoot, filepath.Join(repoRoot, "cmd", "bench", "testdata", "fhi", "fhi-rank-mismatch.jsonl"))

	claim, err := buildFHIRankClaim(formula, records)
	if err != nil {
		t.Fatalf("buildFHIRankClaim: %v", err)
	}
	assertStringPath(t, claim, "status", "refused")
	reason := mustStringPath(t, claim, "reason")
	if !strings.Contains(reason, "missing required benchmark coverage for humaneval") {
		t.Fatalf("refusal reason = %q, want missing humaneval coverage", reason)
	}
}

func mustLoadFHIFormula(t *testing.T, repoRoot string) *fhiFormulaConfig {
	t.Helper()

	formula, err := loadFHIFormula(repoRoot)
	if err != nil {
		t.Fatalf("loadFHIFormula: %v", err)
	}
	return formula
}

func mustLoadFHILedger(t *testing.T, repoRoot, path string) []map[string]any {
	t.Helper()

	records, err := loadFHIRecords(repoRoot, path)
	if err != nil {
		t.Fatalf("loadFHIRecords: %v", err)
	}
	return records
}

func assertStringPath(t *testing.T, doc map[string]any, path, want string) {
	t.Helper()

	got, ok := lookupAnyPath(doc, path)
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

func assertNumberPath(t *testing.T, doc map[string]any, path string, want float64) {
	t.Helper()

	got, ok := lookupAnyPath(doc, path)
	if !ok {
		t.Fatalf("missing %s", path)
	}
	var gotFloat float64
	switch v := got.(type) {
	case float64:
		gotFloat = v
	case int:
		gotFloat = float64(v)
	case int64:
		gotFloat = float64(v)
	default:
		t.Fatalf("%s = %T, want numeric", path, got)
	}
	if gotFloat != want {
		t.Fatalf("%s = %v, want %v", path, gotFloat, want)
	}
}

func mustStringPath(t *testing.T, doc map[string]any, path string) string {
	t.Helper()

	got, ok := lookupAnyPath(doc, path)
	if !ok {
		t.Fatalf("missing %s", path)
	}
	str, ok := got.(string)
	if !ok {
		t.Fatalf("%s = %T, want string", path, got)
	}
	return str
}

func lookupAnyPath(value any, path string) (any, bool) {
	cur := value
	for _, segment := range strings.Split(path, ".") {
		switch node := cur.(type) {
		case map[string]any:
			next, ok := node[segment]
			if !ok {
				return nil, false
			}
			cur = next
		default:
			rv := reflect.ValueOf(cur)
			if rv.Kind() == reflect.Slice || rv.Kind() == reflect.Array {
				idx, ok := parseIndex(segment)
				if !ok || idx < 0 || idx >= rv.Len() {
					return nil, false
				}
				cur = rv.Index(idx).Interface()
				continue
			}
			return nil, false
		}
	}
	return cur, true
}

func parseIndex(segment string) (int, bool) {
	var idx int
	for _, r := range segment {
		if r < '0' || r > '9' {
			return 0, false
		}
		idx = idx*10 + int(r-'0')
	}
	return idx, true
}
