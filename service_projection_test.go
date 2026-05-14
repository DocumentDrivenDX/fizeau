package fizeau

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/easel/fizeau/internal/harnesses"
	"github.com/stretchr/testify/require"
)

// TestProjectQuotaStatus_AllStateValues drives projectQuotaStatus across
// every harnesses.QuotaStateValue defined in CONTRACT-004. The projection
// must string-cast State into QuotaState.Status verbatim when no Reason
// is supplied. Per the CONTRACT-004 projection table, LastError stays
// nil on the success path — including when State signals absence.
func TestProjectQuotaStatus_AllStateValues(t *testing.T) {
	cases := []struct {
		state      harnesses.QuotaStateValue
		wantStatus string
	}{
		{harnesses.QuotaOK, "ok"},
		{harnesses.QuotaStale, "stale"},
		{harnesses.QuotaBlocked, "blocked"},
		{harnesses.QuotaUnavailable, "unavailable"},
		{harnesses.QuotaUnauthenticated, "unauthenticated"},
		{harnesses.QuotaUnknown, "unknown"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(string(tc.state), func(t *testing.T) {
			got := projectQuotaStatus(harnesses.QuotaStatus{State: tc.state})
			require.NotNil(t, got)
			require.Equal(t, tc.wantStatus, got.Status)
			require.Nil(t, got.LastError, "state-driven absence must not populate LastError")
		})
	}
}

// TestProjectQuotaStatus_AllRoutingPreferences drives every
// RoutingPreference value through the projection. The projection MUST
// NOT leak RoutingPreference into QuotaState (CONTRACT-004 invariant:
// routing preference is internal-only). Verified by re-marshaling the
// projection to JSON and asserting no routing-preference key surfaces.
func TestProjectQuotaStatus_AllRoutingPreferences(t *testing.T) {
	cases := []struct {
		name string
		pref harnesses.RoutingPreference
	}{
		{"unknown", harnesses.RoutingPreferenceUnknown},
		{"available", harnesses.RoutingPreferenceAvailable},
		{"blocked", harnesses.RoutingPreferenceBlocked},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := projectQuotaStatus(harnesses.QuotaStatus{
				State:             harnesses.QuotaOK,
				RoutingPreference: tc.pref,
			})
			require.NotNil(t, got)
			payload, err := json.Marshal(got)
			require.NoError(t, err)
			var asMap map[string]any
			require.NoError(t, json.Unmarshal(payload, &asMap))
			_, hasPref := asMap["routing_preference"]
			require.Falsef(t, hasPref, "QuotaState JSON leaked routing_preference: %s", payload)
			_, hasCamel := asMap["RoutingPreference"]
			require.Falsef(t, hasCamel, "QuotaState JSON leaked RoutingPreference: %s", payload)
		})
	}
}

// TestProjectQuotaStatus_ReasonPresentAndAbsent drives the two
// reason-handling branches: absent Reason yields a bare state string;
// present Reason produces "<state> (<reason>)". The reason-appended
// form is what fixtures/quota-gemini.json captures for the Pro-tier
// blocked case.
func TestProjectQuotaStatus_ReasonPresentAndAbsent(t *testing.T) {
	absent := projectQuotaStatus(harnesses.QuotaStatus{State: harnesses.QuotaOK})
	require.Equal(t, "ok", absent.Status)

	present := projectQuotaStatus(harnesses.QuotaStatus{
		State:  harnesses.QuotaOK,
		Reason: "Pro tier blocked",
	})
	require.Equal(t, "ok (Pro tier blocked)", present.Status)
}

// TestProjectQuotaStatus_DetailDoesNotLeak documents that QuotaStatus.Detail
// is a diagnostic-only field and the projection MUST NOT surface it on
// QuotaState. Populated and empty Detail maps both project to the same
// output shape.
func TestProjectQuotaStatus_DetailDoesNotLeak(t *testing.T) {
	withDetail := projectQuotaStatus(harnesses.QuotaStatus{
		State: harnesses.QuotaOK,
		Detail: map[string]string{
			"probe":   "pty",
			"comment": "captured at boot",
		},
	})
	empty := projectQuotaStatus(harnesses.QuotaStatus{
		State:  harnesses.QuotaOK,
		Detail: map[string]string{},
	})
	require.Equal(t, marshalProjection(t, empty), marshalProjection(t, withDetail))
}

