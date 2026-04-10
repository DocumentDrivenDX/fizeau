package main

import (
	"os"
	"path/filepath"
	"testing"

	agentConfig "github.com/DocumentDrivenDX/agent/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveProviderForRun_DefaultProvider(t *testing.T) {
	cfg := &agentConfig.Config{
		Providers: map[string]agentConfig.ProviderConfig{
			"local": {
				Type:    "openai-compat",
				BaseURL: "http://localhost:1234/v1",
				Model:   "configured-model",
			},
		},
		Default: "local",
	}

	selection, p, pc, err := resolveProviderForRun(cfg, "", "", "", agentConfig.ProviderOverrides{})
	require.NoError(t, err)
	assert.NotNil(t, p)
	assert.Equal(t, "local", selection.Route)
	assert.Equal(t, "local", selection.Provider)
	assert.Equal(t, "", selection.ResolvedModelRef)
	assert.Equal(t, "configured-model", selection.ResolvedModel)
	assert.Equal(t, "configured-model", pc.Model)
}

func TestResolveProviderForRun_ModelRef(t *testing.T) {
	cfg := &agentConfig.Config{
		Providers: map[string]agentConfig.ProviderConfig{
			"cloud": {
				Type:   "anthropic",
				APIKey: "test",
			},
		},
		Default: "cloud",
	}

	selection, p, pc, err := resolveProviderForRun(cfg, "", "", "", agentConfig.ProviderOverrides{
		ModelRef: "code-smart",
	})
	require.NoError(t, err)
	assert.NotNil(t, p)
	assert.Equal(t, "cloud", selection.Route)
	assert.Equal(t, "cloud", selection.Provider)
	assert.Equal(t, "claude-sonnet-4", selection.ResolvedModelRef)
	assert.Equal(t, "claude-sonnet-4-20250514", selection.ResolvedModel)
	assert.Equal(t, "claude-sonnet-4-20250514", pc.Model)
}

func TestResolveProviderForRun_DeprecatedModelRefRejectedByDefault(t *testing.T) {
	cfg := &agentConfig.Config{
		Providers: map[string]agentConfig.ProviderConfig{
			"cloud": {
				Type:   "anthropic",
				APIKey: "test",
			},
		},
		Default: "cloud",
	}

	_, _, _, err := resolveProviderForRun(cfg, "", "", "", agentConfig.ProviderOverrides{
		ModelRef: "claude-sonnet-3.7",
	})
	require.Error(t, err)
}

func TestResolveProviderForRun_DeprecatedModelRefAllowed(t *testing.T) {
	cfg := &agentConfig.Config{
		Providers: map[string]agentConfig.ProviderConfig{
			"cloud": {
				Type:   "anthropic",
				APIKey: "test",
			},
		},
		Default: "cloud",
	}

	selection, p, pc, err := resolveProviderForRun(cfg, "", "", "", agentConfig.ProviderOverrides{
		ModelRef:        "claude-sonnet-3.7",
		AllowDeprecated: true,
	})
	require.NoError(t, err)
	assert.NotNil(t, p)
	assert.Equal(t, "claude-sonnet-3.7", selection.ResolvedModelRef)
	assert.Equal(t, "claude-3-7-sonnet-20250219", selection.ResolvedModel)
	assert.Equal(t, "claude-3-7-sonnet-20250219", pc.Model)
}

func TestResolveProviderForRun_ExplicitModelWins(t *testing.T) {
	cfg := &agentConfig.Config{
		Providers: map[string]agentConfig.ProviderConfig{
			"cloud": {
				Type:   "anthropic",
				APIKey: "test",
				Model:  "configured-model",
			},
		},
		Default: "cloud",
	}

	selection, p, pc, err := resolveProviderForRun(cfg, "", "", "", agentConfig.ProviderOverrides{
		Model:    "exact-model",
		ModelRef: "code-smart",
	})
	require.NoError(t, err)
	assert.NotNil(t, p)
	assert.Equal(t, "", selection.ResolvedModelRef)
	assert.Equal(t, "exact-model", selection.ResolvedModel)
	assert.Equal(t, "exact-model", pc.Model)
}

func TestResolveProviderForRun_ModelRouteByExplicitModel(t *testing.T) {
	workDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(workDir, ".agent"), 0o755))
	cfg := &agentConfig.Config{
		Providers: map[string]agentConfig.ProviderConfig{
			"bragi": {
				Type:    "openai-compat",
				BaseURL: "http://bragi:1234/v1",
				Model:   "provider-default",
			},
		},
		ModelRoutes: map[string]agentConfig.ModelRouteConfig{
			"qwen3.5-27b": {
				Strategy: "ordered-failover",
				Candidates: []agentConfig.ModelRouteCandidateConfig{
					{Provider: "bragi", Model: "qwen3.5-27b"},
				},
			},
		},
		Default: "bragi",
	}

	selection, p, pc, err := resolveProviderForRun(cfg, workDir, "", "", agentConfig.ProviderOverrides{
		Model: "qwen3.5-27b",
	})
	require.NoError(t, err)
	assert.NotNil(t, p)
	assert.Equal(t, "qwen3.5-27b", selection.Route)
	assert.Equal(t, "bragi", selection.Provider)
	assert.Equal(t, "qwen3.5-27b", selection.ResolvedModel)
	assert.Equal(t, "qwen3.5-27b", pc.Model)
}

