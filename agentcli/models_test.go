package agentcli

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"testing"
	"time"

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
		return cmdModelsList(snapshot, modelsCommandOptions{JSON: true})
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
		return cmdModelsDetail(snapshot, nil, nil, nil, modelsCommandOptions{Ref: "claude-subscription/claude-sonnet-4-20250514"})
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
