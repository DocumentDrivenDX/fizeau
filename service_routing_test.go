package agent

import (
	"context"
	"errors"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DocumentDrivenDX/agent/internal/harnesses"
	"github.com/DocumentDrivenDX/agent/internal/modelcatalog"
	"github.com/DocumentDrivenDX/agent/internal/routing"
)

func TestRouteCandidateFromInternalMapsFields(t *testing.T) {
	candidate := routing.Candidate{
		Harness:            "agent",
		Provider:           "local",
		Endpoint:           "primary",
		Model:              "model-a",
		Score:              42.5,
		CostUSDPer1kTokens: 0.012,
		CostSource:         routing.CostSourceCatalog,
		Eligible:           true,
		Reason:             "profile=cheap; score=42.5",
	}

	got := routeCandidateFromInternal(candidate)
	if got.Harness != candidate.Harness ||
		got.Provider != candidate.Provider ||
		got.Endpoint != candidate.Endpoint ||
		got.Model != candidate.Model ||
		got.Score != candidate.Score ||
		got.CostUSDPer1kTokens != candidate.CostUSDPer1kTokens ||
		got.CostSource != candidate.CostSource ||
		got.Eligible != candidate.Eligible {
		t.Fatalf("routeCandidateFromInternal()=%#v, want fields from %#v", got, candidate)
	}
	if got.Reason != candidate.Reason {
		t.Fatalf("eligible Reason=%q, want %q", got.Reason, candidate.Reason)
	}

	rejected := candidate
	rejected.Eligible = false
	rejected.Reason = "model not in harness allow-list"
	got = routeCandidateFromInternal(rejected)
	if got.Reason != rejected.Reason {
		t.Fatalf("rejected Reason=%q, want %q", got.Reason, rejected.Reason)
	}
}

func TestResolveRouteSuccessIncludesCandidates(t *testing.T) {
	svc := publicRouteTraceService(&fakeServiceConfig{
		providers: map[string]ServiceProviderEntry{
			"local": {Type: "test", BaseURL: "http://127.0.0.1:9999/v1", Model: "model-a"},
		},
		names:       []string{"local"},
		defaultName: "local",
	})

	dec, err := svc.ResolveRoute(context.Background(), RouteRequest{
		Harness: "agent",
		Model:   "model-a",
	})
	if err != nil {
		t.Fatalf("ResolveRoute: %v", err)
	}
	if dec == nil {
		t.Fatal("ResolveRoute returned nil decision")
	}
	if dec.Harness != "agent" || dec.Provider != "local" || dec.Model != "model-a" {
		t.Fatalf("decision=%#v, want agent/local/model-a", dec)
	}
	if len(dec.Candidates) != 1 {
		t.Fatalf("Candidates length=%d, want 1: %#v", len(dec.Candidates), dec.Candidates)
	}
	candidate := dec.Candidates[0]
	if !candidate.Eligible || candidate.Harness != "agent" || candidate.Provider != "local" || candidate.Model != "model-a" {
		t.Fatalf("candidate=%#v, want eligible agent/local/model-a", candidate)
	}
	if !strings.Contains(candidate.Reason, "score=") {
		t.Fatalf("eligible candidate Reason=%q, want scoring reason", candidate.Reason)
	}
}

func TestResolveRouteErrorIncludesCandidatesAndTraceError(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "redacted")
	t.Setenv("GOOGLE_API_KEY", "")
	t.Setenv("GOOGLE_GENAI_USE_VERTEXAI", "")
	t.Setenv("GOOGLE_GENAI_USE_GCA", "")
	t.Setenv("GEMINI_CLI_USE_COMPUTE_ADC", "")
	t.Setenv("CLOUD_SHELL", "")

	registry := harnesses.NewRegistry()
	registry.LookPath = func(file string) (string, error) {
		if file == "gemini" {
			return "/usr/bin/gemini", nil
		}
		return "", os.ErrNotExist
	}
	svc := &service{
		opts:     ServiceOptions{},
		registry: registry,
		hub:      newSessionHub(),
	}

	dec, err := svc.ResolveRoute(context.Background(), RouteRequest{
		Model: "minimax/minimax-m2.7",
	})
	if err == nil {
		t.Fatal("ResolveRoute expected no viable candidate error")
	}
	if dec == nil {
		t.Fatal("ResolveRoute error path returned nil decision")
	}
	if dec.Harness != "" || dec.Provider != "" || dec.Model != "" {
		t.Fatalf("error decision selected a candidate: %#v", dec)
	}
	if len(dec.Candidates) == 0 {
		t.Fatal("error decision Candidates is empty")
	}

	var noViable *routing.NoViableCandidateError
	if !errors.As(err, &noViable) {
		t.Fatalf("errors.As no viable: %T %v", err, err)
	}
	var traced DecisionWithCandidates
	if !errors.As(err, &traced) {
		t.Fatalf("errors.As DecisionWithCandidates: %T %v", err, err)
	}
	tracedCandidates := traced.RouteCandidates()
	if len(tracedCandidates) != len(dec.Candidates) {
		t.Fatalf("traced candidates length=%d, decision candidates length=%d", len(tracedCandidates), len(dec.Candidates))
	}
	tracedCandidates[0].Reason = "mutated"
	if dec.Candidates[0].Reason == "mutated" {
		t.Fatal("RouteCandidates returned an alias of the decision candidates")
	}

	if !strings.Contains(err.Error(), "no viable routing candidate") {
		t.Fatalf("error=%q, want no viable routing candidate detail", err.Error())
	}
}

