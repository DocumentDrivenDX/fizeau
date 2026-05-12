package modelsnapshot

import (
	"context"
	"os/exec"
	"sort"
	"time"

	"github.com/easel/fizeau/internal/discoverycache"
	"github.com/easel/fizeau/internal/modelcatalog"
)

// Assemble builds a ModelSnapshot from configured providers, cache-backed
// discovery, runtime status cache state, and model catalog metadata.
func Assemble(ctx context.Context, cfg *Config, cat *modelcatalog.Catalog, cache *discoverycache.Cache) (ModelSnapshot, error) {
	return AssembleWithOptions(ctx, cfg, cat, cache, AssembleOptions{Refresh: RefreshBackground})
}

// AssembleWithOptions builds a ModelSnapshot with explicit cache refresh
// behavior for CLI and routing consumers.
func AssembleWithOptions(ctx context.Context, cfg *Config, cat *modelcatalog.Catalog, cache *discoverycache.Cache, opts AssembleOptions) (ModelSnapshot, error) {
	asOf := nowUTC()
	snapshot := ModelSnapshot{
		AsOf:    asOf,
		Sources: map[string]SourceMeta{},
	}
	if cfg == nil {
		return snapshot, nil
	}

	providers := enumerateProviders(cfg)
	names := make([]string, 0, len(providers))
	for name := range providers {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, providerName := range names {
		pcfg := providers[providerName]
		discovered := discoverProvider(ctx, providerName, pcfg, cache, opts)
		for source, meta := range discovered.Sources {
			snapshot.Sources[source] = meta
		}
		includeByDefault := effectiveProviderIncludeByDefault(providerName, pcfg, cat)
		billing := effectiveProviderBilling(providerName, pcfg, cat)
		status := providerStatus(cache, providerName)
		for _, discoveredModel := range discovered.Models {
			model := KnownModel{
				Provider:         providerName,
				ProviderType:     discoveredModel.ProviderType,
				Harness:          discoveredModel.Harness,
				ID:               discoveredModel.ID,
				Configured:       discoveredModel.Configured,
				EndpointName:     discoveredModel.EndpointName,
				EndpointBaseURL:  discoveredModel.EndpointBaseURL,
				ServerInstance:   discoveredModel.ServerInstance,
				Billing:          billing,
				IncludeByDefault: includeByDefault,
				DiscoveredVia:    discoveredModel.Via,
				DiscoveredAt:     discoveredModel.DiscoveredAt,
				Status:           status,
			}
			model = enrichModel(model, includeByDefault, cat)
			model = attachRuntimeSignals(model, cache)
			snapshot.Models = append(snapshot.Models, model)
		}
	}
	sort.Slice(snapshot.Models, func(i, j int) bool {
		if snapshot.Models[i].Provider != snapshot.Models[j].Provider {
			return snapshot.Models[i].Provider < snapshot.Models[j].Provider
		}
		if snapshot.Models[i].ProviderType != snapshot.Models[j].ProviderType {
			return snapshot.Models[i].ProviderType < snapshot.Models[j].ProviderType
		}
		if snapshot.Models[i].Harness != snapshot.Models[j].Harness {
			return snapshot.Models[i].Harness < snapshot.Models[j].Harness
		}
		if snapshot.Models[i].EndpointName != snapshot.Models[j].EndpointName {
			return snapshot.Models[i].EndpointName < snapshot.Models[j].EndpointName
		}
		if snapshot.Models[i].EndpointBaseURL != snapshot.Models[j].EndpointBaseURL {
			return snapshot.Models[i].EndpointBaseURL < snapshot.Models[j].EndpointBaseURL
		}
		if snapshot.Models[i].ServerInstance != snapshot.Models[j].ServerInstance {
			return snapshot.Models[i].ServerInstance < snapshot.Models[j].ServerInstance
		}
		return snapshot.Models[i].ID < snapshot.Models[j].ID
	})
	return snapshot, nil
}

func enumerateProviders(cfg *Config) map[string]ProviderConfig {
	out := make(map[string]ProviderConfig, len(cfg.Providers)+2)
	for name, pc := range cfg.Providers {
		out[name] = pc
	}
	addImplicitHarness(out, "claude-subscription", "claude", "claude")
	addImplicitHarness(out, "codex-subscription", "codex", "codex")
	return out
}

func addImplicitHarness(providers map[string]ProviderConfig, name, providerType, binary string) {
	if _, exists := providers[name]; exists {
		return
	}
	if _, err := exec.LookPath(binary); err != nil {
		return
	}
	include := true
	providers[name] = ProviderConfig{
		Type:             providerType,
		IncludeByDefault: &include,
		Billing:          string(modelcatalog.BillingModelSubscription),
	}
}

func nowUTC() time.Time {
	return time.Now().UTC()
}
