package fizeau

import (
	"context"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/easel/fizeau/internal/discoverycache"
	"github.com/easel/fizeau/internal/harnesses"
	claudeharness "github.com/easel/fizeau/internal/harnesses/claude"
	"github.com/easel/fizeau/internal/modelcatalog"
	"github.com/easel/fizeau/internal/routing"
	"gopkg.in/yaml.v3"
)

func TestRouteCandidateFromInternalMapsFields(t *testing.T) {
	candidate := routing.Candidate{
		Harness:            "fiz",
		Provider:           "local",
		Endpoint:           "primary",
		Model:              "model-a",
		Score:              42.5,
		CostUSDPer1kTokens: 0.012,
		CostSource:         routing.CostSourceCatalog,
		Power:              7,
		ContextLength:      200000,
		ContextSource:      routing.ContextSourceCatalog,
		ContextHeadroom:    150000,
		Eligible:           true,
		Reason:             "profile=cheap; score=42.5",
		LatencyMS:          123,
		SpeedTPS:           55,
		SuccessRate:        0.8,
		CostClass:          "local",
		QuotaOK:            true,
		QuotaPercentUsed:   25,
		QuotaTrend:         routing.QuotaTrendHealthy,
		StickyAffinity:     10,
		ScoreComponents: map[string]float64{
			"base":                100,
			"cost":                -4,
			"deployment_locality": 12,
			"quota_health":        6,
			"utilization":         -3,
			"performance":         9,
			"power":               18,
			"context_headroom":    0.15,
			"sticky_affinity":     10,
		},
	}

	got := routeCandidateFromInternal(candidate, RoutePowerPolicy{MinPower: 6, MaxPower: 8})
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
	if got.Components.Power != 7 || got.Components.SpeedTPS != 55 || got.Components.QuotaPercentUsed != 25 || got.Components.QuotaTrend != routing.QuotaTrendHealthy {
		t.Fatalf("components=%#v, want power/speed/quota inputs from candidate", got.Components)
	}
	if got.Components.Utilization != 0 {
		t.Fatalf("components utilization=%v, want zero when unavailable", got.Components.Utilization)
	}
	if got.Components.StickyAffinity != 10 {
		t.Fatalf("components sticky affinity=%v, want 10 from sticky match", got.Components.StickyAffinity)
	}
	if got.Components.PowerWeightedCapability != 18 {
		t.Fatalf("components power_weighted_capability=%v, want 18", got.Components.PowerWeightedCapability)
	}
	if got.Components.PowerHintFit != 0 {
		t.Fatalf("components power_hint_fit=%v, want 0 within bounds", got.Components.PowerHintFit)
	}
	if got.Components.LatencyWeight != 9 {
		t.Fatalf("components latency_weight=%v, want 9", got.Components.LatencyWeight)
	}
	if got.Components.PlacementBonus != 12+10 {
		t.Fatalf("components placement_bonus=%v, want 22", got.Components.PlacementBonus)
	}
	if got.Components.QuotaBonus != 6 {
		t.Fatalf("components quota_bonus=%v, want 6", got.Components.QuotaBonus)
	}
	if got.Components.MarginalCostPenalty != 4 {
		t.Fatalf("components marginal_cost_penalty=%v, want 4", got.Components.MarginalCostPenalty)
	}
	if got.Components.AvailabilityPenalty != 3 {
		t.Fatalf("components availability_penalty=%v, want 3", got.Components.AvailabilityPenalty)
	}
	if got.Components.StaleSignalPenalty != 0 {
		t.Fatalf("components stale_signal_penalty=%v, want 0", got.Components.StaleSignalPenalty)
	}
	if got.ContextLength != candidate.ContextLength || got.ContextSource != candidate.ContextSource {
		t.Fatalf("context evidence=%d/%q, want %d/%q", got.ContextLength, got.ContextSource, candidate.ContextLength, candidate.ContextSource)
	}
	if got.Components.ContextHeadroom != candidate.ContextHeadroom {
		t.Fatalf("context headroom=%d, want %d", got.Components.ContextHeadroom, candidate.ContextHeadroom)
	}

	rejected := candidate
	rejected.Eligible = false
	rejected.Reason = "model not in harness allow-list"
	got = routeCandidateFromInternal(rejected, RoutePowerPolicy{})
	if got.Reason != rejected.Reason {
		t.Fatalf("rejected Reason=%q, want %q", got.Reason, rejected.Reason)
	}
}

func TestResolveRouteSuccessIncludesCandidates(t *testing.T) {
	svc := publicRouteTraceService(&fakeServiceConfig{
		providers: map[string]ServiceProviderEntry{
			"local": {Type: "test", BaseURL: "http://127.0.0.1:9999/v1", ServerInstance: "127.0.0.1:9999", Model: "model-a"},
		},
		names:       []string{"local"},
		defaultName: "local",
	})

	dec, err := svc.ResolveRoute(context.Background(), RouteRequest{
		Harness:               "fiz",
		Model:                 "model-a",
		EstimatedPromptTokens: 1_000,
	})
	if err != nil {
		t.Fatalf("ResolveRoute: %v", err)
	}
	if dec == nil {
		t.Fatal("ResolveRoute returned nil decision")
	}
	if dec.Harness != "fiz" || dec.Provider != "local" || dec.Model != "model-a" {
		t.Fatalf("decision=%#v, want fiz/local/model-a", dec)
	}
	if dec.ServerInstance == "" {
		t.Fatalf("decision=%#v, want server_instance", dec)
	}
	if len(dec.Candidates) != 1 {
		t.Fatalf("Candidates length=%d, want 1: %#v", len(dec.Candidates), dec.Candidates)
	}
	candidate := dec.Candidates[0]
	if !candidate.Eligible || candidate.Harness != "fiz" || candidate.Provider != "local" || candidate.Model != "model-a" {
		t.Fatalf("candidate=%#v, want eligible fiz/local/model-a", candidate)
	}
	if candidate.ServerInstance == "" {
		t.Fatalf("candidate=%#v, want server_instance", candidate)
	}
	if candidate.ContextLength == 0 || candidate.ContextSource != ContextSourceDefault {
		t.Fatalf("candidate context = %d/%q, want default context evidence", candidate.ContextLength, candidate.ContextSource)
	}
	if candidate.Components.ContextHeadroom == 0 {
		t.Fatalf("candidate context headroom should be populated for eligible candidates: %#v", candidate.Components)
	}
	if !strings.Contains(candidate.Reason, "score=") {
		t.Fatalf("eligible candidate Reason=%q, want scoring reason", candidate.Reason)
	}
}

