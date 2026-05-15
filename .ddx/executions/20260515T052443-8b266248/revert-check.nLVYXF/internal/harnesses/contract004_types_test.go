package harnesses

import (
	"errors"
	"testing"
)

// TestRoutingPreferenceIdentity pins the iota ordering of RoutingPreference
// values so callers (and the projection helpers landing in later beads) can
// rely on the constants being stable rather than reshuffled by a future edit
// to the const block.
func TestRoutingPreferenceIdentity(t *testing.T) {
	cases := []struct {
		name string
		got  RoutingPreference
		want int
	}{
		{"unknown", RoutingPreferenceUnknown, 0},
		{"available", RoutingPreferenceAvailable, 1},
		{"blocked", RoutingPreferenceBlocked, 2},
	}
	for _, tc := range cases {
		if int(tc.got) != tc.want {
			t.Errorf("RoutingPreference %s = %d, want %d", tc.name, int(tc.got), tc.want)
		}
	}

	// Identity: two references to the same constant compare equal.
	if RoutingPreferenceAvailable != RoutingPreferenceAvailable {
		t.Error("RoutingPreferenceAvailable not equal to itself")
	}
	// Distinctness: every enumerated value is distinct.
	all := []RoutingPreference{
		RoutingPreferenceUnknown,
		RoutingPreferenceAvailable,
		RoutingPreferenceBlocked,
	}
	seen := map[RoutingPreference]struct{}{}
	for _, v := range all {
		if _, dup := seen[v]; dup {
			t.Errorf("duplicate RoutingPreference value: %d", v)
		}
		seen[v] = struct{}{}
	}
}

// TestQuotaStateValueStringMapping pins each QuotaStateValue's underlying
// string form. The projection layer (later beads) string-casts these into
// the public CONTRACT-003 QuotaState.Status surface, so the spelling is
// part of the public JSON contract.
func TestQuotaStateValueStringMapping(t *testing.T) {
	cases := []struct {
		got  QuotaStateValue
		want string
	}{
		{QuotaOK, "ok"},
		{QuotaStale, "stale"},
		{QuotaBlocked, "blocked"},
		{QuotaUnavailable, "unavailable"},
		{QuotaUnauthenticated, "unauthenticated"},
		{QuotaUnknown, "unknown"},
	}
	for _, tc := range cases {
		if string(tc.got) != tc.want {
			t.Errorf("QuotaStateValue %q, want %q", string(tc.got), tc.want)
		}
	}

	// Distinctness: every enumerated value is distinct.
	seen := map[QuotaStateValue]struct{}{}
	for _, tc := range cases {
		if _, dup := seen[tc.got]; dup {
			t.Errorf("duplicate QuotaStateValue: %q", tc.got)
		}
		seen[tc.got] = struct{}{}
	}
}

// TestErrAliasNotResolvableIdentity ensures the sentinel is a non-nil,
// errors.Is-comparable singleton with a stable message. Callers (notably
// the conformance suite landing in BEAD-HARNESS-IF-02) rely on
// errors.Is(err, ErrAliasNotResolvable) to assert negative-path coverage
// of ResolveModelAlias.
func TestErrAliasNotResolvableIdentity(t *testing.T) {
	if ErrAliasNotResolvable == nil {
		t.Fatal("ErrAliasNotResolvable is nil")
	}
	if got, want := ErrAliasNotResolvable.Error(), "model alias not resolvable from snapshot"; got != want {
		t.Errorf("ErrAliasNotResolvable.Error() = %q, want %q", got, want)
	}
	if !errors.Is(ErrAliasNotResolvable, ErrAliasNotResolvable) {
		t.Error("errors.Is(ErrAliasNotResolvable, ErrAliasNotResolvable) = false")
	}
	wrapped := errWrap{inner: ErrAliasNotResolvable}
	if !errors.Is(wrapped, ErrAliasNotResolvable) {
		t.Error("errors.Is on wrapped sentinel = false")
	}
	other := errors.New("model alias not resolvable from snapshot")
	if errors.Is(other, ErrAliasNotResolvable) {
		t.Error("a freshly-allocated error with the same message must not match the sentinel")
	}
}

type errWrap struct{ inner error }

func (e errWrap) Error() string { return "wrapped: " + e.inner.Error() }
func (e errWrap) Unwrap() error { return e.inner }
