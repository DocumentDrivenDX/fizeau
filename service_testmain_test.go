package fizeau_test

import (
	"os"
	"path/filepath"
	"testing"
)

// TestMain isolates the agent_test package from the developer's live
// $HOME/.config/fizeau/config.yaml. Without this, calls like
// agent.New(agent.ServiceOptions{}) auto-load the user's real config —
// which has caused verify-worktree gates at older base revisions to fail
// when the live config contains provider types that revision didn't yet
// know about (agent-27806ad5).
//
// Per-test t.Setenv("HOME", ...) calls still take precedence for tests
// that need to plant a specific config on disk.
func TestMain(m *testing.M) {
	os.Exit(runTests(m))
}

func runTests(m *testing.M) int {
	tmp, err := os.MkdirTemp("", "agent-test-home-")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(tmp)

	os.Setenv("HOME", tmp)
	os.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, ".config"))
	os.Setenv("FIZEAU_CACHE_DIR", filepath.Join(tmp, "cache"))

	return m.Run()
}
