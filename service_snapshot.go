package fizeau

import (
	"context"

	"github.com/easel/fizeau/internal/discoverycache"
	"github.com/easel/fizeau/internal/modelcatalog"
	"github.com/easel/fizeau/internal/modelsnapshot"
)

func assembleModelSnapshotFromServiceConfig(ctx context.Context, sc ServiceConfig, cat *modelcatalog.Catalog, cacheRoot string) (modelsnapshot.ModelSnapshot, error) {
	return assembleModelSnapshotFromServiceConfigWithOptions(ctx, sc, cat, cacheRoot, modelsnapshot.AssembleOptions{Refresh: modelsnapshot.RefreshBackground})
}

func assembleModelSnapshotFromServiceConfigWithOptions(ctx context.Context, sc ServiceConfig, cat *modelcatalog.Catalog, cacheRoot string, opts modelsnapshot.AssembleOptions) (modelsnapshot.ModelSnapshot, error) {
	var cfg *modelsnapshot.Config
	if sc != nil {
		cfg = serviceConfigToModelSnapshotConfig(sc)
	}
	cache := &discoverycache.Cache{Root: cacheRoot}
	return modelsnapshot.AssembleWithOptions(ctx, cfg, cat, cache, opts)
}

func serviceConfigToModelSnapshotConfig(sc ServiceConfig) *modelsnapshot.Config {
	if sc == nil {
		return nil
	}
	cfg := &modelsnapshot.Config{
		Default:   sc.DefaultProviderName(),
		Providers: make(map[string]modelsnapshot.ProviderConfig, len(sc.ProviderNames())),
	}
	for _, name := range sc.ProviderNames() {
		entry, ok := sc.Provider(name)
		if !ok {
			continue
		}
		cfg.Providers[name] = modelSnapshotProviderConfig(entry)
	}
	return cfg
}

func modelSnapshotProviderConfig(entry ServiceProviderEntry) modelsnapshot.ProviderConfig {
	out := modelsnapshot.ProviderConfig{
		Type:             entry.Type,
		BaseURL:          entry.BaseURL,
		ServerInstance:   entry.ServerInstance,
		Endpoints:        make([]modelsnapshot.ProviderEndpoint, 0, len(entry.Endpoints)),
		APIKey:           entry.APIKey,
		Headers:          cloneStringMap(entry.Headers),
		Model:            entry.Model,
		Billing:          string(entry.Billing),
		IncludeByDefault: nil,
		ContextWindow:    entry.ContextWindow,
		ConfigError:      entry.ConfigError,
		DailyTokenBudget: entry.DailyTokenBudget,
	}
	if entry.IncludeByDefaultSet {
		v := entry.IncludeByDefault
		out.IncludeByDefault = &v
		out.IncludeByDefaultSet = true
	}
	for _, endpoint := range entry.Endpoints {
		out.Endpoints = append(out.Endpoints, modelsnapshot.ProviderEndpoint{
			Name:           endpoint.Name,
			BaseURL:        endpoint.BaseURL,
			ServerInstance: endpoint.ServerInstance,
		})
	}
	return out
}

func cloneStringMap(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]string, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}
