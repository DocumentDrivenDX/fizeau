package agentcli_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/easel/fizeau/internal/discoverycache"
	"github.com/easel/fizeau/internal/modelregistry"
	"github.com/easel/fizeau/internal/runtimesignals"
	"github.com/stretchr/testify/require"
)

func TestCLI_ResolveRouteMatchesModelsSnapshotRows(t *testing.T) {
	exe := buildAgentCLI(t)
	workDir := t.TempDir()
	home := t.TempDir()
	cacheDir := t.TempDir()

	alpha := newCountedOpenAIServer(t, http.StatusOK, "qwen3.5-27b", "alpha-ok")
	beta := newCountedOpenAIServer(t, http.StatusOK, "qwen3.5-27b", "beta-ok")
	alpha.setModels("qwen3.5-27b")
	beta.setModels("qwen3.5-27b")

	writeTempConfig(t, workDir, `
providers:
  alpha:
    type: lmstudio
    include_by_default: true
    endpoints:
      - name: primary
        base_url: `+alpha.baseURL()+`
  beta:
    type: openrouter
    include_by_default: true
    endpoints:
      - name: secondary
        base_url: `+beta.baseURL()+`
default: alpha
`)

	cache := &discoverycache.Cache{Root: cacheDir}
	writeSnapshotDiscoveryFixture(t, cache, testDiscoverySourceName("alpha", "primary", alpha.baseURL(), ""), time.Date(2026, 5, 12, 15, 0, 0, 0, time.UTC), []string{"qwen3.5-27b"})
	writeSnapshotDiscoveryFixture(t, cache, testDiscoverySourceName("beta", "secondary", beta.baseURL(), ""), time.Date(2026, 5, 12, 15, 0, 0, 0, time.UTC), []string{"qwen3.5-27b"})

	quotaAlpha := 17
	quotaBeta := 3
	require.NoError(t, runtimesignals.Write(cache, runtimesignals.Signal{
		Provider:         "alpha",
		Status:           runtimesignals.StatusAvailable,
		QuotaRemaining:   &quotaAlpha,
		RecentP50Latency: 120 * time.Millisecond,
		RecordedAt:       time.Date(2026, 5, 12, 15, 0, 0, 0, time.UTC),
	}))
	require.NoError(t, runtimesignals.Write(cache, runtimesignals.Signal{
		Provider:         "beta",
		Status:           runtimesignals.StatusAvailable,
		QuotaRemaining:   &quotaBeta,
		RecentP50Latency: 240 * time.Millisecond,
		RecordedAt:       time.Date(2026, 5, 12, 15, 0, 0, 0, time.UTC),
	}))

	env := testEnvWithHome(home, map[string]string{
		"PATH":             "",
		"FIZEAU_CACHE_DIR": cacheDir,
	})

	models := runBuiltCLI(t, exe, workDir, env, "--work-dir", workDir, "--json", "models")
	require.Equal(t, 0, models.exitCode, "stderr=%s stdout=%s", models.stderr, models.stdout)

	var snapshot modelregistry.ModelSnapshot
	require.NoError(t, json.Unmarshal([]byte(models.stdout), &snapshot), "stdout=%s", models.stdout)
	require.Len(t, snapshot.Models, 2)

	rowsByKey := make(map[string]modelregistry.KnownModel, len(snapshot.Models))
	for _, row := range snapshot.Models {
		key := row.Provider + "\x00" + row.ID + "\x00" + row.EndpointName + "\x00" + row.ServerInstance
		rowsByKey[key] = row
		require.NotNil(t, row.QuotaRemaining)
		require.NotZero(t, row.RecentP50Latency)
	}

	out := runBuiltCLI(t, exe, workDir, env, "--work-dir", workDir, "route-status", "--policy", "air-gapped", "--model", "qwen3.5-27b", "--json")
	require.Equal(t, 0, out.exitCode, "stderr=%s stdout=%s", out.stderr, out.stdout)

	type candidate struct {
		Provider       string `json:"provider"`
		Endpoint       string `json:"endpoint"`
		ServerInstance string `json:"server_instance"`
		Model          string `json:"model"`
		Eligible       bool   `json:"eligible"`
		FilterReason   string `json:"filter_reason"`
		Winner         bool   `json:"winner"`
	}
	var parsed struct {
		SelectedEndpoint       string      `json:"selected_endpoint"`
		SelectedServerInstance string      `json:"selected_server_instance"`
		Winner                 *candidate  `json:"winner"`
		Candidates             []candidate `json:"candidates"`
	}
	require.NoError(t, json.Unmarshal([]byte(out.stdout), &parsed), "stdout=%s", out.stdout)
	var candidates []candidate
	for _, c := range parsed.Candidates {
		if c.Provider == "alpha" || c.Provider == "beta" {
			candidates = append(candidates, c)
		}
	}
	require.NotEmpty(t, candidates)

	for _, c := range candidates {
		key := c.Provider + "\x00" + c.Model + "\x00" + c.Endpoint + "\x00" + c.ServerInstance
		row, ok := rowsByKey[key]
		require.True(t, ok, "route-status candidate should match a models row: %+v", c)
		require.Equal(t, "qwen3.5-27b", row.ID)
	}

	require.NotEmpty(t, candidates)

	require.NotNil(t, parsed.Winner)
	require.Equal(t, "alpha", parsed.Winner.Provider)
	require.Equal(t, "primary", parsed.Winner.Endpoint)
	require.Equal(t, parsed.Winner.Endpoint, parsed.SelectedEndpoint)
	require.NotEmpty(t, parsed.SelectedServerInstance)
}
