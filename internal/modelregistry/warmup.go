package modelregistry

import (
	"context"

	"github.com/easel/fizeau/internal/config"
	"github.com/easel/fizeau/internal/discoverycache"
	"github.com/easel/fizeau/internal/modelcatalog"
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
	case RefreshRouting, RefreshAll:
		return AssembleWithOptions(ctx, cfg, cat, cache, AssembleOptions{Refresh: RefreshForce})
	default:
		return AssembleWithOptions(ctx, cfg, cat, cache, AssembleOptions{Refresh: RefreshForce})
	}
}
