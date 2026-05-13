package agentcli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	agentConfig "github.com/easel/fizeau/internal/config"
	"github.com/easel/fizeau/internal/discoverycache"
	"github.com/easel/fizeau/internal/modelcatalog"
	"github.com/easel/fizeau/internal/modelregistry"
	"github.com/stretchr/testify/require"
)

func TestModelsListJSONEmitsSnapshot(t *testing.T) {
	snapshot := modelregistry.ModelSnapshot{
		AsOf: time.Date(2026, 5, 12, 13, 0, 0, 0, time.UTC),
		Models: []modelregistry.KnownModel{{
			Provider:              "studio",
			ProviderType:          "lmstudio",
			Harness:               "claude",
			ID:                    "qwen3.5-27b",
			EndpointName:          "vidar",
			EndpointBaseURL:       "http://vidar:1234/v1",
			ServerInstance:        "vidar:1234",
			Billing:               "fixed",
			IncludeByDefault:      true,
			DiscoveredVia:         modelregistry.SourceNativeAPI,
			DiscoveredAt:          time.Date(2026, 5, 12, 12, 55, 0, 0, time.UTC),
			QuotaRemaining:        intPtr(42),
			RecentP50Latency:      75 * time.Millisecond,
			ActualCashSpend:       false,
			EffectiveCost:         0.0375,
			EffectiveCostSource:   "catalog",
			SupportsTools:         true,
			DeploymentClass:       "managed_cloud_frontier",
			HealthFreshnessAt:     time.Date(2026, 5, 12, 12, 56, 0, 0, time.UTC),
			HealthFreshnessSource: "runtime",
			QuotaFreshnessAt:      time.Date(2026, 5, 12, 12, 56, 0, 0, time.UTC),
			QuotaFreshnessSource:  "runtime",
		}},
		Sources: map[string]modelregistry.SourceMeta{
			"studio": {LastRefreshedAt: time.Date(2026, 5, 12, 13, 0, 0, 0, time.UTC)},
		},
	}

	out := captureStdout(t, func() int {
		return cmdModelsList(snapshot, modelsCommandOptions{JSON: true}, summarizeModelsFreshness(snapshot))
	})

	var got modelregistry.ModelSnapshot
	require.NoError(t, json.Unmarshal([]byte(out), &got), "stdout=%s", out)
	var generic map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(out), &generic), "stdout=%s", out)
	for _, key := range []string{"models"} {
		if _, ok := generic[key]; !ok {
			t.Fatalf("missing %q in models JSON: %s", key, out)
		}
	}
	var rows []map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(generic["models"], &rows), "stdout=%s", out)
	require.NotEmpty(t, rows)
	for _, key := range []string{"actual_cash_spend", "effective_cost", "health_freshness_at", "quota_freshness_at", "model_discovery_freshness_at"} {
		if _, ok := rows[0][key]; !ok {
			t.Fatalf("missing %q in models JSON row: %s", key, out)
		}
	}
	require.Len(t, got.Models, 1)
	row := got.Models[0]
	require.Equal(t, "studio", row.Provider)
	require.Equal(t, "lmstudio", row.ProviderType)
	require.Equal(t, "claude", row.Harness)
	require.Equal(t, "vidar", row.EndpointName)
	require.Equal(t, "http://vidar:1234/v1", row.EndpointBaseURL)
	require.Equal(t, "vidar:1234", row.ServerInstance)
	require.NotNil(t, row.QuotaRemaining)
	require.Equal(t, 42, *row.QuotaRemaining)
	require.Equal(t, 75*time.Millisecond, row.RecentP50Latency)
	require.False(t, row.ActualCashSpend)
	require.Equal(t, 0.0375, row.EffectiveCost)
	require.Equal(t, "catalog", row.EffectiveCostSource)
	require.True(t, row.SupportsTools)
	require.Equal(t, "managed_cloud_frontier", row.DeploymentClass)
	require.Equal(t, modelregistry.SourceNativeAPI, row.DiscoveredVia)
	require.Equal(t, time.Date(2026, 5, 12, 12, 55, 0, 0, time.UTC), row.DiscoveredAt)
	require.Equal(t, time.Date(2026, 5, 12, 12, 56, 0, 0, time.UTC), row.HealthFreshnessAt)
	require.Equal(t, "runtime", row.HealthFreshnessSource)
	require.Equal(t, time.Date(2026, 5, 12, 12, 56, 0, 0, time.UTC), row.QuotaFreshnessAt)
	require.Equal(t, "runtime", row.QuotaFreshnessSource)
}

