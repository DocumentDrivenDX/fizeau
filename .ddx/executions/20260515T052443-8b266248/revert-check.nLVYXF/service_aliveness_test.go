package fizeau

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/easel/fizeau/internal/discoverycache"
	"github.com/easel/fizeau/internal/modelsnapshot"
	"github.com/easel/fizeau/internal/routehealth"
)

// TestServiceStartup_ProbesConfiguredProviders asserts that startupAlivenessProbe
// (called by Service.New) runs one TCP-connect probe per configured non-cloud
// provider within the startup-bounded time (AC #1).
func TestServiceStartup_ProbesConfiguredProviders(t *testing.T) {
	var mu sync.Mutex
	var probed []string

	fakeProber := ProviderAlivenessProber(func(_ context.Context, provider, _ string) bool {
		mu.Lock()
		probed = append(probed, provider)
		mu.Unlock()
		return false
	})

	sc := &fakeServiceConfig{
		names: []string{"bragi", "openrouter"},
		providers: map[string]ServiceProviderEntry{
			"bragi": {
				// llama-server uses fixed (local) billing — should be probed.
				Type:    "llama-server",
				BaseURL: "http://bragi:1234",
			},
			"openrouter": {
				// openrouter uses per-token billing — should NOT be probed.
				Type:    "openrouter",
				BaseURL: "https://openrouter.ai/api/v1",
			},
		},
	}

	svc := newTestService(t, ServiceOptions{
		ServiceConfig:   sc,
		AlivenessProber: fakeProber,
	})
	svc.providerProbe = routehealth.NewProbeStore()

	// startupAlivenessProbe is what Service.New() calls; test it directly.
	svc.startupAlivenessProbe(context.Background())

	mu.Lock()
	defer mu.Unlock()

	if len(probed) != 1 {
		t.Fatalf("expected 1 probe (bragi only, skip cloud openrouter), got %d: %v", len(probed), probed)
	}
	if probed[0] != "bragi" {
		t.Fatalf("expected probe for bragi, got %q", probed[0])
	}

	// Verify the result is recorded in the probe store.
	r, ok := svc.providerProbe.LastProbe("bragi", "")
	if !ok {
		t.Fatal("no probe record for bragi in store")
	}
	if r.LastProbeSuccess {
		t.Error("expected probe failure recorded (prober returned false)")
	}
}

// TestServiceStartup_TotalTimeoutBoundsProbes asserts that the total startup
// probe time is bounded regardless of provider count.
func TestServiceStartup_TotalTimeoutBoundsProbes(t *testing.T) {
	// Three local providers; prober hangs for 100ms each.
	blocked := make(chan struct{})
	var probeCount int
	var mu sync.Mutex
	fakeProber := ProviderAlivenessProber(func(ctx context.Context, provider, _ string) bool {
		mu.Lock()
		probeCount++
		mu.Unlock()
		select {
		case <-ctx.Done():
			return false
		case <-blocked: // never unblocked in this test
			return true
		}
	})

	targets := []alivenessEndpoint{
		{provider: "a", baseURL: "http://a:1234"},
		{provider: "b", baseURL: "http://b:1234"},
		{provider: "c", baseURL: "http://c:1234"},
	}

	store := routehealth.NewProbeStore()
	start := time.Now()
	runStartupAlivenessProbes(context.Background(), targets, store, fakeProber, 50*time.Millisecond)
	elapsed := time.Since(start)

	if elapsed > 500*time.Millisecond {
		t.Errorf("startup probe took %v, expected < 500ms (bounded by 50ms timeout)", elapsed)
	}
}

func TestServiceStartup_TotalTimeoutDoesNotRecordUnprobedProvidersAsFailed(t *testing.T) {
	blocked := make(chan struct{})
	var probeCount int
	var mu sync.Mutex
	fakeProber := ProviderAlivenessProber(func(ctx context.Context, _, _ string) bool {
		mu.Lock()
		probeCount++
		mu.Unlock()
		select {
		case <-ctx.Done():
			return false
		case <-blocked:
			return true
		}
	})

	targets := []alivenessEndpoint{
		{provider: "a", baseURL: "http://a:1234"},
		{provider: "b", baseURL: "http://b:1234"},
		{provider: "c", baseURL: "http://c:1234"},
	}

	store := routehealth.NewProbeStore()
	runStartupAlivenessProbes(context.Background(), targets, store, fakeProber, 20*time.Millisecond)

	mu.Lock()
	gotProbeCount := probeCount
	mu.Unlock()
	if gotProbeCount != 1 {
		t.Fatalf("expected only the first probe to run before timeout, got %d probes", gotProbeCount)
	}
	record, ok := store.LastProbe("a", "")
	if !ok {
		t.Fatal("missing failed startup probe record for attempted provider")
	}
	if record.LastProbeSuccess {
		t.Fatal("attempted provider recorded success, want failed")
	}
	for _, provider := range []string{"b", "c"} {
		if _, ok := store.LastProbe(provider, ""); ok {
			t.Fatalf("provider %q was never probed but has a startup probe record", provider)
		}
	}
}