func TestResolveProviderForRun_DefaultModelRouteOverridesDefaultProvider(t *testing.T) {
	workDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(workDir, ".agent"), 0o755))
	cfg := &agentConfig.Config{
		Providers: map[string]agentConfig.ProviderConfig{
			"vidar": {
				Type:    "openai-compat",
				BaseURL: "http://vidar:1234/v1",
				Model:   "provider-default",
			},
			"openrouter": {
				Type:    "openai-compat",
				BaseURL: "https://openrouter.ai/api/v1",
				Model:   "provider-fallback",
			},
		},
		Routing: agentConfig.RoutingConfig{
			DefaultModel: "qwen3.5-27b",
		},
		ModelRoutes: map[string]agentConfig.ModelRouteConfig{
			"qwen3.5-27b": {
				Strategy: "ordered-failover",
				Candidates: []agentConfig.ModelRouteCandidateConfig{
					{Provider: "openrouter", Model: "qwen/qwen3.5-27b"},
				},
			},
		},
		Default: "vidar",
	}

	selection, p, pc, err := resolveProviderForRun(cfg, workDir, "", "", agentConfig.ProviderOverrides{})
	require.NoError(t, err)
	assert.NotNil(t, p)
	assert.Equal(t, "qwen3.5-27b", selection.Route)
	assert.Equal(t, "openrouter", selection.Provider)
	assert.Equal(t, "qwen/qwen3.5-27b", selection.ResolvedModel)
	assert.Equal(t, "qwen/qwen3.5-27b", pc.Model)
}

func TestResolveProviderForRun_ModelRefRouteUsesCanonicalTarget(t *testing.T) {
	workDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(workDir, ".agent"), 0o755))
	cfg := &agentConfig.Config{
		Providers: map[string]agentConfig.ProviderConfig{
			"cloud": {
				Type:    "openai-compat",
				BaseURL: "https://openrouter.ai/api/v1",
			},
		},
		ModelRoutes: map[string]agentConfig.ModelRouteConfig{
			"qwen3-coder-next": {
				Strategy: "ordered-failover",
				Candidates: []agentConfig.ModelRouteCandidateConfig{
					{Provider: "cloud", Model: "qwen/qwen3-coder-next"},
				},
			},
		},
		Default: "cloud",
	}

	selection, p, pc, err := resolveProviderForRun(cfg, workDir, "", "", agentConfig.ProviderOverrides{
		ModelRef: "code-fast",
	})
	require.NoError(t, err)
	assert.NotNil(t, p)
	assert.Equal(t, "qwen3-coder-next", selection.Route)
	assert.Equal(t, "code-fast", selection.RequestedModelRef)
	assert.Equal(t, "qwen3-coder-next", selection.ResolvedModelRef)
	assert.Equal(t, "qwen/qwen3-coder-next", selection.ResolvedModel)
	assert.Equal(t, "qwen/qwen3-coder-next", pc.Model)
}

func TestResolveProviderForRun_BackendRoundRobinSelectionAttribution(t *testing.T) {
	workDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(workDir, ".agent"), 0o755))
	cfg := &agentConfig.Config{
		Providers: map[string]agentConfig.ProviderConfig{
			"vidar": {
				Type:    "openai-compat",
				BaseURL: "http://vidar:1234/v1",
			},
			"bragi": {
				Type:    "openai-compat",
				BaseURL: "http://bragi:1234/v1",
			},
		},
		Backends: map[string]agentConfig.BackendPoolConfig{
			"code-pool": {
				ModelRef:  "code-fast",
				Providers: []string{"vidar", "bragi"},
				Strategy:  "round-robin",
			},
		},
		DefaultBackend: "code-pool",
	}

	firstSelection, firstProvider, firstConfig, err := resolveProviderForRun(cfg, workDir, "", "", agentConfig.ProviderOverrides{})
	require.NoError(t, err)
	assert.NotNil(t, firstProvider)
	assert.Equal(t, "code-pool", firstSelection.Route)
	assert.Equal(t, "vidar", firstSelection.Provider)
	assert.Equal(t, "qwen3-coder-next", firstSelection.ResolvedModelRef)
	assert.Equal(t, "qwen/qwen3-coder-next", firstSelection.ResolvedModel)
	assert.Equal(t, "qwen/qwen3-coder-next", firstConfig.Model)

	secondSelection, secondProvider, secondConfig, err := resolveProviderForRun(cfg, workDir, "", "", agentConfig.ProviderOverrides{})
	require.NoError(t, err)
	assert.NotNil(t, secondProvider)
	assert.Equal(t, "code-pool", secondSelection.Route)
	assert.Equal(t, "bragi", secondSelection.Provider)
	assert.Equal(t, "qwen3-coder-next", secondSelection.ResolvedModelRef)
	assert.Equal(t, "qwen/qwen3-coder-next", secondSelection.ResolvedModel)
	assert.Equal(t, "qwen/qwen3-coder-next", secondConfig.Model)
}
