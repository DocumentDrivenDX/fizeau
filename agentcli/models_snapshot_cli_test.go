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
	writeSnapshotDiscoveryFixture(t, cache, testDiscoverySourceName("studio", "alpha", server.baseURL(), ""), time.Date(2026, 5, 12, 15, 0, 0, 0, time.UTC), []string{"stale-model"})

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

func TestModelsSnapshotJSONIncludesEffectiveCostAndFreshness(t *testing.T) {
	exe := buildAgentCLI(t)
	workDir := t.TempDir()
	home := t.TempDir()
	cacheDir := t.TempDir()

	writeTempConfig(t, workDir, `
providers:
  codex-subscription:
    type: codex
    billing: subscription
default: codex-subscription
`)

	cache := &discoverycache.Cache{Root: cacheDir}
	capturedAt := time.Date(2026, 5, 12, 16, 0, 0, 0, time.UTC)
	writeSnapshotDiscoveryFixture(t, cache, "codex-subscription", capturedAt, []string{"gpt-5.5", "gpt-5.4-mini"})
	healthAt := capturedAt.Add(7 * time.Minute)
	remaining := 21
	require.NoError(t, runtimesignals.Write(cache, runtimesignals.Signal{
		Provider:         "codex-subscription",
		Status:           runtimesignals.StatusAvailable,
		QuotaRemaining:   &remaining,
		RecentP50Latency: 90 * time.Millisecond,
		RecordedAt:       healthAt,
	}))

	env := testEnvWithHome(home, map[string]string{
		"PATH":             "",
		"FIZEAU_CACHE_DIR": cacheDir,
	})

	res := runBuiltCLI(t, exe, workDir, env, "--work-dir", workDir, "models", "--json", "--no-refresh")
	require.Equal(t, 0, res.exitCode, "stderr=%s stdout=%s", res.stderr, res.stdout)

	var snapshot modelregistry.ModelSnapshot
	require.NoError(t, json.Unmarshal([]byte(res.stdout), &snapshot), "stdout=%s", res.stdout)
	require.Len(t, snapshot.Models, 2)
	var generic map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(res.stdout), &generic), "stdout=%s", res.stdout)
	for _, key := range []string{"models"} {
		if _, ok := generic[key]; !ok {
			t.Fatalf("missing %q in models JSON: %s", key, res.stdout)
		}
	}
	var jsonRows []map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(generic["models"], &jsonRows), "stdout=%s", res.stdout)
	require.NotEmpty(t, jsonRows)
	for _, key := range []string{"actual_cash_spend", "effective_cost", "effective_cost_source", "health_freshness_at", "quota_freshness_at", "model_discovery_freshness_at"} {
		if _, ok := jsonRows[0][key]; !ok {
			t.Fatalf("missing %q in models JSON row: %s", key, res.stdout)
		}
	}

	modelRows := map[string]modelregistry.KnownModel{}
	for _, row := range snapshot.Models {
		modelRows[row.ID] = row
	}
	subscription := modelRows["gpt-5.5"]
	require.False(t, subscription.ActualCashSpend)
	require.Equal(t, "subscription_shadow", subscription.EffectiveCostSource)
	require.Equal(t, capturedAt, subscription.DiscoveredAt)
	require.Equal(t, modelregistry.SourceHarnessPTY, subscription.DiscoveredVia)
	require.Equal(t, healthAt, subscription.HealthFreshnessAt)
	require.Equal(t, "runtime", subscription.HealthFreshnessSource)
	require.Equal(t, healthAt, subscription.QuotaFreshnessAt)
	require.Equal(t, "runtime", subscription.QuotaFreshnessSource)
	require.NotZero(t, subscription.EffectiveCost)
	require.True(t, subscription.EffectiveCost > modelRows["gpt-5.4-mini"].EffectiveCost)
}
