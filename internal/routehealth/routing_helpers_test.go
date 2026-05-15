package routehealth

import (
	"errors"
	"testing"
	"time"

	"github.com/easel/fizeau/internal/routing"
)

func TestEffectivePowerPolicyAppliesCatalogBounds(t *testing.T) {
	policy := EffectivePowerPolicy(PowerRequest{
		Policy:   "default",
		MinPower: 5,
		MaxPower: 10,
	}, func(name string) (PolicySpec, bool) {
		if name != "default" {
			return PolicySpec{}, false
		}
		return PolicySpec{Name: "default", MinPower: 7, MaxPower: 8}, true
	})

	if policy.PolicyName != "default" || policy.MinPower != 7 || policy.MaxPower != 8 {
		t.Fatalf("policy=%#v, want default 7..8", policy)
	}
}

func TestPowerBoundsForRequestKeepsExplicitModelPins(t *testing.T) {
	minPower, maxPower := PowerBoundsForRequest(PowerRequest{
		Model:    "pinned-model",
		MinPower: 2,
		MaxPower: 11,
	}, PowerPolicy{
		PolicyName: "default",
		MinPower:   7,
		MaxPower:   8,
	})

	if minPower != 2 || maxPower != 11 {
		t.Fatalf("bounds=%d..%d, want explicit 2..11", minPower, maxPower)
	}
}

func TestApplyAttemptCooldownsPromotesDispatchabilityFailures(t *testing.T) {
	recordedAt := time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)
	in := &routing.Inputs{
		Harnesses: []routing.HarnessEntry{{Name: "codex"}},
	}

	ApplyAttemptCooldowns(in, []Record{
		{
			Key: Key{
				Provider: "bragi",
			},
			Error:      `dial tcp 192.168.0.10:1234: connection refused`,
			RecordedAt: recordedAt,
		},
		{
			Key: Key{
				Harness: "codex",
			},
			RecordedAt: recordedAt,
		},
	}, 45*time.Second)

	if got := in.ProviderCooldowns["bragi"]; !got.Equal(recordedAt) {
		t.Fatalf("ProviderCooldowns[bragi]=%v, want %v", got, recordedAt)
	}
	if got := in.ProviderUnreachable["bragi"]; !got.Equal(recordedAt) {
		t.Fatalf("ProviderUnreachable[bragi]=%v, want %v", got, recordedAt)
	}
	if in.CooldownDuration != 45*time.Second {
		t.Fatalf("CooldownDuration=%v, want 45s", in.CooldownDuration)
	}
	if !in.Harnesses[0].InCooldown {
		t.Fatal("expected codex harness cooldown to be applied")
	}
}

func TestCandidateCooldownMatchesEndpointProviderRefs(t *testing.T) {
	recordedAt := time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)
	cooldown := CandidateCooldown([]Record{
		{
			Key: Key{
				Provider: "bragi@primary",
				Model:    "qwen",
			},
			Reason:     "timeout",
			Error:      "context deadline exceeded",
			RecordedAt: recordedAt,
		},
	}, "bragi", "primary", "qwen", 30*time.Second)

	if cooldown == nil {
		t.Fatal("expected cooldown for bragi@primary")
	}
	if cooldown.Reason != "timeout" {
		t.Fatalf("Reason=%q, want timeout", cooldown.Reason)
	}
	if !cooldown.LastAttempt.Equal(recordedAt) {
		t.Fatalf("LastAttempt=%v, want %v", cooldown.LastAttempt, recordedAt)
	}
}

func TestProviderCooldownsFromSnapshotErrorsUsesLongestProviderPrefix(t *testing.T) {
	now := time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)
	got := ProviderCooldownsFromSnapshotErrors([]SnapshotSource{
		{
			Name:            "rg-bragi-club-3090-props",
			Error:           `dial tcp 10.0.0.8:1234: i/o timeout`,
			LastRefreshedAt: now.Add(-5 * time.Second),
		},
		{
			Name:            "rg-bragi-metadata",
			Error:           "authentication failed",
			LastRefreshedAt: now.Add(-5 * time.Second),
		},
	}, []string{"rg-bragi", "rg-bragi-club-3090"}, now, 30*time.Second)

	if len(got) != 1 {
		t.Fatalf("cooldowns=%v, want one match", got)
	}
	if _, ok := got["rg-bragi-club-3090"]; !ok {
		t.Fatalf("cooldowns=%v, want longest configured provider name", got)
	}
}

func TestShouldEscalateOnErrorRejectsHardPinErrors(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "harness model incompatible",
			err:  &routing.ErrHarnessModelIncompatible{Harness: "codex", Model: "gpt-5.5"},
			want: false,
		},
		{
			name: "unsatisfiable pin",
			err:  &routing.ErrUnsatisfiablePin{Pin: "provider=bragi", Reason: "unknown provider"},
			want: false,
		},
		{
			name: "policy filtered pin",
			err:  &routing.ErrPolicyRequirementUnsatisfied{Policy: "air-gapped", Requirement: "no_remote"},
			want: false,
		},
		{
			name: "no viable candidate",
			err:  &routing.NoViableCandidateError{Rejected: 2},
			want: true,
		},
		{
			name: "wrapped no viable candidate",
			err:  errors.New("wrapper: " + (&routing.NoViableCandidateError{Rejected: 1}).Error()),
			want: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ShouldEscalateOnError(tc.err); got != tc.want {
				t.Fatalf("ShouldEscalateOnError(%T)=%v, want %v", tc.err, got, tc.want)
			}
		})
	}
}
