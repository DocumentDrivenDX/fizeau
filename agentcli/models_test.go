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
			Provider:         "studio",
			ProviderType:     "lmstudio",
			Harness:          "claude",
			ID:               "qwen3.5-27b",
			EndpointName:     "vidar",
			EndpointBaseURL:  "http://vidar:1234/v1",
			ServerInstance:   "vidar:1234",
			Billing:          "fixed",
			IncludeByDefault: true,
			QuotaRemaining:   intPtr(42),
			RecentP50Latency: 75 * time.Millisecond,
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
}

func TestModelsDetailRuntimeFields(t *testing.T) {
	snapshot := modelregistry.ModelSnapshot{
		AsOf: time.Date(2026, 5, 12, 13, 0, 0, 0, time.UTC),
		Models: []modelregistry.KnownModel{{
			Provider:         "claude-subscription",
			ProviderType:     "claude",
			Harness:          "claude",
			ID:               "claude-sonnet-4-20250514",
			EndpointName:     "claude-subscription",
			EndpointBaseURL:  "",
			ServerInstance:   "claude-subscription",
			Billing:          "subscription",
			IncludeByDefault: true,
			QuotaRemaining:   intPtr(17),
			RecentP50Latency: 120 * time.Millisecond,
			Status:           modelregistry.StatusAvailable,
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
