package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/agent/internal/harnesses"
)

func TestExecuteHTTPProviderHarnessResolvesByType(t *testing.T) {
	cases := []struct {
		name         string
		harness      string
		providerName string
		providerType string
	}{
		{name: "lmstudio", harness: "lmstudio", providerName: "bragi", providerType: "lmstudio"},
		{name: "omlx", harness: "omlx", providerName: "vidar-omlx", providerType: "omlx"},
		{name: "openrouter", harness: "openrouter", providerName: "router", providerType: "openrouter"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := openAIChatServer(t, "pong")
			defer srv.Close()

			sc := &fakeServiceConfig{
				providers: map[string]ServiceProviderEntry{
					tc.providerName: {
						Type:    tc.providerType,
						BaseURL: srv.URL + "/v1",
						APIKey:  "test",
						Model:   "configured-model",
					},
				},
				names:       []string{tc.providerName},
				defaultName: tc.providerName,
			}
			svc, err := New(ServiceOptions{ServiceConfig: sc})
			if err != nil {
				t.Fatalf("New: %v", err)
			}

			final := executeAndFinal(t, svc, ServiceExecuteRequest{
				Prompt:          "ping",
				Harness:         tc.harness,
				Timeout:         5 * time.Second,
				ProviderTimeout: 2 * time.Second,
			})
			if final.Status != "success" {
				t.Fatalf("Status = %q, want success (error=%q)", final.Status, final.Error)
			}
			if final.RoutingActual == nil {
				t.Fatal("RoutingActual is nil")
			}
			if final.RoutingActual.Harness != tc.harness {
				t.Fatalf("RoutingActual.Harness = %q, want %q", final.RoutingActual.Harness, tc.harness)
			}
			if final.RoutingActual.Provider != tc.providerName {
				t.Fatalf("RoutingActual.Provider = %q, want %q", final.RoutingActual.Provider, tc.providerName)
			}
			if final.RoutingActual.Model != "configured-model" {
				t.Fatalf("RoutingActual.Model = %q, want configured-model", final.RoutingActual.Model)
			}
		})
	}
}

func TestResolveConfiguredNativeProviderSelectionOrder(t *testing.T) {
	t.Run("exact name wins over type fallback", func(t *testing.T) {
		sc := &fakeServiceConfig{
			providers: map[string]ServiceProviderEntry{
				"lmstudio": {Type: "openai", BaseURL: "http://exact.invalid/v1", Model: "exact-model"},
				"bragi":    {Type: "lmstudio", BaseURL: "http://bragi.invalid/v1", Model: "type-model"},
			},
			names:       []string{"bragi", "lmstudio"},
			defaultName: "bragi",
		}
		svc := &service{opts: ServiceOptions{ServiceConfig: sc}}
		got := svc.resolveConfiguredNativeProvider(ServiceExecuteRequest{Provider: "lmstudio"})
		if got.Name != "lmstudio" {
			t.Fatalf("resolved provider = %q, want literal lmstudio", got.Name)
		}
		if got.Entry.Model != "exact-model" {
			t.Fatalf("resolved model = %q, want exact-model", got.Entry.Model)
		}
	})

	t.Run("matching default provider is preferred", func(t *testing.T) {
		sc := &fakeServiceConfig{
			providers: map[string]ServiceProviderEntry{
				"bragi": {Type: "lmstudio", BaseURL: "http://bragi.invalid/v1"},
				"vidar": {Type: "lmstudio", BaseURL: "http://vidar.invalid/v1"},
			},
			names:       []string{"vidar", "bragi"},
			defaultName: "bragi",
		}
		svc := &service{opts: ServiceOptions{ServiceConfig: sc}}
		got := svc.resolveConfiguredNativeProvider(ServiceExecuteRequest{Harness: "lmstudio"})
		if got.Name != "bragi" {
			t.Fatalf("resolved provider = %q, want default bragi", got.Name)
		}
	})

	t.Run("stable order is used when default type mismatches", func(t *testing.T) {
		sc := &fakeServiceConfig{
			providers: map[string]ServiceProviderEntry{
				"router": {Type: "openrouter", BaseURL: "http://router.invalid/v1"},
				"vidar":  {Type: "lmstudio", BaseURL: "http://vidar.invalid/v1"},
				"bragi":  {Type: "lmstudio", BaseURL: "http://bragi.invalid/v1"},
			},
			names:       []string{"router", "vidar", "bragi"},
			defaultName: "router",
		}
		svc := &service{opts: ServiceOptions{ServiceConfig: sc}}
		got := svc.resolveConfiguredNativeProvider(ServiceExecuteRequest{Harness: "lmstudio"})
		if got.Name != "vidar" {
			t.Fatalf("resolved provider = %q, want first matching ProviderNames entry vidar", got.Name)
		}
	})

	t.Run("agent harness still falls back to default provider", func(t *testing.T) {
		sc := &fakeServiceConfig{
			providers: map[string]ServiceProviderEntry{
				"bragi": {Type: "lmstudio", BaseURL: "http://bragi.invalid/v1"},
			},
			names:       []string{"bragi"},
			defaultName: "bragi",
		}
		svc := &service{opts: ServiceOptions{ServiceConfig: sc}}
		got := svc.resolveConfiguredNativeProvider(ServiceExecuteRequest{Harness: "agent"})
		if got.Name != "bragi" {
			t.Fatalf("resolved provider = %q, want default bragi", got.Name)
		}
	})
}