func TestBuildRoutingInputsPopulatesEndpointProviderCostsFromCatalog(t *testing.T) {
	catalog := loadRoutingFixtureCatalog(t, `
version: 4
generated_at: 2026-04-22T00:00:00Z
models:
  alpha-provider-model:
    family: same
    status: active
    cost_input_per_m: 1
    cost_output_per_m: 3
    surfaces: {agent.openai: alpha-provider-model}
  beta-provider-model:
    family: same
    status: active
    cost_input_per_m: 2
    cost_output_per_m: 4
    surfaces: {agent.openai: beta-provider-model}
  gamma-provider-model:
    family: same
    status: active
    cost_input_per_m: 4
    cost_output_per_m: 8
    surfaces: {agent.openai: gamma-provider-model}
targets:
  alpha:
    family: same
    candidates: [alpha-provider-model]
  beta:
    family: same
    candidates: [beta-provider-model]
  gamma:
    family: same
    candidates: [gamma-provider-model]
`)
	restore := replaceRoutingCatalogForTest(t, catalog)
	defer restore()

	sc := &fakeServiceConfig{
		providers: map[string]ServiceProviderEntry{
			"alpha": {Type: "openai", BaseURL: "http://alpha.invalid/v1", Model: "alpha-provider-model"},
			"beta":  {Type: "openai", BaseURL: "http://beta.invalid/v1", Model: "beta-provider-model"},
			"gamma": {Type: "openai", BaseURL: "http://gamma.invalid/v1", Model: "gamma-provider-model"},
		},
		names:       []string{"alpha", "beta", "gamma"},
		defaultName: "alpha",
	}
	svc := &service{opts: ServiceOptions{ServiceConfig: sc}, registry: harnesses.NewRegistry()}
	in := svc.buildRoutingInputs(context.Background())

	got := providerCostsByName(in, "agent")
	assertProviderCost(t, got, "alpha", 0.002, routing.CostSourceCatalog)
	assertProviderCost(t, got, "beta", 0.003, routing.CostSourceCatalog)
	assertProviderCost(t, got, "gamma", 0.006, routing.CostSourceCatalog)
}

func TestSubscriptionEffectiveCostCurveByHarnessAndBand(t *testing.T) {
	catalog := loadRoutingFixtureCatalog(t, `
version: 4
generated_at: 2026-04-22T00:00:00Z
models:
  priced-model:
    family: priced
    status: active
    cost_input_per_m: 10
    cost_output_per_m: 10
    surfaces:
      agent.openai: priced-model
      codex: priced-model
      claude-code: priced-model
      gemini: priced-model
targets:
  priced:
    family: priced
    candidates: [priced-model]
`)
	svc := &service{}
	tests := []struct {
		name        string
		usedPercent int
		want        float64
	}{
		{name: "free", usedPercent: 70, want: 0},
		{name: "low", usedPercent: 75, want: 0.001},
		{name: "medium", usedPercent: 85, want: 0.003},
		{name: "high", usedPercent: 92, want: 0.012},
	}
	for _, harness := range []string{"claude", "codex", "gemini"} {
		for _, tt := range tests {
			t.Run(harness+"/"+tt.name, func(t *testing.T) {
				entry := routing.HarnessEntry{
					Name:             harness,
					IsSubscription:   true,
					DefaultModel:     "priced-model",
					QuotaPercentUsed: tt.usedPercent,
				}
				svc.applySubscriptionRoutingCost(&entry, catalog)
				if len(entry.Providers) != 1 {
					t.Fatalf("providers=%#v, want one subscription provider", entry.Providers)
				}
				got := entry.Providers[0]
				if got.CostUSDPer1kTokens != tt.want || got.CostSource != routing.CostSourceSubscription {
					t.Fatalf("effective cost=(%v,%q), want (%v,%q)", got.CostUSDPer1kTokens, got.CostSource, tt.want, routing.CostSourceSubscription)
				}
			})
		}
	}
}

