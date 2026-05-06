package fizeau

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/fizeau/internal/harnesses"
	claudeharness "github.com/DocumentDrivenDX/fizeau/internal/harnesses/claude"
	codexharness "github.com/DocumentDrivenDX/fizeau/internal/harnesses/codex"
)

// fakeServiceConfig implements ServiceConfig for tests.
type fakeServiceConfig struct {
	providers      map[string]ServiceProviderEntry
	names          []string
	defaultName    string
	healthCooldown time.Duration
	workDir        string
}

func (f *fakeServiceConfig) ProviderNames() []string     { return f.names }
func (f *fakeServiceConfig) DefaultProviderName() string { return f.defaultName }
func (f *fakeServiceConfig) Provider(name string) (ServiceProviderEntry, bool) {
	e, ok := f.providers[name]
	return e, ok
}
func (f *fakeServiceConfig) HealthCooldown() time.Duration { return f.healthCooldown }
func (f *fakeServiceConfig) WorkDir() string               { return f.workDir }
func (f *fakeServiceConfig) SessionLogDir() string {
	if f.workDir == "" {
		return ""
	}
	return filepath.Join(f.workDir, ".fizeau", "sessions")
}
func TestListProviders_NoServiceConfig(t *testing.T) {
	svc := newTestService(t, ServiceOptions{})
	_, err := svc.ListProviders(context.Background())
	if err == nil {
		t.Fatal("expected error when ServiceConfig is nil")
	}
}

func TestListProviders_Connected(t *testing.T) {
	// Spin up a fake /v1/models server.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" || r.URL.Path == "/models" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{
					{"id": "model-a"},
					{"id": "model-b"},
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer ts.Close()

	sc := &fakeServiceConfig{
		providers: map[string]ServiceProviderEntry{
			"local": {Type: "lmstudio", BaseURL: ts.URL + "/v1", Model: "model-a"},
		},
		names:       []string{"local"},
		defaultName: "local",
	}
	svc := newTestService(t, ServiceOptions{ServiceConfig: sc})

	infos, err := svc.ListProviders(context.Background())
	if err != nil {
		t.Fatalf("ListProviders: %v", err)
	}
	if len(infos) != 1 {
		t.Fatalf("want 1 provider, got %d", len(infos))
	}
	info := infos[0]
	if info.Name != "local" {
		t.Errorf("Name: got %q, want %q", info.Name, "local")
	}
	if info.Status != "connected" {
		t.Errorf("Status: got %q, want %q", info.Status, "connected")
	}
	if info.ModelCount != 2 {
		t.Errorf("ModelCount: got %d, want 2", info.ModelCount)
	}
	if !info.IsDefault {
		t.Error("IsDefault should be true for the default provider")
	}
	if info.DefaultModel != "model-a" {
		t.Errorf("DefaultModel: got %q, want %q", info.DefaultModel, "model-a")
	}
	if info.Type != "lmstudio" {
		t.Errorf("Type: got %q, want %q", info.Type, "lmstudio")
	}
	if slices.Contains(info.Capabilities, "reasoning_control") {
		t.Fatalf("lmstudio capabilities must not claim reasoning_control: %#v", info.Capabilities)
	}
	if len(info.EndpointStatus) != 1 {
		t.Fatalf("EndpointStatus length: got %d, want 1", len(info.EndpointStatus))
	}
	if info.EndpointStatus[0].Status != "connected" || info.EndpointStatus[0].ModelCount != 2 || info.EndpointStatus[0].LastSuccessAt.IsZero() {
		t.Fatalf("EndpointStatus[0]: %#v", info.EndpointStatus[0])
	}
	if info.LastError != nil {
		t.Fatalf("LastError: got %#v, want nil", info.LastError)
	}
}

func TestListProviders_LlamaServerConnected(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" || r.URL.Path == "/models" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{{"id": "llama-3.1"}},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer ts.Close()

	sc := &fakeServiceConfig{
		providers: map[string]ServiceProviderEntry{
			"llama": {Type: "llama-server", BaseURL: ts.URL + "/v1"},
		},
		names:       []string{"llama"},
		defaultName: "llama",
	}
	svc := newTestService(t, ServiceOptions{ServiceConfig: sc})

	infos, err := svc.ListProviders(context.Background())
	if err != nil {
		t.Fatalf("ListProviders: %v", err)
	}
	if len(infos) != 1 {
		t.Fatalf("want 1 provider, got %d", len(infos))
	}
	if infos[0].Type != "llama-server" {
		t.Fatalf("provider type = %q, want llama-server", infos[0].Type)
	}
	if infos[0].Status != "connected" {
		t.Fatalf("provider status = %q, want connected", infos[0].Status)
	}
}

