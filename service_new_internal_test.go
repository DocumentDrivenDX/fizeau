package fizeau

import "testing"

func TestNew_PersistRouteHealthDefaultsDisabled(t *testing.T) {
	rawSvc, err := New(ServiceOptions{
		ServiceConfig:       &fakeServiceConfig{workDir: t.TempDir()},
		QuotaRefreshContext: canceledRefreshContext(),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	svc := rawSvc.(*service)
	if svc.opts.PersistRouteHealth != "" {
		t.Fatalf("PersistRouteHealth = %q, want empty by default", svc.opts.PersistRouteHealth)
	}
}
