package agentcli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/easel/fizeau/internal/discoverycache"
	"github.com/easel/fizeau/internal/runtimesignals"
	"github.com/stretchr/testify/require"
)

type snapshotAutoroutingFixture struct {
	Providers []snapshotAutoroutingProviderFixture `json:"providers"`
}

type snapshotAutoroutingProviderFixture struct {
	Name                string                              `json:"name"`
	Type                string                              `json:"type"`
	BaseURL             string                              `json:"base_url"`
	ServerInstance      string                              `json:"server_instance"`
	EndpointName        string                              `json:"endpoint_name"`
	Model               string                              `json:"model"`
	IncludeByDefault    bool                                `json:"include_by_default"`
	IncludeByDefaultSet bool                                `json:"include_by_default_set"`
	Discovery           snapshotAutoroutingDiscoveryFixture `json:"discovery"`
	Runtime             *runtimesignals.Signal              `json:"runtime,omitempty"`
}

type snapshotAutoroutingDiscoveryFixture struct {
	CapturedAt time.Time `json:"captured_at"`
	Models     []string  `json:"models"`
	Stale      bool      `json:"stale,omitempty"`
}

func TestSnapshotAutoroutingCLI(t *testing.T) {
	exe := buildAgentCLI(t)
	workDir := t.TempDir()
	home := t.TempDir()
	cacheDir := t.TempDir()

	fixture := loadSnapshotAutoroutingFixture(t)
	manifestPath := snapshotAutoroutingManifestPath(t)
	writeTempConfig(t, workDir, renderSnapshotAutoroutingConfig(t, fixture, manifestPath, "http://127.0.0.1:1/v1"))

	cache := &discoverycache.Cache{Root: cacheDir}
	seedSnapshotAutoroutingCache(t, cache, fixture, "http://127.0.0.1:1/v1")

	env := testEnvWithHome(home, map[string]string{
		"PATH":             "",
		"FIZEAU_CACHE_DIR": cacheDir,
	})

	models := runBuiltCLI(t, exe, workDir, env, "--work-dir", workDir, "models", "--json", "--no-refresh")
	require.Equal(t, 0, models.exitCode, "stderr=%s stdout=%s", models.stderr, models.stdout)
	var generic map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(models.stdout), &generic), "stdout=%s", models.stdout)
	var freshness struct {
		FreshnessState string `json:"freshness_state"`
		FreshnessHint  string `json:"freshness_hint"`
	}
	require.NoError(t, json.Unmarshal([]byte(models.stdout), &freshness), "stdout=%s", models.stdout)
	require.Equal(t, "stale", freshness.FreshnessState)
	require.Contains(t, freshness.FreshnessHint, "fiz models --refresh")

	out := runBuiltCLI(t, exe, workDir, env, "--work-dir", workDir, "route-status", "--model", "gpt-5.4-mini", "--json")
	require.Equal(t, 0, out.exitCode, "stderr=%s stdout=%s", out.stderr, out.stdout)

	type candidate struct {
		Provider        string `json:"provider"`
		Endpoint        string `json:"endpoint"`
		ServerInstance  string `json:"server_instance"`
		Model           string `json:"model"`
		Eligible        bool   `json:"eligible"`
		FilterReason    string `json:"filter_reason"`
		SourceStatus    string `json:"source_status"`
		ActualCashSpend bool   `json:"actual_cash_spend"`
		Winner          bool   `json:"winner"`
	}
	var parsed struct {
		SelectedEndpoint       string      `json:"selected_endpoint"`
		SelectedServerInstance string      `json:"selected_server_instance"`
		Winner                 *candidate  `json:"winner"`
		Candidates             []candidate `json:"candidates"`
	}
	require.NoError(t, json.Unmarshal([]byte(out.stdout), &parsed), "stdout=%s", out.stdout)
	require.NotNil(t, parsed.Winner)
	require.Equal(t, "local-stale", parsed.Winner.Provider)
	require.Equal(t, "gpt-5.4-mini", parsed.Winner.Model)
	require.False(t, parsed.Winner.ActualCashSpend)
	require.Equal(t, "local-stale-1", parsed.SelectedServerInstance)

	var sawUnknown bool
	for _, c := range parsed.Candidates {
		if c.Provider == "local-stale" && c.Model == "gpt-5.4-mini" {
			sawUnknown = true
			require.Equal(t, "unknown", c.SourceStatus)
			require.True(t, c.Eligible)
		}
	}
	require.True(t, sawUnknown, "missing local-stale candidate: %s", out.stdout)
}