func TestResolveRouteSnapshotProviderPowerCorrelation(t *testing.T) {
	t.Setenv("PATH", "")
	cacheDir := t.TempDir()
	t.Setenv("FIZEAU_CACHE_DIR", cacheDir)

	var probeHits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		probeHits.Add(1)
		w.WriteHeader(http.StatusTeapot)
	}))
	defer srv.Close()

	cache := &discoverycache.Cache{Root: cacheDir}
	capturedAt := time.Date(2026, 5, 12, 15, 0, 0, 0, time.UTC)
	writeSnapshotDiscoveryFixture(t, cache, testDiscoverySourceName("alpha", "alpha", srv.URL+"/v1", "alpha-1"), capturedAt, []string{"shared-model", "medium-model"})
	writeSnapshotDiscoveryFixture(t, cache, testDiscoverySourceName("beta", "beta", srv.URL+"/v1", "beta-1"), capturedAt, []string{"shared-model", "high-model"})
	writeSnapshotDiscoveryFixture(t, cache, testDiscoverySourceName("gamma", "gamma", srv.URL+"/v1", "gamma-1"), capturedAt, []string{"catalog-only-model"})

	catalog := loadRoutingFixtureCatalog(t, `
version: 5
generated_at: 2026-05-12T00:00:00Z
catalog_version: test
policies:
  default:
    min_power: 1
    max_power: 10
    allow_local: true
models:
  shared-model:
    family: shared
    status: active
    power: 6
    context_window: 16384
    surfaces:
      embedded-openai: shared-model
  medium-model:
    family: tier
    status: active
    power: 5
    context_window: 8192
    surfaces:
      embedded-openai: medium-model
  high-model:
    family: tier
    status: active
    power: 10
    context_window: 32768
    surfaces:
      embedded-openai: high-model
  catalog-only-model:
    family: tier
    status: active
    power: 8
    exact_pin_only: true
    context_window: 4096
    surfaces:
      embedded-openai: catalog-only-model
`)
	t.Cleanup(replaceRoutingCatalogForTest(t, catalog))

	sc := &fakeServiceConfig{
		providers: map[string]ServiceProviderEntry{
			"alpha": {
				Type:           "openai",
				BaseURL:        srv.URL + "/v1",
				APIKey:         "alpha-key",
				ServerInstance: "alpha-1",
				Model:          "medium-model",
			},
			"beta": {
				Type:           "openai",
				BaseURL:        srv.URL + "/v1",
				APIKey:         "beta-key",
				ServerInstance: "beta-1",
				Model:          "high-model",
			},
			"gamma": {
				Type:                "openai",
				BaseURL:             srv.URL + "/v1",
				APIKey:              "gamma-key",
				ServerInstance:      "gamma-1",
				Model:               "catalog-only-model",
				IncludeByDefault:    false,
				IncludeByDefaultSet: true,
			},
		},
		names:       []string{"alpha", "beta", "gamma"},
		defaultName: "alpha",
	}

	newSvc := func(t *testing.T) *service {
		t.Helper()
		return newTestService(t, ServiceOptions{ServiceConfig: sc})
	}

	t.Run("power", func(t *testing.T) {
		svc := newSvc(t)
		dec, err := svc.ResolveRoute(context.Background(), RouteRequest{})
		if err != nil {
			t.Fatalf("ResolveRoute: %v", err)
		}
		if probeHits.Load() != 0 {
			t.Fatalf("unexpected discovery probe count = %d", probeHits.Load())
		}
		if dec == nil {
			t.Fatal("ResolveRoute returned nil decision")
		}
		if dec.Provider != "beta" || dec.Model != "high-model" {
			t.Fatalf("decision=%#v, want snapshot-backed high-model winner", dec)
		}
	})

	t.Run("provider pin", func(t *testing.T) {
		svc := newSvc(t)
		dec, err := svc.ResolveRoute(context.Background(), RouteRequest{
			Provider: "alpha",
			Model:    "medium-model",
		})
		if err != nil {
			t.Fatalf("ResolveRoute: %v", err)
		}
		if dec == nil {
			t.Fatal("ResolveRoute returned nil decision")
		}
		if dec.Provider != "alpha" || dec.Model != "medium-model" {
			t.Fatalf("decision=%#v, want hard-pinned alpha/medium-model", dec)
		}
		if probeHits.Load() != 0 {
			t.Fatalf("unexpected discovery probe count = %d", probeHits.Load())
		}
	})

	t.Run("exact model pin", func(t *testing.T) {
		svc := newSvc(t)
		dec, err := svc.ResolveRoute(context.Background(), RouteRequest{
			Model: "catalog-only-model",
		})
		if err != nil {
			t.Fatalf("ResolveRoute: %v", err)
		}
		if dec == nil {
			t.Fatal("ResolveRoute returned nil decision")
		}
		if dec.Provider != "gamma" || dec.Model != "catalog-only-model" {
			t.Fatalf("decision=%#v, want exact-pinned gamma/catalog-only-model", dec)
		}
		if probeHits.Load() != 0 {
			t.Fatalf("unexpected discovery probe count = %d", probeHits.Load())
		}
	})

	t.Run("correlation", func(t *testing.T) {
		svc := newSvc(t)
		first, err := svc.ResolveRoute(context.Background(), RouteRequest{
			Model:         "shared-model",
			CorrelationID: "snapshot-sticky",
		})
		if err != nil {
			t.Fatalf("first ResolveRoute: %v", err)
		}
		second, err := svc.ResolveRoute(context.Background(), RouteRequest{
			Model:         "shared-model",
			CorrelationID: "snapshot-sticky",
		})
		if err != nil {
			t.Fatalf("second ResolveRoute: %v", err)
		}
		if first == nil || second == nil {
			t.Fatalf("decisions=%#v %#v, want non-nil", first, second)
		}
		if first.ServerInstance == "" || second.ServerInstance == "" {
			t.Fatalf("server instances=%q %q, want sticky selection", first.ServerInstance, second.ServerInstance)
		}
		if first.ServerInstance != second.ServerInstance {
			t.Fatalf("sticky server instance changed: first=%q second=%q", first.ServerInstance, second.ServerInstance)
		}
		if probeHits.Load() != 0 {
			t.Fatalf("unexpected discovery probe count = %d", probeHits.Load())
		}
	})

	if probeHits.Load() != 0 {
		t.Fatalf("unexpected discovery probe count = %d", probeHits.Load())
	}
}

