package modelregistry

import (
	"context"
	"os/exec"
	"sort"
	"time"

	"github.com/easel/fizeau/internal/config"
	"github.com/easel/fizeau/internal/discoverycache"
	"github.com/easel/fizeau/internal/modelcatalog"
)

// RefreshMode controls whether Assemble refreshes stale source cache data.
type RefreshMode int

const (
	// RefreshBackground preserves the default stale-while-revalidate behavior:
	// return cached data immediately and refresh stale sources in the background.
	RefreshBackground RefreshMode = iota
	// RefreshNone returns cached data only.
	RefreshNone
	// RefreshForce refreshes sources synchronously before reading them.
	RefreshForce
)

// AssembleOptions controls snapshot assembly behavior.
type AssembleOptions struct {
	Refresh RefreshMode
}

// Assemble builds a ModelSnapshot from configured providers, cache-backed
// discovery, runtime status cache state, and model catalog metadata.
func Assemble(ctx context.Context, cfg *config.Config, cat *modelcatalog.Catalog, cache *discoverycache.Cache) (ModelSnapshot, error) {
	return AssembleWithOptions(ctx, cfg, cat, cache, AssembleOptions{Refresh: RefreshBackground})
}

// AssembleWithOptions builds a ModelSnapshot with explicit cache refresh
// behavior for CLI and routing consumers.
func AssembleWithOptions(ctx context.Context, cfg *config.Config, cat *modelcatalog.Catalog, cache *discoverycache.Cache, opts AssembleOptions) (ModelSnapshot, error) {
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
		status := providerStatus(cache, providerName)
		for _, discoveredModel := range discovered.Models {
			model := KnownModel{
				Provider:      providerName,
				ID:            discoveredModel.ID,
				Configured:    discoveredModel.Configured,
				DiscoveredVia: discoveredModel.Via,
				DiscoveredAt:  discoveredModel.DiscoveredAt,
				Status:        status,
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
		return snapshot.Models[i].ID < snapshot.Models[j].ID
	})
	return snapshot, nil
}

// ActiveSources returns the cache sources that are currently configured and
// should be preserved by cache garbage collection.
func ActiveSources(cfg *config.Config) []discoverycache.Source {
	if cfg == nil {
		return nil
	}
	providers := enumerateProviders(cfg)
	names := make([]string, 0, len(providers))
	for name := range providers {
		names = append(names, name)
	}
	sort.Strings(names)

	sources := make([]discoverycache.Source, 0, len(names)*2)
	for _, providerName := range names {
		pcfg := providers[providerName]
		switch normalizeProviderType(pcfg.Type) {
		case "claude", "codex":
			sources = append(sources, discoverycache.Source{
				Tier:            "discovery",
				Name:            providerName,
				TTL:             discoveryTTLPTY,
				RefreshDeadline: discoveryRefreshDeadlinePTY,
			})
		case "openai", "openrouter", "vidar-ds4", "sindri-llamacpp", "ds4", "lmstudio", "llama-server", "omlx", "rapid-mlx", "vllm", "ollama", "minimax", "qwen", "zai":
			sources = append(sources, discoverySource(providerName, discoveryTTLForProvider(pcfg), discoveryRefreshDeadlineHTTP))
			if normalizeProviderType(pcfg.Type) == "ds4" || normalizeProviderType(pcfg.Type) == "vidar-ds4" {
				sources = append(sources, discoverySource(providerName+"-props", discoveryTTLHTTPLocal, discoveryRefreshDeadlineHTTP))
			}
		default:
			if pcfg.Model != "" {
				sources = append(sources, discoverySource(providerName, discoveryTTLHTTPRemote, discoveryRefreshDeadlineHTTP))
			}
		}
		sources = append(sources, discoverycache.Source{
			Tier:            "runtime",
			Name:            providerName,
			TTL:             5 * time.Minute,
			RefreshDeadline: 5 * time.Second,
		})
	}
	return sources
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
