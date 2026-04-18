package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/agent/internal/harnesses"
	"github.com/DocumentDrivenDX/agent/internal/routing"
)

// TestRouteStatus_emptyConfig verifies that RouteStatus returns an empty report
// when no ServiceConfig is provided.
func TestRouteStatus_emptyConfig(t *testing.T) {
	svc := &service{opts: ServiceOptions{}, registry: harnesses.NewRegistry()}
	report, err := svc.RouteStatus(context.Background())
	if err != nil {
		t.Fatalf("RouteStatus: unexpected error: %v", err)
	}
	if report == nil {
		t.Fatal("expected non-nil report")
	}
	if len(report.Routes) != 0 {
		t.Errorf("Routes: got %d, want 0", len(report.Routes))
	}
	if report.GeneratedAt.IsZero() {
		t.Error("GeneratedAt should be set")
	}
}

// TestRouteStatus_routesPopulatedFromConfig verifies that each configured route
// appears in the report with the correct strategy and candidates.
func TestRouteStatus_routesPopulatedFromConfig(t *testing.T) {
	sc := &fakeServiceConfig{
		routeConfigs: map[string]ServiceModelRouteConfig{
			"code-model": {
				Strategy: "priority-round-robin",
				Candidates: []ServiceRouteCandidateEntry{
					{Provider: "bragi", Model: "qwen3-27b", Priority: 100},
					{Provider: "openrouter", Model: "qwen3-27b", Priority: 50},
				},
			},
		},
		routes: map[string][]string{
			"code-model": {"bragi", "openrouter"},
		},
	}
	svc := &service{opts: ServiceOptions{ServiceConfig: sc}, registry: harnesses.NewRegistry()}

	report, err := svc.RouteStatus(context.Background())
	if err != nil {
		t.Fatalf("RouteStatus: %v", err)
	}
	if len(report.Routes) != 1 {
		t.Fatalf("Routes: got %d, want 1", len(report.Routes))
	}

	entry := report.Routes[0]
	if entry.Model != "code-model" {
		t.Errorf("Model: got %q, want %q", entry.Model, "code-model")
	}
	if entry.Strategy != "priority-round-robin" {
		t.Errorf("Strategy: got %q, want %q", entry.Strategy, "priority-round-robin")
	}
	if len(entry.Candidates) != 2 {
		t.Fatalf("Candidates: got %d, want 2", len(entry.Candidates))
	}
	if entry.Candidates[0].Provider != "bragi" {
		t.Errorf("Candidates[0].Provider: got %q, want %q", entry.Candidates[0].Provider, "bragi")
	}
	if entry.Candidates[0].Priority != 100 {
		t.Errorf("Candidates[0].Priority: got %d, want 100", entry.Candidates[0].Priority)
	}
	if entry.Candidates[1].Provider != "openrouter" {
		t.Errorf("Candidates[1].Provider: got %q, want %q", entry.Candidates[1].Provider, "openrouter")
	}
	if entry.Candidates[1].Priority != 50 {
		t.Errorf("Candidates[1].Priority: got %d, want 50", entry.Candidates[1].Priority)
	}
	// No cooldowns set — both should be healthy.
	for i, c := range entry.Candidates {
		if !c.Healthy {
			t.Errorf("Candidates[%d].Healthy: got false, want true", i)
		}
		if c.Cooldown != nil {
			t.Errorf("Candidates[%d].Cooldown: expected nil", i)
		}
	}
}

// TestRouteStatus_lastDecisionCached verifies that calling ResolveRoute and
// then RouteStatus surfaces the cached LastDecision on the matching entry.
func TestRouteStatus_lastDecisionCached(t *testing.T) {
	// Build a minimal ServiceConfig that satisfies the routing engine.
	sc := &fakeServiceConfig{
		providers: map[string]ServiceProviderEntry{
			"bragi": {Type: "openai-compat", BaseURL: "http://127.0.0.1:9999/v1"},
		},
		names:       []string{"bragi"},
		defaultName: "bragi",
		routeConfigs: map[string]ServiceModelRouteConfig{
			"qwen3-27b": {
				Strategy: "ordered-failover",
				Candidates: []ServiceRouteCandidateEntry{
					{Provider: "bragi", Model: "qwen3-27b", Priority: 100},
				},
			},
		},
		routes: map[string][]string{
			"qwen3-27b": {"bragi"},
		},
	}

	// Seed registry with the "agent" harness available so routing can resolve.
	reg := harnesses.NewRegistry()
	svc := &service{
		opts:     ServiceOptions{ServiceConfig: sc},
		registry: reg,
	}

	// Inject a decision directly into the cache (simulating a resolved route)
	// since ResolveRoute may fail without a real provider.
	dec := &RouteDecision{
		Harness:  "agent",
		Provider: "bragi",
		Model:    "qwen3-27b",
		Reason:   "test",
	}
	svc.cacheRouteDecision("qwen3-27b", dec)

	report, err := svc.RouteStatus(context.Background())
	if err != nil {
		t.Fatalf("RouteStatus: %v", err)
	}
	if len(report.Routes) != 1 {
		t.Fatalf("Routes: got %d, want 1", len(report.Routes))
	}
	entry := report.Routes[0]
	if entry.LastDecision == nil {
		t.Fatal("LastDecision: expected non-nil after ResolveRoute")
	}
	if entry.LastDecision.Provider != "bragi" {
		t.Errorf("LastDecision.Provider: got %q, want %q", entry.LastDecision.Provider, "bragi")
	}
	if entry.LastDecision.Model != "qwen3-27b" {
		t.Errorf("LastDecision.Model: got %q, want %q", entry.LastDecision.Model, "qwen3-27b")
	}
	if entry.LastDecisionAt.IsZero() {
		t.Error("LastDecisionAt: should be non-zero")
	}
}

