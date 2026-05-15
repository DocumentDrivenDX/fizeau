package fizeau

import (
	"context"
	"strings"
	"testing"
)

func TestServiceExecuteRequestHasPowerBounds(t *testing.T) {
	requireStructHasField(t, "service.go", "ServiceExecuteRequest", "MinPower")
	requireStructHasField(t, "service.go", "ServiceExecuteRequest", "MaxPower")
}

func TestRouteRequestHasPowerBounds(t *testing.T) {
	requireStructHasField(t, "service.go", "RouteRequest", "MinPower")
	requireStructHasField(t, "service.go", "RouteRequest", "MaxPower")
}

func TestValidatePowerBounds(t *testing.T) {
	for _, tc := range []struct {
		name string
		min  int
		max  int
	}{
		{name: "unset"},
		{name: "min only", min: 5},
		{name: "max only", max: 8},
		{name: "range", min: 3, max: 8},
		{name: "same", min: 7, max: 7},
	} {
		if err := ValidatePowerBounds(tc.min, tc.max); err != nil {
			t.Errorf("%s: ValidatePowerBounds(%d, %d) = %v, want nil", tc.name, tc.min, tc.max, err)
		}
	}

	for _, tc := range []struct {
		name string
		min  int
		max  int
	}{
		{name: "negative min", min: -1},
		{name: "negative max", max: -1},
		{name: "max below min", min: 8, max: 3},
	} {
		if err := ValidatePowerBounds(tc.min, tc.max); err == nil {
			t.Errorf("%s: ValidatePowerBounds(%d, %d) = nil, want error", tc.name, tc.min, tc.max)
		}
	}
}

func TestServiceExecuteRequestRejectsInvalidPowerBounds(t *testing.T) {
	svc, err := New(ServiceOptions{
		ServiceConfig:       &fakeServiceConfig{},
		QuotaRefreshContext: canceledRefreshContext(),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ch, err := svc.Execute(context.Background(), ServiceExecuteRequest{
		Prompt:   "irrelevant",
		Harness:  "fiz",
		MinPower: 9,
		MaxPower: 4,
	})
	if err == nil {
		t.Fatal("expected Execute to reject invalid power bounds")
	}
	if ch != nil {
		t.Fatalf("expected no event channel for boundary error, got %#v", ch)
	}
	if !strings.Contains(err.Error(), "power") && !strings.Contains(err.Error(), "Power") {
		t.Fatalf("error should mention power, got %v", err)
	}
}

func TestResolveRouteRejectsInvalidPowerBounds(t *testing.T) {
	svc, err := New(ServiceOptions{
		ServiceConfig:       &fakeServiceConfig{},
		QuotaRefreshContext: canceledRefreshContext(),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	_, err = svc.ResolveRoute(context.Background(), RouteRequest{MinPower: -1})
	if err == nil {
		t.Fatal("expected ResolveRoute to reject invalid power bounds")
	}
}