func TestServiceRouteSnapshotCatalogOnlyModelRejected(t *testing.T) {
	catalog := loadRoutingFixtureCatalog(t, `
version: 5
generated_at: 2026-05-12T00:00:00Z
catalog_version: test
policies:
  default:
    min_power: 1
    max_power: 10
    allow_local: true
models:
  gpt-5.5:
    family: gpt
    status: active
    power: 10
    surfaces:
      embedded-openai: gpt-5.5
  catalog-only-model:
    family: test
    status: active
    power: 5
    exact_pin_only: true
    surfaces:
      embedded-openai: catalog-only-model
`)
	t.Cleanup(replaceRoutingCatalogForTest(t, catalog))

	svc := publicRouteTraceService(&fakeServiceConfig{
		providers: map[string]ServiceProviderEntry{
			"known":        {Type: "test", BaseURL: "http://known.invalid/v1", Model: "gpt-5.5"},
			"catalog-only": {Type: "test", BaseURL: "http://pin.invalid/v1", Model: "catalog-only-model"},
		},
		names:       []string{"known", "catalog-only"},
		defaultName: "known",
	})

	dec, err := svc.ResolveRoute(context.Background(), RouteRequest{})
	if err != nil {
		t.Fatalf("ResolveRoute: %v", err)
	}
	if dec == nil {
		t.Fatal("ResolveRoute returned nil decision")
	}
	if dec.Model != "gpt-5.5" {
		t.Fatalf("decision=%#v, want gpt-5.5 winner from snapshot-backed eligibility", dec)
	}
	var sawCatalogOnly bool
	for _, candidate := range dec.Candidates {
		if candidate.Provider != "catalog-only" {
			continue
		}
		sawCatalogOnly = true
		if candidate.Eligible {
			t.Fatalf("catalog-only candidate should be rejected by snapshot-backed eligibility: %#v", candidate)
		}
		if candidate.FilterReason != string(routing.FilterReasonExactPinOnly) {
			t.Fatalf("catalog-only FilterReason=%q, want %q", candidate.FilterReason, routing.FilterReasonExactPinOnly)
		}
	}
	if !sawCatalogOnly {
		t.Fatalf("missing catalog-only candidate in %#v", dec.Candidates)
	}
}

func TestServiceRouteHardPinBypassesSnapshotEligibility(t *testing.T) {
	catalog := loadRoutingFixtureCatalog(t, `
version: 5
generated_at: 2026-05-12T00:00:00Z
catalog_version: test
policies:
  default:
    min_power: 1
    max_power: 10
    allow_local: true
models:
  gpt-5.5:
    family: gpt
    status: active
    power: 10
    surfaces:
      embedded-openai: gpt-5.5
  catalog-only-model:
    family: test
    status: active
    power: 5
    exact_pin_only: true
    surfaces:
      embedded-openai: catalog-only-model
`)
	t.Cleanup(replaceRoutingCatalogForTest(t, catalog))

	svc := publicRouteTraceService(&fakeServiceConfig{
		providers: map[string]ServiceProviderEntry{
			"known":        {Type: "test", BaseURL: "http://known.invalid/v1", Model: "gpt-5.5"},
			"catalog-only": {Type: "test", BaseURL: "http://pin.invalid/v1", Model: "catalog-only-model"},
		},
		names:       []string{"known", "catalog-only"},
		defaultName: "known",
	})

	dec, err := svc.ResolveRoute(context.Background(), RouteRequest{
		Provider: "catalog-only",
		Model:    "catalog-only-model",
	})
	if err != nil {
		t.Fatalf("ResolveRoute hard pin: %v", err)
	}
	if dec == nil {
		t.Fatal("ResolveRoute hard pin returned nil decision")
	}
	if dec.Provider != "catalog-only" || dec.Model != "catalog-only-model" {
		t.Fatalf("decision=%#v, want hard-pinned catalog-only/model", dec)
	}
}

func TestServiceTranslatesPolicyAirGappedToRequireNoRemote(t *testing.T) {
	catalog := loadRoutingFixtureCatalog(t, `
version: 5
generated_at: 2026-05-08T00:00:00Z
catalog_version: test
policies:
  default:
    min_power: 5
    max_power: 8
    allow_local: true
  cheap:
    min_power: 5
    max_power: 5
    allow_local: true
  smart:
    min_power: 9
    max_power: 10
    allow_local: false
  air-gapped:
    min_power: 5
    max_power: 5
    allow_local: true
    require: [no_remote]
models:
  remote-model:
    family: example
    status: active
    provider_system: openrouter
    deployment_class: managed_cloud_frontier
    power: 5
    surfaces:
      agent.openai: remote-model
`)
	t.Cleanup(replaceRoutingCatalogForTest(t, catalog))

	svc := newTestService(t, ServiceOptions{
		ServiceConfig: &fakeServiceConfig{
			providers: map[string]ServiceProviderEntry{
				"openrouter": {Type: "openrouter", BaseURL: "http://remote.invalid/v1", Model: "remote-model"},
			},
			names:       []string{"openrouter"},
			defaultName: "openrouter",
		},
	})

	_, err := svc.ResolveRoute(context.Background(), RouteRequest{
		Policy:   "air-gapped",
		Provider: "openrouter",
	})
	if err == nil {
		t.Fatal("expected air-gapped policy to reject remote provider pin")
	}
	var typed *ErrPolicyRequirementUnsatisfied
	if !errors.As(err, &typed) {
		t.Fatalf("errors.As ErrPolicyRequirementUnsatisfied: %T %v", err, err)
	}
	if typed.Policy != "air-gapped" || typed.Requirement != "no_remote" || typed.AttemptedPin != "openrouter" {
		t.Fatalf("ErrPolicyRequirementUnsatisfied=%#v, want air-gapped/no_remote/openrouter", typed)
	}
}

