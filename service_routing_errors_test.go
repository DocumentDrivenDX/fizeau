package fizeau

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/fizeau/internal/harnesses"
	claudeharness "github.com/DocumentDrivenDX/fizeau/internal/harnesses/claude"
)

func TestResolveRouteExplicitHarnessModelIncompatible(t *testing.T) {
	svc := testRoutingErrorService()

	_, err := svc.ResolveRoute(context.Background(), RouteRequest{
		Harness: "gemini",
		Model:   "minimax/minimax-m2.7",
	})
	if err == nil {
		t.Fatal("expected explicit harness/model incompatibility")
	}
	if !errors.Is(err, ErrHarnessModelIncompatible{}) {
		t.Fatalf("errors.Is should match ErrHarnessModelIncompatible: %T %v", err, err)
	}
	var typed *ErrHarnessModelIncompatible
	if !errors.As(err, &typed) {
		t.Fatalf("errors.As should extract ErrHarnessModelIncompatible: %T %v", err, err)
	}
	if typed.Harness != "gemini" {
		t.Fatalf("Harness=%q, want gemini", typed.Harness)
	}
	if typed.Model != "minimax/minimax-m2.7" {
		t.Fatalf("Model=%q, want minimax/minimax-m2.7", typed.Model)
	}
	want := []string{"gemini-2.5-pro", "gemini-2.5-flash", "gemini-2.5-flash-lite", "gemini", "gemini-2.5"}
	if !slices.Equal(typed.SupportedModels, want) {
		t.Fatalf("SupportedModels=%v, want %v", typed.SupportedModels, want)
	}
}

func TestValidateExplicitHarnessModelAcceptsClaudeDiscoveredFamilyVersion(t *testing.T) {
	registry := harnesses.NewRegistry()
	cfg, ok := registry.Get("claude")
	if !ok {
		t.Fatal("missing claude registry entry")
	}

	if err := validateExplicitHarnessModel("claude", cfg, "opus-4.7", ""); err != nil {
		t.Fatalf("opus-4.7 should be accepted as a discovered Claude family version: %v", err)
	}
	err := validateExplicitHarnessModel("claude", cfg, "opus-9.9", "")
	if err == nil {
		t.Fatal("expected unknown claude family version to be rejected")
	}
	var typed *ErrHarnessModelIncompatible
	if !errors.As(err, &typed) {
		t.Fatalf("expected ErrHarnessModelIncompatible, got %T %v", err, err)
	}
	if !slices.Contains(typed.SupportedModels, "opus-4.7") {
		t.Fatalf("supported models should include discovered opus version, got %v", typed.SupportedModels)
	}
}

func TestValidateExplicitHarnessModelPiAcceptsLocalProviderPin(t *testing.T) {
	registry := harnesses.NewRegistry()
	cfg, ok := registry.Get("pi")
	if !ok {
		t.Fatal("missing pi registry entry")
	}

	// With an explicit provider pin, a non-Gemini model ID must be accepted:
	// pi itself owns per-provider model validation.
	if err := validateExplicitHarnessModel("pi", cfg, "qwen3.6-27b", "lmstudio"); err != nil {
		t.Fatalf("pi+lmstudio+qwen should be accepted: %v", err)
	}
	if err := validateExplicitHarnessModel("pi", cfg, "qwen3.6-27b", "omlx"); err != nil {
		t.Fatalf("pi+omlx+qwen should be accepted: %v", err)
	}

	// Without a provider pin, the strict Gemini-only gate still applies.
	err := validateExplicitHarnessModel("pi", cfg, "qwen3.6-27b", "")
	if err == nil {
		t.Fatal("expected pi to reject non-Gemini model without provider pin")
	}
	var typed *ErrHarnessModelIncompatible
	if !errors.As(err, &typed) {
		t.Fatalf("expected ErrHarnessModelIncompatible, got %T %v", err, err)
	}

	// Regression: Gemini defaults still validate cleanly.
	if err := validateExplicitHarnessModel("pi", cfg, "gemini-2.5-flash", ""); err != nil {
		t.Fatalf("gemini-2.5-flash must remain valid for pi: %v", err)
	}
	if err := validateExplicitHarnessModel("pi", cfg, "gemini-2.5-pro", ""); err != nil {
		t.Fatalf("gemini-2.5-pro must remain valid for pi: %v", err)
	}
}

