package fizeau

import (
	"context"
	"testing"

	"github.com/easel/fizeau/internal/harnesses"
)

type testServiceOption func(*service)

func newTestService(t testing.TB, opts ServiceOptions, options ...testServiceOption) *service {
	t.Helper()

	svc := &service{
		opts:             opts,
		registry:         harnesses.NewRegistry(),
		harnessInstances: defaultHarnessInstances(),
	}
	for _, option := range options {
		option(svc)
	}
	return svc
}

func canceledRefreshContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

func TestNewTestServiceInitializesCommonRuntimeState(t *testing.T) {
	svc := newTestService(t, ServiceOptions{})
	if svc.registry == nil {
		t.Fatal("registry is nil")
	}
}
