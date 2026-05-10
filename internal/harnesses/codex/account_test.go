package codex

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/easel/fizeau/internal/harnesses"
	"github.com/stretchr/testify/require"
)

func TestReadCodexAccountFromIDToken(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth.json")
	idToken := testJWT(map[string]any{
		"email": "dev@example.com",
		codexAuthNamespace: map[string]any{
			"chatgpt_plan_type": "pro",
			"organizations": []map[string]any{
				{"title": "secondary"},
				{"title": "primary", "is_default": true},
			},
		},
	})
	auth := map[string]any{"tokens": map[string]any{"id_token": idToken}}
	raw, err := json.Marshal(auth)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, raw, 0o600))

	account, ok := ReadCodexAccountFrom(path)
	require.True(t, ok)
	require.Equal(t, "dev@example.com", account.Email)
	require.Equal(t, "ChatGPT Pro", account.PlanType)
	require.Equal(t, "primary", account.OrgName)
}

func TestReadCodexAccountFromAPIKeyOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"OPENAI_API_KEY":"redacted"}`), 0o600))

	account, ok := ReadCodexAccountFrom(path)
	require.True(t, ok)
	require.Equal(t, "OpenAI API key", account.PlanType)
}

func TestCodexAccountSupportsAutoRouting(t *testing.T) {
	require.True(t, codexAccountSupportsAutoRouting(accountWithPlan("ChatGPT Pro")))
	require.True(t, codexAccountSupportsAutoRouting(accountWithPlan("ChatGPT Plus")))
	require.False(t, codexAccountSupportsAutoRouting(accountWithPlan("ChatGPT Free")))
	require.False(t, codexAccountSupportsAutoRouting(accountWithPlan("OpenAI API key")))
	require.False(t, codexAccountSupportsAutoRouting(nil))
}

func TestReadCodexAccountFromMalformedAuth(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"tokens":{"id_token":"not-a-jwt"}}`), 0o600))

	account, ok := ReadCodexAccountFrom(path)
	require.False(t, ok)
	require.Nil(t, account)
}

func accountWithPlan(plan string) *harnesses.AccountInfo {
	return &harnesses.AccountInfo{PlanType: plan}
}

func testJWT(claims map[string]any) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`))
	payload, _ := json.Marshal(claims)
	return header + "." + base64.RawURLEncoding.EncodeToString(payload) + "."
}
