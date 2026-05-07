package fizeau

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/fizeau/internal/harnesses"
	"github.com/DocumentDrivenDX/fizeau/internal/provider/utilization"
	"github.com/DocumentDrivenDX/fizeau/internal/routehealth"
)

func TestResolveRouteStickyLeaseReusesEndpoint(t *testing.T) {
	originalProbe := probeOpenAIModelsForDiscovery
	defer func() { probeOpenAIModelsForDiscovery = originalProbe }()
	probeOpenAIModelsForDiscovery = func(ctx context.Context, baseURL, apiKey string) ([]string, error) {
		switch {
		case strings.Contains(baseURL, "desk-a"):
			return []string{"qwen/qwen3.6"}, nil
		case strings.Contains(baseURL, "desk-b"):
			return []string{"qwen/qwen3.6"}, nil
		default:
			return nil, nil
		}
	}

	sc := &fakeServiceConfig{
		providers: map[string]ServiceProviderEntry{
			"local": {
				Type: "lmstudio",
				Endpoints: []ServiceProviderEndpoint{
					{Name: "desk-a", BaseURL: "http://desk-a.invalid/v1", ServerInstance: "desk-a"},
					{Name: "desk-b", BaseURL: "http://desk-b.invalid/v1", ServerInstance: "desk-b"},
				},
				Model: "qwen/qwen3.6",
			},
		},
		names:          []string{"local"},
		defaultName:    "local",
		healthCooldown: 20 * time.Millisecond,
	}
	svc := &service{
		opts:        ServiceOptions{ServiceConfig: sc},
		registry:    harnesses.NewRegistry(),
		hub:         newSessionHub(),
		catalog:     newCatalogCache(catalogCacheOptions{}),
		routeHealth: routehealth.NewStore(),
		routeLeases: routehealth.NewLeaseStore(),
	}

	now := time.Now().UTC()
	// Make desk-b the initial winner.
	requireNoError(t, svc.RecordRouteAttempt(context.Background(), RouteAttempt{
		Provider:  "local@desk-b",
		Endpoint:  "desk-b",
		Model:     "qwen/qwen3.6",
		Status:    "success",
		Duration:  10 * time.Millisecond,
		Timestamp: now,
	}))
	requireNoError(t, svc.RecordRouteAttempt(context.Background(), RouteAttempt{
		Provider:  "local@desk-a",
		Endpoint:  "desk-a",
		Model:     "qwen/qwen3.6",
		Status:    "failed",
		Duration:  80 * time.Millisecond,
		Timestamp: now,
	}))

	first, err := svc.ResolveRoute(context.Background(), RouteRequest{
		Harness:       "fiz",
		Model:         "qwen/qwen3.6",
		CorrelationID: "bead-sticky",
	})
	if err != nil {
		t.Fatalf("ResolveRoute first: %v", err)
	}
	if first.Provider != "local@desk-b" || first.Endpoint != "desk-b" {
		t.Fatalf("first decision=%#v, want desk-b", first)
	}
	if !first.Sticky.KeyPresent || first.Sticky.Assignment != "acquired" {
		t.Fatalf("first sticky evidence=%#v, want acquired sticky lease", first.Sticky)
	}

	time.Sleep(30 * time.Millisecond)
	now = time.Now().UTC()
	// Reverse the live metrics so the baseline would now prefer desk-a.
	requireNoError(t, svc.RecordRouteAttempt(context.Background(), RouteAttempt{
		Provider:  "local@desk-a",
		Endpoint:  "desk-a",
		Model:     "qwen/qwen3.6",
		Status:    "success",
		Duration:  10 * time.Millisecond,
		Timestamp: now,
	}))
	requireNoError(t, svc.RecordRouteAttempt(context.Background(), RouteAttempt{
		Provider:  "local@desk-b",
		Endpoint:  "desk-b",
		Model:     "qwen/qwen3.6",
		Status:    "failed",
		Duration:  90 * time.Millisecond,
		Timestamp: now,
	}))

	second, err := svc.ResolveRoute(context.Background(), RouteRequest{
		Harness:       "fiz",
		Model:         "qwen/qwen3.6",
		CorrelationID: "bead-sticky",
	})
	if err != nil {
		t.Fatalf("ResolveRoute second: %v", err)
	}
	if second.Provider != "local@desk-b" || second.Endpoint != "desk-b" {
		t.Fatalf("sticky decision=%#v, want reused desk-b despite reversed baseline", second)
	}
	if second.Sticky.Assignment != "reused" {
		t.Fatalf("second sticky evidence=%#v, want reused", second.Sticky)
	}
}

