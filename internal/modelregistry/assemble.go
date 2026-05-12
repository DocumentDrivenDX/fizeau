package modelregistry

import (
	"context"
	"os/exec"
	"sort"

	"github.com/easel/fizeau/internal/config"
	"github.com/easel/fizeau/internal/discoverycache"
	"github.com/easel/fizeau/internal/modelcatalog"
)

// Assemble builds a ModelSnapshot from configured providers, cache-backed
// discovery, runtime status cache state, and model catalog metadata.
func Assemble(ctx context.Context, cfg *config.Config, cat *modelcatalog.Catalog, cache *discoverycache.Cache) (ModelSnapshot, error) {
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
		discovered := discoverProvider(ctx, providerName, pcfg, cache)
		for source, meta := range discovered.Sources {
			snapshot.Sources[source] = meta
		}
		includeByDefault := effectiveProviderIncludeByDefault(providerName, pcfg, cat)
		status := providerStatus(cache, providerName)
		for _, discoveredModel := range discovered.Models {
			model := KnownModel{
				Provider:      providerName,
				ID:            discoveredModel.ID,
				DiscoveredVia: discoveredModel.Via,
				DiscoveredAt:  discoveredModel.DiscoveredAt,
				Status:        status,
			}
			snapshot.Models = append(snapshot.Models, enrichModel(model, includeByDefault, cat))
		}
	}
	sort.Slice(snapshot.Models, func(i, j int) bool {
		if snapshot.Models[i].Provider != snapshot.Models[j].Provider {
			return snapshot.Models[i].Provider < snapshot.Models[j].Provider
		}
		return snapshot.Models[i].ID < snapshot.Models[j].ID
	})
	return snapshot, nil
}

func enumerateProviders(cfg *config.Config) map[string]config.ProviderConfig {
	out := make(map[string]config.ProviderConfig, len(cfg.Providers)+2)
	for name, pc := range cfg.Providers {
		out[name] = pc
	}
	addImplicitHarness(out, "claude-subscription", "claude", "claude")
	addImplicitHarness(out, "codex-subscription", "codex", "codex")
	return out
}

func addImplicitHarness(providers map[string]config.ProviderConfig, name, providerType, binary string) {
	if _, exists := providers[name]; exists {
		return
	}
	if _, err := exec.LookPath(binary); err != nil {
		return
	}
	include := true
	providers[name] = config.ProviderConfig{
		Type:             providerType,
		IncludeByDefault: &include,
		Billing:          string(modelcatalog.BillingModelSubscription),
	}
}