func TestListProviders_OMLXAdvertisesReasoningControl(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" || r.URL.Path == "/models" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{{"id": "Qwen3.6-27B-MLX-8bit"}}})
			return
		}
		http.NotFound(w, r)
	}))
	defer ts.Close()

	sc := &fakeServiceConfig{
		providers: map[string]ServiceProviderEntry{
			"vidar-omlx": {Type: "omlx", BaseURL: ts.URL + "/v1", Model: "Qwen3.6-27B-MLX-8bit"},
		},
		names:       []string{"vidar-omlx"},
		defaultName: "vidar-omlx",
	}
	svc := newTestService(t, ServiceOptions{ServiceConfig: sc})

	infos, err := svc.ListProviders(context.Background())
	if err != nil {
		t.Fatalf("ListProviders: %v", err)
	}
	if len(infos) != 1 {
		t.Fatalf("want 1 provider, got %d", len(infos))
	}
	if !slices.Contains(infos[0].Capabilities, "reasoning_control") {
		t.Fatalf("omlx capabilities must include reasoning_control: %#v", infos[0].Capabilities)
	}
}

func TestListProviders_Unreachable(t *testing.T) {
	sc := &fakeServiceConfig{
		providers: map[string]ServiceProviderEntry{
			"remote": {Type: "lmstudio", BaseURL: "http://127.0.0.1:19999/v1"},
		},
		names:       []string{"remote"},
		defaultName: "remote",
	}
	svc := newTestService(t, ServiceOptions{ServiceConfig: sc})

	infos, err := svc.ListProviders(context.Background())
	if err != nil {
		t.Fatalf("ListProviders: %v", err)
	}
	if len(infos) != 1 {
		t.Fatalf("want 1 provider, got %d", len(infos))
	}
	if infos[0].Status != "unreachable" {
		t.Errorf("Status: got %q, want %q", infos[0].Status, "unreachable")
	}
	if infos[0].LastError == nil || infos[0].LastError.Type != "unavailable" {
		t.Fatalf("LastError: got %#v, want unavailable", infos[0].LastError)
	}
	if len(infos[0].EndpointStatus) == 0 || infos[0].EndpointStatus[0].Status != "unreachable" {
		t.Fatalf("EndpointStatus: %#v", infos[0].EndpointStatus)
	}
}

func TestProviderStatus_EndpointDownSurfaced(t *testing.T) {
	healthy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" && r.URL.Path != "/models" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{{"id": "healthy-model"}},
		})
	}))
	defer healthy.Close()

	sc := &fakeServiceConfig{
		providers: map[string]ServiceProviderEntry{
			"omlx": {
				Type: "omlx",
				Endpoints: []ServiceProviderEndpoint{
					{Name: "dead", BaseURL: "http://127.0.0.1:19999/v1"},
					{Name: "healthy", BaseURL: healthy.URL + "/v1"},
				},
			},
		},
		names:       []string{"omlx"},
		defaultName: "omlx",
	}
	svc := newTestService(t, ServiceOptions{ServiceConfig: sc})

	infos, err := svc.ListProviders(context.Background())
	if err != nil {
		t.Fatalf("ListProviders: %v", err)
	}
	if len(infos) != 1 {
		t.Fatalf("want 1 provider, got %d", len(infos))
	}
	info := infos[0]
	if info.Status != "connected" {
		t.Fatalf("Status: got %q, want connected", info.Status)
	}
	if info.ModelCount != 1 {
		t.Fatalf("ModelCount: got %d, want 1", info.ModelCount)
	}
	if len(info.EndpointStatus) != 2 {
		t.Fatalf("EndpointStatus length: got %d, want 2", len(info.EndpointStatus))
	}
	byName := map[string]EndpointStatus{}
	for _, status := range info.EndpointStatus {
		byName[status.Name] = status
	}
	dead := byName["dead"]
	if dead.Status != "unreachable" {
		t.Fatalf("dead endpoint status: got %#v", dead)
	}
	if dead.LastError == nil || !strings.Contains(strings.ToLower(dead.LastError.Detail), "connection refused") {
		t.Fatalf("dead endpoint last error: got %#v, want connection refused detail", dead.LastError)
	}
	healthyStatus := byName["healthy"]
	if healthyStatus.Status != "connected" || healthyStatus.ModelCount != 1 || healthyStatus.LastSuccessAt.IsZero() {
		t.Fatalf("healthy endpoint status: %#v", healthyStatus)
	}
}

func TestListProviders_Anthropic(t *testing.T) {
	sc := &fakeServiceConfig{
		providers: map[string]ServiceProviderEntry{
			"claude-api": {Type: "anthropic", APIKey: "sk-test"},
		},
		names:       []string{"claude-api"},
		defaultName: "claude-api",
	}
	svc := newTestService(t, ServiceOptions{ServiceConfig: sc})

	infos, err := svc.ListProviders(context.Background())
	if err != nil {
		t.Fatalf("ListProviders: %v", err)
	}
	if len(infos) != 1 {
		t.Fatalf("want 1 provider, got %d", len(infos))
	}
	info := infos[0]
	if info.Status != "connected" {
		t.Errorf("anthropic with key: Status got %q, want %q", info.Status, "connected")
	}
	if info.Type != "anthropic" {
		t.Errorf("Type: got %q, want %q", info.Type, "anthropic")
	}
}