func TestModelsDetailRuntimeFields(t *testing.T) {
	snapshot := modelregistry.ModelSnapshot{
		AsOf: time.Date(2026, 5, 12, 13, 0, 0, 0, time.UTC),
		Models: []modelregistry.KnownModel{{
			Provider:              "claude-subscription",
			ProviderType:          "claude",
			Harness:               "claude",
			ID:                    "claude-sonnet-4-20250514",
			EndpointName:          "claude-subscription",
			EndpointBaseURL:       "",
			ServerInstance:        "claude-subscription",
			Billing:               "subscription",
			IncludeByDefault:      true,
			DiscoveredVia:         modelregistry.SourceHarnessPTY,
			DiscoveredAt:          time.Date(2026, 5, 12, 12, 55, 0, 0, time.UTC),
			QuotaRemaining:        intPtr(17),
			RecentP50Latency:      120 * time.Millisecond,
			Status:                modelregistry.StatusAvailable,
			HealthFreshnessAt:     time.Date(2026, 5, 12, 12, 58, 0, 0, time.UTC),
			HealthFreshnessSource: "runtime",
			QuotaFreshnessAt:      time.Date(2026, 5, 12, 12, 58, 0, 0, time.UTC),
			QuotaFreshnessSource:  "runtime",
			ActualCashSpend:       false,
			EffectiveCost:         0.0125,
			EffectiveCostSource:   "subscription_shadow",
			SupportsTools:         true,
			DeploymentClass:       "managed_cloud_frontier",
		}},
		Sources: map[string]modelregistry.SourceMeta{
			"claude-subscription": {LastRefreshedAt: time.Date(2026, 5, 12, 13, 0, 0, 0, time.UTC)},
		},
	}

	out := captureStdout(t, func() int {
		return cmdModelsDetail(snapshot, nil, nil, nil, modelsCommandOptions{Ref: "claude-subscription/claude-sonnet-4-20250514"}, summarizeModelsFreshness(snapshot))
	})

	require.Contains(t, out, "Identity: claude-subscription/claude-sonnet-4-20250514 harness=claude provider_type=claude endpoint_name=claude-subscription server_instance=claude-subscription")
	require.Contains(t, out, "KnownModel: {Provider:claude-subscription ProviderType:claude Harness:claude")
	require.Contains(t, out, "ActualCashSpend: false")
	require.Contains(t, out, "EffectiveCost: 0.0125 source=subscription_shadow")
	require.Contains(t, out, "HealthFreshness: at=2026-05-12T12:58:00Z source=runtime")
	require.Contains(t, out, "QuotaFreshness: at=2026-05-12T12:58:00Z source=runtime")
	require.Contains(t, out, "ModelDiscoveryFreshness: at=2026-05-12T12:55:00Z source=harness_pty")
	require.Contains(t, out, "RuntimeQuotaRemaining: 17")
	require.Contains(t, out, "RecentP50Latency: 120ms")
}

func captureStdout(t *testing.T, fn func() int) string {
	t.Helper()
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w
	defer func() {
		os.Stdout = oldStdout
	}()

	exitCode := fn()
	require.Equal(t, 0, exitCode)
	require.NoError(t, w.Close())

	var buf bytes.Buffer
	_, err = io.Copy(&buf, r)
	require.NoError(t, err)
	require.NoError(t, r.Close())
	return buf.String()
}

func intPtr(v int) *int {
	return &v
}