func TestResolveRoutePolicyReportsEffectivePowerPolicy(t *testing.T) {
	catalog := loadRoutingFixtureCatalog(t, `
version: 5
generated_at: 2026-05-06T00:00:00Z
catalog_version: test
policies:
  default:
    min_power: 7
    max_power: 8
    allow_local: true
  smart:
    min_power: 9
    max_power: 10
    allow_local: false
models:
  provider-default:
    family: example
    status: active
    power: 7
    surfaces:
      agent.openai: provider-default
  catalog-smart:
    family: example
    status: active
    power: 9
    surfaces:
      agent.openai: catalog-smart
`)
	t.Cleanup(replaceRoutingCatalogForTest(t, catalog))

	svc := newTestService(t, ServiceOptions{
		ServiceConfig: &fakeServiceConfig{
			providers: map[string]ServiceProviderEntry{
				"local": {Type: "test", BaseURL: "http://127.0.0.1:9999/v1", Model: "provider-default"},
			},
			names:       []string{"local"},
			defaultName: "local",
		},
	})

	dec, err := svc.ResolveRoute(context.Background(), RouteRequest{Policy: "default"})
	if err != nil {
		t.Fatalf("ResolveRoute: %v", err)
	}
	if dec == nil {
		t.Fatal("ResolveRoute returned nil decision")
	}
	if dec.RequestedPolicy != "default" {
		t.Fatalf("RequestedPolicy=%q, want default", dec.RequestedPolicy)
	}
	if dec.PowerPolicy.PolicyName != "default" || dec.PowerPolicy.MinPower != 7 || dec.PowerPolicy.MaxPower != 8 {
		t.Fatalf("PowerPolicy=%#v, want default 7..8", dec.PowerPolicy)
	}
	if dec.Model != "provider-default" {
		t.Fatalf("Model=%q, want provider-default without treating policy as a model ref", dec.Model)
	}
}

func TestResolveRoutePolicyAppliesEffectivePowerPolicyBeforeFiltering(t *testing.T) {
	catalog := loadRoutingFixtureCatalog(t, `
version: 5
generated_at: 2026-05-06T00:00:00Z
catalog_version: test
policies:
  default:
    min_power: 7
    max_power: 8
    allow_local: true
models:
  power-5:
    family: example
    status: active
    power: 5
    surfaces:
      agent.openai: power-5
  power-7:
    family: example
    status: active
    power: 7
    surfaces:
      agent.openai: power-7
  power-9:
    family: example
    status: active
    power: 9
    surfaces:
      agent.openai: power-9
`)
	t.Cleanup(replaceRoutingCatalogForTest(t, catalog))

	svc := newTestService(t, ServiceOptions{
		ServiceConfig: &fakeServiceConfig{
			providers: map[string]ServiceProviderEntry{
				"power-5": {Type: "test", BaseURL: "http://127.0.0.1:1111/v1", Model: "power-5"},
				"power-7": {Type: "test", BaseURL: "http://127.0.0.1:2222/v1", Model: "power-7"},
				"power-9": {Type: "test", BaseURL: "http://127.0.0.1:3333/v1", Model: "power-9"},
			},
			names:       []string{"power-5", "power-7", "power-9"},
			defaultName: "power-7",
		},
	})

	dec, err := svc.ResolveRoute(context.Background(), RouteRequest{
		Policy: "default",
	})
	if err != nil {
		t.Fatalf("ResolveRoute: %v", err)
	}
	if dec == nil {
		t.Fatal("ResolveRoute returned nil decision")
	}
	if dec.Model != "power-7" {
		t.Fatalf("decision=%#v, want power-7 winner", dec)
	}
	if dec.RequestedPolicy != "default" {
		t.Fatalf("RequestedPolicy=%q, want default", dec.RequestedPolicy)
	}
	if dec.PowerPolicy.PolicyName != "default" || dec.PowerPolicy.MinPower != 7 || dec.PowerPolicy.MaxPower != 8 {
		t.Fatalf("PowerPolicy=%#v, want default 7..8", dec.PowerPolicy)
	}

	var sawBelowTarget, sawAboveTarget bool
	for _, candidate := range dec.Candidates {
		switch candidate.Model {
		case "power-5":
			if !candidate.Eligible {
				t.Fatalf("power-5 candidate should remain eligible under soft power scoring: %#v", candidate)
			}
			if candidate.FilterReason != "" {
				t.Fatalf("power-5 FilterReason=%q, want empty under soft power scoring", candidate.FilterReason)
			}
			sawBelowTarget = true
		case "power-7":
			if !candidate.Eligible {
				t.Fatalf("power-7 candidate should remain eligible under default: %#v", candidate)
			}
		case "power-9":
			if !candidate.Eligible {
				t.Fatalf("power-9 candidate should remain eligible under soft power scoring: %#v", candidate)
			}
			if candidate.FilterReason != "" {
				t.Fatalf("power-9 FilterReason=%q, want empty under soft power scoring", candidate.FilterReason)
			}
			sawAboveTarget = true
		}
	}
	if !sawBelowTarget || !sawAboveTarget {
		t.Fatalf("decision candidates did not cover the full power-policy trace: %#v", dec.Candidates)
	}
}

func TestProviderUsesLiveDiscovery_LlamaServer(t *testing.T) {
	if !providerUsesLiveDiscovery("llama-server") {
		t.Fatal("expected llama-server to use live discovery")
	}
}

