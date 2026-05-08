package routing

import (
	"errors"
	"testing"
)

func TestPinPinConflictHarnessIncompatibleWithModel(t *testing.T) {
	in := Inputs{Harnesses: []HarnessEntry{{
		Name:                "claude",
		Surface:             "claude",
		CostClass:           "medium",
		IsSubscription:      true,
		AutoRoutingEligible: true,
		Available:           true,
		ExactPinSupport:     true,
		SupportedModels:     []string{"opus-4.7"},
		SupportsTools:       true,
	}}}

	_, err := Resolve(Request{Harness: "claude", Model: "qwen3.6"}, in)
	if err == nil {
		t.Fatal("expected harness/model pin conflict")
	}
	var typed *ErrUnsatisfiablePin
	if !errors.As(err, &typed) {
		t.Fatalf("errors.As ErrUnsatisfiablePin: %T %v", err, err)
	}
	if typed.Pin != "harness=claude+model=qwen3.6" {
		t.Fatalf("Pin=%q, want harness=claude+model=qwen3.6", typed.Pin)
	}
}
