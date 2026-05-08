package agentcli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DocumentDrivenDX/fizeau"
	agentConfig "github.com/DocumentDrivenDX/fizeau/internal/config"
	"github.com/DocumentDrivenDX/fizeau/internal/prompt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func routeModelsServer(t *testing.T, models ...string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			http.NotFound(w, r)
			return
		}
		data := make([]map[string]string, 0, len(models))
		for _, model := range models {
			data = append(data, map[string]string{"id": model})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": data})
	}))
}

func isolateCatalogHome(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
}

func TestResolveProviderForRun_DefaultProvider(t *testing.T) {
	cfg := &agentConfig.Config{
		Providers: map[string]agentConfig.ProviderConfig{
			"local": {
				Type:    "lmstudio",
				BaseURL: "http://localhost:1234/v1",
				Model:   "configured-model",
			},
		},
		Default: "local",
	}

	selection, p, pc, err := resolveProviderForRun(cfg, "", "", "", "", agentConfig.ProviderOverrides{})
	require.NoError(t, err)
	assert.NotNil(t, p)
	assert.Equal(t, "local", selection.Route)
	assert.Equal(t, "local", selection.Provider)
	assert.Equal(t, "", selection.ResolvedModelRef)
	assert.Equal(t, "configured-model", selection.ResolvedModel)
	assert.Equal(t, "configured-model", pc.Model)
}

func TestResolvePreset(t *testing.T) {
	cfg := &agentConfig.Config{Preset: "cheap"}

	got, err := resolvePreset("smart", cfg)
	require.NoError(t, err)
	assert.Equal(t, "smart", got)

	got, err = resolvePreset("", cfg)
	require.NoError(t, err)
	assert.Equal(t, "cheap", got)

	got, err = resolvePreset("", &agentConfig.Config{})
	require.NoError(t, err)
	assert.Equal(t, "default", got)

	// Deprecated aliases are now rejected.
	for _, alias := range []string{"agent", "worker", "cursor", "claude", "codex"} {
		_, err := resolvePreset(alias, cfg)
		require.Error(t, err, "alias %q should be rejected", alias)
		assert.Contains(t, err.Error(), "unknown preset")
	}
}

func TestResolveRunReasoningNormalizesExplicitValues(t *testing.T) {
	cfg := &agentConfig.Config{}
	got, err := resolveRunReasoning(cfg, providerSelection{ReasoningDefault: fizeau.ReasoningHigh}, "x-high")
	require.NoError(t, err)
	assert.Equal(t, fizeau.ReasoningXHigh, got)

	got, err = resolveRunReasoning(cfg, providerSelection{ReasoningDefault: fizeau.ReasoningHigh}, "auto")
	require.NoError(t, err)
	assert.Equal(t, fizeau.ReasoningHigh, got)
}

func TestBuildToolsForPreset_IncludesTaskTool(t *testing.T) {
	tools := buildToolsForPreset(t.TempDir(), "default")

	var names []string
	for _, tool := range tools {
		names = append(names, tool.Name())
	}

	assert.Contains(t, names, "task")
	assert.Contains(t, names, "patch")
	assert.Contains(t, names, "find")
	assert.NotContains(t, names, "glob")
	assert.Contains(t, names, "grep")
	assert.Contains(t, names, "ls")
	assert.NotContains(t, names, "anchor_edit")
}

func TestBuildToolsForPreset_DefaultIncludesTaskTool(t *testing.T) {
	tools := buildToolsForPreset(t.TempDir(), "default")

	var names []string
	for _, tool := range tools {
		names = append(names, tool.Name())
	}

	assert.Contains(t, names, "task")
}

func TestBuildToolsForPresetWithAnchors_RegistersAnchorEditAndStoreBackedRead(t *testing.T) {
	workDir := t.TempDir()
	tools := buildToolsForPresetWithAnchors(workDir, "default", true)

	var read *fizeau.ReadTool
	var names []string
	for _, tool := range tools {
		names = append(names, tool.Name())
		if rt, ok := tool.(*fizeau.ReadTool); ok {
			read = rt
		}
	}

	require.NotNil(t, read)
	assert.NotNil(t, read.AnchorStore)
	assert.Contains(t, names, "anchor_edit")
}

func TestBuildToolsForPresetWithAnchors_DisabledKeepsLegacyReadAndNoAnchorEdit(t *testing.T) {
	tools := buildToolsForPresetWithAnchors(t.TempDir(), "default", false)

	var read *fizeau.ReadTool
	var names []string
	for _, tool := range tools {
		names = append(names, tool.Name())
		if rt, ok := tool.(*fizeau.ReadTool); ok {
			read = rt
		}
	}

	require.NotNil(t, read)
	assert.Nil(t, read.AnchorStore)
	assert.NotContains(t, names, "anchor_edit")
}

