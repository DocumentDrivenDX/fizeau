package harnesses

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestAdapterReasoningValueOmitsOff(t *testing.T) {
	for _, req := range []ExecuteRequest{
		{Reasoning: "off"},
		{Reasoning: "0"},
	} {
		if got := AdapterReasoningValue(req); got != "" {
			t.Fatalf("AdapterReasoningValue(%+v) = %q, want empty", req, got)
		}
	}
}

func TestAdapterReasoningValueNormalizesReasoning(t *testing.T) {
	got := AdapterReasoningValue(ExecuteRequest{Reasoning: "off"})
	if got != "" {
		t.Fatalf("Reasoning=off should suppress adapter flag, got %q", got)
	}
	got = AdapterReasoningValue(ExecuteRequest{Reasoning: "x-high"})
	if got != "xhigh" {
		t.Fatalf("Reasoning x-high should normalize and win, got %q", got)
	}
}

func TestResolveRunnerReasoningSnapsUnsupportedDiscoveryLevelAndLogs(t *testing.T) {
	cache := NewModelDiscoveryCache(func(harnessName, source string) (ModelDiscoverySnapshot, error) {
		return ModelDiscoverySnapshot{
			CapturedAt:      time.Now().UTC(),
			ReasoningLevels: []string{"low", "medium"},
			Source:          source,
		}, nil
	})
	got := ResolveRunnerReasoningWithCache(cache, "codex", "high")
	if got.ResolvedReasoning != "medium" {
		t.Fatalf("resolved reasoning = %q, want medium", got.ResolvedReasoning)
	}
	if got.Source != "snapped" {
		t.Fatalf("source = %q, want snapped", got.Source)
	}
	if got.Warning == "" {
		t.Fatal("expected warning")
	}

	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))
	prev := slog.Default()
	slog.SetDefault(logger)
	defer slog.SetDefault(prev)
	LogRunnerReasoningWarning(got)
	logged := buf.String()
	for _, want := range []string{
		"level=WARN",
		"requested_reasoning=high",
		"resolved_reasoning=medium",
		"reason=unsupported_effort_snapped_to_nearest_supported",
	} {
		if !strings.Contains(logged, want) {
			t.Fatalf("log missing %q:\n%s", want, logged)
		}
	}
}

func TestResolveRunnerReasoningKeepsSupportedDiscoveryLevelWithoutWarning(t *testing.T) {
	cache := NewModelDiscoveryCache(func(harnessName, source string) (ModelDiscoverySnapshot, error) {
		return ModelDiscoverySnapshot{
			CapturedAt:      time.Now().UTC(),
			ReasoningLevels: []string{"low", "medium", "high"},
			Source:          source,
		}, nil
	})
	got := ResolveRunnerReasoningWithCache(cache, "codex", "high")
	if got.ResolvedReasoning != "high" {
		t.Fatalf("resolved reasoning = %q, want high", got.ResolvedReasoning)
	}
	if got.Source != "caller" {
		t.Fatalf("source = %q, want caller", got.Source)
	}
	if got.Warning != "" {
		t.Fatalf("warning = %q, want empty", got.Warning)
	}
}

func TestResolveRunnerReasoningEmptyDiscoveryLevelsFallsThrough(t *testing.T) {
	cache := NewModelDiscoveryCache(func(harnessName, source string) (ModelDiscoverySnapshot, error) {
		return ModelDiscoverySnapshot{
			CapturedAt: time.Now().UTC(),
			Source:     source,
		}, nil
	})
	got := ResolveRunnerReasoningWithCache(cache, "codex", "high")
	if got.ResolvedReasoning != "high" {
		t.Fatalf("resolved reasoning = %q, want high", got.ResolvedReasoning)
	}
	if got.Warning != "" {
		t.Fatalf("warning = %q, want empty", got.Warning)
	}
}

func TestRunnerReasoningResolutionEventCarriesResolvedEffortAndSource(t *testing.T) {
	ev := RunnerReasoningResolutionEvent(ReasoningActual{
		Harness:            "codex",
		RequestedReasoning: "high",
		ResolvedReasoning:  "medium",
		Source:             "snapped",
	}, map[string]string{"bead_id": "b"}, nil)
	if ev.Type != EventTypeRoutingDecision {
		t.Fatalf("event type = %q, want routing_decision", ev.Type)
	}
	if ev.Metadata["bead_id"] != "b" {
		t.Fatalf("metadata = %#v", ev.Metadata)
	}
	if ev.Time.IsZero() {
		t.Fatal("event time should be set")
	}
}
