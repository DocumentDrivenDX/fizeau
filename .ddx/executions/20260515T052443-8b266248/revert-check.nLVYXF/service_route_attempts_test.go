package fizeau

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/easel/fizeau/internal/discoverycache"
	"github.com/easel/fizeau/internal/harnesses"
	"github.com/easel/fizeau/internal/routing"
)

func TestRecordRouteAttempt_DemotesFailedProviderForAutomaticRouting(t *testing.T) {
	svc := routeAttemptTestService(t, 30*time.Second)

	before, err := svc.ResolveRoute(context.Background(), RouteRequest{
		Model: "qwen",
	})
	if err != nil {
		t.Fatalf("ResolveRoute before failure: %v", err)
	}
	if before.Provider != "bragi" {
		t.Fatalf("before failure Provider: got %q, want bragi", before.Provider)
	}

	if err := svc.RecordRouteAttempt(context.Background(), RouteAttempt{
		Harness:  "fiz",
		Provider: "bragi",
		Model:    "qwen",
		Status:   "failed",
		Reason:   "timeout",
		Error:    "context deadline exceeded",
	}); err != nil {
		t.Fatalf("RecordRouteAttempt: %v", err)
	}

	after, err := svc.ResolveRoute(context.Background(), RouteRequest{
		Model: "qwen",
	})
	if err != nil {
		t.Fatalf("ResolveRoute after failure: %v", err)
	}
	if after.Provider == "bragi" {
		t.Fatalf("after failure Provider: got bragi, want a non-cooldown provider")
	}

	pinned, err := svc.ResolveRoute(context.Background(), RouteRequest{
		Model:    "qwen",
		Provider: "bragi",
	})
	if err != nil {
		t.Fatalf("ResolveRoute with provider pin after failure: %v", err)
	}
	if pinned.Provider != "bragi" {
		t.Fatalf("provider pin after failure: got %q, want bragi", pinned.Provider)
	}
}

// TestRecordRouteAttempt_DialFailureHardGatesProvider verifies FEAT-004 AC-28
// path: a route-attempt record whose Error matches a dispatchability-failure
// pattern (dial tcp / connection refused / i/o timeout / 5xx gateway) gets
// promoted into ProviderUnreachable so the next ResolveRoute hard-gates the
// provider — distinct from the soft-demotion path (context deadline,
// validation error) which leaves the candidate eligible but down-scored.
// This is the v0.13.1 follow-up to v0.13.0's snapshot-only hard-gate.
func TestRecordRouteAttempt_DialFailureHardGatesProvider(t *testing.T) {
	cases := []struct {
		name string
		err  string
	}{
		{"dial tcp timeout", `openai: Post "http://bragi:1234/v1/chat/completions": dial tcp 100.127.38.115:1234: i/o timeout`},
		{"connection refused", `dial tcp 192.168.2.106:8020: connection refused`},
		{"502 bad gateway", `POST "http://bragi:1234/v1/chat/completions": 502 Bad Gateway `},
		{"no route to host", `dial tcp: no route to host`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			svc := routeAttemptTestService(t, 30*time.Second)

			before, err := svc.ResolveRoute(context.Background(), RouteRequest{Model: "qwen"})
			if err != nil {
				t.Fatalf("ResolveRoute before failure: %v", err)
			}
			if before.Provider != "bragi" {
				t.Fatalf("baseline provider: got %q, want bragi", before.Provider)
			}

			if err := svc.RecordRouteAttempt(context.Background(), RouteAttempt{
				Harness:  "fiz",
				Provider: "bragi",
				Model:    "qwen",
				Status:   "failed",
				Error:    c.err,
			}); err != nil {
				t.Fatalf("RecordRouteAttempt: %v", err)
			}

			// After a dial-class failure, bragi must be hard-gated — its
			// candidate row should be Eligible=false with FilterReasonUnhealthy.
			after, err := svc.ResolveRoute(context.Background(), RouteRequest{Model: "qwen"})
			if err != nil {
				t.Fatalf("ResolveRoute after failure: %v", err)
			}
			if after.Provider == "bragi" {
				t.Fatalf("after dial failure provider: got bragi, want hard-gated to alternative")
			}
			var bragiCand *RouteCandidate
			for i := range after.Candidates {
				if after.Candidates[i].Provider == "bragi" {
					bragiCand = &after.Candidates[i]
					break
				}
			}
			if bragiCand == nil {
				t.Fatal("bragi candidate row missing from decision")
			}
			if bragiCand.Eligible {
				t.Errorf("bragi should be Eligible=false after dial failure; got Eligible=true")
			}
			if !strings.Contains(bragiCand.Reason, "known unreachable") {
				t.Errorf("bragi.Reason = %q, want it to contain 'known unreachable'", bragiCand.Reason)
			}

			// Explicit provider pin still selects bragi (operator bypass).
			pinned, err := svc.ResolveRoute(context.Background(), RouteRequest{Model: "qwen", Provider: "bragi"})
			if err != nil {
				t.Fatalf("ResolveRoute with provider pin: %v", err)
			}
			if pinned.Provider != "bragi" {
				t.Fatalf("explicit pin after dial failure: got %q, want bragi", pinned.Provider)
			}
		})
	}
}