func TestServiceStartup_SkippedProvidersRemainAbsentFromProbeUnreachable(t *testing.T) {
	blocked := make(chan struct{})
	fakeProber := ProviderAlivenessProber(func(ctx context.Context, _, _ string) bool {
		select {
		case <-ctx.Done():
			return false
		case <-blocked:
			return true
		}
	})

	targets := []alivenessEndpoint{
		{provider: "a", baseURL: "http://a:1234"},
		{provider: "b", baseURL: "http://b:1234"},
		{provider: "c", baseURL: "http://c:1234"},
	}

	store := routehealth.NewProbeStore()
	runStartupAlivenessProbes(context.Background(), targets, store, fakeProber, 20*time.Millisecond)

	unreachable := store.UnreachableProviders(time.Now().UTC(), time.Minute)
	if _, ok := unreachable["a"]; !ok {
		t.Fatalf("attempted failed provider missing from ProbeUnreachable: %#v", unreachable)
	}
	for _, provider := range []string{"b", "c"} {
		if _, ok := unreachable[provider]; ok {
			t.Fatalf("skipped provider %q surfaced in ProbeUnreachable: %#v", provider, unreachable)
		}
		if _, ok := store.LastProbe(provider, ""); ok {
			t.Fatalf("skipped provider %q has a probe record", provider)
		}
	}
}

