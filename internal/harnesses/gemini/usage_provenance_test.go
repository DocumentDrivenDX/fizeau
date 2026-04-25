package gemini

import (
	"testing"

	"github.com/DocumentDrivenDX/agent/internal/harnesses"
)

// TestHarnessFinalEventPreservesUsageProvenance asserts that the gemini
// harness preserves the upstream provider's stats envelope verbatim,
// including explicit zero. Per CONTRACT-003, harnesses MUST NOT collapse
// zero with unknown.
func TestHarnessFinalEventPreservesUsageProvenance(t *testing.T) {
	cases := []struct {
		name       string
		output     string
		wantUsage  bool
		wantInput  *int
		wantOutput *int
	}{
		{
			name:       "explicit_zero_stats_preserved",
			output:     `{"stats":{"input_tokens":0,"output_tokens":0,"total_tokens":0}}`,
			wantUsage:  true,
			wantInput:  intPtr(0),
			wantOutput: intPtr(0),
		},
		{
			name:       "positive_stats_preserved",
			output:     `{"stats":{"input_tokens":12,"output_tokens":3,"total_tokens":15}}`,
			wantUsage:  true,
			wantInput:  intPtr(12),
			wantOutput: intPtr(3),
		},
		{
			name:      "no_stats_envelope_stays_unknown",
			output:    `not a json envelope at all`,
			wantUsage: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			agg := parseGeminiUsage(tc.output)
			if agg.HasUsage != tc.wantUsage {
				t.Fatalf("HasUsage: got %v, want %v", agg.HasUsage, tc.wantUsage)
			}

			var usage *harnesses.FinalUsage
			if agg.HasUsage {
				usage = &harnesses.FinalUsage{
					InputTokens:  harnesses.IntPtr(agg.InputTokens),
					OutputTokens: harnesses.IntPtr(agg.OutputTokens),
				}
			}

			if !tc.wantUsage {
				if usage != nil {
					t.Fatalf("harness silently emitted usage when provider sent none: %#v", usage)
				}
				return
			}
			if usage == nil {
				t.Fatalf("harness dropped upstream stats envelope")
			}
			if usage.InputTokens == nil || *usage.InputTokens != *tc.wantInput {
				t.Fatalf("InputTokens: got %#v, want *%d", usage.InputTokens, *tc.wantInput)
			}
			if usage.OutputTokens == nil || *usage.OutputTokens != *tc.wantOutput {
				t.Fatalf("OutputTokens: got %#v, want *%d", usage.OutputTokens, *tc.wantOutput)
			}
		})
	}
}

func intPtr(v int) *int { return &v }
