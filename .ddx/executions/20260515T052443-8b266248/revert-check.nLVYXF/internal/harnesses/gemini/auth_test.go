package gemini

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestReadAuthEvidenceFromDir_OAuthFresh(t *testing.T) {
	now := time.Date(2026, 4, 22, 12, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	writeGeminiTestFile(t, dir, "settings.json", `{"security":{"auth":{"selectedType":"oauth-personal"}}}`, now.Add(-time.Hour))
	writeGeminiTestFile(t, dir, "oauth_creds.json", `{"access_token":"redacted","refresh_token":"redacted","expiry_date":1770000000000}`, now.Add(-time.Hour))
	writeGeminiTestFile(t, dir, "google_accounts.json", `{"active":"dev@example.com"}`, now.Add(-time.Hour))

	snap := readAuthEvidenceFromDir(dir, now)
	if !snap.Authenticated || !snap.Fresh {
		t.Fatalf("auth snapshot: %#v", snap)
	}
	if snap.AuthType != "oauth-personal" {
		t.Fatalf("auth type: got %q", snap.AuthType)
	}
	if snap.Account == nil || snap.Account.Email != "dev@example.com" || snap.Account.PlanType != "Gemini OAuth" {
		t.Fatalf("account: %#v", snap.Account)
	}
}

func TestReadAuthEvidenceFromDir_StaleOAuth(t *testing.T) {
	now := time.Date(2026, 4, 22, 12, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	stale := now.Add(-(geminiAuthFreshnessWindow + time.Hour))
	writeGeminiTestFile(t, dir, "settings.json", `{"security":{"auth":{"selectedType":"oauth-personal"}}}`, stale)
	writeGeminiTestFile(t, dir, "oauth_creds.json", `{"access_token":"redacted","refresh_token":"redacted"}`, stale)

	snap := readAuthEvidenceFromDir(dir, now)
	if !snap.Authenticated {
		t.Fatalf("expected stale-but-authenticated evidence, got %#v", snap)
	}
	if snap.Fresh {
		t.Fatalf("expected stale evidence, got %#v", snap)
	}
}

func TestReadAuthEvidenceFromDir_MissingCredentials(t *testing.T) {
	now := time.Date(2026, 4, 22, 12, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	writeGeminiTestFile(t, dir, "settings.json", `{"security":{"auth":{"selectedType":"oauth-personal"}}}`, now)

	snap := readAuthEvidenceFromDir(dir, now)
	if snap.Authenticated {
		t.Fatalf("missing credentials should be unauthenticated: %#v", snap)
	}
	if snap.Detail == "" {
		t.Fatalf("missing credentials should carry diagnostic detail: %#v", snap)
	}
}

func TestReadAuthEvidenceFromDir_APIKeyMode(t *testing.T) {
	now := time.Date(2026, 4, 22, 12, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	writeGeminiTestFile(t, dir, "settings.json", `{"security":{"auth":{"selectedType":"gemini-api-key"}}}`, now)

	snap := readAuthEvidenceFromDir(dir, now)
	if !snap.Authenticated || !snap.Fresh {
		t.Fatalf("auth snapshot: %#v", snap)
	}
	if snap.Account == nil || snap.Account.PlanType != "Gemini API key" {
		t.Fatalf("account: %#v", snap.Account)
	}
}

func writeGeminiTestFile(t *testing.T, dir, name, content string, modTime time.Time) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	if err := os.Chtimes(path, modTime, modTime); err != nil {
		t.Fatalf("chtimes %s: %v", name, err)
	}
}