func TestListProviders_AnthropicNoKey(t *testing.T) {
	sc := &fakeServiceConfig{
		providers:   map[string]ServiceProviderEntry{"claude-api": {Type: "anthropic"}},
		names:       []string{"claude-api"},
		defaultName: "claude-api",
	}
	svc := newTestService(t, ServiceOptions{ServiceConfig: sc})

	infos, err := svc.ListProviders(context.Background())
	if err != nil {
		t.Fatalf("ListProviders: %v", err)
	}
	if infos[0].Status != "error: api_key not configured" {
		t.Errorf("unexpected status: %s", infos[0].Status)
	}
	if !infos[0].Auth.Unauthenticated {
		t.Fatalf("Auth: got %#v, want unauthenticated", infos[0].Auth)
	}
	if infos[0].LastError == nil || infos[0].LastError.Type != "unauthenticated" {
		t.Fatalf("LastError: got %#v, want unauthenticated", infos[0].LastError)
	}
}

func TestHealthCheck_NoServiceConfig(t *testing.T) {
	svc := newTestService(t, ServiceOptions{})
	err := svc.HealthCheck(context.Background(), HealthTarget{Type: "provider", Name: "x"})
	if err == nil {
		t.Fatal("expected error when ServiceConfig is nil")
	}
}

func TestHealthCheck_Provider_Connected(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"data": []any{}})
	}))
	defer ts.Close()

	sc := &fakeServiceConfig{
		providers: map[string]ServiceProviderEntry{
			"local": {Type: "lmstudio", BaseURL: ts.URL + "/v1"},
		},
	}
	svc := newTestService(t, ServiceOptions{ServiceConfig: sc})

	if err := svc.HealthCheck(context.Background(), HealthTarget{Type: "provider", Name: "local"}); err != nil {
		t.Errorf("HealthCheck connected provider: unexpected error: %v", err)
	}
}

func TestHealthCheckProviders_UnreachableIncludesReason(t *testing.T) {
	sc := &fakeServiceConfig{
		providers: map[string]ServiceProviderEntry{
			"dead": {Type: "lmstudio", BaseURL: "http://127.0.0.1:19999/v1"},
		},
	}
	svc := newTestService(t, ServiceOptions{ServiceConfig: sc})

	err := svc.HealthCheck(context.Background(), HealthTarget{Type: "provider", Name: "dead"})
	if err == nil {
		t.Fatal("expected error for unreachable provider")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "connection refused") {
		t.Fatalf("expected concrete reachability detail, got %v", err)
	}
}

func TestHealthCheck_Provider_NotFound(t *testing.T) {
	sc := &fakeServiceConfig{providers: map[string]ServiceProviderEntry{}}
	svc := newTestService(t, ServiceOptions{ServiceConfig: sc})

	err := svc.HealthCheck(context.Background(), HealthTarget{Type: "provider", Name: "missing"})
	if err == nil {
		t.Fatal("expected error for missing provider")
	}
}

func TestHealthCheck_Harness_Available(t *testing.T) {
	svc := newTestService(t, ServiceOptions{})
	// "agent" is always available (embedded).
	if err := svc.HealthCheck(context.Background(), HealthTarget{Type: "harness", Name: "agent"}); err != nil {
		t.Errorf("HealthCheck embedded harness: unexpected error: %v", err)
	}
}

func TestHealthCheck_Harness_NotRegistered(t *testing.T) {
	svc := newTestService(t, ServiceOptions{})
	err := svc.HealthCheck(context.Background(), HealthTarget{Type: "harness", Name: "nonexistent-harness-xyz"})
	if err == nil {
		t.Fatal("expected error for unregistered harness")
	}
}

func TestHealthCheck_InvalidType(t *testing.T) {
	svc := newTestService(t, ServiceOptions{})
	err := svc.HealthCheck(context.Background(), HealthTarget{Type: "invalid", Name: "x"})
	if err == nil {
		t.Fatal("expected error for invalid HealthTarget.Type")
	}
}