func TestModelsStaleHintAndNoProbes(t *testing.T) {
	workDir, cacheRoot := writeModelsFreshnessFixture(t)
	t.Setenv("FIZEAU_CACHE_DIR", cacheRoot)

	origExecutor := modelsRefreshExecutor
	origRequester := modelsBestEffortRefreshRequester
	t.Cleanup(func() {
		modelsRefreshExecutor = origExecutor
		modelsBestEffortRefreshRequester = origRequester
	})
	calls := 0
	modelsRefreshExecutor = func(context.Context, string, modelregistry.RefreshScope) (modelregistry.ModelSnapshot, *agentConfig.Config, *modelcatalog.Catalog, *discoverycache.Cache, error) {
		calls++
		return modelregistry.ModelSnapshot{}, nil, nil, nil, nil
	}
	modelsBestEffortRefreshRequester = nil

	out := captureStdout(t, func() int {
		return cmdModels(workDir, nil)
	})

	require.Equal(t, 0, calls)
	require.Contains(t, out, "Freshness: stale")
	require.Contains(t, out, "Hint: run fiz models --refresh or start/configure a DDx server freshness heartbeat")
}

func TestModelsCoordinatorRequestAtMostOnce(t *testing.T) {
	workDir, cacheRoot := writeModelsFreshnessFixture(t)
	t.Setenv("FIZEAU_CACHE_DIR", cacheRoot)

	origExecutor := modelsRefreshExecutor
	origRequester := modelsBestEffortRefreshRequester
	t.Cleanup(func() {
		modelsRefreshExecutor = origExecutor
		modelsBestEffortRefreshRequester = origRequester
	})
	modelsRefreshExecutor = func(context.Context, string, modelregistry.RefreshScope) (modelregistry.ModelSnapshot, *agentConfig.Config, *modelcatalog.Catalog, *discoverycache.Cache, error) {
		t.Fatal("blocking refresh executor should not be used on the default path")
		return modelregistry.ModelSnapshot{}, nil, nil, nil, nil
	}
	requests := 0
	modelsBestEffortRefreshRequester = func(context.Context, modelregistry.RefreshScope) {
		requests++
	}

	out := captureStdout(t, func() int {
		return cmdModels(workDir, nil)
	})

	require.Equal(t, 1, requests)
	require.Contains(t, out, "Freshness: stale")
}

func TestModelsRefreshBlocksUntilCompleted(t *testing.T) {
	origExecutor := modelsRefreshExecutor
	origRequester := modelsBestEffortRefreshRequester
	t.Cleanup(func() {
		modelsRefreshExecutor = origExecutor
		modelsBestEffortRefreshRequester = origRequester
	})
	modelsBestEffortRefreshRequester = nil

	workDir := t.TempDir()
	started := make(chan modelregistry.RefreshScope, 1)
	release := make(chan struct{})
	modelsRefreshExecutor = func(ctx context.Context, workDir string, scope modelregistry.RefreshScope) (modelregistry.ModelSnapshot, *agentConfig.Config, *modelcatalog.Catalog, *discoverycache.Cache, error) {
		started <- scope
		<-release
		return scopeRefreshSnapshot(scope), nil, nil, nil, nil
	}

	done := make(chan int, 1)
	go func() {
		done <- cmdModels(workDir, []string{"--refresh"})
	}()

	select {
	case scope := <-started:
		require.Equal(t, modelregistry.RefreshRouting, scope)
	case <-time.After(2 * time.Second):
		t.Fatal("refresh did not start")
	}

	select {
	case code := <-done:
		t.Fatalf("refresh returned before release with exit code %d", code)
	default:
	}

	close(release)
	require.Equal(t, 0, <-done)
}

