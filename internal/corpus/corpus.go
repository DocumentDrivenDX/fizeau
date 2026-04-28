// Package corpus implements the internal benchmark corpus: a curated,
// capability-tagged set of closed beads worth tracking over time.
//
// The corpus is intentionally curation-driven (no auto-promotion). Three
// files describe it:
//
//   - <root>/corpus.yaml — append-only top-level index, one entry per
//     promoted bead. Required fields: bead_id, promoted, promoted_by, and
//     exactly one of {capability, failure_mode}.
//   - <root>/corpus/<bead-id>.yaml — per-bead detail (project_root, base
//     and known-good revisions, difficulty, prompt_kind, notes).
//   - <root>/corpus/capabilities.yaml — controlled vocabulary of
//     capability + failure_mode tags. Every tag referenced by the index
//     must appear here.
//
// Validate loads all three files and cross-checks them; the returned
// errors point at the file + entry that is malformed.
package corpus

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/DocumentDrivenDX/agent/internal/safefs"
	"gopkg.in/yaml.v3"
)

// IndexEntry is one line of corpus.yaml (top-level index).
type IndexEntry struct {
	BeadID      string `yaml:"id"`
	Capability  string `yaml:"capability,omitempty"`
	FailureMode string `yaml:"failure_mode,omitempty"`
	Promoted    string `yaml:"promoted"`
	PromotedBy  string `yaml:"promoted_by"`
}

// Index is the YAML envelope for corpus.yaml.
type Index struct {
	Version int          `yaml:"version"`
	Beads   []IndexEntry `yaml:"beads"`
}

// Detail is the per-bead detail YAML stored at corpus/<bead-id>.yaml.
type Detail struct {
	BeadID       string `yaml:"bead_id"`
	ProjectRoot  string `yaml:"project_root"`
	BaseRev      string `yaml:"base_rev"`
	KnownGoodRev string `yaml:"known_good_rev"`
	Captured     string `yaml:"captured"`
	Capability   string `yaml:"capability,omitempty"`
	FailureMode  string `yaml:"failure_mode,omitempty"`
	Difficulty   string `yaml:"difficulty"`
	PromptKind   string `yaml:"prompt_kind"`
	Notes        string `yaml:"notes"`
}

// Tag is one capability-vocabulary entry in capabilities.yaml.
type Tag struct {
	ID          string `yaml:"id"`
	Area        string `yaml:"area"`
	Kind        string `yaml:"kind"` // "capability" or "failure_mode"
	Description string `yaml:"description"`
}

// Capabilities is the YAML envelope for capabilities.yaml.
type Capabilities struct {
	Version int   `yaml:"version"`
	Tags    []Tag `yaml:"tags"`
}

// ValidDifficulties enumerates accepted difficulty buckets.
var ValidDifficulties = []string{"easy", "medium", "hard"}

// ValidPromptKinds enumerates accepted prompt kinds.
var ValidPromptKinds = []string{
	"implement-with-spec",
	"port-by-analogy",
	"debug-and-fix",
	"review",
}

// Loaded is the result of Load — the three files plus their resolved root.
type Loaded struct {
	Root         string
	Index        Index
	Capabilities Capabilities
	Details      map[string]Detail // keyed by bead_id
	DetailFiles  map[string]string // bead_id → file path on disk
}

// IndexPath returns the conventional index path under root.
func IndexPath(root string) string { return filepath.Join(root, "corpus.yaml") }

// DetailDir returns the conventional per-bead detail directory.
func DetailDir(root string) string { return filepath.Join(root, "corpus") }

// CapabilitiesPath returns the conventional capabilities vocabulary path.
func CapabilitiesPath(root string) string {
	return filepath.Join(root, "corpus", "capabilities.yaml")
}

