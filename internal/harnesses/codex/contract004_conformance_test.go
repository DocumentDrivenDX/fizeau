package codex_test

import (
	"path/filepath"
	"testing"

	"github.com/easel/fizeau/internal/harnesses/codex"
	"github.com/easel/fizeau/internal/harnesses/harnesstest"
)

// TestCodexRunnerHarnessConformance asserts the bare Harness contract.
// Run in an isolated env: non-existent cache, auth, and sessions paths
// plus an empty PATH so neither a stale snapshot nor a real codex
// binary can sneak in.
func TestCodexRunnerHarnessConformance(t *testing.T) {
	isolateCodexRunnerEnv(t)
	harnesstest.RunHarnessConformance(t, &codex.Runner{})
}

// TestCodexRunnerQuotaHarnessConformance asserts QuotaHarness contract:
// QuotaStatus returns a valid value with no error for a cold cache;
// RefreshQuota's probe failure surfaces as State=QuotaUnavailable on a
// valid status value, not as an error.
func TestCodexRunnerQuotaHarnessConformance(t *testing.T) {
	isolateCodexRunnerEnv(t)
	harnesstest.RunQuotaHarnessConformance(t, &codex.Runner{})
}

// TestCodexRunnerAccountHarnessConformance asserts the AccountHarness
// contract against the cold-cache path (no auth.json).
func TestCodexRunnerAccountHarnessConformance(t *testing.T) {
	isolateCodexRunnerEnv(t)
	harnesstest.RunAccountHarnessConformance(t, &codex.Runner{})
}

// TestCodexRunnerModelDiscoveryHarnessConformance asserts ResolveModelAlias
// covers each documented family and rejects out-of-set families with
// ErrAliasNotResolvable.
func TestCodexRunnerModelDiscoveryHarnessConformance(t *testing.T) {
	isolateCodexRunnerEnv(t)
	harnesstest.RunModelDiscoveryHarnessConformance(t, &codex.Runner{})
}

// isolateCodexRunnerEnv points every codex evidence source at a temp
// location that does not exist and clears PATH so the PTY probe cannot
// find a real codex binary. This is the cold-cache + binary-absent
// path the CONTRACT-004 conformance suite expects on every harness.
func isolateCodexRunnerEnv(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("FIZEAU_CODEX_QUOTA_CACHE", filepath.Join(dir, "codex-quota.json"))
	t.Setenv("FIZEAU_CODEX_AUTH", filepath.Join(dir, "auth.json"))
	t.Setenv("FIZEAU_CODEX_SESSIONS_DIR", filepath.Join(dir, "sessions"))
	t.Setenv("CODEX_HOME", filepath.Join(dir, "codex-home"))
	t.Setenv("PATH", "")
}
