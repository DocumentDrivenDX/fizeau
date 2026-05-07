package fizeau

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/fizeau/internal/harnesses"
	"github.com/DocumentDrivenDX/fizeau/internal/serviceimpl"
)

func TestExecuteExplicitHarnessPinsDispatchRequestedRunner(t *testing.T) {
	svc := publicRouteTraceService(&fakeServiceConfig{
		providers: map[string]ServiceProviderEntry{
			"anthropic":  {Type: "anthropic", APIKey: "sk-test"},
			"openrouter": {Type: "openrouter"},
		},
		names:       []string{"anthropic", "openrouter"},
		defaultName: "anthropic",
	})

	cases := []struct {
		name           string
		req            ServiceExecuteRequest
		wantHarness    string
		wantNative     bool
		wantSubprocess bool
	}{
		{
			name: "codex",
			req: ServiceExecuteRequest{
				Prompt:   "hello",
				Harness:  "codex",
				Provider: "anthropic",
				Model:    "gpt-5.4",
			},
			wantHarness:    "codex",
			wantSubprocess: true,
		},
		{
			name: "pi",
			req: ServiceExecuteRequest{
				Prompt:   "hello",
				Harness:  "pi",
				Provider: "openrouter",
				Model:    "gemini-2.5-flash",
			},
			wantHarness:    "pi",
			wantSubprocess: true,
		},
		{
			name: "opencode",
			req: ServiceExecuteRequest{
				Prompt:   "hello",
				Harness:  "opencode",
				Provider: "anthropic",
				Model:    "opencode/gpt-5.4",
			},
			wantHarness:    "opencode",
			wantSubprocess: true,
		},
		{
			name: "fiz",
			req: ServiceExecuteRequest{
				Prompt:   "hello",
				Harness:  "fiz",
				Provider: "openrouter",
				Model:    "gpt-5.4",
			},
			wantHarness: "fiz",
			wantNative:  true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			decision, err := svc.resolveExecuteRoute(tc.req)
			if err != nil {
				t.Fatalf("resolveExecuteRoute: %v", err)
			}
			if decision == nil {
				t.Fatal("resolveExecuteRoute returned nil decision")
			}
			if decision.Harness != tc.wantHarness {
				t.Fatalf("decision.Harness = %q, want %q", decision.Harness, tc.wantHarness)
			}

			var gotNative bool
			var gotSubprocess bool
			var gotRunner string
			serviceimpl.DispatchExecuteRun(context.Background(), serviceimpl.ExecuteDispatchRequest{
				Decision: serviceimpl.ExecuteRunnerDecision{
					Harness:  decision.Harness,
					Provider: decision.Provider,
					Model:    decision.Model,
				},
				Started: time.Now(),
			}, serviceimpl.ExecuteDispatchCallbacks{
				RunNative: func(ctx context.Context) {
					gotNative = true
				},
				RunSubprocess: func(ctx context.Context, runner harnesses.Harness) {
					gotSubprocess = true
					gotRunner = runner.Info().Name
				},
				RunVirtual: func(ctx context.Context) {
					t.Fatal("unexpected virtual dispatch")
				},
				RunScript: func(ctx context.Context) {
					t.Fatal("unexpected script dispatch")
				},
				IsHTTPProvider: func(string) bool {
					return false
				},
				Finalize: func(harnesses.FinalData) {
				},
			})

			if gotNative != tc.wantNative {
				t.Fatalf("RunNative called = %v, want %v", gotNative, tc.wantNative)
			}
			if gotSubprocess != tc.wantSubprocess {
				t.Fatalf("RunSubprocess called = %v, want %v", gotSubprocess, tc.wantSubprocess)
			}
			if tc.wantSubprocess && gotRunner != tc.wantHarness {
				t.Fatalf("subprocess runner = %q, want %q", gotRunner, tc.wantHarness)
			}
		})
	}
}

func TestExecuteExplicitHarnessPinUnknownHarnessFailsWithoutBroaderDispatch(t *testing.T) {
	svc := publicRouteTraceService(&fakeServiceConfig{
		providers: map[string]ServiceProviderEntry{
			"anthropic":  {Type: "anthropic", APIKey: "sk-test"},
			"openrouter": {Type: "openrouter"},
		},
		names:       []string{"anthropic", "openrouter"},
		defaultName: "anthropic",
	})

	ch, err := svc.Execute(context.Background(), ServiceExecuteRequest{
		Prompt:   "hello",
		Harness:  "does-not-exist",
		Provider: "anthropic",
		Model:    "gpt-5.4",
	})
	if err != nil {
		t.Fatalf("Execute: unexpected synchronous error: %v", err)
	}
	final := readFinalEvent(t, ch, 5*time.Second)
	if final.Status != "failed" {
		t.Fatalf("final status = %q, want failed", final.Status)
	}
	if !strings.Contains(final.Error, "unknown harness") {
		t.Fatalf("final error = %q, want unknown harness", final.Error)
	}
}

