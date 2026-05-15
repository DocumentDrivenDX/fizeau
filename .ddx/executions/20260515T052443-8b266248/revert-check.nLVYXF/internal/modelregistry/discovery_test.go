package modelregistry

import (
	"encoding/json"
	"testing"
	"time"
)

func TestParseDiscoveryIDsSupportsADRModelRefMap(t *testing.T) {
	capturedAt := time.Date(2026, 5, 12, 11, 0, 0, 0, time.UTC)
	raw, err := json.Marshal(map[string]any{
		"openrouter/gpt-5.5": map[string]any{
			"captured_at": capturedAt.Format(time.RFC3339),
		},
		"openrouter/anthropic/claude-opus-4-5": map[string]any{},
		"other/gpt-5.4":                        map[string]any{},
	})
	if err != nil {
		t.Fatal(err)
	}

	ids, gotCapturedAt, err := parseDiscoveryIDs(raw, "openrouter")
	if err != nil {
		t.Fatalf("parseDiscoveryIDs() error = %v", err)
	}
	want := []string{"anthropic/claude-opus-4-5", "gpt-5.5"}
	if len(ids) != len(want) {
		t.Fatalf("ids = %#v, want %#v", ids, want)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("ids = %#v, want %#v", ids, want)
		}
	}
	if !gotCapturedAt.Equal(capturedAt) {
		t.Fatalf("capturedAt = %v, want %v", gotCapturedAt, capturedAt)
	}
}