// TestProjectQuotaStatus_AccountFieldNotLeaked documents that
// QuotaStatus.Account is consumed by projectAccountSnapshot (when the
// service decides to project it) and MUST NOT appear inside QuotaState.
// Populated and nil Account both project to the same QuotaState output.
func TestProjectQuotaStatus_AccountFieldNotLeaked(t *testing.T) {
	populated := projectQuotaStatus(harnesses.QuotaStatus{
		State: harnesses.QuotaOK,
		Account: &harnesses.AccountSnapshot{
			Authenticated: true,
			Email:         "user@example.com",
		},
	})
	empty := projectQuotaStatus(harnesses.QuotaStatus{State: harnesses.QuotaOK})
	require.Equal(t, marshalProjection(t, empty), marshalProjection(t, populated))
}

// TestProjectAccountSnapshot_PopulatedAndEmpty drives populated and
// empty AccountSnapshots, including the Detail-string variants.
func TestProjectAccountSnapshot_PopulatedAndEmpty(t *testing.T) {
	t.Run("populated", func(t *testing.T) {
		now := time.Date(2026, 5, 14, 17, 0, 0, 0, time.UTC)
		got := projectAccountSnapshot(harnesses.AccountSnapshot{
			Authenticated: true,
			Email:         "user@example.com",
			PlanType:      "claude_max",
			OrgName:       "Example Org",
			Source:        "~/.claude/.credentials.json",
			CapturedAt:    now,
			Fresh:         true,
			Detail:        "anthropic subscription",
		})
		require.NotNil(t, got)
		require.True(t, got.Authenticated)
		require.False(t, got.Unauthenticated)
		require.Equal(t, "user@example.com", got.Email)
		require.Equal(t, "claude_max", got.PlanType)
		require.Equal(t, "Example Org", got.OrgName)
		require.Equal(t, "anthropic subscription", got.Detail)
		require.True(t, got.Fresh)
		require.Equal(t, now, got.CapturedAt)
	})

	t.Run("empty_detail", func(t *testing.T) {
		got := projectAccountSnapshot(harnesses.AccountSnapshot{
			Authenticated: true,
			Email:         "user@example.com",
			PlanType:      "chatgpt_pro",
			Source:        "~/.codex/auth.json",
			Fresh:         true,
		})
		require.NotNil(t, got)
		require.Equal(t, "", got.Detail)
		require.Equal(t, "", got.OrgName)
	})

	t.Run("unknown_state", func(t *testing.T) {
		// CONTRACT-004: absence of evidence is reported via
		// Authenticated=false && Unauthenticated=false on a valid
		// snapshot. The projection preserves that convention.
		got := projectAccountSnapshot(harnesses.AccountSnapshot{
			Source: "opencode harness has no native account file",
			Fresh:  true,
			Detail: "no auth evidence on disk",
		})
		require.NotNil(t, got)
		require.False(t, got.Authenticated)
		require.False(t, got.Unauthenticated)
		require.Equal(t, "no auth evidence on disk", got.Detail)
	})

	t.Run("unauthenticated", func(t *testing.T) {
		got := projectAccountSnapshot(harnesses.AccountSnapshot{
			Unauthenticated: true,
			Source:          "~/.gemini/oauth_creds.json",
			Detail:          "no auth evidence on disk",
		})
		require.NotNil(t, got)
		require.False(t, got.Authenticated)
		require.True(t, got.Unauthenticated)
	})
}

// TestProjectQuotaStatus_StructuralMatchesFixtures asserts that the
// projection emits public JSON byte-identical to the BEAD-HARNESS-IF-00
// fixtures pinned under testdata/contract-003/pre-refactor/. Each
// fixture's input is the harnesses.QuotaStatus value that the
// corresponding harness will return after CONTRACT-004 migration; this
// proves the projection is the only shape contract the service needs.
func TestProjectQuotaStatus_StructuralMatchesFixtures(t *testing.T) {
	capturedAt := time.Date(2026, 5, 14, 17, 0, 0, 0, time.UTC)
	cases := []struct {
		fixture string
		input   harnesses.QuotaStatus
	}{
		{
			fixture: "quota-claude.json",
			input: harnesses.QuotaStatus{
				Source:     "internal/harnesses/claude/quota_cache.go",
				CapturedAt: capturedAt,
				Fresh:      true,
				State:      harnesses.QuotaOK,
				Windows:    preRefactorQuotaWindowsClaude(),
			},
		},
		{
			fixture: "quota-codex.json",
			input: harnesses.QuotaStatus{
				Source:     "internal/harnesses/codex/quota_cache.go",
				CapturedAt: capturedAt,
				Fresh:      true,
				State:      harnesses.QuotaOK,
				Windows:    preRefactorQuotaWindowsCodex(),
			},
		},
		{
			fixture: "quota-gemini.json",
			input: harnesses.QuotaStatus{
				Source:     "internal/harnesses/gemini/quota_cache.go",
				CapturedAt: capturedAt,
				Fresh:      true,
				State:      harnesses.QuotaOK,
				Reason:     "Pro tier blocked",
				Windows:    preRefactorQuotaWindowsGemini(),
			},
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.fixture, func(t *testing.T) {
			got := projectQuotaStatus(tc.input)
			payload := marshalProjection(t, got)
			want, err := os.ReadFile(filepath.Join(preRefactorFixtureDir, tc.fixture))
			require.NoError(t, err)
			if !bytes.Equal(want, payload) {
				t.Fatalf("projected %s drifted from fixture.\nwant:\n%s\n\ngot:\n%s", tc.fixture, want, payload)
			}
		})
	}
}

