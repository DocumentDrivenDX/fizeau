package fizeau

import (
	"testing"

	"github.com/DocumentDrivenDX/fizeau/internal/harnesses"
)

type testServiceOption func(*service)

func newTestService(t testing.TB, opts ServiceOptions, options ...testServiceOption) *service {
	t.Helper()

	svc := &service{
		opts:     opts,
		registry: harnesses.NewRegistry(),
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
}
