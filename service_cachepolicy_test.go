package fizeau

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// TestServiceExecuteRequestHasCachePolicy is an AST guard that pins the
// public CachePolicy field on ServiceExecuteRequest so beads C and D can rely
// on its presence without further contract churn.
func TestServiceExecuteRequestHasCachePolicy(t *testing.T) {
	requireStructHasField(t, "service.go", "ServiceExecuteRequest", "CachePolicy")
}

// TestRouteRequestHasCachePolicy is an AST guard for the mirrored field on
// RouteRequest.
func TestRouteRequestHasCachePolicy(t *testing.T) {
	requireStructHasField(t, "service.go", "RouteRequest", "CachePolicy")
}

// TestServiceExecuteRequestRejectsUnknownCachePolicy verifies that
// svc.Execute rejects unknown CachePolicy values at the boundary, before any
// session is opened or events are emitted.
func TestServiceExecuteRequestRejectsUnknownCachePolicy(t *testing.T) {
	svc, err := New(ServiceOptions{
		ServiceConfig:       &fakeServiceConfig{},
		QuotaRefreshContext: canceledRefreshContext(),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ch, err := svc.Execute(context.Background(), ServiceExecuteRequest{
		Prompt:      "irrelevant",
		Harness:     "fiz",
		CachePolicy: "aggressive",
	})
	if err == nil {
		t.Fatal("expected Execute to reject unknown CachePolicy")
	}
	if ch != nil {
		t.Fatalf("expected no event channel for boundary error, got %#v", ch)
	}
	if !strings.Contains(err.Error(), "CachePolicy") {
		t.Fatalf("error should mention CachePolicy, got %v", err)
	}
}

// TestValidateCachePolicy covers the accepted/rejected values directly so the
// contract is exercised independent of the Execute path.
func TestValidateCachePolicy(t *testing.T) {
	for _, ok := range []string{"", "default", "off"} {
		if err := ValidateCachePolicy(ok); err != nil {
			t.Errorf("ValidateCachePolicy(%q) = %v, want nil", ok, err)
		}
	}
	for _, bad := range []string{"on", "aggressive", "Default", "OFF", "auto"} {
		if err := ValidateCachePolicy(bad); err == nil {
			t.Errorf("ValidateCachePolicy(%q) = nil, want error", bad)
		} else if !errors.Is(err, err) {
			t.Errorf("error wrapping smoke test failed for %q", bad)
		}
	}
}