func TestRecordRouteAttempt_SuccessClearsFailure(t *testing.T) {
	svc := routeAttemptTestService(t, 30*time.Second)
	if err := svc.RecordRouteAttempt(context.Background(), RouteAttempt{
		Harness:  "fiz",
		Provider: "bragi",
		Model:    "qwen",
		Status:   "failed",
		Error:    "502 bad gateway",
	}); err != nil {
		t.Fatalf("RecordRouteAttempt failed: %v", err)
	}
	if err := svc.RecordRouteAttempt(context.Background(), RouteAttempt{
		Harness:  "fiz",
		Provider: "bragi",
		Model:    "qwen",
		Status:   "success",
	}); err != nil {
		t.Fatalf("RecordRouteAttempt success: %v", err)
	}

	dec, err := svc.ResolveRoute(context.Background(), RouteRequest{
		Model:    "qwen",
		Provider: "bragi",
	})
	if err != nil {
		t.Fatalf("ResolveRoute: %v", err)
	}
	if dec.Provider != "bragi" {
		t.Fatalf("Provider after success clear: got %q, want bragi", dec.Provider)
	}
}

func TestRecordRouteAttempt_TTLExpiryRemovesDemotion(t *testing.T) {
	svc := routeAttemptTestService(t, 10*time.Millisecond)
	if err := svc.RecordRouteAttempt(context.Background(), RouteAttempt{
		Harness:   "fiz",
		Provider:  "bragi",
		Model:     "qwen",
		Status:    "failed",
		Timestamp: time.Now().Add(-time.Second),
	}); err != nil {
		t.Fatalf("RecordRouteAttempt: %v", err)
	}

	dec, err := svc.ResolveRoute(context.Background(), RouteRequest{
		Model:    "qwen",
		Provider: "bragi",
	})
	if err != nil {
		t.Fatalf("ResolveRoute: %v", err)
	}
	if dec.Provider != "bragi" {
		t.Fatalf("Provider after TTL expiry: got %q, want bragi", dec.Provider)
	}
}

func TestRouteStatus_RouteAttemptCooldownSurfaces(t *testing.T) {
	svc := routeAttemptTestService(t, 30*time.Second)
	recordedAt := time.Now().Add(-time.Second).UTC()
	if err := svc.RecordRouteAttempt(context.Background(), RouteAttempt{
		Harness:   "fiz",
		Provider:  "bragi",
		Model:     "qwen",
		Status:    "failed",
		Reason:    "rate_limit",
		Error:     "429 too many requests",
		Timestamp: recordedAt,
	}); err != nil {
		t.Fatalf("RecordRouteAttempt: %v", err)
	}

	report, err := svc.RouteStatus(context.Background())
	if err != nil {
		t.Fatalf("RouteStatus: %v", err)
	}
	if len(report.Routes) != 1 {
		t.Fatalf("Routes: got %d, want 1", len(report.Routes))
	}
	byProvider := make(map[string]RouteCandidateStatus)
	for _, cand := range report.Routes[0].Candidates {
		byProvider[cand.Provider] = cand
	}
	bragi := byProvider["bragi"]
	if bragi.Healthy {
		t.Fatal("bragi should be unhealthy while route-attempt cooldown is active")
	}
	if bragi.Cooldown == nil {
		t.Fatal("bragi cooldown should be populated")
	}
	if bragi.Cooldown.Reason != "rate_limit" {
		t.Fatalf("Cooldown.Reason: got %q, want rate_limit", bragi.Cooldown.Reason)
	}
	if bragi.Cooldown.LastError != "429 too many requests" {
		t.Fatalf("Cooldown.LastError: got %q", bragi.Cooldown.LastError)
	}
	if !bragi.Cooldown.LastAttempt.Equal(recordedAt) {
		t.Fatalf("Cooldown.LastAttempt: got %s, want %s", bragi.Cooldown.LastAttempt, recordedAt)
	}
	if !byProvider["openrouter"].Healthy {
		t.Fatal("openrouter should remain healthy")
	}
}

