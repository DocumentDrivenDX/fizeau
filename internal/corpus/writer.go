package corpus

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// PromoteRequest captures the fields needed to add one bead to the corpus.
// Exactly one of Capability or FailureMode must be set.
type PromoteRequest struct {
	BeadID       string
	ProjectRoot  string
	BaseRev      string
	KnownGoodRev string
	Captured     string // YYYY-MM-DD
	Promoted     string // YYYY-MM-DD; defaults to Captured
	PromotedBy   string
	Capability   string
	FailureMode  string
	Difficulty   string
	PromptKind   string
	Notes        string
}

// Promote appends the request to corpus.yaml and writes the per-bead
// detail file. Both writes are atomic (write temp + rename). If the
// post-write Validate() fails, Promote rolls both files back to their
// pre-call state and returns the validation error.
//
// Promote refuses if the bead is already in corpus.yaml — the caller
// should also gate on "open in beads.jsonl" / "no closing commit"
// before invoking.
func Promote(root string, req PromoteRequest) error {
	if err := req.validate(); err != nil {
		return err
	}

	// Snapshot the index for rollback before any mutation.
	indexPath := IndexPath(root)
	prevIndex, indexExisted, err := readBytesIfExists(indexPath)
	if err != nil {
		return err
	}

	var idx Index
	if indexExisted {
		if err := yaml.Unmarshal(prevIndex, &idx); err != nil {
			return fmt.Errorf("parse %s: %w", indexPath, err)
		}
	} else {
		idx.Version = 1
	}
	for _, e := range idx.Beads {
		if e.BeadID == req.BeadID {
			return fmt.Errorf("bead %q already in corpus.yaml", req.BeadID)
		}
	}
	promoted := req.Promoted
	if promoted == "" {
		promoted = req.Captured
	}
	entry := IndexEntry{
		BeadID:      req.BeadID,
		Capability:  req.Capability,
		FailureMode: req.FailureMode,
		Promoted:    promoted,
		PromotedBy:  req.PromotedBy,
	}
	idx.Beads = append(idx.Beads, entry)
	if idx.Version == 0 {
		idx.Version = 1
	}

	detailPath := filepath.Join(DetailDir(root), req.BeadID+".yaml")
	if _, err := os.Stat(detailPath); err == nil {
		return fmt.Errorf("detail file already exists: %s", detailPath)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("stat %s: %w", detailPath, err)
	}

	detail := Detail{
		BeadID:       req.BeadID,
		ProjectRoot:  req.ProjectRoot,
		BaseRev:      req.BaseRev,
		KnownGoodRev: req.KnownGoodRev,
		Captured:     req.Captured,
		Capability:   req.Capability,
		FailureMode:  req.FailureMode,
		Difficulty:   req.Difficulty,
		PromptKind:   req.PromptKind,
		Notes:        req.Notes,
	}

	indexBytes, err := yaml.Marshal(&idx)
	if err != nil {
		return fmt.Errorf("marshal index: %w", err)
	}
	detailBytes, err := yaml.Marshal(&detail)
	if err != nil {
		return fmt.Errorf("marshal detail: %w", err)
	}

	if err := os.MkdirAll(DetailDir(root), 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", DetailDir(root), err)
	}

	if err := writeFileAtomic(indexPath, indexBytes, 0o644); err != nil {
		return err
	}
	if err := writeFileAtomic(detailPath, detailBytes, 0o644); err != nil {
		// Roll back the index.
		_ = restoreFile(indexPath, prevIndex, indexExisted)
		return err
	}

	if err := Validate(root); err != nil {
		// Roll back both writes.
		_ = os.Remove(detailPath)
		_ = restoreFile(indexPath, prevIndex, indexExisted)
		return fmt.Errorf("post-write validation failed (rolled back): %w", err)
	}
	return nil
}

func (r PromoteRequest) validate() error {
	var errs []error
	if r.BeadID == "" {
		errs = append(errs, errors.New("bead_id is required"))
	}
	if r.ProjectRoot == "" {
		errs = append(errs, errors.New("project_root is required"))
	}
	if r.BaseRev == "" {
		errs = append(errs, errors.New("base_rev is required"))
	}
	if r.KnownGoodRev == "" {
		errs = append(errs, errors.New("known_good_rev is required"))
	}
	if r.Captured == "" {
		errs = append(errs, errors.New("captured is required"))
	}
	if r.PromotedBy == "" {
		errs = append(errs, errors.New("promoted_by is required"))
	}
	if !contains(ValidDifficulties, r.Difficulty) {
		errs = append(errs, fmt.Errorf("difficulty %q is not one of %v", r.Difficulty, ValidDifficulties))
	}
	if !contains(ValidPromptKinds, r.PromptKind) {
		errs = append(errs, fmt.Errorf("prompt_kind %q is not one of %v", r.PromptKind, ValidPromptKinds))
	}
	hasCap := r.Capability != ""
	hasFail := r.FailureMode != ""
	if hasCap == hasFail {
		errs = append(errs, errors.New("exactly one of --capability or --failure-mode must be set"))
	}
	return errors.Join(errs...)
}

func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".corpus-*")
	if err != nil {
		return fmt.Errorf("create temp in %s: %w", dir, err)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("write %s: %w", tmpPath, err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("close %s: %w", tmpPath, err)
	}
	if err := os.Chmod(tmpPath, mode); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("chmod %s: %w", tmpPath, err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("rename %s → %s: %w", tmpPath, path, err)
	}
	return nil
}

func readBytesIfExists(path string) ([]byte, bool, error) {
	data, err := os.ReadFile(path)
	if err == nil {
		return data, true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	return nil, false, fmt.Errorf("read %s: %w", path, err)
}

func restoreFile(path string, prev []byte, existed bool) error {
	if !existed {
		return os.Remove(path)
	}
	return os.WriteFile(path, prev, 0o644)
}
