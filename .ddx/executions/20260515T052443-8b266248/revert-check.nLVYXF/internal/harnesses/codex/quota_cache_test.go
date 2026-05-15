package codex

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/easel/fizeau/internal/harnesses"
)

func TestCodexQuotaSnapshotRoundTrip(t *testing.T) {
	t.Setenv(codexAuthPathEnv, filepath.Join(t.TempDir(), "missing-auth.json"))
	path := filepath.Join(t.TempDir(), "codex-quota.json")
	original := codexQuotaSnapshot{
		CapturedAt: time.Now().UTC().Add(-time.Minute).Truncate(time.Second),
		Source:     "pty",
		Account:    &harnesses.AccountInfo{Email: "dev@example.com", PlanType: "ChatGPT Pro", OrgName: "agent"},
		Windows: []harnesses.QuotaWindow{
			{Name: "5h", LimitID: "codex", WindowMinutes: 300, UsedPercent: 25, State: "ok"},
		},
	}
	if err := writeCodexQuota(path, original); err != nil {
		t.Fatalf("writeCodexQuota: %v", err)
	}
	loaded, ok := readCodexQuotaFrom(path)
	if !ok {
		t.Fatal("readCodexQuotaFrom returned ok=false")
	}
	if !loaded.CapturedAt.Equal(original.CapturedAt) {
		t.Fatalf("CapturedAt: got %v, want %v", loaded.CapturedAt, original.CapturedAt)
	}
	if loaded.Source != "pty" {
		t.Fatalf("Source: got %q, want pty", loaded.Source)
	}
	if loaded.Account == nil || loaded.Account.PlanType != "ChatGPT Pro" || loaded.Account.Email != "dev@example.com" {
		t.Fatalf("Account: got %#v", loaded.Account)
	}
	if len(loaded.Windows) != 1 || loaded.Windows[0].UsedPercent != 25 {
		t.Fatalf("Windows: got %#v", loaded.Windows)
	}
}

func TestReadCodexQuotaUsesDefaultPath(t *testing.T) {
	t.Setenv(codexAuthPathEnv, filepath.Join(t.TempDir(), "missing-auth.json"))
	path := filepath.Join(t.TempDir(), "codex-quota.json")
	t.Setenv(codexQuotaCacheEnv, path)
	if err := writeCodexQuota(path, codexQuotaSnapshot{
		CapturedAt: time.Now().UTC(),
		Source:     "pty",
		Account:    &harnesses.AccountInfo{PlanType: "ChatGPT Pro"},
		Windows:    []harnesses.QuotaWindow{{Name: "5h", State: "ok"}},
	}); err != nil {
		t.Fatalf("writeCodexQuota: %v", err)
	}
	if _, ok := readCodexQuota(); !ok {
		t.Fatal("readCodexQuota returned ok=false")
	}
}

func TestIsCodexQuotaFresh(t *testing.T) {
	t.Setenv(codexAuthPathEnv, filepath.Join(t.TempDir(), "missing-auth.json"))
	now := time.Now().UTC()
	if isCodexQuotaFresh(nil, now, time.Minute) {
		t.Fatal("nil snapshot should not be fresh")
	}
	fresh := &codexQuotaSnapshot{CapturedAt: now.Add(-30 * time.Second)}
	if !isCodexQuotaFresh(fresh, now, time.Minute) {
		t.Fatal("fresh snapshot should be fresh")
	}
	stale := &codexQuotaSnapshot{CapturedAt: now.Add(-2 * time.Minute)}
	if isCodexQuotaFresh(stale, now, time.Minute) {
		t.Fatal("stale snapshot should not be fresh")
	}
}

