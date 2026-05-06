package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestExternalBenchmarkImports(t *testing.T) {
	repoRoot := benchRepoRoot(t)
	schema := compileBenchmarkEvidenceSchema(t)

	cases := []struct {
		name     string
		fixture  string
		wantRows int
		check    func(t *testing.T, records []map[string]any)
	}{
		{
			name:     "rapid-mlx-mhi",
			fixture:  filepath.Join(repoRoot, "cmd", "bench", "testdata", "external-benchmarks", "rapid-mlx-mhi.md"),
			wantRows: 2,
			check: func(t *testing.T, records []map[string]any) {
				t.Helper()
				first := records[0]
				assertString(t, first, "benchmark.name", "mhi")
				assertString(t, first, "subject.provider", "rapid-mlx")
				assertString(t, first, "subject.harness", "Claude Code")
				assertString(t, first, "score.metric", "mhi")
				assertFloat(t, first, "score.value", 0.92)
				assertFloat(t, first, "score.raw_value", 92)

				second := records[1]
				assertString(t, second, "subject.harness", "unknown")
				assertString(t, second, "subject.provider", "rapid-mlx")
				assertFloat(t, second, "score.raw_value", 88)
			},
		},
		{
			name:     "skillsbench",
			fixture:  filepath.Join(repoRoot, "cmd", "bench", "testdata", "external-benchmarks", "skillsbench.csv"),
			wantRows: 1,
			check: func(t *testing.T, records []map[string]any) {
				t.Helper()
				record := records[0]
				assertString(t, record, "benchmark.name", "skillsbench")
				assertString(t, record, "subject.harness", "unknown")
				assertString(t, record, "subject.provider", "unknown")
				assertString(t, record, "score.metric", "pass_rate")
				assertFloat(t, record, "score.value", 0.75)
				assertFloat(t, record, "components.with_skills_pass_rate", 0.75)
				assertFloat(t, record, "components.without_skills_pass_rate", 0.625)
				assertFloat(t, record, "components.normalized_gain", 0.125)
				assertBool(t, record, "denominator.included", true)
			},
		},
		{
			name:     "swebench",
			fixture:  filepath.Join(repoRoot, "cmd", "bench", "testdata", "external-benchmarks", "swebench.csv"),
			wantRows: 2,
			check: func(t *testing.T, records []map[string]any) {
				t.Helper()
				leaderboard := records[0]
				assertString(t, leaderboard, "benchmark.name", "swe-bench")
				assertString(t, leaderboard, "subject.harness", "unknown")
				assertString(t, leaderboard, "subject.provider", "unknown")
				assertString(t, leaderboard, "score.metric", "resolved_rate")
				assertFloat(t, leaderboard, "score.value", 0.54)
				assertFloat(t, leaderboard, "score.passed", 270)
				assertFloat(t, leaderboard, "score.failed", 230)

				task := records[1]
				assertString(t, task, "scope.task_id", "django__django-15926")
				assertString(t, task, "score.metric", "resolved")
				assertFloat(t, task, "score.value", 1)
				assertString(t, task, "components.task_repo", "django/django")
			},
		},
		{
			name:     "humaneval",
			fixture:  filepath.Join(repoRoot, "cmd", "bench", "testdata", "external-benchmarks", "humaneval.jsonl"),
			wantRows: 2,
			check: func(t *testing.T, records []map[string]any) {
				t.Helper()
				aggregate := records[0]
				assertString(t, aggregate, "benchmark.name", "humaneval")
				assertString(t, aggregate, "score.metric", "pass_at_1")
				assertFloat(t, aggregate, "score.value", 0.68)
				assertBool(t, aggregate, "components.fhi_primary", false)
				assertString(t, aggregate, "components.fhi_role", "model_power_component")
				assertContainsString(t, aggregate, "coverage.included_benchmarks", "humaneval")

				result := records[1]
				assertString(t, result, "scope.task_id", "HumanEval/0")
				assertString(t, result, "score.metric", "passed")
				assertFloat(t, result, "score.value", 1)
				assertBool(t, result, "components.fhi_primary", false)
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			outPath := filepath.Join(t.TempDir(), tc.name+".jsonl")
			if code, out := runBenchCLI(t, repoRoot, "evidence", "import-external", "--work-dir", repoRoot, "--source", tc.fixture, "--out", outPath); code != 0 {
				t.Fatalf("import-external exit=%d output=%s", code, out)
			}

			raw, err := os.ReadFile(outPath)
			if err != nil {
				t.Fatalf("read output: %v", err)
			}
			lines := bytes.Split(bytes.TrimSpace(raw), []byte("\n"))
			if got, want := len(lines), tc.wantRows; got != want {
				t.Fatalf("record count = %d, want %d\n%s", got, want, string(raw))
			}

			records := make([]map[string]any, 0, len(lines))
			for _, line := range lines {
				doc := decodeJSONMap(t, line)
				if err := schema.Validate(doc); err != nil {
					t.Fatalf("schema validation failed: %v\n%s", err, string(line))
				}
				got, ok := lookupPath(doc, "source.artifact_sha256")
				if !ok {
					t.Fatalf("missing source.artifact_sha256")
				}
				sha, ok := got.(string)
				if !ok {
					t.Fatalf("source.artifact_sha256 = %T, want string", got)
				}
				if len(sha) != 64 {
					t.Fatalf("source.artifact_sha256 length = %d, want 64", len(sha))
				}
				records = append(records, doc)
			}
			tc.check(t, records)
		})
	}
}
