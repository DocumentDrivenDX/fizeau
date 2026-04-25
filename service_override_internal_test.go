package agent

import (
	"reflect"
	"testing"
)

func TestAxesOverridden_EmptyForUnpinnedRequest(t *testing.T) {
	got := axesOverridden(ServiceExecuteRequest{Profile: "smart", ModelRef: "code-medium"})
	if len(got) != 0 {
		t.Fatalf("axesOverridden(profile-only) = %v, want empty", got)
	}
}

func TestAxesOverridden_TracksEachAxisIndependently(t *testing.T) {
	cases := []struct {
		name string
		req  ServiceExecuteRequest
		want []string
	}{
		{"harness only", ServiceExecuteRequest{Harness: "claude"}, []string{overrideAxisHarness}},
		{"provider only", ServiceExecuteRequest{Provider: "openrouter"}, []string{overrideAxisProvider}},
		{"model only", ServiceExecuteRequest{Model: "opus-4.7"}, []string{overrideAxisModel}},
		{"all three", ServiceExecuteRequest{Harness: "claude", Provider: "openrouter", Model: "opus-4.7"},
			[]string{overrideAxisHarness, overrideAxisProvider, overrideAxisModel}},
		{"harness+model", ServiceExecuteRequest{Harness: "claude", Model: "opus-4.7"},
			[]string{overrideAxisHarness, overrideAxisModel}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := axesOverridden(tc.req); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("axesOverridden(%+v) = %v, want %v", tc.req, got, tc.want)
			}
		})
	}
}

func TestBuildPromptFeatures_NullableEstimatedTokens(t *testing.T) {
	pf := buildPromptFeatures(ServiceExecuteRequest{})
	if pf.EstimatedTokens != nil {
		t.Fatalf("EstimatedTokens for empty request: want nil, got %d", *pf.EstimatedTokens)
	}
	pf = buildPromptFeatures(ServiceExecuteRequest{EstimatedPromptTokens: 12500, RequiresTools: true, Reasoning: "high"})
	if pf.EstimatedTokens == nil || *pf.EstimatedTokens != 12500 {
		t.Fatalf("EstimatedTokens: want 12500, got %v", pf.EstimatedTokens)
	}
	if !pf.RequiresTools {
		t.Fatalf("RequiresTools: want true, got false")
	}
	if pf.Reasoning != "high" {
		t.Fatalf("Reasoning: want high, got %q", pf.Reasoning)
	}
}

func TestOverrideReasonHint_FromMetadata(t *testing.T) {
	if got := overrideReasonHint(ServiceExecuteRequest{}); got != "" {
		t.Fatalf("empty request reason_hint: want empty, got %q", got)
	}
	req := ServiceExecuteRequest{Metadata: map[string]string{"override.reason": "model needs to match training"}}
	if got := overrideReasonHint(req); got != "model needs to match training" {
		t.Fatalf("reason_hint: got %q", got)
	}
}