func TestExecuteHTTPProviderHarnessNoMatchDiagnostic(t *testing.T) {
	sc := &fakeServiceConfig{
		providers: map[string]ServiceProviderEntry{
			"router":     {Type: "openrouter", BaseURL: "http://router.invalid/v1"},
			"vidar-omlx": {Type: "omlx", BaseURL: "http://omlx.invalid/v1"},
		},
		names:       []string{"router", "vidar-omlx"},
		defaultName: "router",
	}
	svc, err := New(ServiceOptions{ServiceConfig: sc})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	final := executeAndFinal(t, svc, ServiceExecuteRequest{
		Prompt:  "ping",
		Harness: "lmstudio",
		Timeout: 2 * time.Second,
	})
	if final.Status != "failed" {
		t.Fatalf("Status = %q, want failed", final.Status)
	}
	for _, want := range []string{
		`harness "lmstudio": no configured provider matches type "lmstudio"`,
		"router (openrouter)",
		"vidar-omlx (omlx)",
	} {
		if !strings.Contains(final.Error, want) {
			t.Fatalf("error %q does not contain %q", final.Error, want)
		}
	}
}

func openAIChatServer(t *testing.T, content string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":      "chatcmpl-test",
			"object":  "chat.completion",
			"created": time.Now().Unix(),
			"model":   "server-model",
			"choices": []map[string]any{
				{
					"index": 0,
					"message": map[string]any{
						"role":    "assistant",
						"content": content,
					},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]int{
				"prompt_tokens":     1,
				"completion_tokens": 1,
				"total_tokens":      2,
			},
		})
	}))
}

func executeAndFinal(t *testing.T, svc DdxAgent, req ServiceExecuteRequest) harnesses.FinalData {
	t.Helper()
	ch, err := svc.Execute(context.Background(), req)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	var final *harnesses.FinalData
	timer := time.NewTimer(10 * time.Second)
	defer timer.Stop()
	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				if final == nil {
					t.Fatal("execute channel closed without final event")
				}
				return *final
			}
			if ev.Type == harnesses.EventTypeFinal {
				var parsed harnesses.FinalData
				if err := json.Unmarshal(ev.Data, &parsed); err != nil {
					t.Fatalf("unmarshal final: %v", err)
				}
				final = &parsed
			}
		case <-timer.C:
			t.Fatal("timed out waiting for final event")
		}
	}
}
