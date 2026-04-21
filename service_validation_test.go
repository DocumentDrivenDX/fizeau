package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestExecuteRejectsUnsupportedSubprocessModelBeforeRun(t *testing.T) {
	svc, err := New(ServiceOptions{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ch, err := svc.Execute(context.Background(), ServiceExecuteRequest{
		Prompt:  "should not run",
		Harness: "codex",
		Model:   "not-a-real-model",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	final := drainValidationFinal(t, ch)
	if final.Status != "failed" {
		t.Fatalf("Status: got %q, want failed", final.Status)
	}
	if !strings.Contains(final.Error, "unsupported model") || !strings.Contains(final.Error, "codex") {
		t.Fatalf("Error: got %q", final.Error)
	}
}

func TestExecuteRejectsUnsupportedSubprocessReasoningBeforeRun(t *testing.T) {
	svc, err := New(ServiceOptions{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ch, err := svc.Execute(context.Background(), ServiceExecuteRequest{
		Prompt:    "should not run",
		Harness:   "claude",
		Reasoning: ReasoningMinimal,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	final := drainValidationFinal(t, ch)
	if final.Status != "failed" {
		t.Fatalf("Status: got %q, want failed", final.Status)
	}
	if !strings.Contains(final.Error, "unsupported reasoning") || !strings.Contains(final.Error, "claude") {
		t.Fatalf("Error: got %q", final.Error)
	}
}

func drainValidationFinal(t *testing.T, ch <-chan ServiceEvent) struct {
	Status string `json:"status"`
	Error  string `json:"error"`
} {
	t.Helper()
	timeout := time.NewTimer(2 * time.Second)
	defer timeout.Stop()
	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				t.Fatal("channel closed before final event")
			}
			if ev.Type != "final" {
				continue
			}
			var payload struct {
				Status string `json:"status"`
				Error  string `json:"error"`
			}
			if err := json.Unmarshal(ev.Data, &payload); err != nil {
				t.Fatalf("unmarshal final: %v", err)
			}
			return payload
		case <-timeout.C:
			t.Fatal("timed out waiting for final event")
		}
	}
}
