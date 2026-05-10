package agentcli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/easel/fizeau"
)

func TestRouteStatusUsesPolicyKey(t *testing.T) {
	out := routeStatusOutput{
		Policy: "cheap",
		PowerPolicy: routeStatusPowerPolicy{
			PolicyName: "cheap",
		},
	}

	data, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	wire := string(data)
	if !strings.Contains(wire, `"policy":"cheap"`) {
		t.Fatalf("JSON missing policy key: %s", wire)
	}
	if !strings.Contains(wire, `"policy_name":"cheap"`) {
		t.Fatalf("JSON missing power_policy.policy_name key: %s", wire)
	}
	legacyKey := `"` + "pro" + "file" + `"`
	if strings.Contains(wire, legacyKey) {
		t.Fatalf("JSON should not contain profile key: %s", wire)
	}
}

// TestRouteStatusOverridesJSONStable snapshots the JSON envelope shape so
// downstream operator tooling (dashboards, alerts) can rely on it. New
// keys may be added behind a flag — never silently changed.
func TestRouteStatusOverridesJSONStable(t *testing.T) {
	start := time.Date(2026, 4, 18, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 4, 25, 0, 0, 0, 0, time.UTC)
	envelope := routeStatusOverridesOutput{
		Since:                    "168h",
		WindowStart:              start,
		WindowEnd:                end,
		AxisFilter:               "harness",
		AutoAcceptanceRate:       0.4,
		OverrideDisagreementRate: 0.5,
		TotalRequests:            5,
		TotalOverrides:           3,
		OverrideClassBreakdown: []fizeau.OverrideClassBucket{
			{
				PromptFeatureBucket: "tokens=small,tools=no,reasoning=none",
				Axis:                "harness",
				Match:               false,
				Count:               2,
				SuccessOutcomes:     1,
				FailedOutcomes:      1,
			},
		},
	}

	var buf bytes.Buffer
	writeRouteStatusOverridesJSON(&buf, envelope)

	// Round-trip parse: schema must decode into a known shape, with every
	// AC-required key present at the top level.
	var generic map[string]json.RawMessage
	if err := json.Unmarshal(buf.Bytes(), &generic); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, buf.String())
	}
	requiredTopLevel := []string{
		"since",
		"window_start",
		"window_end",
		"axis_filter",
		"auto_acceptance_rate",
		"override_disagreement_rate",
		"total_requests",
		"total_overrides",
		"override_class_breakdown",
	}
	for _, k := range requiredTopLevel {
		if _, ok := generic[k]; !ok {
			t.Errorf("required key %q missing from JSON envelope:\n%s", k, buf.String())
		}
	}

	// Bucket schema: every documented field must round-trip.
	var bkt []fizeau.OverrideClassBucket
	if err := json.Unmarshal(generic["override_class_breakdown"], &bkt); err != nil {
		t.Fatalf("decode breakdown: %v", err)
	}
	if len(bkt) != 1 {
		t.Fatalf("breakdown len = %d, want 1", len(bkt))
	}
	got := bkt[0]
	if got.PromptFeatureBucket != "tokens=small,tools=no,reasoning=none" {
		t.Errorf("PromptFeatureBucket = %q", got.PromptFeatureBucket)
	}
	if got.Axis != "harness" || got.Match != false || got.Count != 2 {
		t.Errorf("bucket fields lost in round-trip: %+v", got)
	}
	if got.SuccessOutcomes != 1 || got.FailedOutcomes != 1 {
		t.Errorf("outcome aggregates lost in round-trip: %+v", got)
	}

	// Verify the bucket JSON keys themselves are stable: per ADR-006 §5
	// the breakdown is the operator pivot, so its key names are part of
	// the contract.
	var bucketGeneric []map[string]json.RawMessage
	if err := json.Unmarshal(generic["override_class_breakdown"], &bucketGeneric); err != nil {
		t.Fatalf("decode bucket generic: %v", err)
	}
	requiredBucketKeys := []string{
		"prompt_feature_bucket",
		"axis",
		"match",
		"count",
		"success_outcomes",
		"stalled_outcomes",
		"failed_outcomes",
		"cancelled_outcomes",
		"unknown_outcomes",
	}
	for _, k := range requiredBucketKeys {
		if _, ok := bucketGeneric[0][k]; !ok {
			t.Errorf("bucket key %q missing:\n%s", k, buf.String())
		}
	}

	// Snapshot the JSON output; trailing newline tolerated.
	want := `{
  "since": "168h",
  "window_start": "2026-04-18T00:00:00Z",
  "window_end": "2026-04-25T00:00:00Z",
  "axis_filter": "harness",
  "auto_acceptance_rate": 0.4,
  "override_disagreement_rate": 0.5,
  "total_requests": 5,
  "total_overrides": 3,
  "override_class_breakdown": [
    {
      "prompt_feature_bucket": "tokens=small,tools=no,reasoning=none",
      "axis": "harness",
      "match": false,
      "count": 2,
      "success_outcomes": 1,
      "stalled_outcomes": 0,
      "failed_outcomes": 1,
      "cancelled_outcomes": 0,
      "unknown_outcomes": 0
    }
  ]
}
`
	if got := buf.String(); got != want {
		t.Errorf("JSON envelope drift.\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}

	// Empty axis_filter must omit the field (omitempty contract).
	envelope.AxisFilter = ""
	buf.Reset()
	writeRouteStatusOverridesJSON(&buf, envelope)
	if strings.Contains(buf.String(), `"axis_filter"`) {
		t.Errorf("empty axis_filter should be omitted (omitempty):\n%s", buf.String())
	}
}
