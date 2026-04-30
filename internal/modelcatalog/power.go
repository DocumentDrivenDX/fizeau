package modelcatalog

import (
	"sort"
	"strings"
)

const (
	deploymentClassManagedCloudFrontier = "managed_cloud_frontier"
	deploymentClassPrepaidFrontier      = "prepaid_frontier"
	deploymentClassMeteredCloud         = "metered_cloud"
	deploymentClassLocalFree            = "local_free"
	deploymentClassCommunitySelfHosted  = "community_self_hosted"
)

// BootstrapModelPowers derives conservative power/eligibility defaults from
// comparable model metadata. It is side-effect-free so catalog generation can
// inspect or persist the result explicitly.
func BootstrapModelPowers(models map[string]ModelEntry) map[string]ModelEntry {
	out := make(map[string]ModelEntry, len(models))
	for id, model := range models {
		out[id] = model
	}

	groups := make(map[string][]string)
	for id, model := range out {
		key := model.Family + "\x00" + model.DeploymentClass
		groups[key] = append(groups[key], id)
	}

	for _, ids := range groups {
		sort.Slice(ids, func(i, j int) bool {
			left, right := out[ids[i]], out[ids[j]]
			if cmp := strings.Compare(powerRecency(left), powerRecency(right)); cmp != 0 {
				return cmp > 0
			}
			if leftCost, rightCost := powerCost(left), powerCost(right); leftCost != rightCost {
				return leftCost > rightCost
			}
			if left.SWEBenchVerified != right.SWEBenchVerified {
				return left.SWEBenchVerified > right.SWEBenchVerified
			}
			return ids[i] < ids[j]
		})

		for rank, id := range ids {
			model := out[id]
			if normalizedStatus(model.Status) != statusActive {
				continue
			}
			if rank > 0 && strings.TrimSpace(model.PowerProvenance.OverrideReason) == "" {
				model.Power = 0
				model.ExactPinOnly = true
				out[id] = model
				continue
			}
			if model.Power <= 0 {
				model.Power = defaultPowerForDeploymentClass(model.DeploymentClass)
			}
			if cap := powerCapForDeploymentClass(model.DeploymentClass); model.Power > cap {
				model.Power = cap
			}
			out[id] = model
		}
	}

	return out
}

func powerRecency(model ModelEntry) string {
	if model.PowerProvenance.Recency != "" {
		return model.PowerProvenance.Recency
	}
	return model.BenchmarkAsOf
}

func powerCost(model ModelEntry) float64 {
	input := model.PowerProvenance.CostInputPerM
	if input == 0 {
		input = model.inputCostPerM()
	}
	output := model.PowerProvenance.CostOutputPerM
	if output == 0 {
		output = model.outputCostPerM()
	}
	return input + output
}

func defaultPowerForDeploymentClass(class string) int {
	switch strings.TrimSpace(class) {
	case deploymentClassManagedCloudFrontier, deploymentClassPrepaidFrontier:
		return 9
	case deploymentClassMeteredCloud:
		return 7
	case deploymentClassLocalFree, deploymentClassCommunitySelfHosted:
		return 5
	default:
		return 4
	}
}

func powerCapForDeploymentClass(class string) int {
	switch strings.TrimSpace(class) {
	case deploymentClassLocalFree, deploymentClassCommunitySelfHosted:
		return 6
	default:
		return 10
	}
}
