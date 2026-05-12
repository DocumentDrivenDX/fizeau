package modelregistry

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/easel/fizeau/internal/config"
	"github.com/easel/fizeau/internal/discoverycache"
	"github.com/easel/fizeau/internal/modelcatalog"
	"github.com/easel/fizeau/internal/runtimesignals"
)

func TestAssembleFixtureIncludesDiscoveredProviderModels(t *testing.T) {
	t.Setenv("PATH", "")
	cache := &discoverycache.Cache{Root: t.TempDir()}
	capturedAt := time.Date(2026, 5, 12, 10, 0, 0, 0, time.UTC)
	writeDiscoveryFixture(t, cache, "openrouter", capturedAt, []string{"gpt-5.5", "claude-opus-4.5"})
	writeDiscoveryFixture(t, cache, "sindri-llamacpp", capturedAt, []string{"llama3-70b"})
	writeDiscoveryFixture(t, cache, "vidar-ds4", capturedAt, []string{"deepseek-v4-flash", "gpt-5.4-mini"})

	cfg := &config.Config{Providers: map[string]config.ProviderConfig{
		"openrouter":      {Type: "openrouter", Billing: string(modelcatalog.BillingModelFixed)},
		"sindri-llamacpp": {Type: "sindri-llamacpp", Billing: string(modelcatalog.BillingModelFixed)},
		"vidar-ds4":       {Type: "vidar-ds4", Billing: string(modelcatalog.BillingModelFixed)},
	}}
	cat := loadTestCatalog(t)

	snapshot, err := Assemble(context.Background(), cfg, cat, cache)
	if err != nil {
		t.Fatalf("Assemble() error = %v", err)
	}

	got := map[string]bool{}
	for _, model := range snapshot.Models {
		got[model.Provider+"/"+model.ID] = true
	}
	want := []string{
		"openrouter/gpt-5.5",
		"openrouter/claude-opus-4.5",
		"sindri-llamacpp/llama3-70b",
		"vidar-ds4/deepseek-v4-flash",
		"vidar-ds4/gpt-5.4-mini",
	}
	if len(got) != len(want) {
		t.Fatalf("Assemble() returned %d models, want %d: %#v", len(got), len(want), got)
	}
	for _, key := range want {
		if !got[key] {
			t.Fatalf("Assemble() missing discovered pair %s; got %#v", key, got)
		}
	}
}

func TestAssembleSuppressesCatalogOnlyModels(t *testing.T) {
	t.Setenv("PATH", "")
	cache := &discoverycache.Cache{Root: t.TempDir()}
	writeDiscoveryFixture(t, cache, "openrouter", time.Now().UTC(), []string{"gpt-5.5"})
	cfg := &config.Config{Providers: map[string]config.ProviderConfig{
		"openrouter": {Type: "openrouter", Billing: string(modelcatalog.BillingModelFixed)},
	}}
	cat := loadTestCatalog(t)

	snapshot, err := Assemble(context.Background(), cfg, cat, cache)
	if err != nil {
		t.Fatalf("Assemble() error = %v", err)
	}
	for _, model := range snapshot.Models {
		if model.ID == "catalog-only-model" {
			t.Fatalf("catalog-only model appeared in snapshot: %#v", model)
		}
	}
	if len(snapshot.Models) != 1 {
		t.Fatalf("Assemble() returned %d models, want exactly discovered model", len(snapshot.Models))
	}
}

func TestAssembleSnapshotIncludesRuntimeQuotaAndLatency(t *testing.T) {
	t.Setenv("PATH", "")
	cache := &discoverycache.Cache{Root: t.TempDir()}
	capturedAt := time.Date(2026, 5, 12, 10, 0, 0, 0, time.UTC)
	writeDiscoveryFixture(t, cache, "openrouter", capturedAt, []string{"gpt-5.5"})
	remaining := 42
	requireRuntimeSignal(t, cache, runtimesignals.Signal{
		Provider:         "openrouter",
		Status:           runtimesignals.StatusAvailable,
		QuotaRemaining:   &remaining,
		RecentP50Latency: 75 * time.Millisecond,
		RecordedAt:       capturedAt.Add(2 * time.Minute),
	})

	cfg := &config.Config{Providers: map[string]config.ProviderConfig{
		"openrouter": {Type: "openrouter", Billing: string(modelcatalog.BillingModelFixed)},
	}}
	cat := loadTestCatalog(t)

	snapshot, err := Assemble(context.Background(), cfg, cat, cache)
	if err != nil {
		t.Fatalf("Assemble() error = %v", err)
	}
	if len(snapshot.Models) != 1 {
		t.Fatalf("Assemble() returned %d models, want 1", len(snapshot.Models))
	}
	model := snapshot.Models[0]
	if model.QuotaRemaining == nil {
		t.Fatalf("QuotaRemaining = nil, want populated runtime quota: %#v", model)
	}
	if got := *model.QuotaRemaining; got != 42 {
		t.Fatalf("QuotaRemaining = %d, want 42", got)
	}
	if model.RecentP50Latency != 75*time.Millisecond {
		t.Fatalf("RecentP50Latency = %v, want 75ms", model.RecentP50Latency)
	}
}

func writeDiscoveryFixture(t *testing.T, cache *discoverycache.Cache, source string, capturedAt time.Time, models []string) {
	t.Helper()
	payload, err := json.Marshal(discoveryPayload{
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

func loadTestCatalog(t *testing.T) *modelcatalog.Catalog {
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
  openrouter:
    type: openrouter
    include_by_default: true
  sindri-llamacpp:
    type: llama-server
    include_by_default: true
  vidar-ds4:
    type: ds4
    include_by_default: true
models:
  gpt-5.5:
    family: gpt
    status: active
    provider_system: openai
    quota_pool: openai-frontier
    power: 10
    cost_input_per_m: 1.25
    cost_output_per_m: 10.5
    context_window: 400000
    reasoning_levels: [low, medium, high]
  gpt-5.4-mini:
    family: gpt
    status: active
    provider_system: openai
    power: 6
    cost_input_per_m: 0.20
    cost_output_per_m: 0.80
    context_window: 200000
  claude-opus-4.5:
    family: claude
    status: active
    provider_system: anthropic
    power: 10
    cost_input_per_m: 3
    cost_output_per_m: 15
    context_window: 200000
  catalog-only-model:
    family: test
    status: active
    provider_system: openai
    power: 5
    exact_pin_only: true
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

func requireRuntimeSignal(t *testing.T, cache *discoverycache.Cache, sig runtimesignals.Signal) {
	t.Helper()
	if err := runtimesignals.Write(cache, sig); err != nil {
		t.Fatal(err)
	}
}