func TestServiceStartup_FailedProbeHardGatesRouting(t *testing.T) {
	cacheRoot := t.TempDir()
	t.Setenv("FIZEAU_CACHE_DIR", cacheRoot)
	cache := &discoverycache.Cache{Root: cacheRoot}
	baseURL := "http://127.0.0.1:1/v1"
	writeSnapshotDiscoveryFixture(t, cache, testDiscoverySourceName("down", "down", baseURL, ""), time.Now().UTC(), []string{"qwen3.5-27b"})

	rawSvc, err := New(ServiceOptions{
		ServiceConfig: &fakeServiceConfig{
			providers: map[string]ServiceProviderEntry{
				"down": {
					Type:                "lmstudio",
					BaseURL:             baseURL,
					Model:               "qwen3.5-27b",
					Billing:             BillingModelFixed,
					IncludeByDefault:    true,
					IncludeByDefaultSet: true,
				},
			},
			names:       []string{"down"},
			defaultName: "down",
		},
		QuotaRefreshContext: canceledRefreshContext(),
		AlivenessProber: func(context.Context, string, string) bool {
			return false
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	svc := rawSvc.(*service)
	inputs, _ := svc.buildRoutingInputsWithCatalog(context.Background(), serviceRoutingCatalog(), modelsnapshot.RefreshIfStale)
	if _, ok := inputs.ProbeUnreachable["down"]; !ok {
		t.Fatalf("ProbeUnreachable missing down: %#v", inputs.ProbeUnreachable)
	}
	dec, err := svc.ResolveRoute(context.Background(), RouteRequest{Model: "qwen3.5-27b"})
	if err == nil {
		t.Fatalf("ResolveRoute succeeded, decision=%#v", dec)
	}
	var sawDown bool
	for _, c := range dec.Candidates {
		if c.Provider != "down" {
			continue
		}
		sawDown = true
		if c.Eligible {
			t.Fatalf("down provider should be gated by failed startup probe: %#v", c)
		}
		if c.FilterReason != FilterReasonEndpointUnreachable {
			t.Fatalf("FilterReason=%q, want %q", c.FilterReason, FilterReasonEndpointUnreachable)
		}
	}
	if !sawDown {
		t.Fatalf("missing down provider candidate: %#v", dec.Candidates)
	}
}

// TestProbeLoop_RetriesDeadProvidersOnInterval asserts that a provider marked
// unreachable by probe is re-probed every HealthProbeInterval and recorded back
// to reachable when it comes online (AC #2).
func TestProbeLoop_RetriesDeadProvidersOnInterval(t *testing.T) {
	baseTime := time.Date(2026, 5, 14, 10, 0, 0, 0, time.UTC)
	interval := 60 * time.Second

	var mu sync.Mutex
	probeResults := []bool{false, true} // first call fails, second succeeds
	probeCount := 0

	prober := ProviderAlivenessProber(func(_ context.Context, _ string, _ string) bool {
		mu.Lock()
		defer mu.Unlock()
		idx := probeCount
		probeCount++
		if idx < len(probeResults) {
			return probeResults[idx]
		}
		return true
	})

	store := routehealth.NewProbeStore()
	if !store.ProbeNeeded("never-probed", "", baseTime, interval) {
		t.Fatal("never-probed providers should be considered due for probing")
	}
	targets := []alivenessEndpoint{
		{provider: "bragi", baseURL: "http://bragi:1234"},
	}

	iteration := 0
	nowFn := func() time.Time {
		return baseTime.Add(time.Duration(iteration) * interval)
	}

	loopsDone := 0
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sleepFn := func(_ context.Context, _ time.Duration) bool {
		loopsDone++
		iteration++
		if loopsDone >= 2 {
			cancel()
			return false
		}
		return true
	}

	runAlivenessProbeLoop(ctx, targets, store, prober, interval, nowFn, sleepFn, "")

	mu.Lock()
	gotCount := probeCount
	mu.Unlock()

	if gotCount != 2 {
		t.Fatalf("expected 2 probes (one per loop iteration), got %d", gotCount)
	}

	r, ok := store.LastProbe("bragi", "")
	if !ok {
		t.Fatal("no probe record for bragi after loop")
	}
	if !r.LastProbeSuccess {
		t.Fatal("expected bragi to be reachable after second probe (prober returned true)")
	}
}

// TestProbeLoop_SkipsProvidersWithFreshProbes asserts that providers probed
// recently are not re-probed until the interval elapses.
func TestProbeLoop_SkipsProvidersWithFreshProbes(t *testing.T) {
	baseTime := time.Date(2026, 5, 14, 10, 0, 0, 0, time.UTC)
	interval := 60 * time.Second

	var mu sync.Mutex
	probeCount := 0
	prober := ProviderAlivenessProber(func(_ context.Context, _ string, _ string) bool {
		mu.Lock()
		probeCount++
		mu.Unlock()
		return true
	})

	store := routehealth.NewProbeStore()
	// Pre-record a fresh probe — should skip re-probing in the first iteration.
	store.RecordProbe("bragi", "", true, baseTime.Add(-10*time.Second))

	targets := []alivenessEndpoint{
		{provider: "bragi", baseURL: "http://bragi:1234"},
	}

	iteration := 0
	nowFn := func() time.Time {
		return baseTime.Add(time.Duration(iteration) * interval)
	}

	loopsDone := 0
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sleepFn := func(_ context.Context, _ time.Duration) bool {
		loopsDone++
		iteration++
		if loopsDone >= 1 {
			cancel()
			return false
		}
		return true
	}

	runAlivenessProbeLoop(ctx, targets, store, prober, interval, nowFn, sleepFn, "")

	mu.Lock()
	gotCount := probeCount
	mu.Unlock()

	// Probe at T=0: elapsed = 10s < 60s → skip. So probeCount should be 0.
	if gotCount != 0 {
		t.Errorf("expected 0 probes (fresh probe within interval), got %d", gotCount)
	}
}

func TestExtractHostPort(t *testing.T) {
	cases := []struct {
		baseURL string
		want    string
	}{
		{"http://bragi:1234", "bragi:1234"},
		{"https://example.com", "example.com:443"},
		{"http://localhost", "localhost:80"},
		{"http://127.0.0.1:11434", "127.0.0.1:11434"},
		{"", ""},
		{"not a url ://", ""},
	}
	for _, tc := range cases {
		got := extractHostPort(tc.baseURL)
		if got != tc.want {
			t.Errorf("extractHostPort(%q) = %q, want %q", tc.baseURL, got, tc.want)
		}
	}
}
