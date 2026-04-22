package gemini

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/DocumentDrivenDX/agent/internal/harnesses"
	"github.com/DocumentDrivenDX/agent/internal/safefs"
)

const GeminiAuthFreshnessWindow = 7 * 24 * time.Hour

const (
	authTypeGeminiAPIKey = "gemini-api-key"
	authTypeOAuth        = "oauth-personal"
	authTypeVertexAI     = "vertex-ai"
	authTypeComputeADC   = "compute-default-credentials"
	authTypeGateway      = "gateway"
)

// AuthSnapshot captures non-secret Gemini CLI auth/account evidence. It never
// stores API keys, OAuth tokens, refresh tokens, or raw credential payloads.
type AuthSnapshot struct {
	Authenticated bool
	AuthType      string
	Account       *harnesses.AccountInfo
	CapturedAt    time.Time
	Fresh         bool
	Source        string
	Detail        string
}

// ReadAuthEvidence reads Gemini CLI auth evidence from the user's configured
// environment and ~/.gemini directory. It is best-effort because Gemini CLI has
// no stable non-interactive quota command; routing uses this as auth/account
// freshness evidence and exposes quota as unknown.
func ReadAuthEvidence(now time.Time) AuthSnapshot {
	if now.IsZero() {
		now = time.Now()
	}
	if authType := authTypeFromEnv(); authType != "" {
		return AuthSnapshot{
			Authenticated: true,
			AuthType:      authType,
			Account:       accountForAuthType(authType),
			CapturedAt:    now.UTC(),
			Fresh:         true,
			Source:        "environment",
			Detail:        "Gemini auth selected from environment; secret values are not read",
		}
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return AuthSnapshot{Source: "~/.gemini", Detail: "home directory unavailable"}
	}
	return ReadAuthEvidenceFromDir(filepath.Join(home, ".gemini"), now)
}

// ReadAuthEvidenceFromDir reads non-secret Gemini CLI auth evidence from a
// caller-provided .gemini directory. Tests use this for credential-free replay.
func ReadAuthEvidenceFromDir(dir string, now time.Time) AuthSnapshot {
	if now.IsZero() {
		now = time.Now()
	}
	settingsPath := filepath.Join(dir, "settings.json")
	authType, settingsTime, settingsOK := readSelectedAuthType(settingsPath)
	if authType == "" {
		return AuthSnapshot{
			Source:     settingsPath,
			CapturedAt: settingsTime,
			Fresh:      settingsOK && isFresh(settingsTime, now),
			Detail:     "Gemini CLI auth method not configured",
		}
	}

	switch authType {
	case authTypeOAuth:
		return readOAuthEvidence(dir, authType, settingsTime, now)
	case authTypeGeminiAPIKey, authTypeVertexAI, authTypeGateway, authTypeComputeADC:
		return AuthSnapshot{
			Authenticated: true,
			AuthType:      authType,
			Account:       accountForAuthType(authType),
			CapturedAt:    settingsTime,
			Fresh:         isFresh(settingsTime, now),
			Source:        settingsPath,
			Detail:        "Gemini CLI auth method configured; secret material is external to DDx",
		}
	default:
		return AuthSnapshot{
			AuthType:   authType,
			Source:     settingsPath,
			CapturedAt: settingsTime,
			Fresh:      isFresh(settingsTime, now),
			Detail:     "Gemini CLI auth method is not recognized by DDx",
		}
	}
}

func readSelectedAuthType(path string) (string, time.Time, bool) {
	st, err := os.Stat(path)
	if err != nil || st.IsDir() {
		return "", time.Time{}, false
	}
	data, err := safefs.ReadFile(path)
	if err != nil {
		return "", st.ModTime().UTC(), false
	}
	var settings struct {
		Security struct {
			Auth struct {
				SelectedType string `json:"selectedType"`
			} `json:"auth"`
		} `json:"security"`
	}
	if err := json.Unmarshal(data, &settings); err != nil {
		return "", st.ModTime().UTC(), false
	}
	return strings.TrimSpace(settings.Security.Auth.SelectedType), st.ModTime().UTC(), true
}

