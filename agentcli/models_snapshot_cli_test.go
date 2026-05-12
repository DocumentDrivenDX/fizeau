package agentcli_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/easel/fizeau/internal/discoverycache"
	"github.com/easel/fizeau/internal/modelregistry"
	"github.com/stretchr/testify/require"
)

func TestModelsSnapshotRefreshModes(t *testing.T) {
	exe := buildAgentCLI(t)
	workDir := t.TempDir()
	home := t.TempDir()
	cacheDir := t.TempDir()

	server := newCountedOpenAIServer(t, http.StatusOK, "fresh-model", "ok")
	server.setModels("fresh-model")

	writeTempConfig(t, workDir, `
providers:
  studio:
    type: lmstudio
    endpoints:
      - name: alpha
        base_url: `+server.baseURL()+`
default: studio
`)

	cache := &discoverycache.Cache{Root: cacheDir}
	writeSnapshotDiscoveryFixture(t, cache, "studio-alpha", time.Date(2026, 5, 12, 15, 0, 0, 0, time.UTC), []string{"stale-model"})

	env := testEnvWithHome(home, map[string]string{
		"PATH":             "",
		"FIZEAU_CACHE_DIR": cacheDir,
	})

	stale := runBuiltCLI(t, exe, workDir, env, "--work-dir", workDir, "models", "--json", "--no-refresh")
	require.Equal(t, 0, stale.exitCode, "stderr=%s stdout=%s", stale.stderr, stale.stdout)

	var staleSnapshot modelregistry.ModelSnapshot
	require.NoError(t, json.Unmarshal([]byte(stale.stdout), &staleSnapshot), "stdout=%s", stale.stdout)
	require.Len(t, staleSnapshot.Models, 1)
	require.Equal(t, "stale-model", staleSnapshot.Models[0].ID)
	require.Equal(t, 0, server.modelsCallCount(), "no-refresh should not probe /v1/models")

	fresh := runBuiltCLI(t, exe, workDir, env, "--work-dir", workDir, "models", "--json", "--refresh")
	require.Equal(t, 0, fresh.exitCode, "stderr=%s stdout=%s", fresh.stderr, fresh.stdout)
	require.Equal(t, 1, server.modelsCallCount(), "refresh should force a live /v1/models probe")
	require.Contains(t, fresh.stdout, "fresh-model")
	require.NotContains(t, fresh.stdout, "stale-model")
}