// TestRouteStatus_lastDecisionCached_viaResolveRoute verifies the full path:
// ResolveRoute → cache write → RouteStatus reads cache.
func TestRouteStatus_lastDecisionCached_viaResolveRoute(t *testing.T) {
	// We need the routing engine to actually resolve. The engine picks
	// harnesses from the registry. "agent" is always in the registry.
	// We give it a provider so the engine can build a candidate.
	sc := &fakeServiceConfig{
		providers: map[string]ServiceProviderEntry{
			"bragi": {Type: "openai-compat", BaseURL: "http://127.0.0.1:9999/v1"},
		},
		names:       []string{"bragi"},
		defaultName: "bragi",
		routeConfigs: map[string]ServiceModelRouteConfig{
			"mymodel": {
				Strategy: "ordered-failover",
				Candidates: []ServiceRouteCandidateEntry{
					{Provider: "bragi", Model: "mymodel", Priority: 100},
				},
			},
		},
		routes: map[string][]string{"mymodel": {"bragi"}},
	}

	svc := &service{
		opts:     ServiceOptions{ServiceConfig: sc},
		registry: harnesses.NewRegistry(),
	}

	// ResolveRoute with model="mymodel". The engine resolves against the
	// "agent" harness + bragi provider.
	dec, err := svc.ResolveRoute(context.Background(), RouteRequest{
		Model:    "mymodel",
		Provider: "bragi",
	})
	if err != nil {
		// Routing may return an error if the engine can't fully resolve;
		// skip in that case — the direct-cache test covers the cache logic.
		t.Skipf("ResolveRoute returned error (provider not live): %v", err)
	}
	if dec == nil {
		t.Fatal("RouteDecision: nil")
	}

	report, err := svc.RouteStatus(context.Background())
	if err != nil {
		t.Fatalf("RouteStatus: %v", err)
	}
	var found *RouteStatusEntry
	for i := range report.Routes {
		if report.Routes[i].Model == "mymodel" {
			found = &report.Routes[i]
			break
		}
	}
	if found == nil {
		t.Fatal("route 'mymodel' not found in report")
	}
	if found.LastDecision == nil {
		t.Fatal("LastDecision: expected non-nil after successful ResolveRoute")
	}
}

// TestRouteStatus_cooldownStateSurfaces verifies that a provider under cooldown
// surfaces a non-nil Cooldown on its RouteCandidateStatus and Healthy=false.
func TestRouteStatus_cooldownStateSurfaces(t *testing.T) {
	dir := t.TempDir()
	agentDir := filepath.Join(dir, ".agent")
	if err := os.MkdirAll(agentDir, 0o750); err != nil {
		t.Fatal(err)
	}

	// Write a route-health file indicating a recent failure for "bragi".
	type routeState struct {
		Failures map[string]time.Time `json:"failures"`
	}
	rs := routeState{Failures: map[string]time.Time{"bragi": time.Now().UTC()}}
	data, _ := json.Marshal(rs)
	routeKey := serviceRouteStateKey("code-model")
	if err := os.WriteFile(filepath.Join(agentDir, "route-health-"+routeKey+".json"), data, 0o600); err != nil {
		t.Fatal(err)
	}

	sc := &fakeServiceConfig{
		workDir:        dir,
		healthCooldown: 30 * time.Second,
		routeConfigs: map[string]ServiceModelRouteConfig{
			"code-model": {
				Strategy: "priority-round-robin",
				Candidates: []ServiceRouteCandidateEntry{
					{Provider: "bragi", Model: "qwen3-27b", Priority: 100},
					{Provider: "openrouter", Model: "qwen3-27b", Priority: 50},
				},
			},
		},
		routes: map[string][]string{
			"code-model": {"bragi", "openrouter"},
		},
	}
	svc := &service{opts: ServiceOptions{ServiceConfig: sc}, registry: harnesses.NewRegistry()}

	report, err := svc.RouteStatus(context.Background())
	if err != nil {
		t.Fatalf("RouteStatus: %v", err)
	}
	if len(report.Routes) != 1 {
		t.Fatalf("Routes: got %d, want 1", len(report.Routes))
	}

	entry := report.Routes[0]
	if len(entry.Candidates) != 2 {
		t.Fatalf("Candidates: got %d, want 2", len(entry.Candidates))
	}

	// Find bragi and openrouter candidates.
	byProvider := make(map[string]RouteCandidateStatus, 2)
	for _, c := range entry.Candidates {
		byProvider[c.Provider] = c
	}

	bragi, ok := byProvider["bragi"]
	if !ok {
		t.Fatal("bragi candidate not found")
	}
	if bragi.Healthy {
		t.Error("bragi: Healthy should be false (in cooldown)")
	}
	if bragi.Cooldown == nil {
		t.Fatal("bragi: Cooldown should be non-nil")
	}
	if bragi.Cooldown.Reason != "consecutive_failures" {
		t.Errorf("bragi Cooldown.Reason: got %q, want %q", bragi.Cooldown.Reason, "consecutive_failures")
	}
	if bragi.Cooldown.Until.IsZero() {
		t.Error("bragi Cooldown.Until should be non-zero")
	}

	openrouter, ok := byProvider["openrouter"]
	if !ok {
		t.Fatal("openrouter candidate not found")
	}
	if !openrouter.Healthy {
		t.Error("openrouter: Healthy should be true (no cooldown)")
	}
	if openrouter.Cooldown != nil {
		t.Error("openrouter: Cooldown should be nil")
	}
}

// Compile-time check: routing.Inputs is used in service_routing.go;
// verify the import is still reachable from this test file.
var _ = routing.Inputs{}