func TestRouteAttempts_ProviderModelKeying(t *testing.T) {
	svc := newTestService(t, ServiceOptions{})
	ctx := context.Background()

	keyX := routing.ProviderModelKey("providerA", "", "modelX")
	keyY := routing.ProviderModelKey("providerA", "", "modelY")

	for i := 0; i < 3; i++ {
		if err := svc.RecordRouteAttempt(ctx, RouteAttempt{
			Harness:  "fiz",
			Provider: "providerA",
			Model:    "modelX",
			Status:   "failed",
			Error:    "boom",
			Duration: 100 * time.Millisecond,
		}); err != nil {
			t.Fatalf("RecordRouteAttempt failure %d on modelX: %v", i, err)
		}
	}

	successRate, latencyMS := svc.routeMetricSignals(time.Now(), 30*time.Second)
	if got, want := successRate[keyX], 0.0; got != want {
		t.Fatalf("after 3 failures on modelX: successRate[%s]=%v, want %v", keyX, got, want)
	}
	if _, ok := successRate[keyY]; ok {
		t.Fatalf("modelY signal should be untouched by modelX failures, got successRate[%s]=%v", keyY, successRate[keyY])
	}
	if _, ok := latencyMS[keyY]; ok {
		t.Fatalf("modelY latency should be untouched by modelX failures, got latencyMS[%s]=%v", keyY, latencyMS[keyY])
	}

	if err := svc.RecordRouteAttempt(ctx, RouteAttempt{
		Harness:  "fiz",
		Provider: "providerA",
		Model:    "modelY",
		Status:   "success",
		Duration: 50 * time.Millisecond,
	}); err != nil {
		t.Fatalf("RecordRouteAttempt success on modelY: %v", err)
	}

	successRate, _ = svc.routeMetricSignals(time.Now(), 30*time.Second)
	if got, want := successRate[keyX], 0.0; got != want {
		t.Fatalf("after success on modelY: successRate[%s]=%v, want %v (cross-pollution)", keyX, got, want)
	}
	if got, want := successRate[keyY], 1.0; got != want {
		t.Fatalf("after success on modelY: successRate[%s]=%v, want %v", keyY, got, want)
	}
}

func TestResolveRoute_CodexUsesDurableQuotaCache(t *testing.T) {
	dir := t.TempDir()
	codexQuotaPath := filepath.Join(dir, "codex-quota.json")
	t.Setenv("FIZEAU_CODEX_QUOTA_CACHE", codexQuotaPath)
	t.Setenv("FIZEAU_CLAUDE_QUOTA_CACHE", filepath.Join(dir, "missing-claude-quota.json"))
	writeCodexQuotaCacheFile(t, codexQuotaPath, time.Now().UTC(), "pty",
		&harnesses.AccountInfo{PlanType: "ChatGPT Pro"},
		[]harnesses.QuotaWindow{
			{Name: "5h", WindowMinutes: 300, UsedPercent: 25, State: "ok"},
		},
	)

	registry := harnesses.NewRegistry()
	registry.LookPath = func(file string) (string, error) {
		return filepath.Join(dir, file), nil
	}
	svc := &service{opts: ServiceOptions{}, registry: registry}
	dec, err := svc.ResolveRoute(context.Background(), RouteRequest{Harness: "codex", Model: "gpt-5.5"})
	if err != nil {
		t.Fatalf("ResolveRoute: %v", err)
	}
	if dec.Harness != "codex" || dec.Model != "gpt-5.5" {
		t.Fatalf("ResolveRoute: got harness=%q model=%q, want codex gpt-5.5", dec.Harness, dec.Model)
	}
}