func TestBuildRoutingInputsHonorsLocalCostOption(t *testing.T) {
	sc := &fakeServiceConfig{
		providers: map[string]ServiceProviderEntry{
			"local": {Type: "lmstudio", BaseURL: "http://127.0.0.1:1234/v1", Model: "local-model"},
		},
		names:       []string{"local"},
		defaultName: "local",
	}

	unsetSvc := &service{opts: ServiceOptions{ServiceConfig: sc}, registry: harnesses.NewRegistry()}
	unset := providerCostsByName(unsetSvc.buildRoutingInputs(context.Background()), "agent")
	assertProviderCost(t, unset, "local", 0, routing.CostSourceUnknown)

	setSvc := &service{
		opts:     ServiceOptions{ServiceConfig: sc, LocalCostUSDPer1kTokens: 0.0042},
		registry: harnesses.NewRegistry(),
	}
	set := providerCostsByName(setSvc.buildRoutingInputs(context.Background()), "agent")
	assertProviderCost(t, set, "local", 0.0042, routing.CostSourceUserConfig)
}

func TestResolveRouteNearQuotaClaudeDemotesBelowOpenRouter(t *testing.T) {
	catalog := loadRoutingFixtureCatalog(t, `
version: 4
generated_at: 2026-04-22T00:00:00Z
models:
  sonnet-4.6:
    family: claude-sonnet
    status: active
    cost_input_per_m: 3
    cost_output_per_m: 15
    surfaces:
      agent.openai: sonnet-4.6
      claude-code: sonnet-4.6
targets:
  standard:
    family: claude-sonnet
    candidates: [sonnet-4.6]
`)
	svc := &service{}

	claude := routing.HarnessEntry{
		Name:                "claude",
		Surface:             "claude",
		CostClass:           "medium",
		IsSubscription:      true,
		AutoRoutingEligible: true,
		Available:           true,
		QuotaOK:             true,
		QuotaPercentUsed:    92,
		QuotaTrend:          routing.QuotaTrendExhausting,
		SubscriptionOK:      true,
		DefaultModel:        "sonnet-4.6",
		ExactPinSupport:     true,
		SupportsTools:       true,
	}
	svc.applySubscriptionRoutingCost(&claude, catalog)

	openrouterProvider := routing.ProviderEntry{
		Name:          "openrouter",
		BaseURL:       "https://openrouter.ai/api/v1",
		DefaultModel:  "sonnet-4.6",
		SupportsTools: true,
	}
	svc.applyEndpointRoutingCost(&openrouterProvider, ServiceProviderEntry{
		Type:    "openrouter",
		BaseURL: "https://openrouter.ai/api/v1",
		Model:   "sonnet-4.6",
	}, catalog)

	in := routing.Inputs{
		Harnesses: []routing.HarnessEntry{
			claude,
			{
				Name:                "agent",
				Surface:             "embedded-openai",
				CostClass:           "medium",
				AutoRoutingEligible: true,
				Available:           true,
				QuotaOK:             true,
				SubscriptionOK:      true,
				ExactPinSupport:     true,
				SupportsTools:       true,
				Providers:           []routing.ProviderEntry{openrouterProvider},
			},
		},
		ObservedSpeedTPS: map[string]float64{
			// Neutralize Claude's near-quota score penalty so the final base
			// scores tie and the cost tiebreak is the deciding dimension.
			routing.ProviderModelKey("", "", "sonnet-4.6"): 1900,
		},
	}
	dec, err := routing.Resolve(routing.Request{
		Profile:            "standard",
		Model:              "sonnet-4.6",
		ProviderPreference: routing.ProviderPreferenceSubscriptionFirst,
	}, in)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if dec.Harness != "agent" || dec.Provider != "openrouter" {
		t.Fatalf("near-quota route selected harness=%q provider=%q, want agent/openrouter", dec.Harness, dec.Provider)
	}
	var claudeCandidate, openrouterCandidate routing.Candidate
	for _, candidate := range dec.Candidates {
		switch {
		case candidate.Harness == "claude":
			claudeCandidate = candidate
		case candidate.Harness == "agent" && candidate.Provider == "openrouter":
			openrouterCandidate = candidate
		}
	}
	if claudeCandidate.Harness == "" || openrouterCandidate.Harness == "" {
		t.Fatalf("expected claude and openrouter candidates in trace: %#v", dec.Candidates)
	}
	if claudeCandidate.Score != openrouterCandidate.Score {
		t.Fatalf("candidate scores should tie before cost tiebreak: claude=%.1f openrouter=%.1f", claudeCandidate.Score, openrouterCandidate.Score)
	}
	if claudeCandidate.CostSource != routing.CostSourceSubscription || !floatNear(claudeCandidate.CostUSDPer1kTokens, 0.0108) {
		t.Fatalf("claude cost metadata=%#v, want 92%% subscription cost 0.0108", claudeCandidate)
	}
	if openrouterCandidate.CostSource != routing.CostSourceCatalog || !floatNear(openrouterCandidate.CostUSDPer1kTokens, 0.009) {
		t.Fatalf("openrouter cost metadata=%#v, want catalog cost 0.009", openrouterCandidate)
	}
	if !(openrouterCandidate.CostUSDPer1kTokens < claudeCandidate.CostUSDPer1kTokens) {
		t.Fatalf("openrouter cost %.4f should be below claude %.4f", openrouterCandidate.CostUSDPer1kTokens, claudeCandidate.CostUSDPer1kTokens)
	}
}

