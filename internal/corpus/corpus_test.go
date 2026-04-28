package corpus

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fixture builds a complete, valid synthetic corpus under root and
// returns it. Tests then mutate the returned strings and rewrite
// individual files to drive specific rejection cases.
type fixture struct {
	root         string
	indexYAML    string
	capYAML      string
	detailFiles  map[string]string // bead_id → YAML content
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "corpus"), 0o755); err != nil {
		t.Fatal(err)
	}
	f := &fixture{
		root: root,
		indexYAML: `version: 1
beads:
  - id: agent-aaaaaaaa
    capability: tag-cap
    promoted: 2026-04-27
    promoted_by: tester
  - id: agent-bbbbbbbb
    failure_mode: tag-fail
    promoted: 2026-04-27
    promoted_by: tester
`,
		capYAML: `version: 1
tags:
  - id: tag-cap
    area: cli
    kind: capability
    description: A capability tag.
  - id: tag-fail
    area: provider
    kind: failure_mode
    description: A failure_mode tag.
`,
		detailFiles: map[string]string{
			"agent-aaaaaaaa": `bead_id: agent-aaaaaaaa
project_root: /tmp/sample
base_rev: deadbeef
known_good_rev: cafef00d
captured: 2026-04-27
capability: tag-cap
difficulty: medium
prompt_kind: implement-with-spec
notes: Sample.
`,
			"agent-bbbbbbbb": `bead_id: agent-bbbbbbbb
project_root: /tmp/sample
base_rev: deadbeef
known_good_rev: cafef00d
captured: 2026-04-27
failure_mode: tag-fail
difficulty: hard
prompt_kind: debug-and-fix
notes: Sample.
`,
		},
	}
	f.write(t)
	return f
}