func TestBuildSystemPromptForRun_WithAnchorsAddsAnchorModeAddendum(t *testing.T) {
	workDir := t.TempDir()
	tools := buildToolsForPresetWithAnchors(workDir, "default", true)

	systemPrompt := buildSystemPromptForRun("default", tools, nil, nil, workDir, true, "")

	assert.Contains(t, systemPrompt, "# Anchor Mode")
	assert.Contains(t, systemPrompt, "File read output prefixes each line with anchor words.")
	assert.Contains(t, systemPrompt, "use anchor_edit instead of edit or write")
	assert.Contains(t, systemPrompt, "Do not mix edit/write with anchor_edit on the same file.")
	assert.Contains(t, systemPrompt, "re-read the file before using anchor_edit so you have fresh anchors")
}

func TestBuildSystemPromptForRun_WithoutAnchorsOmitsAnchorModeAddendum(t *testing.T) {
	workDir := t.TempDir()
	tools := buildToolsForPresetWithAnchors(workDir, "default", false)

	systemPrompt := buildSystemPromptForRun("default", tools, nil, []prompt.ContextFile{
		{Path: "AGENTS.md", Content: "Project rules."},
	}, workDir, false, "Extra caller instructions.")

	assert.NotContains(t, systemPrompt, "# Anchor Mode")
	assert.NotContains(t, systemPrompt, "File read output prefixes each line with anchor words.")
	assert.NotContains(t, systemPrompt, "use anchor_edit instead of edit or write")
	assert.Contains(t, systemPrompt, "Extra caller instructions.")
	assert.Contains(t, systemPrompt, "Project rules.")
}

func TestRun_AcceptsAnchorsFlag(t *testing.T) {
	var stdout, stderr strings.Builder
	code := Run(Options{
		Args:   []string{"--anchors", "--version"},
		Stdout: &stdout,
		Stderr: &stderr,
	})

	require.Equal(t, 0, code)
	assert.Contains(t, stdout.String(), "fiz ")
	assert.Empty(t, stderr.String())
}

func TestBuildServiceExecuteRequestPreservesNativeLoopSettings(t *testing.T) {
	workDir := t.TempDir()
	tools := buildToolsForPreset(workDir, "default")
	serviceReq := buildServiceExecuteRequest(serviceExecuteRequestParams{
		Prompt:                  "hi",
		SystemPrompt:            "system",
		Tools:                   tools,
		WorkDir:                 workDir,
		SelectedProvider:        "local",
		SelectedRoute:           "local",
		RequestedModel:          "test-model",
		ResolvedModel:           "test-model",
		ResolvedModelRef:        "code-smart",
		Reasoning:               fizeau.ReasoningLow,
		MaxIterations:           7,
		MaxTokens:               2048,
		ReasoningByteLimit:      4096,
		CompactionContextWindow: 128000,
		CompactionReserveTokens: 4096,
		ToolPreset:              "default",
	})

	require.Len(t, serviceReq.Tools, len(tools))
	assert.Equal(t, "default", serviceReq.ToolPreset)
	assert.Equal(t, toolNames(tools), toolNames(serviceReq.Tools))
	assert.Equal(t, workDir, serviceReq.WorkDir)
	assert.Equal(t, "fiz", serviceReq.Harness)
	assert.Equal(t, "local", serviceReq.Provider)
	assert.Equal(t, "test-model", serviceReq.Model)
	assert.True(t, serviceReq.NoStream == false)
	assert.Equal(t, 7, serviceReq.MaxIterations)
	assert.Equal(t, 2048, serviceReq.MaxTokens)
	assert.Equal(t, 4096, serviceReq.ReasoningByteLimit)
	assert.Equal(t, 128000, serviceReq.CompactionContextWindow)
	assert.Equal(t, 4096, serviceReq.CompactionReserveTokens)
}

func toolNames(tools []fizeau.Tool) []string {
	names := make([]string, len(tools))
	for i, tool := range tools {
		names[i] = tool.Name()
	}
	return names
}

