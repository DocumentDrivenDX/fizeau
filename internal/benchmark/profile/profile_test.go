package profile

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func writeFile(path string, data []byte) error {
	return os.WriteFile(path, data, 0o644)
}

// repoProfilesDir returns the path to scripts/benchmark/profiles/ relative
// to this test file (internal/benchmark/profile/). Three levels up.
func repoProfilesDir(t *testing.T) string {
	t.Helper()
	return filepath.Join("..", "..", "..", "scripts", "benchmark", "profiles")
}

func TestLoadDir_AllShippedProfilesValidate(t *testing.T) {
	profiles, err := LoadDir(repoProfilesDir(t))
	require.NoError(t, err)
	require.NotEmpty(t, profiles, "expected at least one shipped profile")

	ids := map[string]*Profile{}
	for _, p := range profiles {
		require.NoError(t, p.Validate(), "profile %s failed validation", p.ID)
		ids[p.ID] = p
	}

	// SD-010 §3 + bead AC: smoke + noop fixtures must ship in v1.
	require.Contains(t, ids, "smoke")
	require.Contains(t, ids, "noop")
	// Phase A.1 anchor: gpt-5-mini via OpenRouter.
	require.Contains(t, ids, "gpt-5-mini")
	// TerminalBench medium comparison lanes.
	require.Contains(t, ids, "fiz-harness-claude-sonnet-4-6")
	require.Contains(t, ids, "fiz-harness-codex-gpt-5-4-mini")
	require.Contains(t, ids, "fiz-harness-pi-gpt-5-4-mini")
	require.Contains(t, ids, "fiz-harness-opencode-gpt-5-4-mini")
	require.Contains(t, ids, "fiz-openrouter-claude-sonnet-4-6")
	require.Contains(t, ids, "fiz-openrouter-gpt-5-4-mini")
}

func TestLoad_GPT5MiniAnchorProfile(t *testing.T) {
	p, err := Load(filepath.Join(repoProfilesDir(t), "gpt-5-mini.yaml"))
	require.NoError(t, err)
	require.Equal(t, "gpt-5-mini", p.ID)
	require.Equal(t, ProviderOpenAICompat, p.Provider.Type)
	require.Equal(t, "openai/gpt-5-mini", p.Provider.Model)
	require.Equal(t, "OPENROUTER_API_KEY", p.Provider.APIKeyEnv)
	// Census §5 pricing snapshot — values must stay inside the ≤ $3/Mtok
	// output bracket that selected this row.
	require.Equal(t, 2.00, p.Pricing.OutputUSDPerMTok)
	require.Equal(t, 0.25, p.Pricing.InputUSDPerMTok)
	require.Equal(t, 0.05, p.Pricing.CachedInputUSDPerMTok)
	require.LessOrEqual(t, p.Pricing.OutputUSDPerMTok, 3.00)
	require.Equal(t, 16384, p.Limits.MaxOutputTokens)
	require.Greater(t, p.Limits.ContextTokens, 0)
	require.Greater(t, p.Limits.RateLimitRPM, 0)
	require.Greater(t, p.Limits.RateLimitTPM, 0)
	require.Equal(t, "medium", p.Sampling.Reasoning)
	require.Equal(t, "2026-04-30", p.Versioning.ResolvedAt)
	require.NotEmpty(t, p.Versioning.Snapshot)
}

func TestLoad_SmokeProfileFields(t *testing.T) {
	p, err := Load(filepath.Join(repoProfilesDir(t), "smoke.yaml"))
	require.NoError(t, err)
	require.Equal(t, "smoke", p.ID)
	require.Equal(t, ProviderOpenAICompat, p.Provider.Type)
	require.Equal(t, "OPENAI_API_KEY", p.Provider.APIKeyEnv)
	require.Greater(t, p.Limits.MaxOutputTokens, 0)
	require.Greater(t, p.Limits.ContextTokens, 0)
	require.NotEmpty(t, p.Versioning.ResolvedAt)
	require.NotEmpty(t, p.Path)
}

func TestLoad_NoopProfileFields(t *testing.T) {
	p, err := Load(filepath.Join(repoProfilesDir(t), "noop.yaml"))
	require.NoError(t, err)
	require.Equal(t, "noop", p.ID)
	require.Equal(t, ProviderOpenAICompat, p.Provider.Type)
	require.Equal(t, 0.0, p.Pricing.InputUSDPerMTok)
	require.Equal(t, "noop-fixture", p.Versioning.Snapshot)
}

func TestValidate_RejectsMissingFields(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(p *Profile)
		wantSub string
	}{
		{"missing id", func(p *Profile) { p.ID = "" }, "id is required"},
		{"bad provider type", func(p *Profile) { p.Provider.Type = "bedrock" }, "provider.type"},
		{"missing model", func(p *Profile) { p.Provider.Model = "" }, "provider.model"},
		{"missing base_url", func(p *Profile) { p.Provider.BaseURL = "" }, "provider.base_url"},
		{"missing api_key_env", func(p *Profile) { p.Provider.APIKeyEnv = "" }, "provider.api_key_env"},
		{"negative pricing", func(p *Profile) { p.Pricing.InputUSDPerMTok = -1 }, "pricing"},
		{"zero max_output", func(p *Profile) { p.Limits.MaxOutputTokens = 0 }, "max_output_tokens"},
		{"zero context", func(p *Profile) { p.Limits.ContextTokens = 0 }, "context_tokens"},
		{"missing resolved_at", func(p *Profile) { p.Versioning.ResolvedAt = "" }, "resolved_at"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := goodProfile()
			tc.mutate(&p)
			err := p.Validate()
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.wantSub)
		})
	}
}

func TestLoad_RejectsUnknownFields(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.yaml")
	require.NoError(t, writeFile(bad, []byte(`
id: bad
provider: {type: openai, model: m, base_url: https://x, api_key_env: K}
pricing: {input_usd_per_mtok: 1, output_usd_per_mtok: 1, cached_input_usd_per_mtok: 0}
limits: {max_output_tokens: 1, context_tokens: 1, rate_limit_rpm: 1, rate_limit_tpm: 1}
sampling: {temperature: 0}
versioning: {resolved_at: "2026-04-29", snapshot: ""}
unknown_top_level: oops
`)))
	_, err := Load(bad)
	require.Error(t, err)
}

func goodProfile() Profile {
	return Profile{
		ID: "x",
		Provider: Provider{
			Type:      ProviderOpenAI,
			Model:     "m",
			BaseURL:   "https://api.example.com",
			APIKeyEnv: "K",
		},
		Pricing:    Pricing{InputUSDPerMTok: 1, OutputUSDPerMTok: 1},
		Limits:     Limits{MaxOutputTokens: 1024, ContextTokens: 8192},
		Sampling:   Sampling{Temperature: 0},
		Versioning: Versioning{ResolvedAt: "2026-04-29"},
	}
}
