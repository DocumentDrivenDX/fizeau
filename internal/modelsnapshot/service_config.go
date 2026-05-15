package modelsnapshot

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/easel/fizeau/internal/discoverycache"
	"github.com/easel/fizeau/internal/modelcatalog"
)

// AssembleServiceConfigWithOptions assembles a snapshot from the service-owned
// snapshot config adapter and the configured cache root.
func AssembleServiceConfigWithOptions(ctx context.Context, cfg *Config, cat *modelcatalog.Catalog, cacheRoot string, opts AssembleOptions) (ModelSnapshot, error) {
	cache := &discoverycache.Cache{Root: cacheRoot}
	return AssembleWithOptions(ctx, cfg, cat, cache, opts)
}

// DefaultCacheRoot resolves the service snapshot cache root using the same
// environment override contract as the root service facade.
func DefaultCacheRoot(getenv func(string) string, userCacheDir func() (string, error)) (string, error) {
	if override := strings.TrimSpace(getenv("FIZEAU_CACHE_DIR")); override != "" {
		return override, nil
	}
	cacheDir, err := userCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cacheDir, "fizeau"), nil
}