func TestProviderTypeUsesFixedBilling_LlamaServer(t *testing.T) {
	if !providerTypeUsesFixedBilling("llama-server") {
		t.Fatal("expected llama-server to count as a fixed-billing endpoint")
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

	var noMatch *ErrModelConstraintNoMatch
	if !errors.As(err, &noMatch) {
		t.Fatalf("errors.As no-match: %T %v", err, err)
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

	if !strings.Contains(err.Error(), "no matching model") {
		t.Fatalf("error=%q, want no matching model detail", err.Error())
	}
}

func TestRoutingInputsUseClaudeQuotaWindows(t *testing.T) {
	dir := t.TempDir()
	cachePath := filepath.Join(dir, "claude-quota.json")
	t.Setenv("FIZEAU_CLAUDE_QUOTA_CACHE", cachePath)

	if err := claudeharness.WriteClaudeQuota(cachePath, claudeharness.ClaudeQuotaSnapshot{
		CapturedAt:        time.Now().UTC(),
		FiveHourRemaining: 90,
		FiveHourLimit:     100,
		WeeklyRemaining:   90,
		WeeklyLimit:       100,
		Source:            "pty",
		Account:           &harnesses.AccountInfo{PlanType: "Claude Max"},
		Windows: []harnesses.QuotaWindow{
			{Name: "extra", LimitID: "claude-extra", UsedPercent: 100, State: "exhausted"},
		},
	}); err != nil {
		t.Fatalf("WriteClaudeQuota: %v", err)
	}

	registry := harnesses.NewRegistry()
	registry.LookPath = func(file string) (string, error) {
		if file == "claude" {
			return "/usr/bin/claude", nil
		}
		return "", os.ErrNotExist
	}
	svc := &service{opts: ServiceOptions{}, registry: registry}

	inputs, _ := svc.buildRoutingInputsWithCatalog(context.Background(), nil)
	claudeEntry, ok := findRoutingHarnessEntry(inputs.Harnesses, "claude")
	if !ok {
		t.Fatalf("missing claude entry in %#v", inputs.Harnesses)
	}
	if claudeEntry.QuotaOK {
		t.Fatalf("Claude QuotaOK=true, want false for exhausted window: %#v", claudeEntry)
	}
	if claudeEntry.SubscriptionOK {
		t.Fatalf("Claude SubscriptionOK=true, want false for exhausted window: %#v", claudeEntry)
	}
	if claudeEntry.QuotaPercentUsed != 100 || claudeEntry.QuotaTrend != routing.QuotaTrendExhausting {
		t.Fatalf("Claude quota components=%d/%q, want 100/%q", claudeEntry.QuotaPercentUsed, claudeEntry.QuotaTrend, routing.QuotaTrendExhausting)
	}
	if !strings.Contains(claudeEntry.QuotaReason, "exhausted claude-extra") {
		t.Fatalf("Claude QuotaReason=%q, want exhausted claude-extra detail", claudeEntry.QuotaReason)
	}
}

func findRoutingHarnessEntry(entries []routing.HarnessEntry, name string) (routing.HarnessEntry, bool) {
	for _, entry := range entries {
		if entry.Name == name {
			return entry, true
		}
	}
	return routing.HarnessEntry{}, false
}

func TestResolveRouteSnapshotFreshCacheSkipsDiscoveryProbe(t *testing.T) {
	t.Setenv("PATH", "")
	cacheDir := t.TempDir()
	t.Setenv("FIZEAU_CACHE_DIR", cacheDir)

	var probeHits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		probeHits.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	cache := &discoverycache.Cache{Root: cacheDir}
	writeSnapshotDiscoveryFixture(t, cache, testDiscoverySourceName("alpha", "alpha", srv.URL+"/v1", "alpha-1"), time.Date(2026, 5, 12, 15, 5, 0, 0, time.UTC), []string{"model-a"})

	catalog := loadRoutingFixtureCatalog(t, `
version: 5
generated_at: 2026-05-12T00:00:00Z
catalog_version: test
policies:
  default:
    min_power: 1
    max_power: 10
    allow_local: true
models:
  model-a:
    family: test
    status: active
    power: 7
    context_window: 8192
    surfaces:
      embedded-openai: model-a
`)
	t.Cleanup(replaceRoutingCatalogForTest(t, catalog))

	sc := &fakeServiceConfig{
		providers: map[string]ServiceProviderEntry{
			"alpha": {Type: "openai", BaseURL: srv.URL + "/v1", APIKey: "alpha-key", ServerInstance: "alpha-1", Model: "model-a"},
		},
		names:       []string{"alpha"},
		defaultName: "alpha",
	}
	svc := newTestService(t, ServiceOptions{ServiceConfig: sc})

	dec, err := svc.ResolveRoute(context.Background(), RouteRequest{})
	if err != nil {
		t.Fatalf("ResolveRoute: %v", err)
	}
	if dec == nil {
		t.Fatal("ResolveRoute returned nil decision")
	}
	if dec.Model != "model-a" || dec.Provider != "alpha" {
		t.Fatalf("decision=%#v, want snapshot-backed alpha/model-a", dec)
	}
	if probeHits.Load() != 0 {
		t.Fatalf("unexpected discovery probe count = %d", probeHits.Load())
	}
}

func TestBuildRoutingInputsPopulatesEndpointProviderCostsFromCatalog(t *testing.T) {
	catalog := loadRoutingFixtureCatalog(t, `
version: 5
generated_at: 2026-04-22T00:00:00Z
policies:
  default:
    min_power: 1
    max_power: 10
    allow_local: true
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
	svc := newTestService(t, ServiceOptions{ServiceConfig: sc})
	in := svc.buildRoutingInputs(context.Background())

	got := providerCostsByName(in, "fiz")
	assertProviderCost(t, got, "alpha", 0.002, routing.CostSourceCatalog)
	assertProviderCost(t, got, "beta", 0.003, routing.CostSourceCatalog)
	assertProviderCost(t, got, "gamma", 0.006, routing.CostSourceCatalog)
}

func TestSubscriptionEffectiveCostCurveByHarnessAndBand(t *testing.T) {
	catalog := loadRoutingFixtureCatalog(t, `
version: 5
generated_at: 2026-04-22T00:00:00Z
policies:
  default:
    min_power: 1
    max_power: 10
    allow_local: true
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

	unsetSvc := newTestService(t, ServiceOptions{ServiceConfig: sc})
	unset := providerCostsByName(unsetSvc.buildRoutingInputs(context.Background()), "fiz")
	assertProviderCost(t, unset, "local", 0, routing.CostSourceUnknown)

	setSvc := &service{
		opts:     ServiceOptions{ServiceConfig: sc, LocalCostUSDPer1kTokens: 0.0042},
		registry: harnesses.NewRegistry(),
	}
	set := providerCostsByName(setSvc.buildRoutingInputs(context.Background()), "fiz")
	assertProviderCost(t, set, "local", 0.0042, routing.CostSourceUserConfig)
}

func TestResolveRouteNearQuotaClaudeDemotesBelowOpenRouter(t *testing.T) {
	catalog := loadRoutingFixtureCatalog(t, `
version: 5
generated_at: 2026-04-22T00:00:00Z
policies:
  default:
    min_power: 1
    max_power: 10
    allow_local: true
models:
  sonnet-4.6:
    family: claude-sonnet
    status: active
    cost_input_per_m: 3
    cost_output_per_m: 15
    surfaces:
      agent.openai: sonnet-4.6
      claude-code: sonnet-4.6
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
				Name:                "fiz",
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
			// Neutralize Claude's near-quota score penalty and keep both
			// candidates on identical performance evidence so the final base
			// scores tie and the cost tiebreak is the deciding dimension.
			routing.ProviderModelKey("", "", "sonnet-4.6"):           1900,
			routing.ProviderModelKey("openrouter", "", "sonnet-4.6"): 1900,
		},
	}
	dec, err := routing.Resolve(routing.Request{
		Policy:             "default",
		Model:              "sonnet-4.6",
		ProviderPreference: routing.ProviderPreferenceSubscriptionFirst,
	}, in)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if dec.Harness != "fiz" || dec.Provider != "openrouter" {
		t.Fatalf("near-quota route selected harness=%q provider=%q, want fiz/openrouter", dec.Harness, dec.Provider)
	}
	var claudeCandidate, openrouterCandidate routing.Candidate
	for _, candidate := range dec.Candidates {
		switch {
		case candidate.Harness == "claude":
			claudeCandidate = candidate
		case candidate.Harness == "fiz" && candidate.Provider == "openrouter":
			openrouterCandidate = candidate
		}
	}
	if claudeCandidate.Harness == "" || openrouterCandidate.Harness == "" {
		t.Fatalf("expected claude and openrouter candidates in trace: %#v", dec.Candidates)
	}
	if claudeCandidate.Score >= openrouterCandidate.Score {
		t.Fatalf("openrouter should outrank near-quota Claude before any cost tiebreak: claude=%.1f openrouter=%.1f", claudeCandidate.Score, openrouterCandidate.Score)
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
	contents = normalizeRoutingFixtureManifest(t, contents)
	path := filepath.Join(t.TempDir(), "models.yaml")
	requireNoError(t, os.WriteFile(path, []byte(contents), 0o600))
	catalog, err := modelcatalog.Load(modelcatalog.LoadOptions{ManifestPath: path, RequireExternal: true})
	if err != nil {
		t.Fatalf("Load fixture catalog: %v", err)
	}
	return catalog
}

func normalizeRoutingFixtureManifest(t *testing.T, contents string) string {
	t.Helper()
	var doc map[string]any
	if err := yaml.Unmarshal([]byte(contents), &doc); err != nil {
		t.Fatalf("parse fixture manifest: %v", err)
	}
	if version, _ := intFromYAML(doc["version"]); version == 5 {
		return contents
	}
	doc["version"] = 5

	if _, ok := doc["policies"]; !ok {
		policies := make(map[string]any)
		if profiles, ok := doc["profiles"].(map[string]any); ok {
			for name, raw := range profiles {
				entry, _ := raw.(map[string]any)
				minPower, ok := intFromYAML(entry["min_power"])
				if !ok || minPower <= 0 {
					minPower = 1
				}
				maxPower, ok := intFromYAML(entry["max_power"])
				if !ok || maxPower <= 0 {
					maxPower = 10
				}
				if _, exists := policies[name]; !exists {
					policies[name] = map[string]any{
						"min_power":   minPower,
						"max_power":   maxPower,
						"allow_local": true,
					}
				}
			}
		}
		if _, ok := policies["default"]; !ok {
			policies["default"] = map[string]any{
				"min_power":   1,
				"max_power":   10,
				"allow_local": true,
			}
		}
		doc["policies"] = policies
	}
	delete(doc, "profiles")
	delete(doc, "targets")

	out, err := yaml.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal fixture manifest: %v", err)
	}
	return string(out)
}

func intFromYAML(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int64:
		return int(n), true
	case float64:
		return int(n), true
	default:
		return 0, false
	}
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

// gateFixtureCatalog returns a catalog used by the ContextWindow / RequiresTools
// gate-firing tests below. It declares two concrete models on the agent.openai
// surface: small-ctx-model has a 4096-token context window (and supports
// tools), while no-tools-model has a generous context window but is marked
// no_tools=true so RequiresTools=true requests are rejected against it.
const gateFixtureCatalogYAML = `
version: 5
generated_at: 2026-04-25T00:00:00Z
policies:
  default:
    min_power: 1
    max_power: 10
    allow_local: true
models:
  small-ctx-model:
    family: gate
    status: active
    context_window: 4096
    surfaces: {agent.openai: small-ctx-model}
  no-tools-model:
    family: gate
    status: active
    context_window: 200000
    no_tools: true
    surfaces: {agent.openai: no-tools-model}
`

func newGateFixtureService(t *testing.T, providerModel string) *service {
	t.Helper()
	catalog := loadRoutingFixtureCatalog(t, gateFixtureCatalogYAML)
	t.Cleanup(replaceRoutingCatalogForTest(t, catalog))

	sc := &fakeServiceConfig{
		providers: map[string]ServiceProviderEntry{
			"local": {Type: "test", BaseURL: "http://127.0.0.1:9999/v1", Model: providerModel},
		},
		names:       []string{"local"},
		defaultName: "local",
	}
	return publicRouteTraceService(sc)
}

func findCandidate(t *testing.T, dec *RouteDecision, harness, provider string) RouteCandidate {
	t.Helper()
	if dec == nil {
		t.Fatal("nil decision")
	}
	for _, c := range dec.Candidates {
		if c.Harness == harness && c.Provider == provider {
			return c
		}
	}
	t.Fatalf("candidate harness=%q provider=%q not found in %#v", harness, provider, dec.Candidates)
	return RouteCandidate{}
}

// TestResolveRoute_FiltersByEstimatedPromptTokens proves the engine's
// context-window gate fires end-to-end: with ContextWindows wired from the
// catalog, a 1M-token prompt against a 4096-token model is marked ineligible
// with FilterReasonContextTooSmall.
func TestResolveRoute_FiltersByEstimatedPromptTokens(t *testing.T) {
	svc := newGateFixtureService(t, "small-ctx-model")

	dec, _ := svc.ResolveRoute(context.Background(), RouteRequest{
		Harness:               "fiz",
		Model:                 "small-ctx-model",
		EstimatedPromptTokens: 1_000_000,
	})
	if dec == nil {
		t.Fatal("ResolveRoute returned nil decision")
	}
	candidate := findCandidate(t, dec, "fiz", "local")
	if candidate.Eligible {
		t.Fatalf("candidate eligible with 1M tokens against 4096 context: %#v", candidate)
	}
	if candidate.FilterReason != FilterReasonContextTooSmall {
		t.Fatalf("FilterReason=%q, want %q (Reason=%q)", candidate.FilterReason, FilterReasonContextTooSmall, candidate.Reason)
	}
}

// TestResolveRoute_FiltersByRequiresTools proves the RequiresTools gate fires
// when the catalog marks the resolved model with no_tools=true.
func TestResolveRoute_FiltersByRequiresTools(t *testing.T) {
	svc := newGateFixtureService(t, "no-tools-model")

	dec, _ := svc.ResolveRoute(context.Background(), RouteRequest{
		Harness:       "fiz",
		Model:         "no-tools-model",
		RequiresTools: true,
	})
	if dec == nil {
		t.Fatal("ResolveRoute returned nil decision")
	}
	candidate := findCandidate(t, dec, "fiz", "local")
	if candidate.Eligible {
		t.Fatalf("candidate eligible despite no_tools=true: %#v", candidate)
	}
	if candidate.FilterReason != FilterReasonNoToolSupport {
		t.Fatalf("FilterReason=%q, want %q (Reason=%q)", candidate.FilterReason, FilterReasonNoToolSupport, candidate.Reason)
	}
}

// TestResolveRoute_NoOpWhenZero proves that an unset EstimatedPromptTokens /
// RequiresTools request does not change which candidates are eligible compared
// to a baseline request — no spurious filtering on the same model that the
// previous two tests reject under stress.
func TestResolveRoute_NoOpWhenZero(t *testing.T) {
	smallCtxSvc := newGateFixtureService(t, "small-ctx-model")
	noToolsSvc := newGateFixtureService(t, "no-tools-model")

	smallDec, err := smallCtxSvc.ResolveRoute(context.Background(), RouteRequest{
		Harness: "fiz",
		Model:   "small-ctx-model",
	})
	if err != nil {
		t.Fatalf("ResolveRoute small-ctx-model: %v", err)
	}
	smallCandidate := findCandidate(t, smallDec, "fiz", "local")
	if !smallCandidate.Eligible {
		t.Fatalf("small-ctx-model candidate ineligible without EstimatedPromptTokens: %#v", smallCandidate)
	}
	if smallCandidate.FilterReason != "" {
		t.Fatalf("small-ctx-model FilterReason=%q, want empty (no-op gate)", smallCandidate.FilterReason)
	}

	noToolsDec, err := noToolsSvc.ResolveRoute(context.Background(), RouteRequest{
		Harness: "fiz",
		Model:   "no-tools-model",
	})
	if err != nil {
		t.Fatalf("ResolveRoute no-tools-model: %v", err)
	}
	noToolsCandidate := findCandidate(t, noToolsDec, "fiz", "local")
	if !noToolsCandidate.Eligible {
		t.Fatalf("no-tools-model candidate ineligible without RequiresTools=true: %#v", noToolsCandidate)
	}
	if noToolsCandidate.FilterReason != "" {
		t.Fatalf("no-tools-model FilterReason=%q, want empty (no-op gate)", noToolsCandidate.FilterReason)
	}
}

// TestBuildRoutingInputsWiresContextWindowsFromCatalog asserts the structural
// wiring requested by the bead: ProviderEntry.ContextWindows is populated for
// the configured DefaultModel from the catalog so the engine's context-window
// gate has data to act on.
func TestBuildRoutingInputsWiresContextWindowsFromCatalog(t *testing.T) {
	catalog := loadRoutingFixtureCatalog(t, gateFixtureCatalogYAML)
	t.Cleanup(replaceRoutingCatalogForTest(t, catalog))

	sc := &fakeServiceConfig{
		providers: map[string]ServiceProviderEntry{
			"local": {Type: "test", BaseURL: "http://127.0.0.1:9999/v1", Model: "small-ctx-model"},
		},
		names:       []string{"local"},
		defaultName: "local",
	}
	svc := newTestService(t, ServiceOptions{ServiceConfig: sc})

	in := svc.buildRoutingInputs(context.Background())
	providers := providerCostsByName(in, "fiz")
	provider, ok := providers["local"]
	if !ok {
		t.Fatalf("fiz/local provider not in inputs: %#v", providers)
	}
	if got := provider.ContextWindows["small-ctx-model"]; got != 4096 {
		t.Fatalf("ContextWindows[small-ctx-model]=%d, want 4096 (full map=%#v)", got, provider.ContextWindows)
	}
	if got := provider.ContextWindowSources["small-ctx-model"]; got != routing.ContextSourceCatalog {
		t.Fatalf("ContextWindowSources[small-ctx-model]=%q, want %q (full map=%#v)", got, routing.ContextSourceCatalog, provider.ContextWindowSources)
	}
}

// TestResolveRoute_LivenessEscalation exercises the policy→tier ladder
// (cheap → default → smart) wired into ResolveRoute. When every candidate
// at the requested tier is filtered out (here: per-provider context-window
// rejection driven by the catalog), ResolveRoute walks the ladder and
// returns the first higher-tier candidate that still satisfies the request.
// When the entire remaining ladder is also empty, ResolveRoute surfaces
// the precise *ErrNoLiveProvider error rather than the engine's
// "no viable routing candidate" jargon.
func TestResolveRoute_LivenessEscalation(t *testing.T) {
	const fixtureCatalog = `
version: 5
generated_at: 2026-04-25T00:00:00Z
policies:
  default:
    min_power: 5
    max_power: 5
    allow_local: true
  cheap:
    min_power: 5
    max_power: 5
    allow_local: true
  smart:
    min_power: 8
    max_power: 8
    allow_local: true
models:
  medium-model:
    family: tier
    status: active
    power: 5
    context_window: 4096
    surfaces: {agent.openai: medium-model}
  high-model:
    family: tier
    status: active
    power: 8
    context_window: 200000
    surfaces: {agent.openai: high-model}
`

	newSvc := func(t *testing.T) (*service, func()) {
		t.Helper()
		// Block claude/codex/gemini subprocess harnesses from the
		// candidate set so the test isolates the fiz harness's
		// per-provider tier escalation behavior.
		t.Setenv("GEMINI_API_KEY", "")
		t.Setenv("GOOGLE_API_KEY", "")
		t.Setenv("GOOGLE_GENAI_USE_VERTEXAI", "")
		t.Setenv("GOOGLE_GENAI_USE_GCA", "")
		t.Setenv("GEMINI_CLI_USE_COMPUTE_ADC", "")
		t.Setenv("CLOUD_SHELL", "")

		mediumSrv := openAIModelChatServer(t, []string{"medium-model"}, "medium-model", "ok")
		highSrv := openAIModelChatServer(t, []string{"high-model"}, "high-model", "ok")
		catalog := loadRoutingFixtureCatalog(t, fixtureCatalog)
		restore := replaceRoutingCatalogForTest(t, catalog)
		sc := &fakeServiceConfig{
			providers: map[string]ServiceProviderEntry{
				"alpha-medium": {Type: "openai", BaseURL: mediumSrv.URL + "/v1", APIKey: "k", Model: "medium-model"},
				"beta-high":    {Type: "openai", BaseURL: highSrv.URL + "/v1", APIKey: "k", Model: "high-model"},
			},
			names:       []string{"alpha-medium", "beta-high"},
			defaultName: "alpha-medium",
		}
		registry := harnesses.NewRegistry()
		registry.LookPath = func(string) (string, error) { return "", os.ErrNotExist }
		svc := &service{
			opts:     ServiceOptions{ServiceConfig: sc},
			registry: registry,
			hub:      newSessionHub(),
			catalog:  newCatalogCache(catalogCacheOptions{}),
		}
		cleanup := func() {
			mediumSrv.Close()
			highSrv.Close()
			restore()
		}
		return svc, cleanup
	}

	t.Run("escalates_when_lower_tier_filtered_out", func(t *testing.T) {
		svc, cleanup := newSvc(t)
		defer cleanup()

		// Record a route attempt failure on the lower-tier provider so the
		// real cooldown bookkeeping path (applyRouteAttemptCooldowns) runs
		// against this fixture, exercising the AC's "real ServiceConfig +
		// cooldown fixture" requirement.
		if err := svc.RecordRouteAttempt(context.Background(), RouteAttempt{
			Harness:  "fiz",
			Provider: "alpha-medium",
			Model:    "medium-model",
			Status:   "failed",
			Reason:   "synthetic unhealthy fixture",
		}); err != nil {
			t.Fatalf("RecordRouteAttempt: %v", err)
		}

		dec, err := svc.ResolveRoute(context.Background(), RouteRequest{
			Policy:                "default",
			EstimatedPromptTokens: 50_000,
		})
		if err != nil {
			t.Fatalf("ResolveRoute: %v", err)
		}
		if dec == nil || dec.Harness != "fiz" {
			t.Fatalf("decision=%#v, want fiz harness", dec)
		}
		if dec.Provider != "beta-high" {
			t.Fatalf("decision provider=%q, want beta-high (escalated to smart tier)", dec.Provider)
		}
		if dec.Model != "high-model" {
			t.Fatalf("decision model=%q, want high-model", dec.Model)
		}
	})

	t.Run("returns_no_live_provider_when_ladder_exhausted", func(t *testing.T) {
		svc, cleanup := newSvc(t)
		defer cleanup()

		dec, err := svc.ResolveRoute(context.Background(), RouteRequest{
			Policy:                "default",
			EstimatedPromptTokens: 1_000_000, // exceeds both 4096 and 200000 contexts
		})
		if err == nil {
			t.Fatalf("ResolveRoute returned no error, decision=%#v", dec)
		}
		if !strings.Contains(err.Error(), "no live provider") {
			t.Fatalf("error=%q, want 'no live provider' message", err.Error())
		}
		if strings.Contains(err.Error(), "no viable routing candidate") {
			t.Fatalf("error=%q must NOT contain engine 'no viable routing candidate' jargon", err.Error())
		}
		var noLive *ErrNoLiveProvider
		if !errors.As(err, &noLive) {
			t.Fatalf("errors.As ErrNoLiveProvider: %T %v", err, err)
		}
		if noLive.StartingPolicy != "default" {
			t.Fatalf("StartingPolicy=%q, want default", noLive.StartingPolicy)
		}
		if noLive.PromptTokens != 1_000_000 {
			t.Fatalf("PromptTokens=%d, want 1000000", noLive.PromptTokens)
		}
	})
}

// TestResolveRouteAutoResolvesToTierDefaultBeforeGate proves that when a
// request specifies Reasoning=auto, the routing engine resolves it to the
// catalog's surface_policy reasoning_default for the requested profile/surface
// BEFORE invoking the capability gate. Without this resolution an off-only
// candidate could win under a profile whose surface default is "high",
// silently dropping reasoning the operator implicitly asked for.
func TestResolveRouteAutoResolvesToTierDefaultBeforeGate(t *testing.T) {
	// off-only harness: SupportedReasoning is empty so the gate accepts
	// "off" (which imposes no requirement) but rejects any named level.
	offOnly := func() routing.HarnessEntry {
		return routing.HarnessEntry{
			Name:                "off-only",
			Surface:             "test-surface",
			CostClass:           "medium",
			AutoRoutingEligible: true,
			Available:           true,
			QuotaOK:             true,
			SubscriptionOK:      true,
			ExactPinSupport:     true,
			DefaultModel:        "off-model",
			SupportsTools:       true,
		}
	}

	resolver := func(profile, surface string) (string, bool) {
		switch profile {
		case "cheap":
			return "off", true
		case "smart":
			return "high", true
		}
		return "", false
	}

	t.Run("cheap_resolves_to_off_gate_passes", func(t *testing.T) {
		dec, err := routing.Resolve(routing.Request{
			Policy:    "cheap",
			Reasoning: "auto",
		}, routing.Inputs{
			Harnesses:         []routing.HarnessEntry{offOnly()},
			ReasoningResolver: resolver,
		})
		if err != nil {
			t.Fatalf("Resolve cheap+auto: %v", err)
		}
		if dec.Harness != "off-only" || dec.Model != "off-model" {
			t.Fatalf("decision=%#v, want off-only/off-model (auto must resolve to off and pass)", dec)
		}
	})

	t.Run("smart_resolves_to_high_gate_rejects", func(t *testing.T) {
		dec, err := routing.Resolve(routing.Request{
			Policy:    "smart",
			Reasoning: "auto",
		}, routing.Inputs{
			Harnesses:         []routing.HarnessEntry{offOnly()},
			ReasoningResolver: resolver,
		})
		if err == nil {
			t.Fatalf("Resolve smart+auto: expected NoViableCandidateError, got decision=%#v", dec)
		}
		var noViable *routing.NoViableCandidateError
		if !errors.As(err, &noViable) {
			t.Fatalf("error=%T %v, want *routing.NoViableCandidateError", err, err)
		}
		if dec == nil || len(dec.Candidates) != 1 {
			t.Fatalf("Candidates=%#v, want exactly one off-only candidate", dec)
		}
		c := dec.Candidates[0]
		if c.Harness != "off-only" || c.Eligible {
			t.Fatalf("candidate=%#v, want ineligible off-only", c)
		}
		if c.FilterReason != routing.FilterReasonReasoningUnsupported {
			t.Fatalf("FilterReason=%q, want %q (Reason=%q)", c.FilterReason, routing.FilterReasonReasoningUnsupported, c.Reason)
		}
	})

	t.Run("unset_reasoning_does_not_resolve", func(t *testing.T) {
		// Regression guard: the Reasoning=unset path keeps its existing
		// "no requirement" behavior — only Reasoning=auto triggers
		// surface_policy resolution.
		dec, err := routing.Resolve(routing.Request{
			Policy: "smart",
		}, routing.Inputs{
			Harnesses:         []routing.HarnessEntry{offOnly()},
			ReasoningResolver: resolver,
		})
		if err != nil {
			t.Fatalf("Resolve smart+unset: %v", err)
		}
		if dec.Harness != "off-only" {
			t.Fatalf("unset+smart decision=%#v, want off-only (unset must not trigger auto resolution)", dec)
		}
	})
}

func TestDecisionWithCandidatesCopiesInput(t *testing.T) {
	candidates := []RouteCandidate{{Harness: "fiz", Reason: "original"}}
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