func publicRouteTraceService(sc ServiceConfig) *service {
	return &service{
		opts:     ServiceOptions{ServiceConfig: sc},
		registry: harnesses.NewRegistry(),
		hub:      newSessionHub(),
	}
}

func loadRoutingFixtureCatalog(t *testing.T, contents string) *modelcatalog.Catalog {
	t.Helper()
	path := filepath.Join(t.TempDir(), "models.yaml")
	requireNoError(t, os.WriteFile(path, []byte(contents), 0o600))
	catalog, err := modelcatalog.Load(modelcatalog.LoadOptions{ManifestPath: path, RequireExternal: true})
	if err != nil {
		t.Fatalf("Load fixture catalog: %v", err)
	}
	return catalog
}

func replaceRoutingCatalogForTest(t *testing.T, catalog *modelcatalog.Catalog) func() {
	t.Helper()
	old := loadRoutingCatalog
	loadRoutingCatalog = func() (*modelcatalog.Catalog, error) {
		return catalog, nil
	}
	return func() { loadRoutingCatalog = old }
}

func providerCostsByName(in routing.Inputs, harness string) map[string]routing.ProviderEntry {
	out := make(map[string]routing.ProviderEntry)
	for _, h := range in.Harnesses {
		if h.Name != harness {
			continue
		}
		for _, p := range h.Providers {
			out[p.Name] = p
		}
	}
	return out
}

func assertProviderCost(t *testing.T, providers map[string]routing.ProviderEntry, name string, wantCost float64, wantSource string) {
	t.Helper()
	provider, ok := providers[name]
	if !ok {
		t.Fatalf("provider %q not found in %#v", name, providers)
	}
	if provider.CostUSDPer1kTokens != wantCost || provider.CostSource != wantSource {
		t.Fatalf("provider %q cost=(%v,%q), want (%v,%q)", name, provider.CostUSDPer1kTokens, provider.CostSource, wantCost, wantSource)
	}
}

func requireNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func floatNear(got, want float64) bool {
	return math.Abs(got-want) < 1e-12
}

func TestDecisionWithCandidatesCopiesInput(t *testing.T) {
	candidates := []RouteCandidate{{Harness: "agent", Reason: "original"}}
	err := withRouteCandidates(errors.New("no viable routing candidate"), candidates)

	candidates[0].Reason = "changed"
	var traced DecisionWithCandidates
	if !errors.As(err, &traced) {
		t.Fatalf("errors.As DecisionWithCandidates: %T %v", err, err)
	}
	got := traced.RouteCandidates()
	if len(got) != 1 || got[0].Reason != "original" {
		t.Fatalf("RouteCandidates=%#v, want copied original candidate", got)
	}
}
