package agent_test

import (
	"encoding/json"
	"strings"
	"testing"

	agent "github.com/DocumentDrivenDX/agent"
)

// TestServiceFinalUsage_DistinguishesZeroFromUnknown asserts that the public
// ServiceFinalUsage payload distinguishes "harness emitted explicit zero"
// from "harness did not emit this dimension", in both Go-struct shape and
// JSON round-trip. CONTRACT-003 forbids harness emitters from collapsing
// these two states.
func TestServiceFinalUsage_DistinguishesZeroFromUnknown(t *testing.T) {
	zero := 0
	knownZero := agent.ServiceFinalUsage{
		InputTokens:  &zero,
		OutputTokens: &zero,
		TotalTokens:  &zero,
		Source:       "native_stream",
	}
	unknown := agent.ServiceFinalUsage{
		Source: "native_stream",
	}

	// In-memory representations are distinguishable.
	if knownZero.InputTokens == nil {
		t.Fatal("knownZero.InputTokens should be non-nil pointer to 0")
	}
	if *knownZero.InputTokens != 0 {
		t.Fatalf("knownZero.InputTokens should be 0, got %d", *knownZero.InputTokens)
	}
	if unknown.InputTokens != nil {
		t.Fatal("unknown.InputTokens should be nil pointer (dimension not emitted)")
	}

	// JSON round-trip preserves the distinction:
	//   nil pointer            -> field omitted (omitempty)
	//   non-nil pointer to 0   -> field present with value 0
	knownRaw, err := json.Marshal(knownZero)
	if err != nil {
		t.Fatalf("marshal knownZero: %v", err)
	}
	unknownRaw, err := json.Marshal(unknown)
	if err != nil {
		t.Fatalf("marshal unknown: %v", err)
	}
	if !strings.Contains(string(knownRaw), `"input_tokens":0`) {
		t.Fatalf("explicit zero must serialize as input_tokens:0, got %s", knownRaw)
	}
	if strings.Contains(string(unknownRaw), `"input_tokens"`) {
		t.Fatalf("unknown dimension must be omitted from JSON, got %s", unknownRaw)
	}

	// Unmarshalling preserves provenance: present-with-zero round-trips to
	// non-nil *int(0); absent round-trips to nil.
	var knownBack agent.ServiceFinalUsage
	if err := json.Unmarshal(knownRaw, &knownBack); err != nil {
		t.Fatalf("unmarshal knownRaw: %v", err)
	}
	if knownBack.InputTokens == nil {
		t.Fatal("round-tripped knownZero.InputTokens should be non-nil pointer to 0")
	}
	if *knownBack.InputTokens != 0 {
		t.Fatalf("round-tripped value should be 0, got %d", *knownBack.InputTokens)
	}

	var unknownBack agent.ServiceFinalUsage
	if err := json.Unmarshal(unknownRaw, &unknownBack); err != nil {
		t.Fatalf("unmarshal unknownRaw: %v", err)
	}
	if unknownBack.InputTokens != nil {
		t.Fatalf("round-tripped unknown.InputTokens should be nil, got %#v", unknownBack.InputTokens)
	}

	// A consumer that distinguishes via "is the pointer nil" must give
	// distinct answers for the two payloads.
	if isKnown(knownZero) == isKnown(unknown) {
		t.Fatal("ServiceFinalUsage failed to distinguish explicit zero from unknown")
	}
}

func isKnown(u agent.ServiceFinalUsage) bool {
	return u.InputTokens != nil
}
