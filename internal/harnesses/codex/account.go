package codex

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/DocumentDrivenDX/fizeau/internal/harnesses"
	"github.com/DocumentDrivenDX/fizeau/internal/safefs"
)

const codexAuthPathEnv = "FIZEAU_CODEX_AUTH"

const codexAuthNamespace = "https://api.openai.com/auth"

// CodexAuthPath returns the local Codex auth file used for account metadata.
func CodexAuthPath() (string, error) {
	if path := os.Getenv(codexAuthPathEnv); path != "" {
		return path, nil
	}
	if home := os.Getenv("CODEX_HOME"); home != "" {
		return filepath.Join(home, "auth.json"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".codex", "auth.json"), nil
}

// ReadCodexAccount extracts non-secret account metadata from Codex auth state.
func ReadCodexAccount() (*harnesses.AccountInfo, bool) {
	path, err := CodexAuthPath()
	if err != nil {
		return nil, false
	}
	return ReadCodexAccountFrom(path)
}

// ReadCodexAccountFrom reads auth.json and extracts email, plan, and org info.
func ReadCodexAccountFrom(path string) (*harnesses.AccountInfo, bool) {
	data, err := safefs.ReadFile(path)
	if err != nil {
		return nil, false
	}
	var auth struct {
		Tokens struct {
			IDToken string `json:"id_token"`
		} `json:"tokens"`
		OpenAIAPIKey string `json:"OPENAI_API_KEY"`
	}
	if err := json.Unmarshal(data, &auth); err != nil {
		return nil, false
	}
	if auth.Tokens.IDToken != "" {
		if account, ok := codexAccountFromIDToken(auth.Tokens.IDToken); ok {
			return account, true
		}
	}
	if auth.OpenAIAPIKey != "" {
		return &harnesses.AccountInfo{PlanType: "OpenAI API key"}, true
	}
	return nil, false
}

func codexAccountFromIDToken(token string) (*harnesses.AccountInfo, bool) {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return nil, false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		payload, err = base64.URLEncoding.DecodeString(parts[1])
		if err != nil {
			return nil, false
		}
	}
	var claims map[string]any
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, false
	}
	account := &harnesses.AccountInfo{
		Email:    stringClaim(claims, "email"),
		PlanType: normalizeCodexPlanType(nestedStringClaim(claims, codexAuthNamespace, "chatgpt_plan_type")),
		OrgName:  defaultCodexOrgName(claims),
	}
	if account.Email == "" && account.PlanType == "" && account.OrgName == "" {
		return nil, false
	}
	return account, true
}

func stringClaim(claims map[string]any, key string) string {
	value, _ := claims[key].(string)
	return strings.TrimSpace(value)
}

func nestedStringClaim(claims map[string]any, namespace, key string) string {
	nested, _ := claims[namespace].(map[string]any)
	if nested == nil {
		return ""
	}
	return stringClaim(nested, key)
}

func defaultCodexOrgName(claims map[string]any) string {
	nested, _ := claims[codexAuthNamespace].(map[string]any)
	if nested == nil {
		return ""
	}
	orgs, _ := nested["organizations"].([]any)
	var first string
	for _, raw := range orgs {
		org, _ := raw.(map[string]any)
		if org == nil {
			continue
		}
		title, _ := org["title"].(string)
		title = strings.TrimSpace(title)
		if title == "" {
			continue
		}
		if first == "" {
			first = title
		}
		if isDefault, _ := org["is_default"].(bool); isDefault {
			return title
		}
	}
	return first
}

func normalizeCodexPlanType(plan string) string {
	plan = strings.TrimSpace(plan)
	if plan == "" {
		return ""
	}
	lower := strings.ToLower(strings.ReplaceAll(plan, "_", " "))
	switch {
	case strings.Contains(lower, "enterprise"):
		return "ChatGPT Enterprise"
	case strings.Contains(lower, "team"):
		return "ChatGPT Team"
	case strings.Contains(lower, "max"):
		return "ChatGPT Max"
	case strings.Contains(lower, "pro"):
		return "ChatGPT Pro"
	case strings.Contains(lower, "plus"):
		return "ChatGPT Plus"
	case strings.Contains(lower, "free"):
		return "ChatGPT Free"
	}
	if strings.HasPrefix(plan, "ChatGPT ") {
		return plan
	}
	return "ChatGPT " + plan
}

func codexAccountSupportsAutoRouting(account *harnesses.AccountInfo) bool {
	if account == nil {
		return false
	}
	switch account.PlanType {
	case "ChatGPT Enterprise", "ChatGPT Team", "ChatGPT Max", "ChatGPT Pro", "ChatGPT Plus":
		return true
	default:
		return false
	}
}