func loadSnapshotAutoroutingFixture(t *testing.T) snapshotAutoroutingFixture {
	t.Helper()
	path := snapshotAutoroutingFixturePath(t)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var fixture snapshotAutoroutingFixture
	require.NoError(t, json.Unmarshal(data, &fixture))
	return fixture
}

func snapshotAutoroutingFixturePath(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	require.NoError(t, err)
	return filepath.Clean(filepath.Join(wd, "..", "testdata", "snapshot-autorouting", "fixtures.json"))
}

func snapshotAutoroutingManifestPath(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	require.NoError(t, err)
	return filepath.Clean(filepath.Join(wd, "..", "testdata", "snapshot-autorouting", "models.yaml"))
}

func renderSnapshotAutoroutingConfig(t *testing.T, fixture snapshotAutoroutingFixture, manifestPath, baseURL string) string {
	t.Helper()
	var b strings.Builder
	b.WriteString("model_catalog:\n")
	b.WriteString("  manifest: ")
	b.WriteString(manifestPath)
	b.WriteString("\nproviders:\n")
	for _, p := range fixture.Providers {
		b.WriteString("  ")
		b.WriteString(p.Name)
		b.WriteString(":\n")
		b.WriteString("    type: ")
		b.WriteString(p.Type)
		b.WriteString("\n")
		b.WriteString("    base_url: ")
		b.WriteString(baseURL)
		b.WriteString("\n")
		b.WriteString("    server_instance: ")
		b.WriteString(p.ServerInstance)
		b.WriteString("\n")
		b.WriteString("    model: ")
		b.WriteString(p.Model)
		b.WriteString("\n")
		b.WriteString("    include_by_default: ")
		if p.IncludeByDefault {
			b.WriteString("true")
		} else {
			b.WriteString("false")
		}
		b.WriteString("\n")
		b.WriteString("    endpoints:\n")
		b.WriteString("      - name: ")
		if p.EndpointName != "" {
			b.WriteString(p.EndpointName)
		} else {
			b.WriteString(p.Name)
		}
		b.WriteString("\n")
		b.WriteString("        base_url: ")
		b.WriteString(baseURL)
		b.WriteString("\n")
		b.WriteString("        server_instance: ")
		b.WriteString(p.ServerInstance)
		b.WriteString("\n")
	}
	b.WriteString("default: ")
	b.WriteString(fixture.Providers[0].Name)
	b.WriteString("\n")
	return b.String()
}

func seedSnapshotAutoroutingCache(t *testing.T, cache *discoverycache.Cache, fixture snapshotAutoroutingFixture, baseURL string) {
	t.Helper()
	for _, p := range fixture.Providers {
		source := testDiscoverySourceName(p.Name, p.EndpointName, baseURL, p.ServerInstance)
		writeSnapshotDiscoveryFixture(t, cache, source, p.Discovery.CapturedAt, append([]string(nil), p.Discovery.Models...))
		if p.Discovery.Stale {
			path := filepath.Join(cache.Root, "discovery", source+".json")
			past := time.Now().Add(-2 * time.Hour)
			require.NoError(t, os.Chtimes(path, past, past))
		}
		if p.Runtime != nil {
			require.NoError(t, runtimesignals.Write(cache, *p.Runtime))
		}
	}
}
