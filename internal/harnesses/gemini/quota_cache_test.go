package gemini

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/easel/fizeau/internal/harnesses"
)

func writeTestQuota(t *testing.T, path string, snap GeminiQuotaSnapshot) {
	t.Helper()
	if err := WriteGeminiQuota(path, snap); err != nil {
		t.Fatalf("WriteGeminiQuota: %v", err)
	}
}

func TestDecideGeminiQuotaRouting_FreshHeadroom(t *testing.T) {
	now := time.Now().UTC()
	snap := &GeminiQuotaSnapshot{
		CapturedAt: now.Add(-1 * time.Minute),
		Source:     "pty",
		Windows: []harnesses.QuotaWindow{
			{Name: "Flash", LimitID: "gemini-flash", UsedPercent: 4, State: "ok"},
			{Name: "Flash Lite", LimitID: "gemini-flash-lite", UsedPercent: 0, State: "ok"},
			{Name: "Pro", LimitID: "gemini-pro", UsedPercent: 100, State: "exhausted"},
		},
	}

	dec := DecideGeminiQuotaRouting(snap, now, 0)
	if !dec.PreferGemini {
		t.Fatalf("fresh snapshot with Flash headroom should prefer gemini: %+v", dec)
	}
	if !dec.Fresh {
		t.Fatal("recent snapshot must be Fresh")
	}
	if !containsString(dec.AvailableTiers, "gemini-flash") || !containsString(dec.AvailableTiers, "gemini-flash-lite") {
		t.Fatalf("available tiers should include flash and flash-lite: %#v", dec.AvailableTiers)
	}
	if !containsString(dec.ExhaustedTiers, "gemini-pro") {
		t.Fatalf("pro at 100%% used must land in ExhaustedTiers: %#v", dec.ExhaustedTiers)
	}
	if !dec.IsGeminiTierAvailable("gemini-2.5-flash") {
		t.Fatal("gemini-2.5-flash should be selectable under fresh Flash headroom")
	}
	if dec.IsGeminiTierAvailable("gemini-2.5-pro") {
		t.Fatal("gemini-2.5-pro must not be selectable while Pro is exhausted")
	}
}

func TestDecideGeminiQuotaRouting_AllExhausted(t *testing.T) {
	now := time.Now().UTC()
	snap := &GeminiQuotaSnapshot{
		CapturedAt: now.Add(-1 * time.Minute),
		Source:     "pty",
		Windows: []harnesses.QuotaWindow{
			{Name: "Flash", LimitID: "gemini-flash", UsedPercent: 100, State: "exhausted"},
			{Name: "Pro", LimitID: "gemini-pro", UsedPercent: 100, State: "exhausted"},
		},
	}
	dec := DecideGeminiQuotaRouting(snap, now, 0)
	if dec.PreferGemini {
		t.Fatal("all-exhausted snapshot must not prefer gemini")
	}
	if len(dec.AvailableTiers) != 0 {
		t.Fatalf("no tier should be available: %#v", dec.AvailableTiers)
	}
	if dec.Reason == "" {
		t.Fatal("decision must include a reason")
	}
}

func TestDecideGeminiQuotaRouting_StaleSnapshot(t *testing.T) {
	now := time.Now().UTC()
	snap := &GeminiQuotaSnapshot{
		CapturedAt: now.Add(-2 * DefaultGeminiQuotaStaleAfter),
		Source:     "pty",
		Windows: []harnesses.QuotaWindow{
			{Name: "Flash", LimitID: "gemini-flash", UsedPercent: 4, State: "ok"},
		},
	}
	dec := DecideGeminiQuotaRouting(snap, now, 0)
	if dec.Fresh {
		t.Fatal("stale snapshot must not be Fresh")
	}
	if dec.PreferGemini {
		t.Fatal("stale snapshot must not prefer gemini")
	}
}

func TestDecideGeminiQuotaRouting_NoSnapshot(t *testing.T) {
	dec := DecideGeminiQuotaRouting(nil, time.Now().UTC(), 0)
	if dec.PreferGemini || dec.SnapshotPresent || dec.Fresh {
		t.Fatalf("nil snapshot must not prefer gemini: %+v", dec)
	}
}

func TestWriteReadGeminiQuotaRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gemini-quota.json")
	t.Setenv("FIZEAU_GEMINI_QUOTA_CACHE", path)

	writeTestQuota(t, path, GeminiQuotaSnapshot{
		Source: "pty",
		Windows: []harnesses.QuotaWindow{
			{Name: "Flash", LimitID: "gemini-flash", UsedPercent: 4, State: "ok", ResetsAt: "9:13 PM (23h 46m)"},
			{Name: "Flash Lite", LimitID: "gemini-flash-lite", UsedPercent: 0, State: "ok", ResetsAt: "9:27 PM (24h)"},
			{Name: "Pro", LimitID: "gemini-pro", UsedPercent: 100, State: "exhausted"},
		},
	})

	got, ok := ReadGeminiQuota()
	if !ok || got == nil {
		t.Fatal("ReadGeminiQuota returned no snapshot after Write")
	}
	if len(got.Windows) != 3 {
		t.Fatalf("round-trip lost windows: %#v", got.Windows)
	}
	if got.Windows[2].UsedPercent != 100 || got.Windows[2].State != "exhausted" {
		t.Fatalf("round-trip lost exhausted Pro tier: %#v", got.Windows[2])
	}
	if got.CapturedAt.IsZero() {
		t.Fatal("CapturedAt must be populated by Write")
	}
	if got.MaxUsedPercent() != 100 {
		t.Fatalf("MaxUsedPercent should surface exhausted Pro tier: %v", got.MaxUsedPercent())
	}
}

func containsString(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