func (f *fixture) write(t *testing.T) {
	t.Helper()
	if err := os.WriteFile(IndexPath(f.root), []byte(f.indexYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(CapabilitiesPath(f.root), []byte(f.capYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	// Remove pre-existing detail files so test mutations cannot leave stale ones.
	entries, _ := os.ReadDir(DetailDir(f.root))
	for _, e := range entries {
		if e.Name() == "capabilities.yaml" {
			continue
		}
		_ = os.Remove(filepath.Join(DetailDir(f.root), e.Name()))
	}
	for id, body := range f.detailFiles {
		p := filepath.Join(DetailDir(f.root), id+".yaml")
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestValidate_GoodFixture(t *testing.T) {
	f := newFixture(t)
	if err := Validate(f.root); err != nil {
		t.Fatalf("expected valid corpus, got: %v", err)
	}
}

func TestValidate_RejectsMissingPromotedBy(t *testing.T) {
	f := newFixture(t)
	f.indexYAML = `version: 1
beads:
  - id: agent-aaaaaaaa
    capability: tag-cap
    promoted: 2026-04-27
  - id: agent-bbbbbbbb
    failure_mode: tag-fail
    promoted: 2026-04-27
    promoted_by: tester
`
	f.write(t)
	err := Validate(f.root)
	if err == nil || !strings.Contains(err.Error(), "missing promoted_by") {
		t.Fatalf("expected promoted_by error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "agent-aaaaaaaa") {
		t.Fatalf("error should reference offending entry id: %v", err)
	}
}

func TestValidate_RejectsBothCapabilityAndFailureMode(t *testing.T) {
	f := newFixture(t)
	f.indexYAML = `version: 1
beads:
  - id: agent-aaaaaaaa
    capability: tag-cap
    failure_mode: tag-fail
    promoted: 2026-04-27
    promoted_by: tester
  - id: agent-bbbbbbbb
    failure_mode: tag-fail
    promoted: 2026-04-27
    promoted_by: tester
`
	f.write(t)
	err := Validate(f.root)
	if err == nil || !strings.Contains(err.Error(), "exactly one of capability or failure_mode") {
		t.Fatalf("expected exclusive-tag error, got: %v", err)
	}
}

func TestValidate_RejectsNeitherCapabilityNorFailureMode(t *testing.T) {
	f := newFixture(t)
	f.indexYAML = `version: 1
beads:
  - id: agent-aaaaaaaa
    promoted: 2026-04-27
    promoted_by: tester
  - id: agent-bbbbbbbb
    failure_mode: tag-fail
    promoted: 2026-04-27
    promoted_by: tester
`
	f.write(t)
	err := Validate(f.root)
	if err == nil || !strings.Contains(err.Error(), "exactly one of capability or failure_mode") {
		t.Fatalf("expected exclusive-tag error, got: %v", err)
	}
}

func TestValidate_RejectsUnknownCapability(t *testing.T) {
	f := newFixture(t)
	f.indexYAML = `version: 1
beads:
  - id: agent-aaaaaaaa
    capability: not-in-vocab
    promoted: 2026-04-27
    promoted_by: tester
  - id: agent-bbbbbbbb
    failure_mode: tag-fail
    promoted: 2026-04-27
    promoted_by: tester
`
	// Detail still references tag-cap, so update it too:
	f.detailFiles["agent-aaaaaaaa"] = strings.ReplaceAll(
		f.detailFiles["agent-aaaaaaaa"], "capability: tag-cap", "capability: not-in-vocab")
	f.write(t)
	err := Validate(f.root)
	if err == nil || !strings.Contains(err.Error(), `not in capabilities.yaml`) {
		t.Fatalf("expected unknown-tag error, got: %v", err)
	}
}

func TestValidate_RejectsKindMismatch(t *testing.T) {
	f := newFixture(t)
	// tag-cap is declared kind=capability — but the entry uses it as failure_mode.
	f.indexYAML = `version: 1
beads:
  - id: agent-aaaaaaaa
    failure_mode: tag-cap
    promoted: 2026-04-27
    promoted_by: tester
  - id: agent-bbbbbbbb
    failure_mode: tag-fail
    promoted: 2026-04-27
    promoted_by: tester
`
	f.detailFiles["agent-aaaaaaaa"] = strings.ReplaceAll(
		f.detailFiles["agent-aaaaaaaa"], "capability: tag-cap", "failure_mode: tag-cap")
	f.write(t)
	err := Validate(f.root)
	if err == nil || !strings.Contains(err.Error(), "is declared kind=") {
		t.Fatalf("expected kind-mismatch error, got: %v", err)
	}
}

func TestValidate_RejectsDetailNotInIndex(t *testing.T) {
	f := newFixture(t)
	// Add an orphan detail file.
	orphan := `bead_id: agent-cccccccc
project_root: /tmp/sample
base_rev: deadbeef
known_good_rev: cafef00d
captured: 2026-04-27
capability: tag-cap
difficulty: easy
prompt_kind: implement-with-spec
notes: Orphan.
`
	if err := os.WriteFile(filepath.Join(DetailDir(f.root), "agent-cccccccc.yaml"),
		[]byte(orphan), 0o644); err != nil {
		t.Fatal(err)
	}
	err := Validate(f.root)
	if err == nil || !strings.Contains(err.Error(), "no matching entry in") {
		t.Fatalf("expected orphan-detail error, got: %v", err)
	}
}

func TestValidate_RejectsIndexEntryWithoutDetail(t *testing.T) {
	f := newFixture(t)
	delete(f.detailFiles, "agent-aaaaaaaa")
	f.write(t)
	err := Validate(f.root)
	if err == nil || !strings.Contains(err.Error(), "no matching detail file") {
		t.Fatalf("expected missing-detail error, got: %v", err)
	}
}

func TestValidate_RejectsBadDifficultyAndPromptKind(t *testing.T) {
	f := newFixture(t)
	f.detailFiles["agent-aaaaaaaa"] = `bead_id: agent-aaaaaaaa
project_root: /tmp/sample
base_rev: deadbeef
known_good_rev: cafef00d
captured: 2026-04-27
capability: tag-cap
difficulty: trivial
prompt_kind: speedrun
notes: Sample.
`
	f.write(t)
	err := Validate(f.root)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), `difficulty "trivial"`) {
		t.Fatalf("expected difficulty error, got: %v", err)
	}
	if !strings.Contains(err.Error(), `prompt_kind "speedrun"`) {
		t.Fatalf("expected prompt_kind error, got: %v", err)
	}
}

func TestPromote_AppendsAndValidates(t *testing.T) {
	f := newFixture(t)
	req := PromoteRequest{
		BeadID:       "agent-cccccccc",
		ProjectRoot:  "/tmp/sample",
		BaseRev:      "abc123",
		KnownGoodRev: "def456",
		Captured:     "2026-04-27",
		PromotedBy:   "tester",
		Capability:   "tag-cap",
		Difficulty:   "easy",
		PromptKind:   "implement-with-spec",
		Notes:        "promoted by test",
	}
	if err := Promote(f.root, req); err != nil {
		t.Fatalf("Promote: %v", err)
	}
	loaded, err := Load(f.root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !loaded.HasBead("agent-cccccccc") {
		t.Fatal("bead not in index after Promote")
	}
	if _, ok := loaded.Details["agent-cccccccc"]; !ok {
		t.Fatal("detail file not present after Promote")
	}
}

func TestPromote_RejectsAlreadyPromoted(t *testing.T) {
	f := newFixture(t)
	req := PromoteRequest{
		BeadID:       "agent-aaaaaaaa", // already in fixture
		ProjectRoot:  "/tmp/sample",
		BaseRev:      "abc123",
		KnownGoodRev: "def456",
		Captured:     "2026-04-27",
		PromotedBy:   "tester",
		Capability:   "tag-cap",
		Difficulty:   "easy",
		PromptKind:   "implement-with-spec",
		Notes:        "dup",
	}
	err := Promote(f.root, req)
	if err == nil || !strings.Contains(err.Error(), "already in corpus.yaml") {
		t.Fatalf("expected duplicate error, got: %v", err)
	}
}

func TestPromote_RollsBackOnInvalidTag(t *testing.T) {
	f := newFixture(t)
	indexBefore, _ := os.ReadFile(IndexPath(f.root))
	req := PromoteRequest{
		BeadID:       "agent-cccccccc",
		ProjectRoot:  "/tmp/sample",
		BaseRev:      "abc123",
		KnownGoodRev: "def456",
		Captured:     "2026-04-27",
		PromotedBy:   "tester",
		Capability:   "ghost-tag", // not in capabilities.yaml
		Difficulty:   "easy",
		PromptKind:   "implement-with-spec",
		Notes:        "should roll back",
	}
	err := Promote(f.root, req)
	if err == nil || !strings.Contains(err.Error(), "rolled back") {
		t.Fatalf("expected rollback error, got: %v", err)
	}
	indexAfter, _ := os.ReadFile(IndexPath(f.root))
	if string(indexBefore) != string(indexAfter) {
		t.Fatalf("index was not rolled back:\nbefore=%s\nafter=%s", indexBefore, indexAfter)
	}
	if _, err := os.Stat(filepath.Join(DetailDir(f.root), "agent-cccccccc.yaml")); err == nil {
		t.Fatal("detail file should have been rolled back")
	}
}

func TestPromote_RejectsBothTagsSet(t *testing.T) {
	f := newFixture(t)
	req := PromoteRequest{
		BeadID:       "agent-cccccccc",
		ProjectRoot:  "/tmp/sample",
		BaseRev:      "abc123",
		KnownGoodRev: "def456",
		Captured:     "2026-04-27",
		PromotedBy:   "tester",
		Capability:   "tag-cap",
		FailureMode:  "tag-fail",
		Difficulty:   "easy",
		PromptKind:   "implement-with-spec",
		Notes:        "both",
	}
	err := Promote(f.root, req)
	if err == nil || !strings.Contains(err.Error(), "exactly one") {
		t.Fatalf("expected exclusive-tag error, got: %v", err)
	}
}
