package fizeau

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/easel/fizeau/internal/discoverycache"
	"github.com/easel/fizeau/internal/modelcatalog"
	"github.com/easel/fizeau/internal/modelsnapshot"
)

func TestServiceConfigSnapshotParity(t *testing.T) {
	t.Setenv("PATH", "")
	cache := &discoverycache.Cache{Root: t.TempDir()}
	capturedAt := time.Date(2026, 5, 12, 15, 0, 0, 0, time.UTC)
	writeSnapshotDiscoveryFixture(t, cache, "studio-alpha", capturedAt, []string{"qwen3.5-27b"})
	writeSnapshotDiscoveryFixture(t, cache, "studio-beta", capturedAt, []string{"qwen3.5-27b"})
	writeSnapshotDiscoveryFixture(t, cache, "claude-subscription", capturedAt, []string{"claude-sonnet-4-20250514"})

	sc := &snapshotServiceConfig{
		defaultName: "studio",
		providers: map[string]ServiceProviderEntry{
			"studio": {
				Type: "lmstudio",
				Endpoints: []ServiceProviderEndpoint{
					{Name: "alpha", BaseURL: "http://alpha.example/v1", ServerInstance: "alpha"},
					{Name: "beta", BaseURL: "http://beta.example/v1", ServerInstance: "beta"},
				},
			},
			"claude-subscription": {
				Type:    "claude",
				Billing: modelcatalog.BillingModelSubscription,
			},
		},
		names: []string{"studio", "claude-subscription"},
	}
	cat := loadSnapshotTestCatalog(t)

	got, err := assembleModelSnapshotFromServiceConfigWithOptions(context.Background(), sc, cat, cache.Root, modelsnapshot.AssembleOptions{Refresh: modelsnapshot.RefreshNone})
	if err != nil {
		t.Fatalf("assembleModelSnapshotFromServiceConfigWithOptions: %v", err)
	}
	wantCfg := serviceConfigToModelSnapshotConfig(sc)
	want, err := modelsnapshot.AssembleWithOptions(context.Background(), wantCfg, cat, cache, modelsnapshot.AssembleOptions{Refresh: modelsnapshot.RefreshNone})
	if err != nil {
		t.Fatalf("modelsnapshot.AssembleWithOptions: %v", err)
	}

	assertSnapshotRowsEqual(t, got.Models, want.Models)
}

func TestServiceConfigSnapshotRedactsSecrets(t *testing.T) {
	cache := &discoverycache.Cache{Root: t.TempDir()}
	sc := &snapshotServiceConfig{
		defaultName: "openrouter",
		providers: map[string]ServiceProviderEntry{
			"openrouter": {
				Type:    "openrouter",
				BaseURL: "http://127.0.0.1:1/v1",
				APIKey:  "super-secret-key",
				Headers: map[string]string{
					"Authorization": "Bearer super-secret-key",
				},
			},
		},
		names: []string{"openrouter"},
	}
	cat := loadSnapshotTestCatalog(t)

	snapshot, err := assembleModelSnapshotFromServiceConfigWithOptions(context.Background(), sc, cat, cache.Root, modelsnapshot.AssembleOptions{Refresh: modelsnapshot.RefreshForce})
	if err != nil {
		t.Fatalf("assembleModelSnapshotFromServiceConfigWithOptions: %v", err)
	}
	data, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	if strings.Contains(string(data), "super-secret-key") {
		t.Fatalf("snapshot JSON leaked secret material: %s", data)
	}
	for source, meta := range snapshot.Sources {
		if strings.Contains(meta.Error, "super-secret-key") {
			t.Fatalf("source %s error leaked secret material: %q", source, meta.Error)
		}
	}
}

type snapshotServiceConfig struct {
	defaultName string
	names       []string
	providers   map[string]ServiceProviderEntry
}

func (s *snapshotServiceConfig) ProviderNames() []string {
	return append([]string(nil), s.names...)
}

func (s *snapshotServiceConfig) DefaultProviderName() string {
	return s.defaultName
}

func (s *snapshotServiceConfig) Provider(name string) (ServiceProviderEntry, bool) {
	entry, ok := s.providers[name]
	return entry, ok
}

func (s *snapshotServiceConfig) HealthCooldown() time.Duration { return 0 }
func (s *snapshotServiceConfig) WorkDir() string               { return "" }
func (s *snapshotServiceConfig) SessionLogDir() string         { return "" }

func writeSnapshotDiscoveryFixture(t *testing.T, cache *discoverycache.Cache, source string, capturedAt time.Time, models []string) {
	t.Helper()
	payload, err := json.Marshal(struct {
		CapturedAt time.Time `json:"captured_at"`
		Models     []string  `json:"models,omitempty"`
		Source     string    `json:"source,omitempty"`
	}{
		CapturedAt: capturedAt,
		Models:     models,
		Source:     "test-fixture",
	})
	if err != nil {
		t.Fatal(err)
	}
	src := discoverycache.Source{
		Tier:            "discovery",
		Name:            source,
		TTL:             time.Hour,
		RefreshDeadline: time.Second,
	}
	if err := cache.Refresh(src, func(context.Context) ([]byte, error) { return payload, nil }); err != nil {
		t.Fatal(err)
	}
}

func loadSnapshotTestCatalog(t *testing.T) *modelcatalog.Catalog {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "models.yaml")
	data := []byte(`
version: 5
catalog_version: test
policies:
  default:
    min_power: 1
    max_power: 10
providers:
  studio:
    type: lmstudio
    include_by_default: true
  claude-subscription:
    type: claude
    include_by_default: true
models:
  qwen3.5-27b:
    family: qwen
    status: active
    provider_system: openai
    power: 6
    context_window: 32768
  claude-sonnet-4-20250514:
    family: claude
    status: active
    provider_system: anthropic
    power: 8
    context_window: 200000
`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	cat, err := modelcatalog.Load(modelcatalog.LoadOptions{ManifestPath: path, RequireExternal: true})
	if err != nil {
		t.Fatal(err)
	}
	return cat
}

func assertSnapshotRowsEqual(t *testing.T, got, want []modelsnapshot.KnownModel) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("row count = %d, want %d", len(got), len(want))
	}
	index := func(rows []modelsnapshot.KnownModel) map[string]modelsnapshot.KnownModel {
		out := make(map[string]modelsnapshot.KnownModel, len(rows))
		for _, row := range rows {
			key := row.Provider + "|" + row.ID + "|" + row.EndpointName + "|" + row.EndpointBaseURL + "|" + row.ServerInstance
			out[key] = row
		}
		return out
	}
	gotRows := index(got)
	wantRows := index(want)
	if len(gotRows) != len(wantRows) {
		t.Fatalf("row key count = %d, want %d", len(gotRows), len(wantRows))
	}
	for key, wantRow := range wantRows {
		gotRow, ok := gotRows[key]
		if !ok {
			t.Fatalf("missing row %s; got %#v", key, gotRows)
		}
		if gotRow.ProviderType != wantRow.ProviderType || gotRow.Harness != wantRow.Harness || gotRow.Billing != wantRow.Billing {
			t.Fatalf("row %s mismatch:\n got %#v\nwant %#v", key, gotRow, wantRow)
		}
	}
}
