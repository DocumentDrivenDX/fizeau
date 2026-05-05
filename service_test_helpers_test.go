package fizeau

import (
	"testing"

	"github.com/DocumentDrivenDX/fizeau/internal/harnesses"
)

type testServiceOption func(*service)

func newTestService(t testing.TB, opts ServiceOptions, options ...testServiceOption) *service {
	t.Helper()

	svc := &service{
		opts:             opts,
		registry:         harnesses.NewRegistry(),
		hub:              newSessionHub(),
		catalog:          newCatalogCache(catalogCacheOptions{AsyncRefreshTimeout: opts.catalogRefreshTimeout()}),
		routeMetrics:     make(map[routeAttemptKey]routeMetricRecord),
		routingQuality:   newRoutingQualityStore(),
		providerQuota:    NewProviderQuotaStateStore(),
		providerBurnRate: NewProviderBurnRateTracker(),
	}
	for _, option := range options {
		option(svc)
	}
	return svc
}

func TestNewTestServiceInitializesCommonRuntimeState(t *testing.T) {
	svc := newTestService(t, ServiceOptions{})
	if svc.registry == nil {
		t.Fatal("registry is nil")
	}
	if svc.hub == nil {
		t.Fatal("hub is nil")
	}
	if svc.catalog == nil {
		t.Fatal("catalog is nil")
	}
	if svc.routingQuality == nil {
		t.Fatal("routingQuality is nil")
	}
	if svc.providerQuota == nil {
		t.Fatal("providerQuota is nil")
	}
	if svc.providerBurnRate == nil {
		t.Fatal("providerBurnRate is nil")
	}
}