func TestResolveRouteStickyLeaseDistributesNewKeysByLoad(t *testing.T) {
	originalProbe := probeOpenAIModelsForDiscovery
	defer func() { probeOpenAIModelsForDiscovery = originalProbe }()
	probeOpenAIModelsForDiscovery = func(ctx context.Context, baseURL, apiKey string) ([]string, error) {
		switch {
		case strings.Contains(baseURL, "desk-a"):
			return []string{"qwen/qwen3.6"}, nil
		case strings.Contains(baseURL, "desk-b"):
			return []string{"qwen/qwen3.6"}, nil
		default:
			return nil, nil
		}
	}

	sc := &fakeServiceConfig{
		providers: map[string]ServiceProviderEntry{
			"local": {
				Type: "lmstudio",
				Endpoints: []ServiceProviderEndpoint{
					{Name: "desk-a", BaseURL: "http://desk-a.invalid/v1", ServerInstance: "desk-a"},
					{Name: "desk-b", BaseURL: "http://desk-b.invalid/v1", ServerInstance: "desk-b"},
				},
				Model: "qwen/qwen3.6",
			},
		},
		names:          []string{"local"},
		defaultName:    "local",
		healthCooldown: 20 * time.Millisecond,
	}
	svc := &service{
		opts:        ServiceOptions{ServiceConfig: sc},
		registry:    harnesses.NewRegistry(),
		hub:         newSessionHub(),
		catalog:     newCatalogCache(catalogCacheOptions{}),
		routeHealth: routehealth.NewStore(),
		routeLeases: routehealth.NewLeaseStore(),
	}
	svc.routeUtilizationStore().Record("local", "desk-a", "qwen/qwen3.6", utilization.EndpointUtilization{
		ActiveRequests: utilization.Int(1),
		MaxConcurrency: utilization.Int(2),
		Source:         utilization.SourceLlamaSlots,
		Freshness:      utilization.FreshnessFresh,
	})
	svc.routeUtilizationStore().Record("local", "desk-b", "qwen/qwen3.6", utilization.EndpointUtilization{
		ActiveRequests: utilization.Int(0),
		MaxConcurrency: utilization.Int(2),
		Source:         utilization.SourceLlamaSlots,
		Freshness:      utilization.FreshnessFresh,
	})

	first, err := svc.ResolveRoute(context.Background(), RouteRequest{
		Harness:       "fiz",
		Model:         "qwen/qwen3.6",
		CorrelationID: "bead-new-a",
	})
	if err != nil {
		t.Fatalf("ResolveRoute first: %v", err)
	}
	if first.Provider != "local@desk-b" || first.Endpoint != "desk-b" {
		t.Fatalf("first decision=%#v, want desk-b as the least-loaded endpoint", first)
	}
	if first.Utilization.Source != string(utilization.SourceLlamaSlots) {
		t.Fatalf("first utilization source=%q, want %q", first.Utilization.Source, utilization.SourceLlamaSlots)
	}
	if first.Utilization.Freshness != string(utilization.FreshnessFresh) {
		t.Fatalf("first utilization freshness=%q, want fresh", first.Utilization.Freshness)
	}

	second, err := svc.ResolveRoute(context.Background(), RouteRequest{
		Harness:       "fiz",
		Model:         "qwen/qwen3.6",
		CorrelationID: "bead-new-b",
	})
	if err != nil {
		t.Fatalf("ResolveRoute second: %v", err)
	}
	if second.Provider != "local@desk-a" || second.Endpoint != "desk-a" {
		t.Fatalf("second decision=%#v, want desk-a after desk-b lease increased load", second)
	}
	if second.Utilization.Source != string(utilization.SourceLlamaSlots) {
		t.Fatalf("second utilization source=%q, want %q", second.Utilization.Source, utilization.SourceLlamaSlots)
	}
}

