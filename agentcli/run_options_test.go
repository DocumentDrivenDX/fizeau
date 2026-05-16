package agentcli

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
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
		SelectedRoute:    "local",
		RequestedModel:   "qwen3.6-27b",
		ResolvedModel:    "Qwen3.6-27B-MLX-8bit",
	})
	if req.Harness != "codex" {
		t.Fatalf("Harness=%q, want codex", req.Harness)
	}
	if req.Model != "Qwen3.6-27B-MLX-8bit" {
		t.Fatalf("Model=%q, want resolved model", req.Model)
	}
	if req.Provider != "local" {
		t.Fatalf("Provider=%q, want local", req.Provider)
	}
	if req.SelectedRoute != "local" {
		t.Fatalf("SelectedRoute=%q, want local", req.SelectedRoute)
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

func TestRun_UnpinnedRequestDelegatesProviderSelectionToService(t *testing.T) {
	gotReq, providerCalls, code, stdout, stderr := runAndCaptureServiceRequest(t, []string{"run", "-p", "hello"}, `
providers:
  local:
    type: lmstudio
    base_url: {{BASE_URL}}/v1
    api_key: test
    model: configured-model
default: local
`)
	if code != 0 {
		t.Fatalf("Run exit = %d, want 0; stdout=%s stderr=%s", code, stdout, stderr)
	}
	if gotReq.Harness != "" || gotReq.Provider != "" || gotReq.Model != "" || gotReq.SelectedRoute != "" {
		t.Fatalf("request = harness %q provider %q model %q selected_route %q, want service-owned unpinned routing", gotReq.Harness, gotReq.Provider, gotReq.Model, gotReq.SelectedRoute)
	}
	if providerCalls != 0 {
		t.Fatalf("provider was contacted %d times before service routing; want 0", providerCalls)
	}
}

func TestRun_UnpinnedModelIntentDelegatesProviderSelectionToService(t *testing.T) {
	gotReq, providerCalls, code, stdout, stderr := runAndCaptureServiceRequest(t, []string{"run", "--model", "qwen3.5-27b", "-p", "hello"}, `
providers:
  local:
    type: lmstudio
    base_url: {{BASE_URL}}/v1
    api_key: test
default: local
`)
	if code != 0 {
		t.Fatalf("Run exit = %d, want 0; stdout=%s stderr=%s", code, stdout, stderr)
	}
	if gotReq.Harness != "" || gotReq.Provider != "" || gotReq.Model != "qwen3.5-27b" || gotReq.SelectedRoute != "qwen3.5-27b" {
		t.Fatalf("request = harness %q provider %q model %q selected_route %q, want service-owned model intent", gotReq.Harness, gotReq.Provider, gotReq.Model, gotReq.SelectedRoute)
	}
	if providerCalls != 0 {
		t.Fatalf("provider was contacted %d times before service routing; want 0", providerCalls)
	}
}

func TestRun_DefaultModelIntentDelegatesProviderSelectionToService(t *testing.T) {
	gotReq, providerCalls, code, stdout, stderr := runAndCaptureServiceRequest(t, []string{"run", "-p", "hello"}, `
providers:
  local:
    type: lmstudio
    base_url: {{BASE_URL}}/v1
    api_key: test
default: local
routing:
  default_model: qwen3.5-27b
`)
	if code != 0 {
		t.Fatalf("Run exit = %d, want 0; stdout=%s stderr=%s", code, stdout, stderr)
	}
	if gotReq.Harness != "" || gotReq.Provider != "" || gotReq.Model != "" || gotReq.SelectedRoute != "" {
		t.Fatalf("request = harness %q provider %q model %q selected_route %q, want service-owned default model intent", gotReq.Harness, gotReq.Provider, gotReq.Model, gotReq.SelectedRoute)
	}
	if providerCalls != 0 {
		t.Fatalf("provider was contacted %d times before service routing; want 0", providerCalls)
	}
}

func runAndCaptureServiceRequest(t *testing.T, args []string, configBody string) (fizeau.ServiceExecuteRequest, int32, int, string, string) {
	t.Helper()
	oldExecuteViaService := executeViaServiceFn
	t.Cleanup(func() { executeViaServiceFn = oldExecuteViaService })
	isolateCatalogHome(t)
	t.Setenv("FIZEAU_PROVIDER", "")
	t.Setenv("FIZEAU_BASE_URL", "")
	t.Setenv("FIZEAU_API_KEY", "")
	t.Setenv("FIZEAU_MODEL", "")
	t.Setenv("FIZEAU_SKILLS_DIR", "-")

	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		http.Error(w, "provider should not be reached before service routing", http.StatusServiceUnavailable)
	}))
	t.Cleanup(server.Close)

	var gotReq fizeau.ServiceExecuteRequest
	executeViaServiceFn = func(ctx context.Context, req fizeau.ServiceExecuteRequest, selection providerSelection, logDir string, serviceConfig fizeau.ServiceConfig) (cliExecutionResult, error) {
		gotReq = req
		return cliExecutionResult{Status: "success"}, nil
	}

	workDir := t.TempDir()
	writeRunConfigFixture(t, workDir, strings.ReplaceAll(configBody, "{{BASE_URL}}", server.URL))
	runArgs := append([]string{"--work-dir", workDir}, args...)
	var stdout, stderr bytes.Buffer
	code := Run(Options{Args: runArgs, Stdout: &stdout, Stderr: &stderr})
	return gotReq, calls.Load(), code, stdout.String(), stderr.String()
}

func writeRunConfigFixture(t *testing.T, workDir, body string) {
	t.Helper()
	cfgDir := filepath.Join(workDir, ".fizeau")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "config.yaml"), []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
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
