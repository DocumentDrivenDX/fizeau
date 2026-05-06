package agentcli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/DocumentDrivenDX/fizeau"
)

func TestRun_UsesInjectedOutputAndDoesNotExit(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run(Options{
		Args:      []string{"--version"},
		Stdout:    &stdout,
		Stderr:    &stderr,
		Version:   "v-test",
		BuildTime: "2026-04-30T00:00:00Z",
		GitCommit: "abc123",
	})
	if code != 0 {
		t.Fatalf("Run exit = %d, want 0", code)
	}
	if got := stdout.String(); !strings.Contains(got, "fiz v-test") || !strings.Contains(got, "abc123") {
		t.Fatalf("stdout = %q, want injected version output", got)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestRun_PassesHarnessPinIntoServiceRequest(t *testing.T) {
	oldExecuteViaService := executeViaServiceFn
	t.Cleanup(func() {
		executeViaServiceFn = oldExecuteViaService
	})

	var gotReq fizeau.ServiceExecuteRequest
	executeViaServiceFn = func(ctx context.Context, req fizeau.ServiceExecuteRequest, selection providerSelection, logDir string, serviceConfig fizeau.ServiceConfig) (cliExecutionResult, error) {
		gotReq = req
		return cliExecutionResult{Status: "success"}, nil
	}

	t.Setenv("FIZEAU_PROVIDER", "lmstudio")
	t.Setenv("FIZEAU_BASE_URL", "http://localhost:1234/v1")
	t.Setenv("FIZEAU_MODEL", "test-model")

	var stdout, stderr bytes.Buffer
	code := Run(Options{
		Args:   []string{"--harness", "codex", "-p", "hello"},
		Stdout: &stdout,
		Stderr: &stderr,
	})
	if code != 0 {
		t.Fatalf("Run exit = %d, want 0", code)
	}
	if gotReq.Harness != "codex" {
		t.Fatalf("request harness = %q, want codex", gotReq.Harness)
	}
	if gotReq.Prompt != "hello" {
		t.Fatalf("request prompt = %q, want hello", gotReq.Prompt)
	}
	if gotReq.Harness == "" {
		t.Fatal("request harness was empty")
	}
}
