// Package profile is the canonical reader for v7 benchmark profile YAML files
// under scripts/benchmark/profiles/. The schema is frozen by SD-010 §3
// (Harness × Model Matrix Benchmark); additive fields require a spec amendment.
package profile

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// ProviderType enumerates the provider client paths supported by the
// matrix runner. SD-010 §3 forbids silent override; the adapter selects its
// client path by exact match on this field.
type ProviderType string

const (
	ProviderAnthropic    ProviderType = "anthropic"
	ProviderOpenAI       ProviderType = "openai"
	ProviderOpenAICompat ProviderType = "openai-compat"
	ProviderOpenRouter   ProviderType = "openrouter"
	ProviderOMLX         ProviderType = "omlx"
	ProviderLMStudio     ProviderType = "lmstudio"
	ProviderOllama       ProviderType = "ollama"
	ProviderGoogle       ProviderType = "google"
	ProviderRapidMLX     ProviderType = "rapid-mlx"
	ProviderVLLM         ProviderType = "vllm"
)

func (p ProviderType) valid() bool {
	switch p {
	case ProviderAnthropic, ProviderOpenAI, ProviderOpenAICompat, ProviderOpenRouter,
		ProviderOMLX, ProviderLMStudio, ProviderOllama, ProviderGoogle, ProviderRapidMLX, ProviderVLLM:
		return true
	}
	return false
}

// Provider describes how the harness adapter should reach the model API.
type Provider struct {
	Type      ProviderType `yaml:"type"`
	Model     string       `yaml:"model"`
	BaseURL   string       `yaml:"base_url"`
	APIKeyEnv string       `yaml:"api_key_env"`
}

// Pricing is the single source of truth for cost reconciliation. Units are
// USD per million tokens.
type Pricing struct {
	InputUSDPerMTok       float64 `yaml:"input_usd_per_mtok"`
	OutputUSDPerMTok      float64 `yaml:"output_usd_per_mtok"`
	CachedInputUSDPerMTok float64 `yaml:"cached_input_usd_per_mtok"`
}

// Limits captures provider-side ceilings. rate_limit_* are informational in
// v1 (D6 forbids concurrency > 1) and reserved for the follow-up scheduler.
type Limits struct {
	MaxOutputTokens int `yaml:"max_output_tokens"`
	ContextTokens   int `yaml:"context_tokens"`
	RateLimitRPM    int `yaml:"rate_limit_rpm"`
	RateLimitTPM    int `yaml:"rate_limit_tpm"`
}

// Sampling is opaque to the runner; passed verbatim to the adapter's
// apply_profile step. Reasoning is a free-form string ("low" | "medium" |
// "high" | "" depending on the family). Pointer fields are omitted when
// nil so server defaults apply.
type Sampling struct {
	Temperature float64  `yaml:"temperature"`
	Reasoning   string   `yaml:"reasoning,omitempty"`
	TopP        *float64 `yaml:"top_p,omitempty"`
	TopK        *int     `yaml:"top_k,omitempty"`
	MinP        *float64 `yaml:"min_p,omitempty"`
}

// ModelServerInfo is populated at run time by querying the local model server
// (e.g. lmstudio /api/v0/models/<id>). Fields are empty/zero when the server
// does not expose them.
type ModelServerInfo struct {
	Quantization        string `json:"quantization,omitempty"`
	LoadedContextLength int    `json:"loaded_context_length,omitempty"`
	MaxContextLength    int    `json:"max_context_length,omitempty"`
	Source              string `json:"source,omitempty"` // URL queried
}

// Versioning records when the profile was authored and which provider
// snapshot the adapter resolved at apply_profile time.
type Versioning struct {
	ResolvedAt string `yaml:"resolved_at"`
	Snapshot   string `yaml:"snapshot"`
}

// Profile is the in-memory shape of a frozen v1 profile YAML.
type Profile struct {
	ID         string     `yaml:"id"`
	Provider   Provider   `yaml:"provider"`
	Pricing    Pricing    `yaml:"pricing"`
	Limits     Limits     `yaml:"limits"`
	Sampling   Sampling   `yaml:"sampling"`
	Versioning Versioning `yaml:"versioning"`

	// Path is the filesystem path the profile was loaded from. Not part of
	// the YAML; populated by Load / LoadDir for diagnostics and `profiles
	// list` output.
	Path string `yaml:"-"`
}

// Load reads and validates a single profile YAML file at path.
func Load(path string) (*Profile, error) {
	raw, err := os.ReadFile(path) // #nosec G304 -- path is an operator-supplied profile location, scoped by Validate()
	if err != nil {
		return nil, fmt.Errorf("read profile %s: %w", path, err)
	}
	var p Profile
	dec := yaml.NewDecoder(strings.NewReader(string(raw)))
	dec.KnownFields(true)
	if err := dec.Decode(&p); err != nil {
		return nil, fmt.Errorf("parse profile %s: %w", path, err)
	}
	p.Path = path
	if err := p.Validate(); err != nil {
		return nil, fmt.Errorf("validate profile %s: %w", path, err)
	}
	return &p, nil
}

// LoadDir loads every *.yaml / *.yml file under dir, sorted by id. It is the
// entry point used by `fiz-bench profiles list`.
func LoadDir(dir string) ([]*Profile, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read profiles dir %s: %w", dir, err)
	}
	var profiles []*Profile
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if ext != ".yaml" && ext != ".yml" {
			continue
		}
		p, err := Load(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		profiles = append(profiles, p)
	}
	sort.Slice(profiles, func(i, j int) bool { return profiles[i].ID < profiles[j].ID })
	return profiles, nil
}

// Validate checks that every required v1 field is present. The schema is
// frozen by SD-010 §3; missing fields are a hard error rather than a warning.
func (p *Profile) Validate() error {
	if strings.TrimSpace(p.ID) == "" {
		return fmt.Errorf("id is required")
	}
	if !p.Provider.Type.valid() {
		return fmt.Errorf("provider.type %q is not one of anthropic|openai|openrouter|omlx|lmstudio|ollama|google|rapid-mlx|vllm", p.Provider.Type)
	}
	if strings.TrimSpace(p.Provider.Model) == "" {
		return fmt.Errorf("provider.model is required")
	}
	if strings.TrimSpace(p.Provider.BaseURL) == "" {
		return fmt.Errorf("provider.base_url is required")
	}
	if strings.TrimSpace(p.Provider.APIKeyEnv) == "" {
		return fmt.Errorf("provider.api_key_env is required")
	}
	if p.Pricing.InputUSDPerMTok < 0 || p.Pricing.OutputUSDPerMTok < 0 || p.Pricing.CachedInputUSDPerMTok < 0 {
		return fmt.Errorf("pricing.* must be non-negative")
	}
	if p.Limits.MaxOutputTokens <= 0 {
		return fmt.Errorf("limits.max_output_tokens must be > 0")
	}
	if p.Limits.ContextTokens <= 0 {
		return fmt.Errorf("limits.context_tokens must be > 0")
	}
	if strings.TrimSpace(p.Versioning.ResolvedAt) == "" {
		return fmt.Errorf("versioning.resolved_at is required")
	}
	return nil
}