func TestResolveProviderForRun_ModelRef(t *testing.T) {
	isolateCatalogHome(t)
	workDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(workDir, ".fizeau"), 0o755))
	cfg := &agentConfig.Config{
		Providers: map[string]agentConfig.ProviderConfig{
			"cloud": {
				Type:   "anthropic",
				APIKey: "test",
			},
		},
		Default: "cloud",
	}

	selection, p, pc, err := resolveProviderForRun(cfg, workDir, "", "", "code-smart", agentConfig.ProviderOverrides{})
	require.NoError(t, err)
	assert.NotNil(t, p)
	assert.Equal(t, "smart", selection.Route)
	assert.Equal(t, "cloud", selection.Provider)
	assert.Equal(t, "smart", selection.ResolvedModelRef)
	assert.Equal(t, "opus-4.7", selection.ResolvedModel)
	assert.Equal(t, "opus-4.7", pc.Model)
}

func TestResolveProviderForRun_DeprecatedModelRefRejectedByDefault(t *testing.T) {
	isolateCatalogHome(t)
	workDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(workDir, ".fizeau"), 0o755))
	cfg := &agentConfig.Config{
		Providers: map[string]agentConfig.ProviderConfig{
			"cloud": {
				Type:   "anthropic",
				APIKey: "test",
			},
		},
		Default: "cloud",
	}

	_, _, _, err := resolveProviderForRun(cfg, workDir, "", "", "claude-sonnet-3.7", agentConfig.ProviderOverrides{})
	require.Error(t, err)
}

func TestResolveProviderForRun_DeprecatedModelRefAllowed(t *testing.T) {
	isolateCatalogHome(t)
	workDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(workDir, ".fizeau"), 0o755))
	cfg := &agentConfig.Config{
		Providers: map[string]agentConfig.ProviderConfig{
			"cloud": {
				Type:   "anthropic",
				APIKey: "test",
			},
		},
		Default: "cloud",
	}

	selection, p, pc, err := resolveProviderForRun(cfg, workDir, "", "", "claude-sonnet-3.7", agentConfig.ProviderOverrides{
		AllowDeprecated: true,
	})
	require.NoError(t, err)
	assert.NotNil(t, p)
	assert.Equal(t, "claude-3-7-sonnet-20250219", selection.Route)
	assert.Equal(t, "cloud", selection.Provider)
	assert.Equal(t, "claude-3-7-sonnet-20250219", selection.ResolvedModelRef)
	assert.Equal(t, "claude-3-7-sonnet-20250219", selection.ResolvedModel)
	assert.Equal(t, "claude-3-7-sonnet-20250219", pc.Model)
}

func TestResolveProviderForRun_ModelIntentWithoutRouteUsesSmartSelection(t *testing.T) {
	isolateCatalogHome(t)
	workDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(workDir, ".fizeau"), 0o755))
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

	selection, p, pc, err := resolveProviderForRun(cfg, workDir, "", "", "code-smart", agentConfig.ProviderOverrides{
		Model: "exact-model",
	})
	require.NoError(t, err)
	assert.NotNil(t, p)
	assert.Equal(t, "exact-model", selection.Route)
	assert.Equal(t, "cloud", selection.Provider)
	assert.Equal(t, "", selection.ResolvedModelRef)
	assert.Equal(t, "exact-model", selection.ResolvedModel)
	assert.Equal(t, "exact-model", pc.Model)
}

func TestResolveProviderForRun_ExplicitProviderStillUsesExactModelPin(t *testing.T) {
	isolateCatalogHome(t)
	workDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(workDir, ".fizeau"), 0o755))
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

	selection, p, pc, err := resolveProviderForRun(cfg, workDir, "", "cloud", "code-smart", agentConfig.ProviderOverrides{
		Model: "exact-model",
	})
	require.NoError(t, err)
	assert.NotNil(t, p)
	assert.Equal(t, "cloud", selection.Route)
	assert.Equal(t, "cloud", selection.Provider)
	assert.Equal(t, "", selection.ResolvedModelRef)
	assert.Equal(t, "exact-model", selection.ResolvedModel)
	assert.Equal(t, "exact-model", pc.Model)
}

func TestResolveProviderForRun_RoutePlanByExplicitModel(t *testing.T) {
	workDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(workDir, ".fizeau"), 0o755))
	server := routeModelsServer(t, "qwen3.5-27b")
	defer server.Close()
	cfg := &agentConfig.Config{
		Providers: map[string]agentConfig.ProviderConfig{
			"bragi": {
				Type:    "lmstudio",
				BaseURL: server.URL + "/v1",
				Model:   "provider-default",
			},
		},
		Default: "bragi",
	}

	selection, p, pc, err := resolveProviderForRun(cfg, workDir, "", "", "", agentConfig.ProviderOverrides{
		Model: "qwen3.5-27b",
	})
	require.NoError(t, err)
	assert.NotNil(t, p)
	assert.Equal(t, "qwen3.5-27b", selection.Route)
	assert.Equal(t, "bragi", selection.Provider)
	assert.Equal(t, "qwen3.5-27b", selection.ResolvedModel)
	assert.Equal(t, "qwen3.5-27b", pc.Model)
}

