package main

import (
	"path/filepath"
	"time"

	fizeau "github.com/DocumentDrivenDX/fizeau"
	agentConfig "github.com/DocumentDrivenDX/fizeau/internal/config"
)

// configAdapter wraps *config.Config and satisfies fizeau.ServiceConfig.
type configAdapter struct {
	cfg     *agentConfig.Config
	workDir string
}

var _ fizeau.ServiceConfig = (*configAdapter)(nil)

func (a *configAdapter) ProviderNames() []string { return a.cfg.ProviderNames() }

func (a *configAdapter) DefaultProviderName() string { return a.cfg.DefaultName() }

func (a *configAdapter) Provider(name string) (fizeau.ServiceProviderEntry, bool) {
	pc, ok := a.cfg.GetProvider(name)
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
		Type:      pc.Type,
		BaseURL:   pc.BaseURL,
		Endpoints: endpoints,
		APIKey:    pc.APIKey,
		Model:     pc.Model,
	}, true
}

func (a *configAdapter) HealthCooldown() time.Duration { return 0 }

func (a *configAdapter) WorkDir() string { return a.workDir }

func (a *configAdapter) SessionLogDir() string {
	if a.workDir == "" {
		return ""
	}
	return filepath.Join(a.workDir, agentConfig.ProjectConfigDirName(), "sessions")
}
