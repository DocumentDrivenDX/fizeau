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

func TestResolveRoute_RefreshesUnknownLocalHealthBeforeScoring(t *testing.T) {
	cacheRoot := t.TempDir()
	t.Setenv("FIZEAU_CACHE_DIR", cacheRoot)
	cache := &discoverycache.Cache{Root: cacheRoot}
	modelID := "mlx-community/Qwen3.6-27B-8bit"
	baseURL := "http://grendel:8000/v1"
	writeSnapshotDiscoveryFixture(t, cache, testDiscoverySourceName("grendel", "grendel", baseURL, ""), time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC), []string{modelID})

	catalog := loadRoutingFixtureCatalog(t, `
version: 5
generated_at: 2026-05-15T00:00:00Z
catalog_version: test
policies:
  default:
    min_power: 1
    max_power: 10
    allow_local: true
models:
  mlx-community/Qwen3.6-27B-8bit:
    family: qwen
    status: active
    power: 7
    context_window: 32768
    surfaces:
      embedded-openai: mlx-community/Qwen3.6-27B-8bit
`)
	t.Cleanup(replaceRoutingCatalogForTest(t, catalog))

	var mu sync.Mutex
	probeCount := 0
	svc := newResolveRouteProbeTestService(t, &fakeServiceConfig{
		providers: map[string]ServiceProviderEntry{
			"grendel": {
				Type:           "rapid-mlx",
				BaseURL:        baseURL,
				ServerInstance: "grendel",
				Model:          modelID,
			},
		},
		names:       []string{"grendel"},
		defaultName: "grendel",
	}, func(_ context.Context, provider, gotBaseURL string) bool {
		if provider != "grendel" || gotBaseURL != baseURL {
			t.Fatalf("probe target = %q %q, want grendel %q", provider, gotBaseURL, baseURL)
		}
		mu.Lock()
		probeCount++
		mu.Unlock()
		return true
	})

	dec, err := svc.ResolveRoute(context.Background(), RouteRequest{Model: modelID})
	if err != nil {
		t.Fatalf("ResolveRoute: %v", err)
	}
	if dec == nil {
		t.Fatal("ResolveRoute returned nil decision")
	}
	if dec.Provider != "grendel" || dec.Model != modelID {
		t.Fatalf("decision=%#v, want grendel/%s", dec, modelID)
	}
	mu.Lock()
	gotProbeCount := probeCount
	mu.Unlock()
	if gotProbeCount != 1 {
		t.Fatalf("probe count = %d, want 1 route-time refresh probe", gotProbeCount)
	}
	var candidate *RouteCandidate
	for i := range dec.Candidates {
		if dec.Candidates[i].Provider == "grendel" {
			candidate = &dec.Candidates[i]
			break
		}
	}
	if candidate == nil {
		t.Fatalf("missing grendel candidate in %#v", dec.Candidates)
	}
	if !candidate.Eligible {
		t.Fatalf("candidate=%#v, want eligible after successful route-time probe", *candidate)
	}
	if candidate.LastProbeAt.IsZero() || !candidate.LastProbeSuccess {
		t.Fatalf("candidate probe evidence=%#v, want successful route-time probe evidence", *candidate)
	}
}

