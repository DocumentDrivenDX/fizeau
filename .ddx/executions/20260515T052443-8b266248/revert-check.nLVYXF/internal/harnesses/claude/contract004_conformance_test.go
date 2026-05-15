package claude_test

import (
	"path/filepath"
	"testing"

	"github.com/easel/fizeau/internal/harnesses/claude"
	"github.com/easel/fizeau/internal/harnesses/harnesstest"
)

// TestClaudeRunnerHarnessConformance asserts the bare Harness contract.
// Run in an isolated env: a non-existent cache path and an empty PATH
// so neither a stale snapshot nor a real claude binary can sneak in.
func TestClaudeRunnerHarnessConformance(t *testing.T) {
	isolateClaudeRunnerEnv(t)
	harnesstest.RunHarnessConformance(t, &claude.Runner{})
}

// TestClaudeRunnerQuotaHarnessConformance asserts QuotaHarness contract:
// QuotaStatus returns a valid value with no error for a cold cache;
// RefreshQuota's probe failure surfaces as State=QuotaUnavailable on a
// valid status value, not as an error.
func TestClaudeRunnerQuotaHarnessConformance(t *testing.T) {
	isolateClaudeRunnerEnv(t)
	harnesstest.RunQuotaHarnessConformance(t, &claude.Runner{})
}

// TestClaudeRunnerAccountHarnessConformance asserts the AccountHarness
// contract against the cold-cache path (no embedded account evidence).
func TestClaudeRunnerAccountHarnessConformance(t *testing.T) {
	isolateClaudeRunnerEnv(t)
	harnesstest.RunAccountHarnessConformance(t, &claude.Runner{})
}

// TestClaudeRunnerModelDiscoveryHarnessConformance asserts ResolveModelAlias
// covers each documented family and rejects out-of-set families with
// ErrAliasNotResolvable.
func TestClaudeRunnerModelDiscoveryHarnessConformance(t *testing.T) {
	isolateClaudeRunnerEnv(t)
	harnesstest.RunModelDiscoveryHarnessConformance(t, &claude.Runner{})
}

// isolateClaudeRunnerEnv points the quota cache at a temp file that
// doesn't exist and clears PATH so the PTY probe cannot find a real
// claude binary. This is the cold-cache + binary-absent path the
// CONTRACT-004 conformance suite expects on every harness.
func isolateClaudeRunnerEnv(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("FIZEAU_CLAUDE_QUOTA_CACHE", filepath.Join(dir, "claude-quota.json"))
	t.Setenv("PATH", "")
}
