package agentcli

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCmdVersionJSON_ValidJSON(t *testing.T) {
	oldVersion := Version
	oldCommit := GitCommit
	oldBuildTime := BuildTime
	t.Cleanup(func() {
		Version = oldVersion
		GitCommit = oldCommit
		BuildTime = oldBuildTime
	})

	Version = "v1.2.3"
	GitCommit = "abc1234"
	BuildTime = "2026-01-01T00:00:00Z"

	stdout, stderr, code := captureStdIO(t, func() int {
		return cmdVersion([]string{"--json"})
	})
	require.Equal(t, 0, code)
	assert.Empty(t, stderr)

	var out struct {
		Version string `json:"version"`
		Commit  string `json:"commit"`
		Dirty   bool   `json:"dirty"`
		Built   string `json:"built"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &out), "output must be valid JSON: %s", stdout)
	assert.Equal(t, "v1.2.3", out.Version)
	assert.Equal(t, "abc1234", out.Commit)
	assert.Equal(t, "2026-01-01T00:00:00Z", out.Built)
}

func TestCmdVersionJSON_FieldsPresent(t *testing.T) {
	oldVersion := Version
	oldCommit := GitCommit
	oldBuildTime := BuildTime
	t.Cleanup(func() {
		Version = oldVersion
		GitCommit = oldCommit
		BuildTime = oldBuildTime
	})

	Version = "dev"
	GitCommit = ""
	BuildTime = ""

	stdout, _, code := captureStdIO(t, func() int {
		return cmdVersion([]string{"--json"})
	})
	require.Equal(t, 0, code)

	var out map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(stdout), &out), "output must be valid JSON: %s", stdout)

	for _, key := range []string{"version", "commit", "dirty", "built"} {
		_, ok := out[key]
		assert.True(t, ok, "JSON output missing required key %q", key)
	}

	// dirty must be a boolean
	var dirty bool
	require.NoError(t, json.Unmarshal(out["dirty"], &dirty), "dirty field must be boolean")
}

func TestCmdVersionHuman_BackwardCompat(t *testing.T) {
	oldVersion := Version
	oldCommit := GitCommit
	oldBuildTime := BuildTime
	t.Cleanup(func() {
		Version = oldVersion
		GitCommit = oldCommit
		BuildTime = oldBuildTime
	})

	Version = "v0.0.8"
	GitCommit = "deadbeef"
	BuildTime = "2026-01-01T00:00:00Z"

	stdout, _, code := captureStdIO(t, func() int {
		return cmdVersion([]string{"--check-only"})
	})
	require.Equal(t, 0, code)
	assert.Contains(t, stdout, "fiz v0.0.8")
	assert.Contains(t, stdout, "deadbeef")
}
