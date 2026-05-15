package fizeau

import (
	"testing"
	"time"

	"github.com/easel/fizeau/internal/modelsnapshot"
)

// stubProviderConfigSource implements ServiceConfigSource for the
// providerCooldownsFromSnapshotErrors test.
type stubProviderConfigSource struct {
	names []string
}

func (s stubProviderConfigSource) ProviderNames() []string { return s.names }

func TestProviderCooldownsFromSnapshotErrorsDetectsDialFailures(t *testing.T) {
	now := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	cfg := stubProviderConfigSource{names: []string{
		"rg-bragi-club-3090",
		"rg-vidar-omlx",
		"rg-openrouter",
	}}
	snap := modelsnapshot.ModelSnapshot{
		Sources: map[string]modelsnapshot.SourceMeta{
			"rg-bragi-club-3090-props": {
				Error:           `dial tcp 100.127.38.115:1234: i/o timeout`,
				LastRefreshedAt: now.Add(-30 * time.Second),
			},
			"rg-bragi-club-3090": {
				Error:           `Post "http://bragi:8020/v1/models": dial tcp 100.127.38.115:8020: connection refused`,
				LastRefreshedAt: now.Add(-10 * time.Second),
			},
			"rg-vidar-omlx": {
				LastRefreshedAt: now.Add(-1 * time.Minute),
				// no error → reachable
			},
			"rg-openrouter": {
				Error:           "unauthorized: invalid API key", // not a dial failure
				LastRefreshedAt: now.Add(-5 * time.Second),
			},
		},
	}
	got := providerCooldownsFromSnapshotErrors(snap, cfg, now, 5*time.Minute)
	if len(got) != 1 {
		t.Fatalf("expected 1 entry, got %d: %v", len(got), got)
	}
	if _, ok := got["rg-bragi-club-3090"]; !ok {
		t.Errorf("expected rg-bragi-club-3090 to be marked unreachable; got %v", got)
	}
	// Multiple sources for the same provider — should pick the most recent failure.
	if got["rg-bragi-club-3090"] != now.Add(-10*time.Second) {
		t.Errorf("expected most-recent failure timestamp; got %v", got["rg-bragi-club-3090"])
	}
}

func TestProviderCooldownsFromSnapshotErrorsRespectsTTL(t *testing.T) {
	now := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	cfg := stubProviderConfigSource{names: []string{"rg-bragi"}}
	snap := modelsnapshot.ModelSnapshot{
		Sources: map[string]modelsnapshot.SourceMeta{
			"rg-bragi": {
				Error:           "dial tcp 192.168.1.1:8020: i/o timeout",
				LastRefreshedAt: now.Add(-1 * time.Hour), // older than TTL
			},
		},
	}
	got := providerCooldownsFromSnapshotErrors(snap, cfg, now, 5*time.Minute)
	if len(got) != 0 {
		t.Errorf("expected empty (TTL expired), got %v", got)
	}
}

func TestProviderCooldownsFromSnapshotErrorsZeroTimeUsesNow(t *testing.T) {
	now := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	cfg := stubProviderConfigSource{names: []string{"rg-bragi"}}
	snap := modelsnapshot.ModelSnapshot{
		Sources: map[string]modelsnapshot.SourceMeta{
			"rg-bragi-props": {
				Error: "dial tcp 192.168.1.1:8020: i/o timeout",
				// LastRefreshedAt zero — treat as "fresh" (discovery only emits on probe).
			},
		},
	}
	got := providerCooldownsFromSnapshotErrors(snap, cfg, now, 5*time.Minute)
	if len(got) != 1 || got["rg-bragi"] != now {
		t.Errorf("expected rg-bragi cooldown at now, got %v", got)
	}
}

func TestIsSnapshotDialFailure(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"", false},
		{`dial tcp 100.127.38.115:1234: i/o timeout`, true},
		{`Post "http://bragi:8020/v1/models": dial tcp 100.127.38.115:8020: connection refused`, true},
		{"no route to host", true},
		{"network is unreachable", true},
		{"no such host", true},
		{"i/o timeout", true},
		// Real shapes seen in ddx-server.log for bragi:
		{`POST "http://bragi:1234/v1/chat/completions": 502 Bad Gateway `, true},
		{`upstream returned 503 Service Unavailable`, true},
		{`504 gateway timeout`, true},
		{"unauthorized: invalid API key", false},
		{"context deadline exceeded", false},                  // not a dial-class signature
		{"connection reset by peer", false},                   // mid-stream, not dial
		{"provider request timeout: wall-clock 15m0s", false}, // ambiguous; could be slow live host
		{"429 too many requests", false},                      // rate-limit state machine
	}
	for _, c := range cases {
		if got := isSnapshotDialFailure(c.in); got != c.want {
			t.Errorf("isSnapshotDialFailure(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}