func TestResolveProviderForRun_DefaultRoutePlanOverridesDefaultProvider(t *testing.T) {
	workDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(workDir, ".fizeau"), 0o755))
	cfg := &agentConfig.Config{
		Providers: map[string]agentConfig.ProviderConfig{
			"openrouter": {
				Type:    "lmstudio",
				BaseURL: "https://openrouter.ai/api/v1",
				Model:   "qwen3.5-27b",
			},
		},
		Routing: agentConfig.RoutingConfig{
			DefaultModel: "qwen3.5-27b",
		},
		Default: "openrouter",
	}

	selection, p, pc, err := resolveProviderForRun(cfg, workDir, "", "", "", agentConfig.ProviderOverrides{})
	require.NoError(t, err)
	assert.NotNil(t, p)
	assert.Equal(t, "qwen3.5-27b", selection.Route)
	assert.Equal(t, "openrouter", selection.Provider)
	assert.Equal(t, "qwen/qwen3.5-27b", selection.ResolvedModel)
	assert.Equal(t, "qwen/qwen3.5-27b", pc.Model)
}

func TestResolveProviderForRun_ModelRefRouteUsesCanonicalTarget(t *testing.T) {
	isolateCatalogHome(t)
	workDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(workDir, ".fizeau"), 0o755))
	cfg := &agentConfig.Config{
		Providers: map[string]agentConfig.ProviderConfig{
			"cloud": {
				Type:    "lmstudio",
				BaseURL: "https://openrouter.ai/api/v1",
			},
		},
		Default: "cloud",
	}

	selection, p, pc, err := resolveProviderForRun(cfg, workDir, "", "", "code-fast", agentConfig.ProviderOverrides{})
	require.NoError(t, err)
	assert.NotNil(t, p)
	assert.Equal(t, "standard", selection.Route)
	assert.Equal(t, "code-fast", selection.RequestedModelRef)
	assert.Equal(t, "standard", selection.ResolvedModelRef)
	assert.Equal(t, "gpt-5.4-mini", selection.ResolvedModel)
	assert.Equal(t, "gpt-5.4-mini", pc.Model)
}

func TestResolveProviderForRun_BackendRoundRobinSelectionAttribution(t *testing.T) {
	isolateCatalogHome(t)
	workDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(workDir, ".fizeau"), 0o755))
	cfg := &agentConfig.Config{
		Providers: map[string]agentConfig.ProviderConfig{
			"vidar": {
				Type:    "lmstudio",
				BaseURL: "http://vidar:1234/v1",
			},
			"bragi": {
				Type:    "lmstudio",
				BaseURL: "http://bragi:1234/v1",
			},
		},
		Backends: map[string]agentConfig.BackendPoolConfig{
			"code-pool": {
				Model:     "gpt-5.4-mini",
				Providers: []string{"vidar", "bragi"},
				Strategy:  "round-robin",
			},
		},
		DefaultBackend: "code-pool",
	}

	firstSelection, firstProvider, firstConfig, err := resolveProviderForRun(cfg, workDir, "", "", "", agentConfig.ProviderOverrides{})
	require.NoError(t, err)
	assert.NotNil(t, firstProvider)
	assert.Equal(t, "code-pool", firstSelection.Route)
	assert.Equal(t, "vidar", firstSelection.Provider)
	assert.Equal(t, "", firstSelection.ResolvedModelRef)
	assert.Equal(t, "gpt-5.4-mini", firstSelection.ResolvedModel)
	assert.Equal(t, "gpt-5.4-mini", firstConfig.Model)

	secondSelection, secondProvider, secondConfig, err := resolveProviderForRun(cfg, workDir, "", "", "", agentConfig.ProviderOverrides{})
	require.NoError(t, err)
	assert.NotNil(t, secondProvider)
	assert.Equal(t, "code-pool", secondSelection.Route)
	assert.Equal(t, "bragi", secondSelection.Provider)
	assert.Equal(t, "", secondSelection.ResolvedModelRef)
	assert.Equal(t, "gpt-5.4-mini", secondSelection.ResolvedModel)
	assert.Equal(t, "gpt-5.4-mini", secondConfig.Model)
}