func TestBuildRoutingInputs_CodexQuotaStaleOrBlockedIsIneligible(t *testing.T) {
	dir := t.TempDir()
	codexQuotaPath := filepath.Join(dir, "codex-quota.json")
	t.Setenv("FIZEAU_CODEX_QUOTA_CACHE", codexQuotaPath)
	registry := harnesses.NewRegistry()
	svc := &service{opts: ServiceOptions{}, registry: registry}

	writeCodexQuotaCacheFile(t, codexQuotaPath, time.Now().UTC().Add(-20*time.Minute), "pty",
		nil,
		[]harnesses.QuotaWindow{{Name: "5h", UsedPercent: 25, State: "ok"}},
	)
	codex := routingHarnessEntry(t, svc.buildRoutingInputs(context.Background()).Harnesses, "codex")
	if codex.SubscriptionOK || !codex.QuotaStale {
		t.Fatalf("stale codex quota: SubscriptionOK=%v QuotaStale=%v", codex.SubscriptionOK, codex.QuotaStale)
	}

	writeCodexQuotaCacheFile(t, codexQuotaPath, time.Now().UTC(), "pty",
		nil,
		[]harnesses.QuotaWindow{{Name: "5h", UsedPercent: 96, State: "blocked"}},
	)
	codex = routingHarnessEntry(t, svc.buildRoutingInputs(context.Background()).Harnesses, "codex")
	if codex.SubscriptionOK || codex.QuotaTrend != "exhausting" {
		t.Fatalf("blocked codex quota: SubscriptionOK=%v QuotaTrend=%q", codex.SubscriptionOK, codex.QuotaTrend)
	}
}

func TestBuildRoutingInputs_GeminiQuotaGatesAutoRouting(t *testing.T) {
	dir := t.TempDir()
	quotaPath := filepath.Join(dir, "gemini-quota.json")
	t.Setenv("FIZEAU_GEMINI_QUOTA_CACHE", quotaPath)
	t.Setenv("FIZEAU_CODEX_QUOTA_CACHE", filepath.Join(dir, "missing-codex-quota.json"))
	t.Setenv("FIZEAU_CLAUDE_QUOTA_CACHE", filepath.Join(dir, "missing-claude-quota.json"))

	registry := harnesses.NewRegistry()
	svc := &service{opts: ServiceOptions{}, registry: registry}

	// Missing cache: no SubscriptionOK even with fresh auth.
	t.Setenv("GOOGLE_API_KEY", "test")
	gemini := routingHarnessEntry(t, svc.buildRoutingInputs(context.Background()).Harnesses, "gemini")
	if gemini.SubscriptionOK || gemini.QuotaOK {
		t.Fatalf("missing gemini quota cache must keep gemini out of auto-routing: %+v", gemini)
	}

	// Stale snapshot: ineligible even though windows show headroom.
	writeGeminiTestQuota(t, quotaPath, geminiTestQuotaSnapshot{
		CapturedAt: time.Now().UTC().Add(-1 * time.Hour),
		Source:     "pty",
		Windows: []harnesses.QuotaWindow{
			{Name: "Flash", LimitID: "gemini-flash", UsedPercent: 4, State: "ok"},
		},
	})
	gemini = routingHarnessEntry(t, svc.buildRoutingInputs(context.Background()).Harnesses, "gemini")
	if gemini.SubscriptionOK || !gemini.QuotaStale {
		t.Fatalf("stale gemini quota: SubscriptionOK=%v QuotaStale=%v", gemini.SubscriptionOK, gemini.QuotaStale)
	}

	// Fresh but all tiers exhausted: routing must still mark ineligible.
	writeGeminiTestQuota(t, quotaPath, geminiTestQuotaSnapshot{
		CapturedAt: time.Now().UTC(),
		Source:     "pty",
		Windows: []harnesses.QuotaWindow{
			{Name: "Flash", LimitID: "gemini-flash", UsedPercent: 100, State: "exhausted"},
			{Name: "Pro", LimitID: "gemini-pro", UsedPercent: 100, State: "exhausted"},
		},
	})
	gemini = routingHarnessEntry(t, svc.buildRoutingInputs(context.Background()).Harnesses, "gemini")
	if gemini.SubscriptionOK {
		t.Fatalf("all tiers exhausted must block gemini auto-routing: %+v", gemini)
	}
	if gemini.QuotaTrend != routing.QuotaTrendExhausting {
		t.Fatalf("all-exhausted snapshot should report exhausting trend, got %q", gemini.QuotaTrend)
	}

	// Fresh with at least one non-exhausted tier: routing marks eligible.
	writeGeminiTestQuota(t, quotaPath, geminiTestQuotaSnapshot{
		CapturedAt: time.Now().UTC(),
		Source:     "pty",
		Windows: []harnesses.QuotaWindow{
			{Name: "Flash", LimitID: "gemini-flash", UsedPercent: 4, State: "ok"},
			{Name: "Pro", LimitID: "gemini-pro", UsedPercent: 100, State: "exhausted"},
		},
	})
	gemini = routingHarnessEntry(t, svc.buildRoutingInputs(context.Background()).Harnesses, "gemini")
	if !gemini.SubscriptionOK || !gemini.QuotaOK {
		t.Fatalf("fresh gemini quota with headroom should mark gemini SubscriptionOK/QuotaOK: %+v", gemini)
	}
}