func TestResolveRoute_UnknownLocalHealthRefreshFailureIsEvidence(t *testing.T) {
	cacheRoot := t.TempDir()
	t.Setenv("FIZEAU_CACHE_DIR", cacheRoot)
	cache := &discoverycache.Cache{Root: cacheRoot}
	modelID := "mlx-community/Qwen3.6-27B-8bit"
	grendelURL := "http://grendel:8000/v1"
	vidarURL := "http://vidar:8001/v1"
	capturedAt := time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)
	writeSnapshotDiscoveryFixture(t, cache, testDiscoverySourceName("grendel", "grendel", grendelURL, ""), capturedAt, []string{modelID})
	writeSnapshotDiscoveryFixture(t, cache, testDiscoverySourceName("vidar", "vidar", vidarURL, ""), capturedAt, []string{modelID})

	catalog := loadRoutingFixtureCatalog(t, `
version: 5
generated_at: 2026-05-15T00:00:00Z
catalog_version: test
policies:
  default:
    min_power: 1
    max_power: 10
    allow_local: true
models:
  mlx-community/Qwen3.6-27B-8bit:
    family: qwen
    status: active
    power: 7
    context_window: 32768
    surfaces:
      embedded-openai: mlx-community/Qwen3.6-27B-8bit
`)
	t.Cleanup(replaceRoutingCatalogForTest(t, catalog))

	var mu sync.Mutex
	probeCount := 0
	svc := newResolveRouteProbeTestService(t, &fakeServiceConfig{
		providers: map[string]ServiceProviderEntry{
			"grendel": {
				Type:           "rapid-mlx",
				BaseURL:        grendelURL,
				ServerInstance: "grendel",
				Model:          modelID,
			},
			"vidar": {
				Type:           "rapid-mlx",
				BaseURL:        vidarURL,
				ServerInstance: "vidar",
				Model:          modelID,
			},
		},
		names:       []string{"grendel", "vidar"},
		defaultName: "grendel",
	}, func(_ context.Context, provider, _ string) bool {
		mu.Lock()
		probeCount++
		mu.Unlock()
		return provider != "grendel"
	})

	dec, err := svc.ResolveRoute(context.Background(), RouteRequest{Model: modelID})
	if err != nil {
		t.Fatalf("ResolveRoute: %v", err)
	}
	if dec == nil {
		t.Fatal("ResolveRoute returned nil decision")
	}
	if dec.Provider != "vidar" {
		t.Fatalf("decision=%#v, want vidar after grendel refresh failure", dec)
	}
	mu.Lock()
	gotProbeCount := probeCount
	mu.Unlock()
	if gotProbeCount != 2 {
		t.Fatalf("probe count = %d, want 2 route-time refresh probes", gotProbeCount)
	}
	var sawGrendel bool
	for _, candidate := range dec.Candidates {
		if candidate.Provider != "grendel" {
			continue
		}
		sawGrendel = true
		if candidate.Eligible {
			t.Fatalf("grendel candidate=%#v, want rejected after failed route-time probe", candidate)
		}
		if candidate.FilterReason != FilterReasonEndpointUnreachable {
			t.Fatalf("grendel FilterReason=%q, want %q", candidate.FilterReason, FilterReasonEndpointUnreachable)
		}
		if candidate.LastProbeAt.IsZero() || candidate.LastProbeSuccess {
			t.Fatalf("grendel probe evidence=%#v, want recorded failed probe evidence", candidate)
		}
	}
	if !sawGrendel {
		t.Fatalf("missing grendel candidate in %#v", dec.Candidates)
	}
}

func TestResolveRoute_DoesNotProbeFreshHealthOnEveryRun(t *testing.T) {
	cacheRoot := t.TempDir()
	t.Setenv("FIZEAU_CACHE_DIR", cacheRoot)
	cache := &discoverycache.Cache{Root: cacheRoot}
	modelID := "mlx-community/Qwen3.6-27B-8bit"
	baseURL := "http://grendel:8000/v1"
	writeSnapshotDiscoveryFixture(t, cache, testDiscoverySourceName("grendel", "grendel", baseURL, ""), time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC), []string{modelID})

	catalog := loadRoutingFixtureCatalog(t, `
version: 5
generated_at: 2026-05-15T00:00:00Z
catalog_version: test
policies:
  default:
    min_power: 1
    max_power: 10
    allow_local: true
models:
  mlx-community/Qwen3.6-27B-8bit:
    family: qwen
    status: active
    power: 7
    context_window: 32768
    surfaces:
      embedded-openai: mlx-community/Qwen3.6-27B-8bit
`)
	t.Cleanup(replaceRoutingCatalogForTest(t, catalog))

	var mu sync.Mutex
	probeCount := 0
	svc := newResolveRouteProbeTestService(t, &fakeServiceConfig{
		providers: map[string]ServiceProviderEntry{
			"grendel": {
				Type:           "rapid-mlx",
				BaseURL:        baseURL,
				ServerInstance: "grendel",
				Model:          modelID,
			},
		},
		names:       []string{"grendel"},
		defaultName: "grendel",
	}, func(_ context.Context, _, _ string) bool {
		mu.Lock()
		probeCount++
		mu.Unlock()
		return true
	})

	first, err := svc.ResolveRoute(context.Background(), RouteRequest{Model: modelID})
	if err != nil {
		t.Fatalf("first ResolveRoute: %v", err)
	}
	second, err := svc.ResolveRoute(context.Background(), RouteRequest{Model: modelID})
	if err != nil {
		t.Fatalf("second ResolveRoute: %v", err)
	}
	if first == nil || second == nil {
		t.Fatalf("decisions=%#v %#v, want non-nil decisions", first, second)
	}
	mu.Lock()
	gotProbeCount := probeCount
	mu.Unlock()
	if gotProbeCount != 1 {
		t.Fatalf("probe count = %d, want 1 route-time probe across two fresh resolves", gotProbeCount)
	}
}

func newResolveRouteProbeTestService(t *testing.T, sc *fakeServiceConfig, prober ProviderAlivenessProber) *service {
	t.Helper()
	svc := newTestService(t, ServiceOptions{
		ServiceConfig:       sc,
		AlivenessProber:     prober,
		HealthProbeInterval: time.Hour,
	})
	svc.providerProbe = routehealth.NewProbeStore()
	return svc
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