func TestModelsRefreshAllRefreshesNonRoutingFields(t *testing.T) {
	origExecutor := modelsRefreshExecutor
	origRequester := modelsBestEffortRefreshRequester
	t.Cleanup(func() {
		modelsRefreshExecutor = origExecutor
		modelsBestEffortRefreshRequester = origRequester
	})
	modelsBestEffortRefreshRequester = nil
	modelsRefreshExecutor = func(ctx context.Context, workDir string, scope modelregistry.RefreshScope) (modelregistry.ModelSnapshot, *agentConfig.Config, *modelcatalog.Catalog, *discoverycache.Cache, error) {
		return scopeRefreshSnapshot(scope), nil, nil, nil, nil
	}

	workDir := t.TempDir()
	refreshOut := captureStdout(t, func() int {
		return cmdModels(workDir, []string{"--refresh"})
	})
	require.Contains(t, refreshOut, "Freshness: stale")
	require.NotContains(t, refreshOut, "Hint: run fiz models --refresh or start/configure a DDx server freshness heartbeat")

	allOut := captureStdout(t, func() int {
		return cmdModels(workDir, []string{"--refresh-all"})
	})
	require.Contains(t, allOut, "Freshness: fresh")
	require.NotContains(t, allOut, "Hint: run fiz models --refresh or start/configure a DDx server freshness heartbeat")
}

func writeModelsFreshnessFixture(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	workDir := filepath.Join(root, "work")
	cacheRoot := filepath.Join(root, "cache")
	catalogPath := filepath.Join(root, "models.yaml")

	require.NoError(t, os.MkdirAll(filepath.Join(workDir, ".fizeau"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(cacheRoot, "discovery"), 0o755))
	require.NoError(t, os.WriteFile(catalogPath, []byte(minimalModelsCatalogYAML()), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(workDir, ".fizeau", "config.yaml"), []byte(`
model_catalog:
  manifest: `+catalogPath+`
providers:
  claude-subscription:
    type: claude
    billing: subscription
`), 0o600))

	payload := `{"captured_at":"2026-05-12T12:00:00Z","models":["claude-sonnet-4-20250514"],"source":"test-fixture"}`
	sourcePath := filepath.Join(cacheRoot, "discovery", "claude-subscription.json")
	require.NoError(t, os.WriteFile(sourcePath, []byte(payload), 0o600))
	old := time.Now().Add(-48 * time.Hour)
	require.NoError(t, os.Chtimes(sourcePath, old, old))

	return workDir, cacheRoot
}

func minimalModelsCatalogYAML() string {
	return `
version: 5
catalog_version: test
policies:
  default:
    min_power: 1
    max_power: 10
providers:
  claude-subscription:
    type: claude
    include_by_default: true
    billing: subscription
models:
  claude-sonnet-4-20250514:
    family: claude
    status: active
    provider_system: anthropic
    power: 10
    cost_input_per_m: 3
    cost_output_per_m: 15
`
}

func scopeRefreshSnapshot(scope modelregistry.RefreshScope) modelregistry.ModelSnapshot {
	model := modelregistry.KnownModel{
		Provider:         "claude-subscription",
		ProviderType:     "claude",
		Harness:          "claude",
		ID:               "claude-sonnet-4-20250514",
		Configured:       true,
		EndpointName:     "claude-subscription",
		ServerInstance:   "claude-subscription",
		DiscoveredVia:    modelregistry.SourceHarnessPTY,
		DiscoveredAt:     time.Date(2026, 5, 12, 12, 0, 0, 0, time.UTC),
		Status:           modelregistry.StatusAvailable,
		ActualCashSpend:  false,
		EffectiveCost:    0.0125,
		SupportsTools:    true,
		DeploymentClass:  "managed_cloud_frontier",
		IncludeByDefault: true,
	}
	sources := map[string]modelregistry.SourceMeta{
		"claude-subscription": {LastRefreshedAt: time.Date(2026, 5, 12, 12, 0, 0, 0, time.UTC)},
	}
	if scope == modelregistry.RefreshAll {
		sources["claude-subscription:props"] = modelregistry.SourceMeta{LastRefreshedAt: time.Date(2026, 5, 12, 12, 1, 0, 0, time.UTC)}
	} else {
		sources["claude-subscription:props"] = modelregistry.SourceMeta{Stale: true, Error: "refresh_failed: props stale"}
	}
	return modelregistry.ModelSnapshot{
		AsOf:    time.Date(2026, 5, 12, 12, 1, 0, 0, time.UTC),
		Models:  []modelregistry.KnownModel{model},
		Sources: sources,
	}
}