func TestBuildRoutingInputs_SecondaryHarnesses(t *testing.T) {
	registry := harnesses.NewRegistry()
	svc := &service{opts: ServiceOptions{}, registry: registry}
	inputs := svc.buildRoutingInputs(context.Background())

	opencode := routingHarnessEntry(t, inputs.Harnesses, "opencode")
	if opencode.AutoRoutingEligible || opencode.DefaultModel != "opencode/gpt-5.4" {
		t.Fatalf("opencode routing metadata: AutoRoutingEligible=%v DefaultModel=%q", opencode.AutoRoutingEligible, opencode.DefaultModel)
	}
	if !containsRouteString(opencode.SupportedModels, "opencode/gpt-5.4") {
		t.Fatalf("opencode supported models missing default: %v", opencode.SupportedModels)
	}
	if !containsRouteString(opencode.SupportedReasoning, "max") {
		t.Fatalf("opencode reasoning metadata missing max: %v", opencode.SupportedReasoning)
	}

	pi := routingHarnessEntry(t, inputs.Harnesses, "pi")
	if pi.AutoRoutingEligible || pi.DefaultModel != "gemini-2.5-flash" {
		t.Fatalf("pi routing metadata: AutoRoutingEligible=%v DefaultModel=%q", pi.AutoRoutingEligible, pi.DefaultModel)
	}
	if !containsRouteString(pi.SupportedReasoning, "xhigh") {
		t.Fatalf("pi reasoning metadata missing xhigh: %v", pi.SupportedReasoning)
	}

	gemini := routingHarnessEntry(t, inputs.Harnesses, "gemini")
	if gemini.AutoRoutingEligible || gemini.DefaultModel != "gemini-2.5-flash" {
		t.Fatalf("gemini routing metadata: AutoRoutingEligible=%v DefaultModel=%q", gemini.AutoRoutingEligible, gemini.DefaultModel)
	}
	if !containsRouteString(gemini.SupportedModels, "gemini-2.5-pro") || !containsRouteString(gemini.SupportedModels, "gemini-2.5-flash-lite") {
		t.Fatalf("gemini supported models not populated from registry: %v", gemini.SupportedModels)
	}
	if len(gemini.SupportedReasoning) != 0 {
		t.Fatalf("gemini should not advertise reasoning controls: %v", gemini.SupportedReasoning)
	}
}

