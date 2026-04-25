package pi

import (
	"context"
	"strings"
	"testing"

	"github.com/DocumentDrivenDX/agent/internal/harnesses"
)

// TestHarnessFinalEventPreservesUsageProvenance asserts that the pi harness
// preserves the upstream provider's usage values verbatim across the parser
// + runner boundary. Per CONTRACT-003, an explicit upstream zero must round
// through to a non-nil *int(0) on the FinalUsage; absence must remain nil.
func TestHarnessFinalEventPreservesUsageProvenance(t *testing.T) {
	cases := []struct {
		name         string
		input        string
		wantUsage    bool
		wantInput    *int
		wantOutput   *int
		harness      string
	}{
		{
			name:       "explicit_zero_usage_preserved",
			input:      `{"type":"text_end","message":{"usage":{"input":0,"output":0,"cost":{"total":0}}},"response":"silent"}`,
			wantUsage:  true,
			wantInput:  intPtr(0),
			wantOutput: intPtr(0),
			harness:    "pi",
		},
		{
			name:       "positive_usage_preserved",
			input:      `{"type":"text_end","message":{"usage":{"input":12,"output":3,"cost":{"total":0}}},"response":"hi"}`,
			wantUsage:  true,
			wantInput:  intPtr(12),
			wantOutput: intPtr(3),
			harness:    "pi",
		},
		{
			name:      "no_usage_envelope_stays_unknown",
			input:     `{"type":"text_end","message":{"content":[{"type":"text","text":"no usage field"}]},"response":"no usage field"}`,
			wantUsage: false,
			harness:   "pi",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := make(chan harnesses.Event, 8)
			var seq int64
			agg, err := parsePiStream(context.Background(), strings.NewReader(tc.input), out, nil, &seq)
			close(out)
			if err != nil {
				t.Fatalf("parsePiStream: %v", err)
			}

			if agg.HasUsage != tc.wantUsage {
				t.Fatalf("HasUsage: got %v, want %v", agg.HasUsage, tc.wantUsage)
			}

			// Mirror the runner.go gating: emit FinalUsage iff agg.HasUsage,
			// preserving zero verbatim.
			var usage *harnesses.FinalUsage
			if agg.HasUsage {
				usage = &harnesses.FinalUsage{
					InputTokens:  harnesses.IntPtr(agg.InputTokens),
					OutputTokens: harnesses.IntPtr(agg.OutputTokens),
				}
			}

			if !tc.wantUsage {
				if usage != nil {
					t.Fatalf("%s: harness silently emitted usage when provider sent none: %#v", tc.harness, usage)
				}
				return
			}
			if usage == nil {
				t.Fatalf("%s: harness dropped upstream usage envelope", tc.harness)
			}
			if usage.InputTokens == nil || *usage.InputTokens != *tc.wantInput {
				t.Fatalf("%s: InputTokens: got %#v, want *%d", tc.harness, usage.InputTokens, *tc.wantInput)
			}
			if usage.OutputTokens == nil || *usage.OutputTokens != *tc.wantOutput {
				t.Fatalf("%s: OutputTokens: got %#v, want *%d", tc.harness, usage.OutputTokens, *tc.wantOutput)
			}
		})
	}
}

func intPtr(v int) *int { return &v }
