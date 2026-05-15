package fizeau_test

import (
	"errors"
	"os/exec"
	"strings"
	"testing"
)

func TestGitignoreIgnoresDDxRunStateDir(t *testing.T) {
	repoRoot := gitRepoRoot(t)

	assertGitTracked(t, repoRoot, ".ddx/beads.jsonl")

	ignored, output := gitCheckIgnore(t, repoRoot, ".ddx/run-state/example.json")
	if !ignored {
		t.Fatal("expected .ddx/run-state/example.json to be gitignored")
	}
	if output != ".ddx/run-state/example.json" {
		t.Fatalf("git check-ignore output = %q, want %q", output, ".ddx/run-state/example.json")
	}

	ignored, output = gitCheckIgnore(t, repoRoot, ".ddx/beads.jsonl")
	if ignored {
		t.Fatalf("expected tracked DDx audit file to remain unignored, got output %q", output)
	}
}

func gitRepoRoot(t testing.TB) string {
	t.Helper()

	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git rev-parse --show-toplevel: %v", err)
	}

	return strings.TrimSpace(string(out))
}

func assertGitTracked(t testing.TB, repoRoot, path string) {
	t.Helper()

	cmd := exec.Command("git", "ls-files", "--error-unmatch", path)
	cmd.Dir = repoRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git ls-files --error-unmatch %s: %v\n%s", path, err, strings.TrimSpace(string(out)))
	}
}

func gitCheckIgnore(t testing.TB, repoRoot, path string) (bool, string) {
	t.Helper()

	cmd := exec.Command("git", "check-ignore", path)
	cmd.Dir = repoRoot
	out, err := cmd.CombinedOutput()
	trimmed := strings.TrimSpace(string(out))
	if err == nil {
		return true, trimmed
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		return false, trimmed
	}

	t.Fatalf("git check-ignore %s: %v\n%s", path, err, trimmed)
	return false, ""
}
