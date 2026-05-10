package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/easel/fizeau/internal/benchmark/evidence"
)

func TestEvidenceValidateCommand(t *testing.T) {
	repoRoot := benchRepoRoot(t)
	valid := filepath.Join(repoRoot, "cmd", "bench", "testdata", "benchmark-evidence", "valid-record.jsonl")
	invalid := filepath.Join(repoRoot, "cmd", "bench", "testdata", "benchmark-evidence", "invalid-record.jsonl")

	if code, out := runBenchCLI(t, repoRoot, "evidence", "validate", "--work-dir", repoRoot, valid); code != 0 {
		t.Fatalf("validate valid fixture exit=%d output=%s", code, out)
	}

	if code, out := runBenchCLI(t, repoRoot, "evidence", "validate", "--work-dir", repoRoot, invalid); code == 0 {
		t.Fatalf("validate invalid fixture unexpectedly succeeded: %s", out)
	} else if !strings.Contains(out, "artifact_sha256") && !strings.Contains(out, "evidence validate") {
		t.Fatalf("validate invalid fixture error missing useful context: %s", out)
	}
}

func TestEvidenceAppendGeneratesStableRecordID(t *testing.T) {
	repoRoot := benchRepoRoot(t)
	input := filepath.Join(repoRoot, "cmd", "bench", "testdata", "benchmark-evidence", "append-input.jsonl")
	ledger := filepath.Join(t.TempDir(), "ledger.jsonl")

	if code, out := runBenchCLI(t, repoRoot, "evidence", "append", "--work-dir", repoRoot, "--in", input, "--ledger", ledger); code != 0 {
		t.Fatalf("append exit=%d output=%s", code, out)
	}

	record := readSingleJSONLRecord(t, ledger)
	doc := readSingleJSONRecord(t, input)
	wantID, err := evidence.StableRecordID(doc)
	if err != nil {
		t.Fatalf("StableRecordID: %v", err)
	}
	if got := mustStringField(t, record, "record_id"); got != wantID {
		t.Fatalf("ledger record_id = %q, want %q", got, wantID)
	}
}

func TestEvidenceAppendDetectsDuplicates(t *testing.T) {
	repoRoot := benchRepoRoot(t)
	input := filepath.Join(repoRoot, "cmd", "bench", "testdata", "benchmark-evidence", "append-input.jsonl")
	ledger := filepath.Join(t.TempDir(), "ledger.jsonl")

	validator, err := evidence.NewValidator(repoRoot)
	if err != nil {
		t.Fatalf("NewValidator: %v", err)
	}

	report, err := validator.AppendLedger(input, ledger)
	if err != nil {
		t.Fatalf("first append: %v", err)
	}
	if report.Added != 1 {
		t.Fatalf("first append added %d records, want 1", report.Added)
	}

	report, err = validator.AppendLedger(input, ledger)
	var dupErr *evidence.DuplicateRecordsError
	if !errors.As(err, &dupErr) {
		t.Fatalf("second append error = %v, want DuplicateRecordsError", err)
	}
	if report == nil || len(report.Duplicates) == 0 {
		t.Fatalf("second append did not report duplicates: %#v", report)
	}

	raw, err := os.ReadFile(ledger)
	if err != nil {
		t.Fatalf("read ledger: %v", err)
	}
	if got := len(bytes.Split(bytes.TrimSpace(raw), []byte("\n"))); got != 1 {
		t.Fatalf("ledger line count = %d, want 1", got)
	}
}

func runBenchCLI(t *testing.T, repoRoot string, args ...string) (int, string) {
	t.Helper()

	cmd := exec.Command("go", append([]string{"run", "."}, args...)...)
	cmd.Dir = filepath.Join(repoRoot, "cmd", "bench")
	cmd.Env = append(os.Environ(), "GOWORK=off")
	out, err := cmd.CombinedOutput()
	if err == nil {
		return 0, string(out)
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		return exitErr.ExitCode(), string(out)
	}
	t.Fatalf("go run failed: %v\n%s", err, string(out))
	return 1, string(out)
}

func readSingleJSONLRecord(t *testing.T, path string) map[string]any {
	t.Helper()

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read ledger: %v", err)
	}
	lines := bytes.Split(bytes.TrimSpace(raw), []byte("\n"))
	if len(lines) != 1 {
		t.Fatalf("expected 1 ledger record, got %d", len(lines))
	}
	return decodeJSONMap(t, lines[0])
}

func readSingleJSONRecord(t *testing.T, path string) map[string]any {
	t.Helper()

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read record: %v", err)
	}
	return decodeJSONMap(t, raw)
}

func decodeJSONMap(t *testing.T, raw []byte) map[string]any {
	t.Helper()

	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("decode json: %v", err)
	}
	return doc
}

func mustStringField(t *testing.T, doc map[string]any, key string) string {
	t.Helper()

	got, ok := doc[key]
	if !ok {
		t.Fatalf("missing %s", key)
	}
	str, ok := got.(string)
	if !ok {
		t.Fatalf("%s = %T, want string", key, got)
	}
	return str
}