func TestWriteCodexQuotaFillsAccountFromAuth(t *testing.T) {
	dir := t.TempDir()
	authPath := filepath.Join(dir, "auth.json")
	t.Setenv(codexAuthPathEnv, authPath)
	idToken := testJWT(map[string]any{
		"email": "dev@example.com",
		codexAuthNamespace: map[string]any{
			"chatgpt_plan_type": "plus",
			"organizations":     []map[string]any{{"title": "agent"}},
		},
	})
	raw, err := json.Marshal(map[string]any{"tokens": map[string]any{"id_token": idToken}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(authPath, raw, 0o600); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(dir, "codex-quota.json")
	if err := writeCodexQuota(path, codexQuotaSnapshot{
		CapturedAt: time.Now().UTC(),
		Windows:    []harnesses.QuotaWindow{{Name: "5h", State: "ok"}},
	}); err != nil {
		t.Fatalf("writeCodexQuota: %v", err)
	}
	loaded, ok := readCodexQuotaFrom(path)
	if !ok {
		t.Fatal("readCodexQuotaFrom returned ok=false")
	}
	if loaded.Account == nil || loaded.Account.PlanType != "ChatGPT Plus" || loaded.Account.OrgName != "agent" {
		t.Fatalf("Account: got %#v", loaded.Account)
	}
}

func TestReadCodexQuotaMissingAndCorruptReturnFalse(t *testing.T) {
	dir := t.TempDir()
	if snap, ok := readCodexQuotaFrom(filepath.Join(dir, "missing.json")); ok || snap != nil {
		t.Fatalf("missing cache: got snap=%#v ok=%v", snap, ok)
	}
	corrupt := filepath.Join(dir, "corrupt.json")
	if err := os.WriteFile(corrupt, []byte(`{not-json`), 0o600); err != nil {
		t.Fatal(err)
	}
	if snap, ok := readCodexQuotaFrom(corrupt); ok || snap != nil {
		t.Fatalf("corrupt cache: got snap=%#v ok=%v", snap, ok)
	}
}

func TestDecideCodexQuotaRouting(t *testing.T) {
	t.Setenv(codexAuthPathEnv, filepath.Join(t.TempDir(), "missing-auth.json"))
	now := time.Now().UTC()
	cases := []struct {
		name   string
		snap   *codexQuotaSnapshot
		prefer bool
		fresh  bool
	}{
		{name: "missing", snap: nil},
		{
			name: "stale",
			snap: &codexQuotaSnapshot{
				CapturedAt: now.Add(-20 * time.Minute),
				Windows:    []harnesses.QuotaWindow{{Name: "5h", UsedPercent: 10, State: "ok"}},
			},
		},
		{
			name:  "empty windows",
			snap:  &codexQuotaSnapshot{CapturedAt: now},
			fresh: true,
		},
		{
			name: "blocked",
			snap: &codexQuotaSnapshot{
				CapturedAt: now,
				Windows:    []harnesses.QuotaWindow{{Name: "5h", UsedPercent: 95, State: "blocked"}},
				Account:    &harnesses.AccountInfo{PlanType: "ChatGPT Pro"},
			},
			fresh: true,
		},
		{
			name: "missing account",
			snap: &codexQuotaSnapshot{
				CapturedAt: now,
				Windows:    []harnesses.QuotaWindow{{Name: "5h", UsedPercent: 25, State: "ok"}},
			},
			fresh: true,
		},
		{
			name: "api key account",
			snap: &codexQuotaSnapshot{
				CapturedAt: now,
				Windows:    []harnesses.QuotaWindow{{Name: "5h", UsedPercent: 25, State: "ok"}},
				Account:    &harnesses.AccountInfo{PlanType: "OpenAI API key"},
			},
			fresh: true,
		},
		{
			name: "fresh headroom",
			snap: &codexQuotaSnapshot{
				CapturedAt: now,
				Source:     "pty",
				Windows:    []harnesses.QuotaWindow{{Name: "5h", UsedPercent: 25, State: "ok"}},
				Account:    &harnesses.AccountInfo{PlanType: "ChatGPT Pro"},
			},
			prefer: true,
			fresh:  true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dec := decideCodexQuotaRouting(tc.snap, now, defaultCodexQuotaStaleAfter)
			if dec.PreferCodex != tc.prefer {
				t.Fatalf("PreferCodex: got %v, want %v (%s)", dec.PreferCodex, tc.prefer, dec.Reason)
			}
			if dec.Fresh != tc.fresh {
				t.Fatalf("Fresh: got %v, want %v (%s)", dec.Fresh, tc.fresh, dec.Reason)
			}
			if dec.Reason == "" {
				t.Fatal("Reason should be populated")
			}
		})
	}
}
