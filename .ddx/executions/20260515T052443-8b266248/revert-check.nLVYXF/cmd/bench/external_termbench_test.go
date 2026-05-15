package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLoadTermbenchSubset_Canary verifies the three-task canary manifest
// parses cleanly through the same loader the matrix runner uses, and that
// the contract chosen in Step 6 of the harness-matrix plan is preserved:
// exactly the three task IDs fix-git, log-summary-date-ranges, and
// git-leak-recovery, pinned to the same TB-2 commit as the full subset.
// (hello-world was replaced with fix-git; it is not present in TB-2.)
func TestLoadTermbenchSubset_Canary(t *testing.T) {
	path := filepath.Join("..", "..", "scripts", "beadbench", "external", "termbench-subset-canary.json")
	subset, err := loadTermbenchSubset(path)
	if err != nil {
		t.Fatalf("load canary subset: %v", err)
	}

	wantIDs := []string{"fix-git", "log-summary-date-ranges", "git-leak-recovery"}
	if got := len(subset.Tasks); got != len(wantIDs) {
		t.Fatalf("canary must contain exactly %d tasks, got %d", len(wantIDs), got)
	}
	gotIDs := make(map[string]termbenchSubsetEntry, len(subset.Tasks))
	for _, e := range subset.Tasks {
		gotIDs[e.ID] = e
	}
	for _, id := range wantIDs {
		entry, ok := gotIDs[id]
		if !ok {
			t.Errorf("canary missing required task %q", id)
			continue
		}
		if entry.Category == "" || entry.Difficulty == "" || entry.Rationale == "" {
			t.Errorf("task %q must have category, difficulty, and rationale populated", id)
		}
		if entry.Difficulty == "hard" {
			t.Errorf("task %q is hard; canary excludes hard tasks (variance dominates with 3 reps)", id)
		}
	}

	const wantCommit = "53ff2b87d621bdb97b455671f2bd9728b7d86c11"
	if subset.DatasetCommit != wantCommit {
		t.Errorf("canary must pin to TB-2 commit %s, got %q", wantCommit, subset.DatasetCommit)
	}
	if subset.Dataset != "terminal-bench@2.0" {
		t.Errorf("canary dataset = %q, want terminal-bench@2.0", subset.Dataset)
	}
}

func TestLoadTermbenchSubset_LocalWideAllTasksExist(t *testing.T) {
	repoRoot := benchRepoRoot(t)
	path := filepath.Join(repoRoot, "scripts", "beadbench", "external", "termbench-subset-local-wide.json")
	subset, err := loadTermbenchSubset(path)
	if err != nil {
		t.Fatalf("load local-wide subset: %v", err)
	}
	if got, want := len(subset.Tasks), 15; got != want {
		t.Fatalf("local-wide task count = %d, want %d", got, want)
	}
	tasksDir := filepath.Join(repoRoot, "scripts", "benchmark", "external", "terminal-bench-2")
	if _, err := os.Stat(filepath.Join(tasksDir, subset.Tasks[0].ID)); err != nil {
		t.Skipf("terminal-bench-2 tree unavailable in this worktree: %v", err)
	}
	for _, task := range subset.Tasks {
		if _, err := os.Stat(filepath.Join(tasksDir, task.ID)); err != nil {
			t.Fatalf("local-wide task %q must exist under pinned TB-2 tree: %v", task.ID, err)
		}
		if task.Category == "" || task.Difficulty == "" || task.Rationale == "" {
			t.Fatalf("local-wide task %q missing category/difficulty/rationale", task.ID)
		}
	}
}
