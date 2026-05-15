package modelregistry

import (
	"context"
	"sort"

	"github.com/easel/fizeau/internal/config"
	"github.com/easel/fizeau/internal/discoverycache"
	"github.com/easel/fizeau/internal/modelcatalog"
	"github.com/easel/fizeau/internal/runtimesignals"
)

// RefreshScope names the synchronous snapshot refresh scopes exposed to
// long-running callers.
type RefreshScope string

const (
	RefreshRouting RefreshScope = "routing"
	RefreshAll     RefreshScope = "all"
)

// Warmup is the public synchronous refresh entrypoint for callers that want
// to keep the assembled snapshot fresh from a heartbeat.
func Warmup(ctx context.Context, cfg *config.Config, cat *modelcatalog.Catalog, cache *discoverycache.Cache) (ModelSnapshot, error) {
	return RefreshModels(ctx, cfg, cat, cache, RefreshRouting)
}

// RefreshModels synchronously rebuilds the snapshot after refreshing stale
// cache-backed fields through the coordinated refresh path.
func RefreshModels(ctx context.Context, cfg *config.Config, cat *modelcatalog.Catalog, cache *discoverycache.Cache, scope RefreshScope) (ModelSnapshot, error) {
	switch scope {
	case RefreshRouting:
		return AssembleWithOptions(ctx, cfg, cat, cache, AssembleOptions{Refresh: RefreshForce})
	case RefreshAll:
		if _, err := AssembleWithOptions(ctx, cfg, cat, cache, AssembleOptions{Refresh: RefreshForce}); err != nil {
			return ModelSnapshot{}, err
		}
		refreshRuntimeSignals(ctx, cfg, cache)
		return AssembleWithOptions(ctx, cfg, cat, cache, AssembleOptions{Refresh: RefreshNone})
	default:
		return AssembleWithOptions(ctx, cfg, cat, cache, AssembleOptions{Refresh: RefreshForce})
	}
}

func refreshRuntimeSignals(ctx context.Context, cfg *config.Config, cache *discoverycache.Cache) {
	if ctx == nil || cfg == nil || cache == nil {
		return
	}
	providers := enumerateProviders(cfg)
	names := make([]string, 0, len(providers))
	for name := range providers {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, providerName := range names {
		pcfg := providers[providerName]
		_, _ = runtimesignals.Warmup(ctx, cache, providerName, runtimesignals.CollectInput{
			Type:    normalizeProviderType(pcfg.Type),
			BaseURL: firstBaseURL(pcfg),
		})
	}
}
