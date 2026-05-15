package fizeau

import (
	"time"

	"github.com/easel/fizeau/internal/routingquality"
)

// ADR-006 §5: routing-quality is the user-facing measure of how often
// auto-routing produces a decision the caller is willing to live with.
// It is distinct from per-(provider, model) provider-reliability (the
// observed completion rate of a chosen candidate). The two compose:
// routing-quality × provider-reliability ≈ end-to-end completion rate.

// RoutingQualityMetrics is the bundle of three first-class metrics ADR-006
// makes operator-visible. AutoAcceptanceRate and OverrideDisagreementRate are
// fractions in [0,1]; OverrideClassBreakdown is the diagnostic pivot. All
// fields zero when the underlying window contains no requests.
type RoutingQualityMetrics struct {
	// AutoAcceptanceRate = no-override requests / total requests. Higher is
	// better. The headline number for routing health.
	AutoAcceptanceRate float64 `json:"auto_acceptance_rate"`

	// OverrideDisagreementRate = overrides where the user pin differs from
	// auto on at least one overridden axis / total overrides. Lower is
	// better. Coincidental-agreement overrides land in the denominator but
	// not the numerator.
	OverrideDisagreementRate float64 `json:"override_disagreement_rate"`

	// OverrideClassBreakdown is a pivot of (prompt_features bucket,
	// axis_overridden, match_per_axis) → count + outcome aggregates.
	// Sorted deterministically by (PromptFeatureBucket, Axis, Match) so
	// snapshot tests remain stable across runs.
	OverrideClassBreakdown []OverrideClassBucket `json:"override_class_breakdown,omitempty"`

	// TotalRequests is the total Execute count over the metric window
	// (including overridden requests). Surface for operator UIs that want
	// to display "k out of N" alongside the rate.
	TotalRequests int `json:"total_requests"`

	// TotalOverrides is the total override count over the metric window.
	// Equal to TotalRequests-(no-override requests).
	TotalOverrides int `json:"total_overrides"`
}

// OverrideClassBucket is one cell in the override-class pivot.
//
// Each override event contributes one bucket per overridden axis: an
// override that pins both Harness and Model produces two breakdown rows for
// that event, with axis="harness" and axis="model" respectively. The
// PromptFeatureBucket coalesces estimated_tokens / requires_tools /
// reasoning into a coarse string so operators can read the pivot without
// drowning in cardinality.
type OverrideClassBucket struct {
	PromptFeatureBucket string `json:"prompt_feature_bucket"`
	Axis                string `json:"axis"`
	Match               bool   `json:"match"`
	Count               int    `json:"count"`

	SuccessOutcomes   int `json:"success_outcomes"`
	StalledOutcomes   int `json:"stalled_outcomes"`
	FailedOutcomes    int `json:"failed_outcomes"`
	CancelledOutcomes int `json:"cancelled_outcomes"`
	UnknownOutcomes   int `json:"unknown_outcomes"`
}

func routingQualityMetricsFromOverrides(totalRequests int, overrides []ServiceOverrideData) RoutingQualityMetrics {
	internal := make([]routingquality.OverrideData, 0, len(overrides))
	for _, ov := range overrides {
		internal = append(internal, *toRoutingQualityOverride(ov))
	}
	return fromRoutingQualityMetrics(routingquality.ComputeMetrics(totalRequests, internal))
}

// recordRoutingQualityForRequest records one Execute call into the
// service's routing-quality store. ovr may be nil for non-overridden
// requests; when non-nil, the recorded record pointer is stashed onto the
// override context so the fan-out goroutine can back-write the
// post-execution outcome (success / stalled / failed / cancelled) once the
// final event arrives.
func (s *service) recordRoutingQualityForRequest(ovr *overrideContext) {
	if s == nil || s.routingQuality == nil {
		return
	}
	now := time.Now().UTC()
	if ovr == nil {
		s.routingQuality.RecordRequest(now, nil)
		return
	}
	rec := s.routingQuality.RecordRequest(now, toRoutingQualityOverride(ovr.payload))
	ovr.record = rec
}

func toRoutingQualityOverride(ov ServiceOverrideData) *routingquality.OverrideData {
	out := &routingquality.OverrideData{
		AxesOverridden: append([]string(nil), ov.AxesOverridden...),
		MatchPerAxis:   make(map[string]bool, len(ov.MatchPerAxis)),
		PromptFeatures: routingquality.PromptFeatures{
			RequiresTools: ov.PromptFeatures.RequiresTools,
			Reasoning:     ov.PromptFeatures.Reasoning,
		},
	}
	for axis, match := range ov.MatchPerAxis {
		out.MatchPerAxis[axis] = match
	}
	if ov.PromptFeatures.EstimatedTokens != nil {
		tokens := *ov.PromptFeatures.EstimatedTokens
		out.PromptFeatures.EstimatedTokens = &tokens
	}
	if ov.Outcome != nil {
		out.Outcome = &routingquality.Outcome{Status: ov.Outcome.Status}
	}
	return out
}

func fromRoutingQualityMetrics(m routingquality.Metrics) RoutingQualityMetrics {
	out := RoutingQualityMetrics{
		AutoAcceptanceRate:       m.AutoAcceptanceRate,
		OverrideDisagreementRate: m.OverrideDisagreementRate,
		TotalRequests:            m.TotalRequests,
		TotalOverrides:           m.TotalOverrides,
	}
	if len(m.OverrideClassBreakdown) > 0 {
		out.OverrideClassBreakdown = make([]OverrideClassBucket, 0, len(m.OverrideClassBreakdown))
		for _, b := range m.OverrideClassBreakdown {
			out.OverrideClassBreakdown = append(out.OverrideClassBreakdown, OverrideClassBucket{
				PromptFeatureBucket: b.PromptFeatureBucket,
				Axis:                b.Axis,
				Match:               b.Match,
				Count:               b.Count,
				SuccessOutcomes:     b.SuccessOutcomes,
				StalledOutcomes:     b.StalledOutcomes,
				FailedOutcomes:      b.FailedOutcomes,
				CancelledOutcomes:   b.CancelledOutcomes,
				UnknownOutcomes:     b.UnknownOutcomes,
			})
		}
	}
	return out
}
