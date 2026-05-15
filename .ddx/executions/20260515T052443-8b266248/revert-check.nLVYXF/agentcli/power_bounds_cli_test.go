package agentcli_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCLI_RunRejectsInvalidPowerBoundsBeforeDispatch(t *testing.T) {
	exe := buildAgentCLI(t)
	workDir := t.TempDir()
	home := t.TempDir()

	res := runBuiltCLI(t, exe, workDir, testEnvWithHome(home, nil), "--work-dir", workDir, "--min-power", "9", "--max-power", "4", "-p", "hello")
	require.Equal(t, 2, res.exitCode, "stdout=%s stderr=%s", res.stdout, res.stderr)
	assert.Contains(t, res.stderr, "power")
}

func TestCLI_RouteStatusRejectsInvalidPowerBoundsBeforeConfig(t *testing.T) {
	exe := buildAgentCLI(t)
	workDir := t.TempDir()
	home := t.TempDir()

	res := runBuiltCLI(t, exe, workDir, testEnvWithHome(home, nil), "--work-dir", workDir, "route-status", "--min-power", "9", "--max-power", "4")
	require.Equal(t, 2, res.exitCode, "stdout=%s stderr=%s", res.stdout, res.stderr)
	assert.Contains(t, res.stderr, "power")
}
