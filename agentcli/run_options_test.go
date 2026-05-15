package agentcli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/easel/fizeau"
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

func TestBuildServiceExecuteRequest_HarnessPolicyLeavesModelAndProviderUnsetUnlessExplicit(t *testing.T) {
	req := buildServiceExecuteRequest(serviceExecuteRequestParams{
		Prompt:           "hello",
		Harness:          "codex",
		Policy:           "default",
		SelectedProvider: "local",
		RequestedModel:   "qwen3.6-27b",
		ResolvedModel:    "Qwen3.6-27B-MLX-8bit",
	})
	if req.Harness != "codex" {
		t.Fatalf("Harness=%q, want codex", req.Harness)
	}
	if req.Model != "" {
		t.Fatalf("Model=%q, want empty so the service can resolve within codex", req.Model)
	}
	if req.Provider != "" {
		t.Fatalf("Provider=%q, want empty so the service does not inherit the default local provider", req.Provider)
	}

	explicit := buildServiceExecuteRequest(serviceExecuteRequestParams{
		Prompt:           "hello",
		Harness:          "codex",
		SelectedProvider: "openrouter",
		ResolvedModel:    "gpt-5.4",
		ExplicitProvider: true,
		ExplicitModel:    true,
	})
	if explicit.Model != "gpt-5.4" {
		t.Fatalf("explicit Model=%q, want gpt-5.4", explicit.Model)
	}
	if explicit.Provider != "openrouter" {
		t.Fatalf("explicit Provider=%q, want openrouter", explicit.Provider)
	}
}

func TestRunRejectsLegacyModelFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer
	cmd := MountCLI(WithStdout(&stdout), WithStderr(&stderr))
	legacyFlag := "--model" + "-ref"
	cmd.SetArgs([]string{legacyFlag, "x", "run", "hello"})

	err := cmd.Execute()
	if err == nil {
		t.Fatalf("Execute succeeded; stdout=%s stderr=%s", stdout.String(), stderr.String())
	}
	if !strings.Contains(err.Error(), "unknown flag: "+legacyFlag) {
		t.Fatalf("Execute error = %v, want cobra unknown flag for legacy model flag", err)
	}
}

func TestRunRejectsProfileFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer
	cmd := MountCLI(WithStdout(&stdout), WithStderr(&stderr))
	cmd.SetArgs([]string{"--profile", "x", "run", "hello"})

	err := cmd.Execute()
	if err == nil {
		t.Fatalf("Execute succeeded; stdout=%s stderr=%s", stdout.String(), stderr.String())
	}
	if !strings.Contains(err.Error(), "unknown flag: --profile") {
		t.Fatalf("Execute error = %v, want cobra unknown flag for --profile", err)
	}
}