func readOAuthEvidence(dir, authType string, settingsTime, now time.Time) AuthSnapshot {
	oauthPath := filepath.Join(dir, "oauth_creds.json")
	st, err := os.Stat(oauthPath)
	if err != nil || st.IsDir() {
		return AuthSnapshot{
			AuthType:   authType,
			Source:     oauthPath,
			CapturedAt: settingsTime,
			Fresh:      isFresh(settingsTime, now),
			Detail:     "Gemini OAuth credentials are missing",
		}
	}
	data, err := safefs.ReadFile(oauthPath)
	if err != nil {
		return AuthSnapshot{
			AuthType:   authType,
			Source:     oauthPath,
			CapturedAt: st.ModTime().UTC(),
			Fresh:      isFresh(st.ModTime(), now),
			Detail:     "Gemini OAuth credentials could not be read",
		}
	}
	var creds struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiryDate   int64  `json:"expiry_date"`
	}
	if err := json.Unmarshal(data, &creds); err != nil {
		return AuthSnapshot{
			AuthType:   authType,
			Source:     oauthPath,
			CapturedAt: st.ModTime().UTC(),
			Fresh:      isFresh(st.ModTime(), now),
			Detail:     "Gemini OAuth credentials are malformed",
		}
	}
	authenticated := creds.RefreshToken != "" || creds.AccessToken != ""
	if creds.ExpiryDate > 0 && creds.RefreshToken == "" {
		expires := time.UnixMilli(creds.ExpiryDate)
		authenticated = authenticated && expires.After(now)
	}
	capturedAt := maxTime(settingsTime, st.ModTime().UTC())
	return AuthSnapshot{
		Authenticated: authenticated,
		AuthType:      authType,
		Account:       readGoogleAccount(dir),
		CapturedAt:    capturedAt,
		Fresh:         isFresh(capturedAt, now),
		Source:        oauthPath,
		Detail:        oauthDetail(authenticated),
	}
}

func readGoogleAccount(dir string) *harnesses.AccountInfo {
	data, err := safefs.ReadFile(filepath.Join(dir, "google_accounts.json"))
	if err != nil {
		return &harnesses.AccountInfo{PlanType: "Gemini OAuth"}
	}
	var accounts struct {
		Active string `json:"active"`
	}
	if err := json.Unmarshal(data, &accounts); err != nil {
		return &harnesses.AccountInfo{PlanType: "Gemini OAuth"}
	}
	account := &harnesses.AccountInfo{PlanType: "Gemini OAuth"}
	if strings.Contains(accounts.Active, "@") {
		account.Email = accounts.Active
	}
	return account
}

func accountForAuthType(authType string) *harnesses.AccountInfo {
	switch authType {
	case authTypeGeminiAPIKey:
		return &harnesses.AccountInfo{PlanType: "Gemini API key"}
	case authTypeVertexAI:
		return &harnesses.AccountInfo{PlanType: "Vertex AI"}
	case authTypeComputeADC:
		return &harnesses.AccountInfo{PlanType: "Google ADC"}
	case authTypeGateway:
		return &harnesses.AccountInfo{PlanType: "Gemini gateway"}
	default:
		return &harnesses.AccountInfo{PlanType: authType}
	}
}

func authTypeFromEnv() string {
	if os.Getenv("GOOGLE_GENAI_USE_GCA") != "" {
		return authTypeGateway
	}
	if os.Getenv("GOOGLE_GENAI_USE_VERTEXAI") != "" {
		return authTypeVertexAI
	}
	if os.Getenv("GOOGLE_API_KEY") != "" || os.Getenv("GEMINI_API_KEY") != "" {
		return authTypeGeminiAPIKey
	}
	if os.Getenv("CLOUD_SHELL") == "true" || os.Getenv("GEMINI_CLI_USE_COMPUTE_ADC") == "true" {
		return authTypeComputeADC
	}
	return ""
}

func oauthDetail(authenticated bool) string {
	if authenticated {
		return "Gemini OAuth credentials found; token values are not retained"
	}
	return "Gemini OAuth credentials do not contain usable token metadata"
}

func isFresh(capturedAt, now time.Time) bool {
	if capturedAt.IsZero() {
		return false
	}
	return now.Sub(capturedAt) <= GeminiAuthFreshnessWindow
}

func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}
