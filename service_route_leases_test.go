package fizeau

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/fizeau/internal/harnesses"
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
					{Name: "desk-a", BaseURL: "http://desk-a.invalid/v1"},
					{Name: "desk-b", BaseURL: "http://desk-b.invalid/v1"},
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
		Harness:       "agent",
		Model:         "qwen/qwen3.6",
		CorrelationID: "bead-sticky",
	})
	if err != nil {
		t.Fatalf("ResolveRoute first: %v", err)
	}
	if first.Provider != "local@desk-b" || first.Endpoint != "desk-b" {
		t.Fatalf("first decision=%#v, want desk-b", first)
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
		Harness:       "agent",
		Model:         "qwen/qwen3.6",
		CorrelationID: "bead-sticky",
	})
	if err != nil {
		t.Fatalf("ResolveRoute second: %v", err)
	}
	if second.Provider != "local@desk-b" || second.Endpoint != "desk-b" {
		t.Fatalf("sticky decision=%#v, want reused desk-b despite reversed baseline", second)
	}
}