func TestResolveExecuteRouteNormalizesSubprocessAliases(t *testing.T) {
	svc := testRoutingErrorService()

	claudeDecision, err := svc.resolveExecuteRoute(ServiceExecuteRequest{Harness: "claude", Model: "sonnet"})
	if err != nil {
		t.Fatalf("resolve claude sonnet alias: %v", err)
	}
	if claudeDecision.Model != "sonnet" {
		t.Fatalf("claude sonnet alias resolved to %q, want sonnet", claudeDecision.Model)
	}

	claudeOpusDecision, err := svc.resolveExecuteRoute(ServiceExecuteRequest{Harness: "claude", Model: "opus-4.7"})
	if err != nil {
		t.Fatalf("resolve claude opus version: %v", err)
	}
	if claudeOpusDecision.Model != "opus" {
		t.Fatalf("claude opus version normalized to %q, want opus", claudeOpusDecision.Model)
	}

	claudeFullOpusDecision, err := svc.resolveExecuteRoute(ServiceExecuteRequest{Harness: "claude", Model: "claude-opus-4-6"})
	if err != nil {
		t.Fatalf("resolve claude full opus version: %v", err)
	}
	if claudeFullOpusDecision.Model != "opus" {
		t.Fatalf("claude full opus version normalized to %q, want opus", claudeFullOpusDecision.Model)
	}

	codexDecision, err := svc.resolveExecuteRoute(ServiceExecuteRequest{Harness: "codex", Model: "gpt"})
	if err != nil {
		t.Fatalf("resolve codex gpt alias: %v", err)
	}
	if codexDecision.Model != "gpt-5.4" {
		t.Fatalf("codex gpt alias resolved to %q, want gpt-5.4", codexDecision.Model)
	}

	geminiDecision, err := svc.resolveExecuteRoute(ServiceExecuteRequest{Harness: "gemini", Model: "gemini"})
	if err != nil {
		t.Fatalf("resolve gemini alias: %v", err)
	}
	if geminiDecision.Model != "gemini-2.5-pro" {
		t.Fatalf("gemini alias resolved to %q, want gemini-2.5-pro", geminiDecision.Model)
	}
}

func TestResolveExplicitClaudeRejectedWhenFreshQuotaExhausted(t *testing.T) {
	cachePath := filepath.Join(t.TempDir(), "claude-quota.json")
	t.Setenv("FIZEAU_CLAUDE_QUOTA_CACHE", cachePath)
	t.Setenv("DDX_AGENT_CLAUDE_QUOTA_CACHE", "")

	now := time.Now().UTC()
	reset := now.Add(2 * time.Hour).Unix()
	if err := claudeharness.WriteClaudeQuota(cachePath, claudeharness.ClaudeQuotaSnapshot{
		CapturedAt:        now,
		FiveHourRemaining: 0,
		FiveHourLimit:     100,
		WeeklyRemaining:   0,
		WeeklyLimit:       100,
		Windows: []harnesses.QuotaWindow{{
			Name:         "Current week (all models)",
			LimitID:      "weekly-all",
			UsedPercent:  100,
			ResetsAtUnix: reset,
			State:        "exhausted",
		}},
		Source:  "runtime_error",
		Account: &harnesses.AccountInfo{PlanType: "Claude Max"},
	}); err != nil {
		t.Fatalf("WriteClaudeQuota: %v", err)
	}

	svc := testRoutingErrorService()
	_, err := svc.resolveExecuteRoute(ServiceExecuteRequest{Harness: "claude", Model: "opus-4.7"})
	if err == nil {
		t.Fatal("expected exhausted Claude quota to reject explicit claude route")
	}
	var quotaErr *NoViableProviderForNow
	if !errors.As(err, &quotaErr) {
		t.Fatalf("error=%T %v, want NoViableProviderForNow", err, err)
	}
	if !slices.Equal(quotaErr.ExhaustedProviders, []string{"claude"}) {
		t.Fatalf("ExhaustedProviders=%v, want [claude]", quotaErr.ExhaustedProviders)
	}
	if got := quotaErr.RetryAfter.Unix(); got != reset {
		t.Fatalf("RetryAfter unix=%d, want %d", got, reset)
	}
}