func TestNormalizeServiceProviderType(t *testing.T) {
	cases := []struct{ in, want string }{
		{"lmstudio", "lmstudio"},
		{"openai", "openai"},
		{"", "openai"},
		{"anthropic", "anthropic"},
		{"custom", "custom"},
	}
	for _, tc := range cases {
		got := normalizeServiceProviderType(tc.in)
		if got != tc.want {
			t.Errorf("normalizeServiceProviderType(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestHealthCheck_ClaudeRefreshesQuotaWhenStale verifies that HealthCheck
// triggers a quota cache refresh when the cached snapshot is older than
// default quota refresh debounce (15m).
func TestHealthCheck_ClaudeRefreshesQuotaWhenStale(t *testing.T) {
	dir := t.TempDir()
	cachePath := filepath.Join(dir, "claude-quota.json")
	t.Setenv("FIZEAU_CLAUDE_QUOTA_CACHE", cachePath)

	// Write a snapshot older than the 15m debounce.
	staleSnap := claudeharness.ClaudeQuotaSnapshot{
		CapturedAt:        time.Now().UTC().Add(-20 * time.Minute),
		FiveHourRemaining: 80,
		FiveHourLimit:     100,
		WeeklyRemaining:   90,
		WeeklyLimit:       100,
		Source:            "pty",
	}
	if err := claudeharness.WriteClaudeQuota(cachePath, staleSnap); err != nil {
		t.Fatalf("setup: WriteClaudeQuota: %v", err)
	}

	// Inject a fake refresher so no real PTY probe is invoked.
	refreshCalled := false
	setClaudeQuotaRefresherForTest(t, func(timeout time.Duration) ([]harnesses.QuotaWindow, *harnesses.AccountInfo, error) {
		refreshCalled = true
		return []harnesses.QuotaWindow{
			{LimitID: "session", UsedPercent: 20},
			{LimitID: "weekly-all", UsedPercent: 10},
		}, nil, nil
	})

	svc := newTestService(t, ServiceOptions{})
	// HealthCheck for "claude" requires the binary to be discoverable.
	// If claude is not in PATH, the harness is unavailable → the quota refresh
	// is never reached. To keep the test self-contained we call the helper
	// directly rather than going through HealthCheck's availability gate.
	healthCheckRefreshClaudeQuota(context.Background())

	if !refreshCalled {
		t.Error("expected healthCheckClaudeQuotaRefresher to be called for stale cache")
	}

	// Verify the cache was rewritten with a newer timestamp.
	loaded, ok := claudeharness.ReadClaudeQuotaFrom(cachePath)
	if !ok {
		t.Fatal("expected cache file to exist after refresh")
	}
	if !loaded.CapturedAt.After(staleSnap.CapturedAt) {
		t.Errorf("expected cache CapturedAt to be newer than stale snapshot: got %v, stale was %v",
			loaded.CapturedAt, staleSnap.CapturedAt)
	}
	_ = svc
}

// TestHealthCheck_ClaudeSkipsRefreshWhenFresh verifies that HealthCheck does
// NOT invoke the PTY quota refresher when the cached snapshot is younger than
// default quota refresh debounce (15m).
func TestHealthCheck_ClaudeSkipsRefreshWhenFresh(t *testing.T) {
	dir := t.TempDir()
	cachePath := filepath.Join(dir, "claude-quota.json")
	t.Setenv("FIZEAU_CLAUDE_QUOTA_CACHE", cachePath)

	// Write a snapshot that is only 30s old (fresh).
	freshSnap := claudeharness.ClaudeQuotaSnapshot{
		CapturedAt:        time.Now().UTC().Add(-30 * time.Second),
		FiveHourRemaining: 80,
		FiveHourLimit:     100,
		WeeklyRemaining:   90,
		WeeklyLimit:       100,
		Source:            "pty",
		Account:           &harnesses.AccountInfo{PlanType: "Claude Max"},
	}
	if err := claudeharness.WriteClaudeQuota(cachePath, freshSnap); err != nil {
		t.Fatalf("setup: WriteClaudeQuota: %v", err)
	}

	// Inject a fake refresher that must NOT be called.
	refreshCalled := false
	setClaudeQuotaRefresherForTest(t, func(timeout time.Duration) ([]harnesses.QuotaWindow, *harnesses.AccountInfo, error) {
		refreshCalled = true
		return nil, nil, nil
	})

	healthCheckRefreshClaudeQuota(context.Background())

	if refreshCalled {
		t.Error("expected healthCheckClaudeQuotaRefresher NOT to be called for fresh cache")
	}

	// Verify the cache timestamp is unchanged (still matches freshSnap).
	loaded, ok := claudeharness.ReadClaudeQuotaFrom(cachePath)
	if !ok {
		t.Fatal("expected cache file to still exist")
	}
	if !loaded.CapturedAt.Equal(freshSnap.CapturedAt) {
		t.Errorf("cache was unexpectedly rewritten: got CapturedAt %v, want %v",
			loaded.CapturedAt, freshSnap.CapturedAt)
	}
}

// TestHealthCheck_GeminiDoesNotInvokeQuotaProbe verifies that HealthCheck for
// Gemini does not call Claude/Codex PTY quota refreshers. Gemini quota status
// is auth/account-gated until the CLI exposes a stable numeric quota counter.
func TestHealthCheck_GeminiDoesNotInvokeQuotaProbe(t *testing.T) {
	// Inject a counter to detect unexpected calls.
	probeCalled := false
	setClaudeQuotaRefresherForTest(t, func(timeout time.Duration) ([]harnesses.QuotaWindow, *harnesses.AccountInfo, error) {
		probeCalled = true
		return nil, nil, nil
	})

	svc := newTestService(t, ServiceOptions{})
	// "gemini" is registered but unavailable in CI (binary not found).
	// HealthCheck returns an error but must not invoke the quota refresher.
	_ = svc.HealthCheck(context.Background(), HealthTarget{Type: "harness", Name: "gemini"})

	if probeCalled {
		t.Error("healthCheckClaudeQuotaRefresher must not be called for gemini harness")
	}
}

func TestHealthCheck_CodexRefreshesQuotaWhenStale(t *testing.T) {
	dir := t.TempDir()
	cachePath := filepath.Join(dir, "codex-quota.json")
	t.Setenv("FIZEAU_CODEX_QUOTA_CACHE", cachePath)
	disableCodexSessionQuotaReaderForTest(t)

	staleSnap := codexharness.CodexQuotaSnapshot{
		CapturedAt: time.Now().UTC().Add(-20 * time.Minute),
		Source:     "pty",
		Windows:    []harnesses.QuotaWindow{{LimitID: "codex", UsedPercent: 80}},
	}
	if err := codexharness.WriteCodexQuota(cachePath, staleSnap); err != nil {
		t.Fatalf("setup: WriteCodexQuota: %v", err)
	}

	refreshCalled := false
	setCodexQuotaRefresherForTest(t, func(timeout time.Duration) ([]harnesses.QuotaWindow, error) {
		refreshCalled = true
		return []harnesses.QuotaWindow{{LimitID: "codex", Name: "5h", UsedPercent: 10, State: "ok"}}, nil
	})

	healthCheckRefreshCodexQuota(context.Background())

	if !refreshCalled {
		t.Error("expected healthCheckCodexQuotaRefresher to be called for stale cache")
	}
	loaded, ok := codexharness.ReadCodexQuotaFrom(cachePath)
	if !ok {
		t.Fatal("expected cache file to exist after refresh")
	}
	if !loaded.CapturedAt.After(staleSnap.CapturedAt) {
		t.Errorf("expected cache CapturedAt to be newer than stale snapshot: got %v, stale was %v",
			loaded.CapturedAt, staleSnap.CapturedAt)
	}
}

func TestHealthCheck_CodexUsesFreshSessionQuotaBeforePTY(t *testing.T) {
	dir := t.TempDir()
	cachePath := filepath.Join(dir, "codex-quota.json")
	sessionRoot := filepath.Join(dir, "sessions")
	t.Setenv("FIZEAU_CODEX_QUOTA_CACHE", cachePath)
	t.Setenv("FIZEAU_CODEX_SESSIONS_DIR", sessionRoot)
	t.Setenv("FIZEAU_CODEX_AUTH", filepath.Join(dir, "missing-auth.json"))

	staleSnap := codexharness.CodexQuotaSnapshot{
		CapturedAt: time.Now().UTC().Add(-20 * time.Minute),
		Source:     "pty",
		Windows:    []harnesses.QuotaWindow{{LimitID: "codex", UsedPercent: 80}},
		Account:    &harnesses.AccountInfo{PlanType: "ChatGPT Pro"},
	}
	if err := codexharness.WriteCodexQuota(cachePath, staleSnap); err != nil {
		t.Fatalf("setup: WriteCodexQuota: %v", err)
	}
	captured := time.Now().UTC().Add(-time.Minute).Truncate(time.Second)
	writeServiceCodexSessionLine(t, filepath.Join(sessionRoot, "fresh.jsonl"), captured, serviceCodexTokenCountLine(captured, "pro", 12))

	refreshCalled := false
	setCodexQuotaRefresherForTest(t, func(timeout time.Duration) ([]harnesses.QuotaWindow, error) {
		refreshCalled = true
		return []harnesses.QuotaWindow{{LimitID: "codex", Name: "5h", UsedPercent: 1, State: "ok"}}, nil
	})

	healthCheckRefreshCodexQuota(context.Background())

	if refreshCalled {
		t.Fatal("PTY refresher should not be called when fresh session token_count quota is usable")
	}
	loaded, ok := codexharness.ReadCodexQuotaFrom(cachePath)
	if !ok {
		t.Fatal("expected cache file to exist after session refresh")
	}
	if loaded.Source != "codex_session_token_count" {
		t.Fatalf("Source: got %q", loaded.Source)
	}
	if !loaded.CapturedAt.Equal(captured) {
		t.Fatalf("CapturedAt: got %s, want session evidence %s", loaded.CapturedAt, captured)
	}
	if loaded.CapturedAt.After(time.Now().UTC().Add(-30 * time.Second)) {
		t.Fatalf("session-derived CapturedAt appears to be time.Now: %s", loaded.CapturedAt)
	}
	if loaded.Account == nil || loaded.Account.PlanType != "ChatGPT Pro" || loaded.Windows[0].UsedPercent != 12 {
		t.Fatalf("loaded snapshot: %#v", loaded)
	}
}

func TestHealthCheck_CodexFallsBackToPTYForStaleOrNonSubsidizedSessionQuota(t *testing.T) {
	cases := []struct {
		name     string
		planType string
		age      time.Duration
	}{
		{name: "stale", planType: "pro", age: 20 * time.Minute},
		{name: "non_subsidized", planType: "free", age: time.Minute},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			cachePath := filepath.Join(dir, "codex-quota.json")
			sessionRoot := filepath.Join(dir, "sessions")
			t.Setenv("FIZEAU_CODEX_QUOTA_CACHE", cachePath)
			t.Setenv("FIZEAU_CODEX_SESSIONS_DIR", sessionRoot)
			t.Setenv("FIZEAU_CODEX_AUTH", filepath.Join(dir, "missing-auth.json"))
			if err := codexharness.WriteCodexQuota(cachePath, codexharness.CodexQuotaSnapshot{
				CapturedAt: time.Now().UTC().Add(-20 * time.Minute),
				Source:     "pty",
				Windows:    []harnesses.QuotaWindow{{LimitID: "codex", UsedPercent: 80}},
				Account:    &harnesses.AccountInfo{PlanType: "ChatGPT Pro"},
			}); err != nil {
				t.Fatalf("setup: WriteCodexQuota: %v", err)
			}
			captured := time.Now().UTC().Add(-tc.age).Truncate(time.Second)
			writeServiceCodexSessionLine(t, filepath.Join(sessionRoot, "session.jsonl"), captured, serviceCodexTokenCountLine(captured, tc.planType, 12))

			refreshCalled := false
			setCodexQuotaRefresherForTest(t, func(timeout time.Duration) ([]harnesses.QuotaWindow, error) {
				refreshCalled = true
				return []harnesses.QuotaWindow{{LimitID: "codex", Name: "5h", UsedPercent: 3, State: "ok"}}, nil
			})

			healthCheckRefreshCodexQuota(context.Background())
			if !refreshCalled {
				t.Fatal("expected PTY fallback")
			}
			loaded, ok := codexharness.ReadCodexQuotaFrom(cachePath)
			if !ok {
				t.Fatal("expected cache after PTY fallback")
			}
			if loaded.Source != "pty" {
				t.Fatalf("Source after fallback: got %q", loaded.Source)
			}
		})
	}
}

func TestPrimaryQuotaRefresh_AutomaticAndThrottled(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("FIZEAU_CLAUDE_QUOTA_CACHE", filepath.Join(dir, "claude-quota.json"))
	t.Setenv("FIZEAU_CODEX_QUOTA_CACHE", filepath.Join(dir, "codex-quota.json"))
	t.Setenv("FIZEAU_CODEX_AUTH", filepath.Join(dir, "missing-codex-auth.json"))
	disableCodexSessionQuotaReaderForTest(t)
	resetPrimaryQuotaRefreshForTest(t)

	var claudeCalls atomic.Int32
	var codexCalls atomic.Int32
	done := make(chan string, 2)

	setClaudeQuotaRefresherForTest(t, func(timeout time.Duration) ([]harnesses.QuotaWindow, *harnesses.AccountInfo, error) {
		claudeCalls.Add(1)
		done <- "claude"
		return []harnesses.QuotaWindow{
			{LimitID: "session", UsedPercent: 20},
			{LimitID: "weekly-all", UsedPercent: 10},
		}, &harnesses.AccountInfo{PlanType: "Claude Max"}, nil
	})

	setCodexQuotaRefresherForTest(t, func(timeout time.Duration) ([]harnesses.QuotaWindow, error) {
		codexCalls.Add(1)
		done <- "codex"
		return []harnesses.QuotaWindow{{LimitID: "codex", Name: "5h", UsedPercent: 10, State: "ok"}}, nil
	})

	svc := newTestService(t, ServiceOptions{})
	if _, err := svc.ListHarnesses(context.Background()); err != nil {
		t.Fatalf("ListHarnesses: %v", err)
	}
	waitForQuotaRefreshes(t, done, "claude", "codex")

	if _, err := svc.ListHarnesses(context.Background()); err != nil {
		t.Fatalf("ListHarnesses second call: %v", err)
	}
	time.Sleep(25 * time.Millisecond)

	if got := claudeCalls.Load(); got != 1 {
		t.Fatalf("claude refresh calls: got %d, want 1", got)
	}
	if got := codexCalls.Load(); got != 1 {
		t.Fatalf("codex refresh calls: got %d, want 1", got)
	}
}

func TestNewWaitsBrieflyForInvalidQuotaRefresh(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("FIZEAU_CLAUDE_QUOTA_CACHE", filepath.Join(dir, "claude-quota.json"))
	t.Setenv("FIZEAU_CODEX_QUOTA_CACHE", filepath.Join(dir, "codex-quota.json"))
	t.Setenv("FIZEAU_CODEX_AUTH", filepath.Join(dir, "missing-codex-auth.json"))
	disableCodexSessionQuotaReaderForTest(t)
	resetPrimaryQuotaRefreshForTest(t)

	setClaudeQuotaRefresherForTest(t, func(timeout time.Duration) ([]harnesses.QuotaWindow, *harnesses.AccountInfo, error) {
		return []harnesses.QuotaWindow{
			{LimitID: "session", UsedPercent: 20},
			{LimitID: "weekly-all", UsedPercent: 10},
		}, &harnesses.AccountInfo{PlanType: "Claude Max"}, nil
	})

	setCodexQuotaRefresherForTest(t, func(timeout time.Duration) ([]harnesses.QuotaWindow, error) {
		return []harnesses.QuotaWindow{{LimitID: "codex", Name: "5h", UsedPercent: 10, State: "ok"}}, nil
	})

	if _, err := New(ServiceOptions{QuotaRefreshStartupWait: time.Second}); err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, ok := claudeharness.ReadClaudeQuota(); !ok {
		t.Fatal("expected startup wait to allow Claude quota cache write")
	}
	if _, ok := codexharness.ReadCodexQuota(); !ok {
		t.Fatal("expected startup wait to allow Codex quota cache write")
	}
}

func TestNewStartupQuotaRefreshContinuesAfterTimeout(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("FIZEAU_CLAUDE_QUOTA_CACHE", filepath.Join(dir, "claude-quota.json"))
	t.Setenv("FIZEAU_CODEX_QUOTA_CACHE", filepath.Join(dir, "codex-quota.json"))
	t.Setenv("FIZEAU_CODEX_AUTH", filepath.Join(dir, "missing-codex-auth.json"))
	disableCodexSessionQuotaReaderForTest(t)
	resetPrimaryQuotaRefreshForTest(t)

	release := make(chan struct{})
	released := false
	t.Cleanup(func() {
		if !released {
			close(release)
		}
	})

	setClaudeQuotaRefresherForTest(t, func(timeout time.Duration) ([]harnesses.QuotaWindow, *harnesses.AccountInfo, error) {
		<-release
		return []harnesses.QuotaWindow{
			{LimitID: "session", UsedPercent: 20},
			{LimitID: "weekly-all", UsedPercent: 10},
		}, &harnesses.AccountInfo{PlanType: "Claude Max"}, nil
	})

	setCodexQuotaRefresherForTest(t, func(timeout time.Duration) ([]harnesses.QuotaWindow, error) {
		<-release
		return []harnesses.QuotaWindow{{LimitID: "codex", Name: "5h", UsedPercent: 10, State: "ok"}}, nil
	})

	start := time.Now()
	if _, err := New(ServiceOptions{QuotaRefreshStartupWait: 20 * time.Millisecond}); err != nil {
		t.Fatalf("New: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 250*time.Millisecond {
		t.Fatalf("New blocked too long: %v", elapsed)
	}
	close(release)
	released = true
	waitForQuotaRefreshFiles(t,
		filepath.Join(dir, "claude-quota.json"),
		filepath.Join(dir, "codex-quota.json"),
	)
}

func TestPrimaryQuotaRefreshWorkerRefreshesOnTimer(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("FIZEAU_CLAUDE_QUOTA_CACHE", filepath.Join(dir, "claude-quota.json"))
	t.Setenv("FIZEAU_CODEX_QUOTA_CACHE", filepath.Join(dir, "codex-quota.json"))
	t.Setenv("FIZEAU_CODEX_AUTH", filepath.Join(dir, "missing-codex-auth.json"))
	disableCodexSessionQuotaReaderForTest(t)
	resetPrimaryQuotaRefreshForTest(t)

	var claudeCalls atomic.Int32
	var codexCalls atomic.Int32
	setClaudeQuotaRefresherForTest(t, func(timeout time.Duration) ([]harnesses.QuotaWindow, *harnesses.AccountInfo, error) {
		claudeCalls.Add(1)
		return []harnesses.QuotaWindow{
			{LimitID: "session", UsedPercent: 20},
			{LimitID: "weekly-all", UsedPercent: 10},
		}, &harnesses.AccountInfo{PlanType: "Claude Max"}, nil
	})

	setCodexQuotaRefresherForTest(t, func(timeout time.Duration) ([]harnesses.QuotaWindow, error) {
		codexCalls.Add(1)
		return []harnesses.QuotaWindow{{LimitID: "codex", Name: "5h", UsedPercent: 10, State: "ok"}}, nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	if _, err := New(ServiceOptions{
		QuotaRefreshContext:     ctx,
		QuotaRefreshDebounce:    time.Millisecond,
		QuotaRefreshStartupWait: time.Second,
		QuotaRefreshInterval:    5 * time.Millisecond,
	}); err != nil {
		t.Fatalf("New: %v", err)
	}

	deadline := time.After(time.Second)
	for claudeCalls.Load() < 2 || codexCalls.Load() < 2 {
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for timer refreshes: claude=%d codex=%d", claudeCalls.Load(), codexCalls.Load())
		default:
			time.Sleep(time.Millisecond)
		}
	}
}

func TestResolveRouteTriggersAsyncQuotaRefreshWithoutBlockingOnIt(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("GEMINI_API_KEY", "")
	t.Setenv("GOOGLE_API_KEY", "")
	t.Setenv("GOOGLE_GENAI_USE_VERTEXAI", "")
	t.Setenv("GOOGLE_GENAI_USE_GCA", "")
	t.Setenv("GEMINI_CLI_USE_COMPUTE_ADC", "")
	t.Setenv("CLOUD_SHELL", "")
	claudeQuotaPath := filepath.Join(dir, "missing-claude-quota.json")
	codexQuotaPath := filepath.Join(dir, "missing-codex-quota.json")
	t.Setenv("FIZEAU_CLAUDE_QUOTA_CACHE", claudeQuotaPath)
	t.Setenv("FIZEAU_CODEX_QUOTA_CACHE", codexQuotaPath)
	t.Setenv("FIZEAU_CODEX_AUTH", filepath.Join(dir, "missing-codex-auth.json"))
	disableCodexSessionQuotaReaderForTest(t)
	resetPrimaryQuotaRefreshForTest(t)

	claudeStarted := make(chan struct{}, 1)
	codexStarted := make(chan struct{}, 1)
	release := make(chan struct{})
	released := false

	setClaudeQuotaRefresherForTest(t, func(timeout time.Duration) ([]harnesses.QuotaWindow, *harnesses.AccountInfo, error) {
		claudeStarted <- struct{}{}
		<-release
		return []harnesses.QuotaWindow{
			{LimitID: "session", UsedPercent: 20},
			{LimitID: "weekly-all", UsedPercent: 10},
		}, &harnesses.AccountInfo{PlanType: "Claude Max"}, nil
	})

	setCodexQuotaRefresherForTest(t, func(timeout time.Duration) ([]harnesses.QuotaWindow, error) {
		codexStarted <- struct{}{}
		<-release
		return []harnesses.QuotaWindow{{LimitID: "codex", Name: "5h", UsedPercent: 10, State: "ok"}}, nil
	})
	t.Cleanup(func() {
		if !released {
			close(release)
		}
	})

	svc := newTestService(t, ServiceOptions{})
	_, err := svc.ResolveRoute(context.Background(), RouteRequest{Profile: "smart"})
	if err == nil {
		t.Fatal("ResolveRoute should not wait for background quota refresh to make missing-cache subscription harnesses eligible")
	}

	waitForQuotaRefreshStarts(t, claudeStarted, codexStarted)
	close(release)
	released = true
	waitForQuotaRefreshFiles(t, claudeQuotaPath, codexQuotaPath)
}

func resetPrimaryQuotaRefreshForTest(t *testing.T) {
	t.Helper()
	primaryQuotaRefresh.mu.Lock()
	oldLast := primaryQuotaRefresh.lastAttempt
	oldInFlight := primaryQuotaRefresh.inFlight
	primaryQuotaRefresh.lastAttempt = make(map[string]time.Time)
	primaryQuotaRefresh.inFlight = make(map[string]bool)
	primaryQuotaRefresh.mu.Unlock()
	t.Cleanup(func() {
		primaryQuotaRefresh.mu.Lock()
		primaryQuotaRefresh.lastAttempt = oldLast
		primaryQuotaRefresh.inFlight = oldInFlight
		primaryQuotaRefresh.mu.Unlock()
	})
}

func setClaudeQuotaRefresherForTest(t *testing.T, fn func(time.Duration) ([]harnesses.QuotaWindow, *harnesses.AccountInfo, error)) {
	t.Helper()
	healthCheckQuotaProbeMu.Lock()
	orig := healthCheckClaudeQuotaRefresher
	healthCheckClaudeQuotaRefresher = fn
	healthCheckQuotaProbeMu.Unlock()
	t.Cleanup(func() {
		healthCheckQuotaProbeMu.Lock()
		healthCheckClaudeQuotaRefresher = orig
		healthCheckQuotaProbeMu.Unlock()
	})
}

func setCodexQuotaRefresherForTest(t *testing.T, fn func(time.Duration) ([]harnesses.QuotaWindow, error)) {
	t.Helper()
	healthCheckQuotaProbeMu.Lock()
	orig := healthCheckCodexQuotaRefresher
	healthCheckCodexQuotaRefresher = fn
	healthCheckQuotaProbeMu.Unlock()
	t.Cleanup(func() {
		healthCheckQuotaProbeMu.Lock()
		healthCheckCodexQuotaRefresher = orig
		healthCheckQuotaProbeMu.Unlock()
	})
}

func disableCodexSessionQuotaReaderForTest(t *testing.T) {
	t.Helper()
	healthCheckQuotaProbeMu.Lock()
	orig := healthCheckCodexSessionQuotaReader
	healthCheckCodexSessionQuotaReader = func() (*codexharness.CodexQuotaSnapshot, bool) {
		return nil, false
	}
	healthCheckQuotaProbeMu.Unlock()
	t.Cleanup(func() {
		healthCheckQuotaProbeMu.Lock()
		healthCheckCodexSessionQuotaReader = orig
		healthCheckQuotaProbeMu.Unlock()
	})
}

func writeServiceCodexSessionLine(t *testing.T, path string, mtime time.Time, line string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(line+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatal(err)
	}
}

func serviceCodexTokenCountLine(captured time.Time, planType string, used int) string {
	return `{"type":"event_msg","timestamp":"` + captured.Format(time.RFC3339Nano) + `","payload":{"type":"token_count","info":{"rate_limits":{"plan_type":"` + planType + `","primary":{"used_percent":` + strconv.Itoa(used) + `,"window_minutes":300,"resets_at":1776840333,"limit_id":"codex"}}}}}`
}

func waitForQuotaRefreshes(t *testing.T, done <-chan string, want ...string) {
	t.Helper()
	seen := map[string]bool{}
	deadline := time.After(time.Second)
	for len(seen) < len(want) {
		select {
		case name := <-done:
			seen[name] = true
		case <-deadline:
			t.Fatalf("timed out waiting for quota refreshes; saw %v want %v", seen, want)
		}
	}
	for _, name := range want {
		if !seen[name] {
			t.Fatalf("missing quota refresh %q; saw %v", name, seen)
		}
	}
}

func waitForQuotaRefreshStarts(t *testing.T, claudeStarted, codexStarted <-chan struct{}) {
	t.Helper()
	for name, ch := range map[string]<-chan struct{}{
		"claude": claudeStarted,
		"codex":  codexStarted,
	} {
		select {
		case <-ch:
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for %s quota refresh to start", name)
		}
	}
}

func waitForQuotaRefreshFiles(t *testing.T, paths ...string) {
	t.Helper()
	deadline := time.After(time.Second)
	for {
		allPresent := true
		for _, path := range paths {
			if _, err := os.Stat(path); err != nil {
				allPresent = false
				break
			}
		}
		if allPresent {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for quota refresh files: %v", paths)
		default:
			time.Sleep(time.Millisecond)
		}
	}
}
