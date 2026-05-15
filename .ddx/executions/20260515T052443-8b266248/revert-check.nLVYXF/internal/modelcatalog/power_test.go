package modelcatalog

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBootstrapModelPowers_CostRecencyRanksNewestMostExpensiveSameClass(t *testing.T) {
	models := BootstrapModelPowers(map[string]ModelEntry{
		"current": {
			Family:          "frontier",
			Status:          statusActive,
			DeploymentClass: deploymentClassManagedCloudFrontier,
			CostInputPerM:   15,
			CostOutputPerM:  75,
			PowerProvenance: PowerProvenance{Method: "cost_recency", Recency: "2026-04-01"},
		},
		"old": {
			Family:          "frontier",
			Status:          statusActive,
			DeploymentClass: deploymentClassManagedCloudFrontier,
			CostInputPerM:   3,
			CostOutputPerM:  15,
			PowerProvenance: PowerProvenance{Method: "cost_recency", Recency: "2025-10-01"},
		},
	})

	assert.True(t, models["current"].AutoRoutable())
	assert.Equal(t, 9, models["current"].Power)
	assert.False(t, models["old"].AutoRoutable())
	assert.True(t, models["old"].ExactPinOnly)
}

func TestBootstrapModelPowers_DeploymentClassKeepsLocalBelowManagedCloud(t *testing.T) {
	models := BootstrapModelPowers(map[string]ModelEntry{
		"managed": {
			Family:           "tie",
			Status:           statusActive,
			DeploymentClass:  deploymentClassManagedCloudFrontier,
			SWEBenchVerified: 80,
			PowerProvenance: PowerProvenance{
				Method:     "benchmark_cost_recency",
				Benchmarks: map[string]float64{"swe_bench": 80},
				Recency:    "2026-04-01",
			},
		},
		"local": {
			Family:           "tie",
			Status:           statusActive,
			DeploymentClass:  deploymentClassCommunitySelfHosted,
			SWEBenchVerified: 80,
			PowerProvenance: PowerProvenance{
				Method:     "benchmark_cost_recency",
				Benchmarks: map[string]float64{"swe_bench": 80},
				Recency:    "2026-04-01",
			},
		},
	})

	assert.True(t, models["managed"].AutoRoutable())
	assert.True(t, models["local"].AutoRoutable())
	assert.Greater(t, models["managed"].Power, models["local"].Power)
	assert.LessOrEqual(t, models["local"].Power, 6)
}

func TestBootstrapModelPowers_OlderSameFamilyRequiresOverrideForAutoRouting(t *testing.T) {
	models := BootstrapModelPowers(map[string]ModelEntry{
		"latest": {
			Family:          "qwen",
			Status:          statusActive,
			DeploymentClass: deploymentClassMeteredCloud,
			CostInputPerM:   2,
			CostOutputPerM:  8,
			PowerProvenance: PowerProvenance{Method: "cost_recency", Recency: "2026-04-01"},
		},
		"older-no-override": {
			Family:          "qwen",
			Status:          statusActive,
			DeploymentClass: deploymentClassMeteredCloud,
			CostInputPerM:   1,
			CostOutputPerM:  4,
			PowerProvenance: PowerProvenance{Method: "cost_recency", Recency: "2025-10-01"},
		},
		"older-with-override": {
			Family:          "qwen",
			Status:          statusActive,
			DeploymentClass: deploymentClassMeteredCloud,
			Power:           6,
			CostInputPerM:   1,
			CostOutputPerM:  4,
			PowerProvenance: PowerProvenance{
				Method:         "explicit_override",
				Recency:        "2025-10-01",
				OverrideReason: "best cost/power for small-context edits",
			},
		},
	})

	assert.True(t, models["latest"].AutoRoutable())
	assert.False(t, models["older-no-override"].AutoRoutable())
	assert.True(t, models["older-no-override"].ExactPinOnly)
	assert.True(t, models["older-with-override"].AutoRoutable())
	assert.Equal(t, 6, models["older-with-override"].Power)
}
