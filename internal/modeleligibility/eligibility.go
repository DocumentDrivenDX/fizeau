package modeleligibility

import (
	"strings"

	"github.com/easel/fizeau/internal/modelcatalog"
)

const (
	exclusionCatalogUnknown     = "catalog_unknown"
	exclusionCatalogNotRoutable = "catalog_not_auto_routable"
	exclusionProviderExcluded   = "provider_include_by_default_false"
	exclusionStatusUnavailable  = "status_unavailable"
)

// View captures the routing-relevant metadata derived from a model catalog
// entry and the live/provider snapshot state around it.
type View struct {
	Power           int
	ExactPinOnly    bool
	AutoRoutable    bool
	ExclusionReason string
}

// Resolve computes the routing-relevant view for a model ID. includeByDefault
// is the provider-level include flag from the assembled snapshot.
func Resolve(modelID string, includeByDefault bool, status string, cat *modelcatalog.Catalog) View {
	var out View
	var catalogAuto bool

	if cat != nil {
		if entry, ok := cat.LookupModel(modelID); ok {
			out.Power = entry.Power
			if eligibility, ok := cat.ModelEligibility(modelID); ok {
				out.ExactPinOnly = eligibility.ExactPinOnly
				catalogAuto = eligibility.AutoRoutable
			}
		}
	}

	if !catalogAuto {
		if out.Power == 0 {
			out.ExclusionReason = exclusionCatalogUnknown
		} else {
			out.ExclusionReason = exclusionCatalogNotRoutable
		}
	}
	if catalogAuto && !includeByDefault {
		out.ExclusionReason = exclusionProviderExcluded
	}
	if catalogAuto && includeByDefault && !statusAllowsRouting(status) {
		out.ExclusionReason = exclusionStatusUnavailable
	}

	out.AutoRoutable = catalogAuto && includeByDefault && statusAllowsRouting(status)
	return out
}

func statusAllowsRouting(status string) bool {
	switch strings.TrimSpace(status) {
	case "", "available", "rate_limited":
		return true
	default:
		return status != "unknown" && status != "unreachable"
	}
}
