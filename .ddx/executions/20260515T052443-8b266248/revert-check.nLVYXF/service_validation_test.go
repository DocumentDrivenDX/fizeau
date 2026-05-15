package fizeau

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestExecuteRejectsUnsupportedSubprocessModelBeforeRun(t *testing.T) {
	svc, err := New(ServiceOptions{
		ServiceConfig:       &fakeServiceConfig{},
		QuotaRefreshContext: canceledRefreshContext(),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ch, err := svc.Execute(context.Background(), ServiceExecuteRequest{
		Prompt:  "should not run",
		Harness: "codex",
		Model:   "not-a-real-model",
	})
	if err == nil {
		t.Fatal("expected Execute to return typed model incompatibility")
	}
	if ch != nil {
		t.Fatalf("expected no event channel for typed pre-resolution error, got %#v", ch)
	}
	if !errors.Is(err, ErrHarnessModelIncompatible{}) {
		t.Fatalf("errors.Is should match ErrHarnessModelIncompatible: %T %v", err, err)
	}
	var typed *ErrHarnessModelIncompatible
	if !errors.As(err, &typed) {
		t.Fatalf("errors.As should extract ErrHarnessModelIncompatible: %T %v", err, err)
	}
	if typed.Harness != "codex" || typed.Model != "not-a-real-model" {
		t.Fatalf("typed error=%#v, want codex/not-a-real-model", typed)
	}
}

func TestExecuteRejectsUnsupportedSubprocessReasoningBeforeRun(t *testing.T) {
	svc, err := New(ServiceOptions{
		ServiceConfig:       &fakeServiceConfig{},
		QuotaRefreshContext: canceledRefreshContext(),
	})
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
