package modelcatalog

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEmbeddedManifestAutomaticTargetCandidatesHavePower(t *testing.T) {
	catalog, err := Default()
	require.NoError(t, err)

	for _, target := range []string{"code-high", "code-medium", "code-economy"} {
		t.Run(target, func(t *testing.T) {
			candidates := catalog.AllModelsInTier(target)
			require.NotEmpty(t, candidates)

			for _, candidate := range candidates {
				entry := candidate.Entry
				assert.True(t, entry.AutoRoutable(), "%s target candidate %s must be eligible for automatic routing", target, candidate.ID)
				assert.GreaterOrEqual(t, entry.Power, 1, "%s target candidate %s must have power", target, candidate.ID)
				assert.LessOrEqual(t, entry.Power, 10, "%s target candidate %s must stay within the catalog power scale", target, candidate.ID)
				assert.NotEmpty(t, entry.DeploymentClass, "%s target candidate %s must declare deployment class", target, candidate.ID)
				assert.NotEmpty(t, entry.PowerProvenance.Method, "%s target candidate %s must explain power assignment", target, candidate.ID)
			}
		})
	}
}

func TestEmbeddedManifestOlderSameFamilyModelsAreExactPinOnly(t *testing.T) {
	catalog, err := Default()
	require.NoError(t, err)

	for _, id := range []string{
		"gpt-5.4",
		"qwen3.5-7b",
		"claude-sonnet-4-20250514",
		"claude-3-7-sonnet-20250219",
	} {
		entry, ok := catalog.LookupModel(id)
		require.True(t, ok, "model %s must remain inspectable for exact pins", id)
		assert.True(t, entry.ExactPinOnly, "model %s must not be used by automatic routing", id)
		assert.False(t, entry.AutoRoutable(), "model %s must require an exact pin", id)
	}
}

func TestEmbeddedManifestStarterInventorySurfaces(t *testing.T) {
	catalog, err := Default()
	require.NoError(t, err)

	models := catalog.AllModels()
	require.NotEmpty(t, models)

	requiredSurfaces := map[string]bool{
		"agent.openai":    false,
		"agent.anthropic": false,
		"claude-code":     false,
		"codex":           false,
		"gemini":          false,
	}
	hasOpenRouter := false
	hasLocalQwen := false

	for _, entry := range models {
		if entry.OpenRouterID != "" {
			hasOpenRouter = true
		}
		if entry.DeploymentClass == deploymentClassLocalFree && entry.Family == "qwen" {
			hasLocalQwen = true
		}
		for surface := range requiredSurfaces {
			if _, ok := entry.Surfaces[surface]; ok {
				requiredSurfaces[surface] = true
			}
		}
	}

	for surface, found := range requiredSurfaces {
		assert.True(t, found, "embedded catalog must include %s inventory", surface)
	}
	assert.True(t, hasOpenRouter, "embedded catalog must include at least one OpenRouter-backed model")
	assert.True(t, hasLocalQwen, "embedded catalog must include local OpenAI-compatible Qwen inventory")
}

func TestEmbeddedManifestDeploymentClassCapsLocalPowerBelowFrontier(t *testing.T) {
	catalog, err := Default()
	require.NoError(t, err)

	gpt, ok := catalog.LookupModel("gpt-5.5")
	require.True(t, ok)
	opus, ok := catalog.LookupModel("opus-4.7")
	require.True(t, ok)
	qwen, ok := catalog.LookupModel("qwen3.5-27b")
	require.True(t, ok)
	lucebox, ok := catalog.LookupModel("lucebox-dflash")
	require.True(t, ok)

	assert.Equal(t, deploymentClassManagedCloudFrontier, gpt.DeploymentClass)
	assert.Equal(t, deploymentClassManagedCloudFrontier, opus.DeploymentClass)
	assert.Equal(t, deploymentClassLocalFree, qwen.DeploymentClass)
	assert.Equal(t, deploymentClassLocalFree, lucebox.DeploymentClass)
	assert.Greater(t, gpt.Power, qwen.Power)
	assert.Greater(t, opus.Power, lucebox.Power)
	assert.LessOrEqual(t, qwen.Power, 6)
	assert.LessOrEqual(t, lucebox.Power, 6)
}