func TestResolveRoute_GeminiCatalogModelsResolveByConcreteModel(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "redacted")
	dir := t.TempDir()
	quotaPath := filepath.Join(dir, "gemini-quota.json")
	t.Setenv("FIZEAU_GEMINI_QUOTA_CACHE", quotaPath)
	writeGeminiTestQuota(t, quotaPath, geminiTestQuotaSnapshot{
		CapturedAt: time.Now().UTC(),
		Source:     "pty",
		Windows: []harnesses.QuotaWindow{
			{Name: "Flash", LimitID: "gemini-flash", UsedPercent: 4, State: "ok"},
			{Name: "Flash Lite", LimitID: "gemini-flash-lite", UsedPercent: 0, State: "ok"},
			{Name: "Pro", LimitID: "gemini-pro", UsedPercent: 10, State: "ok"},
		},
	})
	registry := harnesses.NewRegistry()
	registry.LookPath = func(file string) (string, error) {
		if file == "gemini" {
			return "/usr/bin/gemini", nil
		}
		return "", os.ErrNotExist
	}
	svc := &service{opts: ServiceOptions{}, registry: registry}

	for policy, model := range map[string]string{
		"smart":   "gemini-2.5-pro",
		"default": "gemini-2.5-flash",
		"cheap":   "gemini-2.5-flash-lite",
	} {
		dec, err := svc.ResolveRoute(context.Background(), RouteRequest{Harness: "gemini", Policy: policy, Model: model})
		if err != nil {
			t.Fatalf("ResolveRoute policy=%s: %v", policy, err)
		}
		if dec.Harness != "gemini" || dec.Model != model {
			t.Fatalf("policy=%s: got harness=%q model=%q, want gemini/%s", policy, dec.Harness, dec.Model, model)
		}
	}
}

func routeAttemptTestService(t *testing.T, cooldown time.Duration) *service {
	t.Helper()
	cacheDir := t.TempDir()
	t.Setenv("FIZEAU_CACHE_DIR", cacheDir)
	t.Setenv("PATH", "")
	cache := &discoverycache.Cache{Root: cacheDir}
	capturedAt := time.Date(2026, 5, 12, 15, 0, 0, 0, time.UTC)
	writeSnapshotDiscoveryFixture(t, cache, testDiscoverySourceName("bragi", "bragi", "http://127.0.0.1:9999/v1", ""), capturedAt, []string{"qwen"})
	writeSnapshotDiscoveryFixture(t, cache, testDiscoverySourceName("openrouter", "openrouter", "https://openrouter.invalid/v1", ""), capturedAt, []string{"qwen"})
	sc := &fakeServiceConfig{
		providers: map[string]ServiceProviderEntry{
			"bragi":      {Type: "lmstudio", BaseURL: "http://127.0.0.1:9999/v1", Model: "qwen"},
			"openrouter": {Type: "openrouter", BaseURL: "https://openrouter.invalid/v1", Model: "qwen"},
		},
		names:          []string{"bragi", "openrouter"},
		defaultName:    "bragi",
		healthCooldown: cooldown,
	}
	return newTestService(t, ServiceOptions{ServiceConfig: sc})
}

func routingHarnessEntry(t *testing.T, entries []routing.HarnessEntry, name string) routing.HarnessEntry {
	t.Helper()
	for _, entry := range entries {
		if entry.Name == name {
			return entry
		}
	}
	t.Fatalf("routing entry %q not found", name)
	return routing.HarnessEntry{}
}

func containsRouteString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

type geminiTestQuotaSnapshot struct {
	CapturedAt time.Time               `json:"captured_at"`
	Windows    []harnesses.QuotaWindow `json:"windows"`
	Source     string                  `json:"source"`
	Account    *harnesses.AccountInfo  `json:"account,omitempty"`
}

func writeCodexQuotaCacheFile(t *testing.T, path string, capturedAt time.Time, source string, account *harnesses.AccountInfo, windows []harnesses.QuotaWindow) {
	t.Helper()
	type codexCache struct {
		CapturedAt time.Time               `json:"captured_at"`
		Windows    []harnesses.QuotaWindow `json:"windows"`
		Source     string                  `json:"source"`
		Account    *harnesses.AccountInfo  `json:"account,omitempty"`
	}
	data, err := json.MarshalIndent(codexCache{CapturedAt: capturedAt, Windows: windows, Source: source, Account: account}, "", "  ")
	if err != nil {
		t.Fatalf("marshal codex quota cache: %v", err)
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir codex quota cache dir: %v", err)
	}
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o600); err != nil {
		t.Fatalf("write codex quota cache: %v", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		t.Fatalf("rename codex quota cache: %v", err)
	}
}

func writeGeminiTestQuota(t *testing.T, path string, snap geminiTestQuotaSnapshot) {
	t.Helper()
	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		t.Fatalf("marshal gemini test quota: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir gemini quota dir: %v", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		t.Fatalf("write gemini quota: %v", err)
	}
}