func TestResolveRouteStickyLeaseAvoidsSaturatedEndpointForNewKey(t *testing.T) {
	originalProbe := probeOpenAIModelsForDiscovery
	defer func() { probeOpenAIModelsForDiscovery = originalProbe }()
	probeOpenAIModelsForDiscovery = func(ctx context.Context, baseURL, apiKey string) ([]string, error) {
		switch {
		case strings.Contains(baseURL, "desk-a"):
			return []string{"qwen/qwen3.6"}, nil
		case strings.Contains(baseURL, "desk-b"):
			return []string{"qwen/qwen3.6"}, nil
		default:
			return nil, nil
		}
	}

	sc := &fakeServiceConfig{
		providers: map[string]ServiceProviderEntry{
			"local": {
				Type: "lmstudio",
				Endpoints: []ServiceProviderEndpoint{
					{Name: "desk-a", BaseURL: "http://desk-a.invalid/v1", ServerInstance: "desk-a"},
					{Name: "desk-b", BaseURL: "http://desk-b.invalid/v1", ServerInstance: "desk-b"},
				},
				Model: "qwen/qwen3.6",
			},
		},
		names:          []string{"local"},
		defaultName:    "local",
		healthCooldown: 20 * time.Millisecond,
	}
	svc := &service{
		opts:        ServiceOptions{ServiceConfig: sc},
		registry:    harnesses.NewRegistry(),
		hub:         newSessionHub(),
		catalog:     newCatalogCache(catalogCacheOptions{}),
		routeHealth: routehealth.NewStore(),
		routeLeases: routehealth.NewLeaseStore(),
	}
	svc.routeUtilizationStore().Record("local", "desk-a", "qwen/qwen3.6", utilization.EndpointUtilization{
		ActiveRequests: utilization.Int(1),
		MaxConcurrency: utilization.Int(1),
		Source:         utilization.SourceLlamaSlots,
		Freshness:      utilization.FreshnessFresh,
	})
	svc.routeLeases.Acquire(time.Now().UTC(), stickyRouteLeaseTTL, routehealth.NormalizeLeaseKey("seed-b", "local", "qwen/qwen3.6"), "local", "desk-b", "qwen/qwen3.6")

	dec, err := svc.ResolveRoute(context.Background(), RouteRequest{
		Harness:       "fiz",
		Model:         "qwen/qwen3.6",
		CorrelationID: "saturated-load-key",
	})
	if err != nil {
		t.Fatalf("ResolveRoute: %v", err)
	}
	if dec.Provider != "local@desk-b" || dec.Endpoint != "desk-b" {
		t.Fatalf("decision=%#v, want desk-b because desk-a is saturated", dec)
	}
}

func TestResolveRouteStickyLeaseIgnoresStaleUtilizationFallback(t *testing.T) {
	originalProbe := probeOpenAIModelsForDiscovery
	defer func() { probeOpenAIModelsForDiscovery = originalProbe }()
	probeOpenAIModelsForDiscovery = func(ctx context.Context, baseURL, apiKey string) ([]string, error) {
		switch {
		case strings.Contains(baseURL, "desk-a"):
			return []string{"qwen/qwen3.6"}, nil
		case strings.Contains(baseURL, "desk-b"):
			return []string{"qwen/qwen3.6"}, nil
		default:
			return nil, nil
		}
	}

	sc := &fakeServiceConfig{
		providers: map[string]ServiceProviderEntry{
			"local": {
				Type: "lmstudio",
				Endpoints: []ServiceProviderEndpoint{
					{Name: "desk-a", BaseURL: "http://desk-a.invalid/v1", ServerInstance: "desk-a"},
					{Name: "desk-b", BaseURL: "http://desk-b.invalid/v1", ServerInstance: "desk-b"},
				},
				Model: "qwen/qwen3.6",
			},
		},
		names:          []string{"local"},
		defaultName:    "local",
		healthCooldown: 20 * time.Millisecond,
	}
	svc := &service{
		opts:        ServiceOptions{ServiceConfig: sc},
		registry:    harnesses.NewRegistry(),
		hub:         newSessionHub(),
		catalog:     newCatalogCache(catalogCacheOptions{}),
		routeHealth: routehealth.NewStore(),
		routeLeases: routehealth.NewLeaseStore(),
	}
	// Stale utilization should be ignored in favor of the in-process lease
	// counts. Make desk-a look idle in stale telemetry but keep more leases
	// on desk-a so desk-b should still win.
	svc.routeUtilizationStore().Record("local", "desk-a", "qwen/qwen3.6", utilization.EndpointUtilization{
		ActiveRequests: utilization.Int(0),
		MaxConcurrency: utilization.Int(2),
		Source:         utilization.SourceLlamaSlots,
		Freshness:      utilization.FreshnessStale,
	})
	svc.routeLeases.Acquire(time.Now().UTC(), stickyRouteLeaseTTL, routehealth.NormalizeLeaseKey("seed-a", "local", "qwen/qwen3.6"), "local", "desk-a", "qwen/qwen3.6")
	svc.routeLeases.Acquire(time.Now().UTC(), stickyRouteLeaseTTL, routehealth.NormalizeLeaseKey("seed-b", "local", "qwen/qwen3.6"), "local", "desk-a", "qwen/qwen3.6")
	svc.routeLeases.Acquire(time.Now().UTC(), stickyRouteLeaseTTL, routehealth.NormalizeLeaseKey("seed-c", "local", "qwen/qwen3.6"), "local", "desk-b", "qwen/qwen3.6")

	dec, err := svc.ResolveRoute(context.Background(), RouteRequest{
		Harness:       "fiz",
		Model:         "qwen/qwen3.6",
		CorrelationID: "stale-load-key",
	})
	if err != nil {
		t.Fatalf("ResolveRoute: %v", err)
	}
	if dec.Provider != "local@desk-b" || dec.Endpoint != "desk-b" {
		t.Fatalf("decision=%#v, want desk-b from lease-count fallback", dec)
	}
}