// Load reads and parses all three files. It does NOT cross-validate; call
// Validate (or call ValidateLoaded) for that. Missing files return an
// error wrapping os.ErrNotExist so callers can distinguish "no corpus
// yet" from a malformed corpus.
func Load(root string) (*Loaded, error) {
	out := &Loaded{
		Root:        root,
		Details:     map[string]Detail{},
		DetailFiles: map[string]string{},
	}

	indexPath := IndexPath(root)
	indexBytes, err := safefs.ReadFile(indexPath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", indexPath, err)
	}
	if err := yaml.Unmarshal(indexBytes, &out.Index); err != nil {
		return nil, fmt.Errorf("parse %s: %w", indexPath, err)
	}

	capPath := CapabilitiesPath(root)
	capBytes, err := safefs.ReadFile(capPath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", capPath, err)
	}
	if err := yaml.Unmarshal(capBytes, &out.Capabilities); err != nil {
		return nil, fmt.Errorf("parse %s: %w", capPath, err)
	}

	detailDir := DetailDir(root)
	entries, err := os.ReadDir(detailDir)
	if err != nil {
		return nil, fmt.Errorf("read dir %s: %w", detailDir, err)
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".yaml") || name == "capabilities.yaml" {
			continue
		}
		p := filepath.Join(detailDir, name)
		raw, err := safefs.ReadFile(p)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", p, err)
		}
		var d Detail
		if err := yaml.Unmarshal(raw, &d); err != nil {
			return nil, fmt.Errorf("parse %s: %w", p, err)
		}
		if d.BeadID == "" {
			return nil, fmt.Errorf("%s: detail file is missing bead_id", p)
		}
		if existing, ok := out.DetailFiles[d.BeadID]; ok {
			return nil, fmt.Errorf("%s: bead_id %q already declared by %s", p, d.BeadID, existing)
		}
		out.Details[d.BeadID] = d
		out.DetailFiles[d.BeadID] = p
	}
	return out, nil
}

// Validate loads and cross-validates the corpus rooted at root.
func Validate(root string) error {
	loaded, err := Load(root)
	if err != nil {
		return err
	}
	return ValidateLoaded(loaded)
}

