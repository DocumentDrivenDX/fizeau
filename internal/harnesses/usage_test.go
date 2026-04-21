package harnesses

import (
	"encoding/json"
	"testing"
)

func TestResolveFinalUsage_SourcePrecedenceAndWarnings(t *testing.T) {
	fresh := true
	stale := false
	candidates := []UsageCandidate{
		{
			Source: UsageSourceStatusOutput,
			Fresh:  &stale,
			Counts: UsageTokenCounts{InputTokens: IntPtr(90), OutputTokens: IntPtr(10), TotalTokens: IntPtr(100)},
		},
		{
			Source: UsageSourceNativeStream,
			Fresh:  &fresh,
			Counts: UsageTokenCounts{InputTokens: IntPtr(100), OutputTokens: IntPtr(20), TotalTokens: IntPtr(120)},
		},
		{
			Source: UsageSourceTranscript,
			Fresh:  &fresh,
			Counts: UsageTokenCounts{InputTokens: IntPtr(100), OutputTokens: IntPtr(19), TotalTokens: IntPtr(119)},
		},
	}

	usage, warnings := ResolveFinalUsage(candidates)
	if usage == nil {
		t.Fatal("expected usage")
	}
	if usage.Source != UsageSourceNativeStream {
		t.Fatalf("source: got %q", usage.Source)
	}
	if usage.InputTokens == nil || *usage.InputTokens != 100 {
		t.Fatalf("input tokens: got %#v", usage.InputTokens)
	}
	if len(warnings) != 2 {
		t.Fatalf("warnings: got %d, want 2 (%#v)", len(warnings), warnings)
	}
	for _, warning := range warnings {
		if warning.Code != UsageWarningDisagreement {
			t.Fatalf("warning code: got %q", warning.Code)
		}
		if len(warning.Sources) != 2 {
			t.Fatalf("warning sources: got %#v", warning.Sources)
		}
	}
}

func TestResolveFinalUsage_TranscriptWinsOverStatusFallback(t *testing.T) {
	candidates := []UsageCandidate{
		{Source: UsageSourceFallback, Counts: UsageTokenCounts{InputTokens: IntPtr(1)}},
		{Source: UsageSourceStatusOutput, Counts: UsageTokenCounts{InputTokens: IntPtr(2)}},
		{Source: UsageSourceTranscript, Counts: UsageTokenCounts{InputTokens: IntPtr(3)}},
	}

	usage, _ := ResolveFinalUsage(candidates)
	if usage == nil || usage.Source != UsageSourceTranscript || usage.InputTokens == nil || *usage.InputTokens != 3 {
		t.Fatalf("usage: %#v", usage)
	}

	usage, _ = ResolveFinalUsage([]UsageCandidate{
		{Source: UsageSourceFallback, Counts: UsageTokenCounts{InputTokens: IntPtr(1)}},
		{Source: UsageSourceStatusOutput, Counts: UsageTokenCounts{InputTokens: IntPtr(2)}},
	})
	if usage == nil || usage.Source != UsageSourceStatusOutput || usage.InputTokens == nil || *usage.InputTokens != 2 {
		t.Fatalf("status-over-fallback usage: %#v", usage)
	}
}

func TestResolveFinalUsage_ZeroDistinctFromUnavailable(t *testing.T) {
	usage, warnings := ResolveFinalUsage([]UsageCandidate{{
		Source: UsageSourceNativeStream,
		Counts: UsageTokenCounts{InputTokens: IntPtr(0), OutputTokens: IntPtr(0), TotalTokens: IntPtr(0)},
	}})
	if len(warnings) != 0 {
		t.Fatalf("warnings: %#v", warnings)
	}
	if usage == nil || usage.InputTokens == nil || usage.TotalTokens == nil {
		t.Fatalf("zero usage should be present, got %#v", usage)
	}
	if *usage.InputTokens != 0 || *usage.TotalTokens != 0 {
		t.Fatalf("zero usage values changed: %#v", usage)
	}

	usage, warnings = ResolveFinalUsage([]UsageCandidate{{
		Source:  UsageSourceNativeStream,
		Warning: "native stream usage changed shape",
	}})
	if usage != nil {
		t.Fatalf("unavailable usage should be nil, got %#v", usage)
	}
	if len(warnings) != 1 || warnings[0].Code != UsageWarningMalformed {
		t.Fatalf("warnings: %#v", warnings)
	}
}

func TestParseUsageJSON_NormalizesKnownTokenShapes(t *testing.T) {
	raw := json.RawMessage(`{
		"prompt_tokens": 12,
		"completion_tokens": 5,
		"prompt_tokens_details": {"cached_tokens": 4},
		"completion_tokens_details": {"reasoning_tokens": 2}
	}`)
	counts, err := ParseUsageJSON(raw)
	if err != nil {
		t.Fatalf("ParseUsageJSON: %v", err)
	}
	assertIntPtr(t, counts.InputTokens, 12, "input")
	assertIntPtr(t, counts.OutputTokens, 5, "output")
	assertIntPtr(t, counts.CacheReadTokens, 4, "cache read")
	assertIntPtr(t, counts.CacheTokens, 4, "cache total")
	assertIntPtr(t, counts.ReasoningTokens, 2, "reasoning")
	assertIntPtr(t, counts.TotalTokens, 17, "computed total")
}

func assertIntPtr(t *testing.T, got *int, want int, label string) {
	t.Helper()
	if got == nil || *got != want {
		t.Fatalf("%s: got %#v, want %d", label, got, want)
	}
}
