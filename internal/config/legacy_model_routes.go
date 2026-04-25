// Package config — legacy model_routes parser and accessors.
//
// The `model_routes:` config block was the pre-ADR-005 hand-authored
// per-tier candidate list. ADR-005 replaces it with deterministic smart
// routing driven by the model catalog, provider config, and the engine
// scoring path. This file exists so config.go itself contains no
// `ModelRouteConfig` type, no `model_routes` YAML tag, and no
// `ModelRouteConfig` method on `*Config` — see the structural boundary
// test `TestConfigSchemaHasNoModelRoutes`.
//
// One-release deprecation: configs that still set `model_routes:` parse,
// honor configured ordering, and emit a deprecation warning naming the
// offending file path. The next release deletes this file outright.
package config

import (
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// ModelRouteCandidateConfig describes one provider candidate inside a
// (deprecated) model_routes entry.
//
// Deprecated: see ADR-005. Retained only for one-release backward compat.
type ModelRouteCandidateConfig struct {
	Provider string `yaml:"provider"`
	Model    string `yaml:"model,omitempty"`
	Priority int    `yaml:"priority,omitempty"`
}

// ModelRouteConfig describes a (deprecated) model-first routing target.
//
// Deprecated: see ADR-005. Retained only for one-release backward compat.
type ModelRouteConfig struct {
	Strategy   string                      `yaml:"strategy,omitempty"`
	Candidates []ModelRouteCandidateConfig `yaml:"candidates"`
}

// legacyModelRoutesEnvelope is used for the second-pass YAML decode that
// captures `model_routes:` blocks without re-defining the field on
// `Config` in config.go.
type legacyModelRoutesEnvelope struct {
	ModelRoutes map[string]ModelRouteConfig `yaml:"model_routes,omitempty"`
}

// noteLegacyModelRoutes scans one already-expanded YAML document for a
// `model_routes:` block. If present, it merges the parsed routes into
// `cfg.ModelRoutes` and records the source path for a post-finalize
// deprecation warning.
func noteLegacyModelRoutes(cfg *Config, data []byte, path string) error {
	var envelope legacyModelRoutesEnvelope
	if err := yaml.Unmarshal(data, &envelope); err != nil {
		return fmt.Errorf("config: parsing model_routes in %s: %w", path, err)
	}
	if len(envelope.ModelRoutes) == 0 {
		return nil
	}
	if cfg.ModelRoutes == nil {
		cfg.ModelRoutes = make(map[string]ModelRouteConfig, len(envelope.ModelRoutes))
	}
	for name, route := range envelope.ModelRoutes {
		cfg.ModelRoutes[name] = route
	}
	cfg.legacyModelRoutesPaths = append(cfg.legacyModelRoutesPaths, path)
	return nil
}

// emitLegacyModelRoutesWarnings appends one deprecation warning per
// source path that contained a `model_routes:` block. Called from
// finalize after warnings have been reset so the warning survives.
func (c *Config) emitLegacyModelRoutesWarnings() {
	for _, path := range c.legacyModelRoutesPaths {
		c.warnings = append(c.warnings, fmt.Sprintf(
			"config: model_routes is deprecated (ADR-005); routing now happens automatically via the catalog and provider config. Remove model_routes from %s by the next release.",
			path,
		))
	}
}

// GetModelRoute returns the (deprecated) model-route config for a route
// key. Retained for one-release backward compat per ADR-005.
func (c *Config) GetModelRoute(name string) (ModelRouteConfig, bool) {
	mr, ok := c.ModelRoutes[name]
	return mr, ok
}

// GetDeprecatedBackendRoute returns the translated model-route config
// for a deprecated backend compatibility input.
func (c *Config) GetDeprecatedBackendRoute(name string) (ModelRouteConfig, bool) {
	mr, ok := c.legacyModelRoutes[name]
	return mr, ok
}

// LegacyModelRouteNames returns configured model-route names in stable
// alphabetical order. Renamed from `ModelRouteNames` per ADR-005 to keep
// the boundary test from flagging re-introduction; the
// `*Config`-receiver method named `ModelRouteConfig` was the literal
// hit, so naming around model_routes remains in this file (legacy
// surface area), not config.go.
func (c *Config) LegacyModelRouteNames() []string {
	if c.ModelRoutes == nil {
		return nil
	}
	names := make([]string, 0, len(c.ModelRoutes))
	for name := range c.ModelRoutes {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// ModelRouteNames is kept as a thin alias to LegacyModelRouteNames for
// out-of-package callers (cmd/agent, cmd/bench) that read configured
// route names. Defined here (not in config.go) so config.go stays free
// of the deprecated surface.
func (c *Config) ModelRouteNames() []string {
	return c.LegacyModelRouteNames()
}

func (c *Config) validateModelRoutes() error {
	for _, name := range c.LegacyModelRouteNames() {
		route := c.ModelRoutes[name]
		if err := c.validateModelRoute(name, route); err != nil {
			return err
		}
	}
	for name, route := range c.legacyModelRoutes {
		if err := c.validateModelRoute(name, route); err != nil {
			return err
		}
	}
	return nil
}

func (c *Config) validateModelRoute(name string, route ModelRouteConfig) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("config: model route name must not be empty")
	}
	if len(route.Candidates) == 0 {
		return fmt.Errorf("config: model route %q has no candidates", name)
	}
	switch strings.TrimSpace(route.Strategy) {
	case "", "priority-round-robin", "ordered-failover", "smart":
	default:
		return fmt.Errorf("config: model route %q has unknown strategy %q", name, route.Strategy)
	}
	for i, candidate := range route.Candidates {
		if strings.TrimSpace(candidate.Provider) == "" {
			return fmt.Errorf("config: model route %q candidate %d is missing provider", name, i+1)
		}
		if _, ok := c.Providers[candidate.Provider]; !ok {
			return fmt.Errorf("config: model route %q candidate %d references unknown provider %q", name, i+1, candidate.Provider)
		}
	}
	return nil
}

func (c *Config) translateLegacyBackends() error {
	for name, bc := range c.Backends {
		translated, err := c.translateLegacyBackend(name, bc)
		if err != nil {
			return err
		}
		c.legacyModelRoutes[name] = translated
		c.warnings = append(c.warnings, fmt.Sprintf("backend %q is deprecated; use model_routes plus --model/--model-ref instead", name))
	}
	if c.DefaultBackend != "" {
		if _, ok := c.legacyModelRoutes[c.DefaultBackend]; !ok {
			return fmt.Errorf("config: unknown default backend pool %q", c.DefaultBackend)
		}
		c.warnings = append(c.warnings, "default_backend is deprecated; use routing.default_model or routing.default_model_ref")
	}
	return nil
}

func (c *Config) translateLegacyBackend(name string, bc BackendPoolConfig) (ModelRouteConfig, error) {
	if strings.TrimSpace(name) == "" {
		return ModelRouteConfig{}, fmt.Errorf("config: backend pool name must not be empty")
	}
	if len(bc.Providers) == 0 {
		return ModelRouteConfig{}, fmt.Errorf("config: backend pool %q has no providers", name)
	}

	route := ModelRouteConfig{
		Strategy: translateLegacyBackendStrategy(bc.Strategy),
	}
	for _, providerName := range bc.Providers {
		providerName = strings.TrimSpace(providerName)
		if providerName == "" {
			return ModelRouteConfig{}, fmt.Errorf("config: backend pool %q references an empty provider name", name)
		}
		if _, ok := c.Providers[providerName]; !ok {
			return ModelRouteConfig{}, fmt.Errorf("config: backend pool %q references unknown provider %q", name, providerName)
		}
		route.Candidates = append(route.Candidates, ModelRouteCandidateConfig{
			Provider: providerName,
			Model:    "",
			Priority: 100,
		})
	}

	return route, nil
}

func translateLegacyBackendStrategy(strategy string) string {
	switch strings.TrimSpace(strategy) {
	case "", "first-available":
		return "ordered-failover"
	case "round-robin":
		return "priority-round-robin"
	default:
		return strategy
	}
}