func TestResolveRouteExplicitProfilePinConflict(t *testing.T) {
	svc := testRoutingErrorService()

	_, err := svc.ResolveRoute(context.Background(), RouteRequest{
		Profile: "local",
		Harness: "claude",
	})
	if err == nil {
		t.Fatal("expected local profile to conflict with claude harness")
	}
	if !errors.Is(err, ErrProfilePinConflict{}) {
		t.Fatalf("errors.Is should match ErrProfilePinConflict: %T %v", err, err)
	}
	var typed *ErrProfilePinConflict
	if !errors.As(err, &typed) {
		t.Fatalf("errors.As should extract ErrProfilePinConflict: %T %v", err, err)
	}
	if typed.Profile != "local" || typed.ConflictingPin != "Harness=claude" || typed.ProfileConstraint != "local-only" {
		t.Fatalf("profile conflict=%#v, want local/Harness=claude/local-only", typed)
	}

	_, err = svc.ResolveRoute(context.Background(), RouteRequest{
		Profile: "smart",
		Harness: "fiz",
	})
	if err == nil {
		t.Fatal("expected smart profile to conflict with local fiz harness")
	}
	var inverse *ErrProfilePinConflict
	if !errors.As(err, &inverse) {
		t.Fatalf("errors.As inverse: %T %v", err, err)
	}
	if inverse.Profile != "smart" || inverse.ConflictingPin != "Harness=fiz" || inverse.ProfileConstraint != "subscription-only" {
		t.Fatalf("inverse profile conflict=%#v, want smart/Harness=fiz/subscription-only", inverse)
	}
}

func TestResolveRouteUnknownProfileIsTyped(t *testing.T) {
	svc := testRoutingErrorService()

	_, err := svc.ResolveRoute(context.Background(), RouteRequest{Profile: "does-not-exist"})
	if err == nil {
		t.Fatal("expected unknown profile error")
	}
	if !errors.Is(err, ErrUnknownProfile{}) {
		t.Fatalf("errors.Is should match ErrUnknownProfile: %T %v", err, err)
	}
	var typed *ErrUnknownProfile
	if !errors.As(err, &typed) {
		t.Fatalf("errors.As should extract ErrUnknownProfile: %T %v", err, err)
	}
	if typed.Profile != "does-not-exist" {
		t.Fatalf("Profile=%q, want does-not-exist", typed.Profile)
	}
}

func TestResolveRouteLegacyCodeProfilesRejectWithReplacementGuidance(t *testing.T) {
	svc := testRoutingErrorService()

	for profile, want := range map[string]string{
		"code-medium": "--profile standard",
		"code-high":   "--profile smart",
	} {
		t.Run(profile, func(t *testing.T) {
			_, err := svc.ResolveRoute(context.Background(), RouteRequest{Profile: profile})
			if err == nil {
				t.Fatalf("expected %s to be rejected", profile)
			}
			if !strings.Contains(err.Error(), want) {
				t.Fatalf("error=%q, want replacement guidance %q", err.Error(), want)
			}
			if !strings.Contains(err.Error(), "--min-power/--max-power") {
				t.Fatalf("error=%q, want numeric power guidance", err.Error())
			}
		})
	}
}

func TestResolveRouteLocalProfileNoLocalCandidateIsTyped(t *testing.T) {
	svc := testRoutingErrorService()

	dec, err := svc.ResolveRoute(context.Background(), RouteRequest{Profile: "local"})
	if err == nil {
		t.Fatal("expected local profile without local candidates to fail")
	}
	if !errors.Is(err, ErrNoProfileCandidate{}) {
		t.Fatalf("errors.Is should match ErrNoProfileCandidate: %T %v", err, err)
	}
	var typed *ErrNoProfileCandidate
	if !errors.As(err, &typed) {
		t.Fatalf("errors.As should extract ErrNoProfileCandidate: %T %v", err, err)
	}
	if typed.Profile != "local" || typed.MissingCapability != "local endpoint" {
		t.Fatalf("ErrNoProfileCandidate=%#v, want local/local endpoint", typed)
	}
	if dec == nil || len(dec.Candidates) == 0 {
		t.Fatalf("expected rejected candidate trace, got decision=%#v", dec)
	}
}

func testRoutingErrorService() *service {
	registry := harnesses.NewRegistry()
	registry.LookPath = func(file string) (string, error) { return "/bin/" + file, nil }
	return &service{
		opts:     ServiceOptions{},
		registry: registry,
		hub:      newSessionHub(),
	}
}