// ValidateLoaded cross-validates an already-loaded corpus. The error
// returned is a single error joining every problem found, so callers
// see all failures at once.
func ValidateLoaded(l *Loaded) error {
	var errs []error

	indexPath := IndexPath(l.Root)
	tagSet := map[string]Tag{}
	for _, t := range l.Capabilities.Tags {
		if t.ID == "" {
			errs = append(errs, fmt.Errorf("%s: capability tag is missing id", CapabilitiesPath(l.Root)))
			continue
		}
		if _, dup := tagSet[t.ID]; dup {
			errs = append(errs, fmt.Errorf("%s: duplicate tag id %q", CapabilitiesPath(l.Root), t.ID))
			continue
		}
		tagSet[t.ID] = t
	}

	indexByID := map[string]IndexEntry{}
	for i, e := range l.Index.Beads {
		loc := fmt.Sprintf("%s entry %d (id=%q)", indexPath, i, e.BeadID)
		if e.BeadID == "" {
			errs = append(errs, fmt.Errorf("%s: missing bead_id (id)", loc))
			continue
		}
		if _, dup := indexByID[e.BeadID]; dup {
			errs = append(errs, fmt.Errorf("%s: duplicate bead_id %q", loc, e.BeadID))
			continue
		}
		indexByID[e.BeadID] = e
		if e.Promoted == "" {
			errs = append(errs, fmt.Errorf("%s: missing promoted (date)", loc))
		}
		if e.PromotedBy == "" {
			errs = append(errs, fmt.Errorf("%s: missing promoted_by", loc))
		}
		hasCap := e.Capability != ""
		hasFail := e.FailureMode != ""
		switch {
		case hasCap && hasFail:
			errs = append(errs, fmt.Errorf("%s: must declare exactly one of capability or failure_mode (both set)", loc))
		case !hasCap && !hasFail:
			errs = append(errs, fmt.Errorf("%s: must declare exactly one of capability or failure_mode (neither set)", loc))
		}
		if hasCap {
			if t, ok := tagSet[e.Capability]; !ok {
				errs = append(errs, fmt.Errorf("%s: capability %q not in capabilities.yaml", loc, e.Capability))
			} else if t.Kind != "" && t.Kind != "capability" {
				errs = append(errs, fmt.Errorf("%s: tag %q is declared kind=%q, not capability", loc, e.Capability, t.Kind))
			}
		}
		if hasFail {
			if t, ok := tagSet[e.FailureMode]; !ok {
				errs = append(errs, fmt.Errorf("%s: failure_mode %q not in capabilities.yaml", loc, e.FailureMode))
			} else if t.Kind != "" && t.Kind != "failure_mode" {
				errs = append(errs, fmt.Errorf("%s: tag %q is declared kind=%q, not failure_mode", loc, e.FailureMode, t.Kind))
			}
		}
	}

	for beadID, d := range l.Details {
		path := l.DetailFiles[beadID]
		if _, ok := indexByID[beadID]; !ok {
			errs = append(errs, fmt.Errorf("%s: bead_id %q has no matching entry in %s", path, beadID, indexPath))
		}
		if d.ProjectRoot == "" {
			errs = append(errs, fmt.Errorf("%s: missing project_root", path))
		}
		if d.BaseRev == "" {
			errs = append(errs, fmt.Errorf("%s: missing base_rev", path))
		}
		if d.KnownGoodRev == "" {
			errs = append(errs, fmt.Errorf("%s: missing known_good_rev", path))
		}
		if d.Captured == "" {
			errs = append(errs, fmt.Errorf("%s: missing captured (date)", path))
		}
		if !contains(ValidDifficulties, d.Difficulty) {
			errs = append(errs, fmt.Errorf("%s: difficulty %q is not one of %v", path, d.Difficulty, ValidDifficulties))
		}
		if !contains(ValidPromptKinds, d.PromptKind) {
			errs = append(errs, fmt.Errorf("%s: prompt_kind %q is not one of %v", path, d.PromptKind, ValidPromptKinds))
		}
		hasCap := d.Capability != ""
		hasFail := d.FailureMode != ""
		if !hasCap && !hasFail {
			errs = append(errs, fmt.Errorf("%s: must declare capability or failure_mode", path))
		}
		if hasCap && hasFail {
			errs = append(errs, fmt.Errorf("%s: must declare only one of capability or failure_mode", path))
		}
		if hasCap {
			if _, ok := tagSet[d.Capability]; !ok {
				errs = append(errs, fmt.Errorf("%s: capability %q not in capabilities.yaml", path, d.Capability))
			}
		}
		if hasFail {
			if _, ok := tagSet[d.FailureMode]; !ok {
				errs = append(errs, fmt.Errorf("%s: failure_mode %q not in capabilities.yaml", path, d.FailureMode))
			}
		}
	}

	// Every index entry should have a matching detail file. (Symmetric
	// to the per-detail check above.) Index-only entries are a warning
	// shape; treat as error so the corpus stays self-consistent.
	for beadID := range indexByID {
		if _, ok := l.Details[beadID]; !ok {
			errs = append(errs, fmt.Errorf("%s: bead_id %q has no matching detail file at %s/%s.yaml",
				indexPath, beadID, DetailDir(l.Root), beadID))
		}
	}

	return errors.Join(errs...)
}

func contains(set []string, s string) bool {
	for _, x := range set {
		if x == s {
			return true
		}
	}
	return false
}

// SortedBeadIDs returns the bead ids declared in the index, sorted.
func (l *Loaded) SortedBeadIDs() []string {
	ids := make([]string, 0, len(l.Index.Beads))
	for _, e := range l.Index.Beads {
		ids = append(ids, e.BeadID)
	}
	sort.Strings(ids)
	return ids
}

// HasBead reports whether the given bead_id is already promoted.
func (l *Loaded) HasBead(id string) bool {
	for _, e := range l.Index.Beads {
		if e.BeadID == id {
			return true
		}
	}
	return false
}
