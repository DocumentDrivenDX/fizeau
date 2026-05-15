package routingquality

import (
	"math"
	"strings"
	"testing"
	"time"
)

func floatEq(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}

func makeOverride(axes []string, matches []string, estimatedTokens int, requiresTools bool, reasoning string, outcomeStatus string) OverrideData {
	mpa := make(map[string]bool, len(axes))
	for _, a := range axes {
		mpa[a] = false
	}
	for _, m := range matches {
		mpa[m] = true
	}
	pf := PromptFeatures{
		RequiresTools: requiresTools,
		Reasoning:     reasoning,
	}
	if estimatedTokens > 0 {
		t := estimatedTokens
		pf.EstimatedTokens = &t
	}
	ov := OverrideData{
		AxesOverridden: append([]string(nil), axes...),
		MatchPerAxis:   mpa,
		PromptFeatures: pf,
	}
	if outcomeStatus != "" {
		ov.Outcome = &Outcome{Status: outcomeStatus}
	}
	return ov
}

func TestMetricsAcceptanceRate(t *testing.T) {
	overrides := make([]OverrideData, 0, 30)
	for i := 0; i < 30; i++ {
		overrides = append(overrides, makeOverride([]string{"model"}, nil, 0, false, "", ""))
	}
	m := ComputeMetrics(100, overrides)
	if !floatEq(m.AutoAcceptanceRate, 0.70) {
		t.Fatalf("AutoAcceptanceRate = %v, want 0.70", m.AutoAcceptanceRate)
	}
}

func TestMetricsDisagreementRate(t *testing.T) {
	overrides := make([]OverrideData, 0, 30)
	for i := 0; i < 18; i++ {
		overrides = append(overrides, makeOverride([]string{"model"}, nil, 0, false, "", ""))
	}
	for i := 0; i < 12; i++ {
		overrides = append(overrides, makeOverride([]string{"model"}, []string{"model"}, 0, false, "", ""))
	}
	m := ComputeMetrics(30, overrides)
	if !floatEq(m.OverrideDisagreementRate, 0.60) {
		t.Fatalf("OverrideDisagreementRate = %v, want 0.60", m.OverrideDisagreementRate)
	}
}

func TestMetricsClassBreakdown(t *testing.T) {
	overrides := []OverrideData{
		makeOverride([]string{"harness"}, nil, 2000, true, "high", "success"),
		makeOverride([]string{"harness"}, nil, 2000, true, "high", "success"),
		makeOverride([]string{"model"}, []string{"model"}, 2000, true, "high", "stalled"),
		makeOverride([]string{"provider"}, nil, 50000, false, "", "failed"),
		makeOverride([]string{"harness"}, nil, 2000, true, "high", "failed"),
	}
	m := ComputeMetrics(5, overrides)
	if len(m.OverrideClassBreakdown) != 3 {
		t.Fatalf("breakdown len = %d, want 3: %+v", len(m.OverrideClassBreakdown), m.OverrideClassBreakdown)
	}
	var harnessBucket *Bucket
	for i := range m.OverrideClassBreakdown {
		b := &m.OverrideClassBreakdown[i]
		if b.Axis == "harness" && !b.Match && strings.Contains(b.PromptFeatureBucket, "tokens=small") {
			harnessBucket = b
		}
	}
	if harnessBucket == nil {
		t.Fatalf("missing harness mismatch bucket: %+v", m.OverrideClassBreakdown)
	}
	if harnessBucket.Count != 3 || harnessBucket.SuccessOutcomes != 2 || harnessBucket.FailedOutcomes != 1 {
		t.Fatalf("harness bucket = %+v, want count=3 success=2 failed=1", harnessBucket)
	}
}

func TestStoreSnapshotRecentAndOutcomeBackWrite(t *testing.T) {
	st := NewStore(1024)
	base := time.Now().UTC().Add(-time.Hour)
	for i := 0; i < 5; i++ {
		st.RecordRequest(base.Add(time.Duration(i)*time.Minute), nil)
	}
	ov := makeOverride([]string{"model"}, nil, 0, false, "", "")
	rec := st.RecordRequest(base.Add(5*time.Minute), &ov)
	StampOutcome(rec, &Outcome{Status: "success"})

	recs := st.SnapshotRecent(0, time.Time{})
	if len(recs) != 6 {
		t.Fatalf("snapshot len = %d, want 6", len(recs))
	}
	m := ComputeMetricsFromRecords(recs)
	if m.TotalRequests != 6 || m.TotalOverrides != 1 {
		t.Fatalf("metrics = %+v", m)
	}
	if !floatEq(m.AutoAcceptanceRate, 5.0/6.0) {
		t.Fatalf("AutoAcceptanceRate = %v, want %v", m.AutoAcceptanceRate, 5.0/6.0)
	}
	if m.OverrideClassBreakdown[0].SuccessOutcomes != 1 {
		t.Fatalf("SuccessOutcomes = %d, want 1", m.OverrideClassBreakdown[0].SuccessOutcomes)
	}
}