// TestProjectAccountSnapshot_StructuralMatchesFixtures asserts that
// projectAccountSnapshot emits byte-identical JSON to the
// BEAD-HARNESS-IF-00 account fixtures.
func TestProjectAccountSnapshot_StructuralMatchesFixtures(t *testing.T) {
	capturedAt := time.Date(2026, 5, 14, 17, 0, 0, 0, time.UTC)
	cases := []struct {
		fixture string
		input   harnesses.AccountSnapshot
	}{
		{
			fixture: "account-claude.json",
			input: harnesses.AccountSnapshot{
				Authenticated: true,
				Email:         "user@example.com",
				PlanType:      "claude_max",
				OrgName:       "Example Org",
				Source:        "~/.claude/.credentials.json",
				CapturedAt:    capturedAt,
				Fresh:         true,
				Detail:        "anthropic subscription",
			},
		},
		{
			fixture: "account-codex.json",
			input: harnesses.AccountSnapshot{
				Authenticated: true,
				Email:         "user@example.com",
				PlanType:      "chatgpt_pro",
				Source:        "~/.codex/auth.json",
				CapturedAt:    capturedAt,
				Fresh:         true,
			},
		},
		{
			fixture: "account-gemini.json",
			input: harnesses.AccountSnapshot{
				Authenticated: true,
				Email:         "user@example.com",
				PlanType:      "gemini_pro",
				Source:        "~/.gemini/oauth_creds.json",
				CapturedAt:    capturedAt,
				Fresh:         true,
				Detail:        "auth evidence cached for 7d",
			},
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.fixture, func(t *testing.T) {
			got := projectAccountSnapshot(tc.input)
			payload := marshalProjection(t, got)
			want, err := os.ReadFile(filepath.Join(preRefactorFixtureDir, tc.fixture))
			require.NoError(t, err)
			if !bytes.Equal(want, payload) {
				t.Fatalf("projected %s drifted from fixture.\nwant:\n%s\n\ngot:\n%s", tc.fixture, want, payload)
			}
		})
	}
}

// TestProjectionHelpersNotYetCalled documents AC #4: BEAD-HARNESS-IF-03
// only adds the helpers; no service code calls them. The first caller
// arrives with BEAD-HARNESS-IF-05B. This grep test fails if anyone
// wires the helpers in early — forcing whoever does so to land it
// behind the matching migration bead.
//
// Allowed call sites: this test file and the helpers' own source.
func TestProjectionHelpersNotYetCalled(t *testing.T) {
	allowed := map[string]bool{
		"service_projection.go":      true,
		"service_projection_test.go": true,
	}
	entries, err := os.ReadDir(".")
	require.NoError(t, err)
	for _, entry := range entries {
		if entry.IsDir() || !hasGoSuffix(entry.Name()) {
			continue
		}
		if allowed[entry.Name()] {
			continue
		}
		data, err := os.ReadFile(entry.Name())
		require.NoError(t, err)
		text := string(data)
		require.NotContainsf(t, text, "projectQuotaStatus(",
			"%s calls projectQuotaStatus; per BEAD-HARNESS-IF-03 AC #4 the helper has no callers yet — migrate behind BEAD-HARNESS-IF-05B+", entry.Name())
		require.NotContainsf(t, text, "projectAccountSnapshot(",
			"%s calls projectAccountSnapshot; per BEAD-HARNESS-IF-03 AC #4 the helper has no callers yet — migrate behind BEAD-HARNESS-IF-05B+", entry.Name())
	}
}

func hasGoSuffix(name string) bool {
	return len(name) > 3 && name[len(name)-3:] == ".go"
}

func marshalProjection(t *testing.T, v any) []byte {
	t.Helper()
	out, err := json.MarshalIndent(v, "", "  ")
	require.NoError(t, err)
	return append(out, '\n')
}
