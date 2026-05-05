package config

import (
	"path/filepath"
	"time"

	fizeau "github.com/DocumentDrivenDX/fizeau"
)

func init() {
	// Register the config loader into the root package so that fizeau.New can
	// load configuration without importing this package directly (which would
	// create an import cycle: config → agent → config).
	fizeau.RegisterConfigLoader(func(dir string) (fizeau.ServiceConfig, error) {
		cfg, err := Load(dir)
		if err != nil {
			return nil, err
		}
		return &configServiceConfig{cfg: cfg, baseDir: dir}, nil
	})
}

// configServiceConfig wraps a loaded *Config and satisfies fizeau.ServiceConfig.
// It is the agent-internal equivalent of DDx's ServiceConfigAdapter.
type configServiceConfig struct {
	cfg     *Config
	baseDir string
}

func NewServiceConfig(cfg *Config, baseDir string) fizeau.ServiceConfig {
	return &configServiceConfig{cfg: cfg, baseDir: baseDir}
}

func (c *configServiceConfig) ProviderNames() []string {
	if c.cfg == nil {
		return nil
	}
	return c.cfg.ProviderNames()
}

func (c *configServiceConfig) DefaultProviderName() string {
	if c.cfg == nil {
		return ""
	}
	return c.cfg.DefaultName()
}

func (c *configServiceConfig) Provider(name string) (fizeau.ServiceProviderEntry, bool) {
	if c.cfg == nil {
		return fizeau.ServiceProviderEntry{}, false
	}
	pc, ok := c.cfg.Providers[name]
	if !ok {
		return fizeau.ServiceProviderEntry{}, false
	}
	endpoints := make([]fizeau.ServiceProviderEndpoint, 0, len(pc.Endpoints))
	for _, endpoint := range pc.Endpoints {
		endpoints = append(endpoints, fizeau.ServiceProviderEndpoint{
			Name:    endpoint.Name,
			BaseURL: endpoint.BaseURL,
		})
	}
	return fizeau.ServiceProviderEntry{
		Type:             pc.Type,
		BaseURL:          pc.BaseURL,
		Endpoints:        endpoints,
		APIKey:           pc.APIKey,
		Model:            pc.Model,
		DailyTokenBudget: pc.DailyTokenBudget,
	}, true
}

func (c *configServiceConfig) HealthCooldown() time.Duration {
	if c.cfg == nil || c.cfg.Routing.HealthCooldown == "" {
		return 0
	}
	d, err := time.ParseDuration(c.cfg.Routing.HealthCooldown)
	if err != nil {
		return 0
	}
	return d
}

func (c *configServiceConfig) WorkDir() string {
	return c.baseDir
}

func (c *configServiceConfig) SessionLogDir() string {
	workDir := c.WorkDir()
	if workDir == "" {
		return ""
	}
	return filepath.Join(workDir, ProjectConfigDirName(), "sessions")
}