func TestExecuteExplicitAgentHarnessNoLongerAliasesNative(t *testing.T) {
	svc := publicRouteTraceService(&fakeServiceConfig{
		providers: map[string]ServiceProviderEntry{
			"openrouter": {Type: "openrouter"},
		},
		names:       []string{"openrouter"},
		defaultName: "openrouter",
	})

	ch, err := svc.Execute(context.Background(), ServiceExecuteRequest{
		Prompt:   "hello",
		Harness:  "agent",
		Provider: "openrouter",
		Model:    "gpt-5.4",
	})
	if err != nil {
		t.Fatalf("Execute: unexpected synchronous error: %v", err)
	}
	final := readFinalEvent(t, ch, 5*time.Second)
	if final.Status != "failed" || !strings.Contains(final.Error, `unknown harness "agent"`) {
		t.Fatalf("final = %#v, want unknown harness for agent", final)
	}
}

func TestExecuteExplicitHarnessPinRejectsUnsupportedCodexCombinationWithoutBroadening(t *testing.T) {
	catalog := loadRoutingFixtureCatalog(t, `
version: 4
generated_at: 2026-05-06T00:00:00Z
models:
  small-ctx-model:
    family: test
    status: active
    surfaces: {agent.openai: small-ctx-model}
targets:
  small-ctx:
    family: test
    candidates: [small-ctx-model]
`)
	t.Cleanup(replaceRoutingCatalogForTest(t, catalog))

	svc := publicRouteTraceService(&fakeServiceConfig{
		providers: map[string]ServiceProviderEntry{
			"local": {Type: "lmstudio", BaseURL: "http://127.0.0.1:9999/v1", Model: "small-ctx-model"},
		},
		names:       []string{"local"},
		defaultName: "local",
	})

	req := ServiceExecuteRequest{
		Prompt:   "hello",
		Harness:  "codex",
		Provider: "local",
		Model:    "small-ctx-model",
	}

	ch, err := svc.Execute(context.Background(), req)
	if err == nil {
		t.Fatal("expected explicit codex pin to fail before dispatch")
	}
	if ch != nil {
		t.Fatalf("expected no event channel for typed pre-resolution error, got %#v", ch)
	}

	var typed *ErrHarnessModelIncompatible
	if !errors.As(err, &typed) {
		t.Fatalf("errors.As should extract ErrHarnessModelIncompatible: %T %v", err, err)
	}
	if typed.Harness != "codex" {
		t.Fatalf("typed Harness = %q, want codex", typed.Harness)
	}
	if typed.Model != "small-ctx-model" {
		t.Fatalf("typed Model = %q, want small-ctx-model", typed.Model)
	}

	// Without the explicit harness pin, the same request is routable.
	decision, routeErr := svc.ResolveRoute(context.Background(), RouteRequest{
		Provider: req.Provider,
		Model:    req.Model,
	})
	if routeErr != nil {
		t.Fatalf("ResolveRoute without harness pin: %v", routeErr)
	}
	if decision == nil {
		t.Fatal("ResolveRoute without harness pin returned nil decision")
	}
	if decision.Harness == "codex" {
		t.Fatalf("ResolveRoute without harness pin still selected codex: %#v", decision)
	}
	if decision.Provider != "local" {
		t.Fatalf("ResolveRoute without harness pin provider = %q, want local", decision.Provider)
	}
}

func readFinalEvent(t *testing.T, ch <-chan ServiceEvent, timeout time.Duration) ServiceFinalData {
	t.Helper()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				t.Fatal("Execute channel closed without final event")
			}
			if ev.Type != "final" {
				continue
			}
			var payload ServiceFinalData
			if err := json.Unmarshal(ev.Data, &payload); err != nil {
				t.Fatalf("unmarshal final event: %v", err)
			}
			return payload
		case <-deadline.C:
			t.Fatalf("timed out after %s waiting for final event", timeout)
			return ServiceFinalData{}
		}
	}
}
