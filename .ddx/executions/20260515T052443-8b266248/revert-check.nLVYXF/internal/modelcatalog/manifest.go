package modelcatalog

import (
	_ "embed"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/easel/fizeau/internal/reasoning"
	"github.com/easel/fizeau/internal/sampling"
	"gopkg.in/yaml.v3"
)

const (
	statusActive     = "active"
	statusDeprecated = "deprecated"
	statusStale      = "stale"
	maxSchemaVersion = 5
)

//go:embed catalog/models.yaml
var embeddedManifest []byte

// LoadOptions configures how a catalog manifest is loaded.
type LoadOptions struct {
	ManifestPath    string
	RequireExternal bool
}

// ModelEntry holds per-model metadata from the v5 manifest.
type ModelEntry struct {
	Family         string `yaml:"family,omitempty"`
	DisplayName    string `yaml:"display_name,omitempty"`
	Status         string `yaml:"status,omitempty"`
	ProviderSystem string `yaml:"provider_system,omitempty"`
	// QuotaPool names the shared quota allocation this model burns. When unset,
	// the model's provider_system is the effective quota pool; for example,
	// openai-hosted models default to "openai" and ds4 models default to "ds4".
	QuotaPool          string                      `yaml:"quota_pool,omitempty"`
	DeploymentClass    string                      `yaml:"deployment_class,omitempty" json:"deployment_class,omitempty"`
	PowerProvenance    PowerProvenance             `yaml:"power_provenance,omitempty" json:"power_provenance,omitempty"`
	CostInputPerM      float64                     `yaml:"cost_input_per_m,omitempty"`
	CostOutputPerM     float64                     `yaml:"cost_output_per_m,omitempty"`
	CostCacheReadPerM  float64                     `yaml:"cost_cache_read_per_m,omitempty"`
	CostCacheWritePerM float64                     `yaml:"cost_cache_write_per_m,omitempty"`
	CostInputPerMTok   float64                     `yaml:"cost_input_per_mtok,omitempty"`
	CostOutputPerMTok  float64                     `yaml:"cost_output_per_mtok,omitempty"`
	SWEBenchVerified   float64                     `yaml:"swe_bench_verified,omitempty"`
	LiveCodeBench      float64                     `yaml:"live_code_bench,omitempty"`
	BenchmarkAsOf      string                      `yaml:"benchmark_as_of,omitempty"`
	OpenRouterID       string                      `yaml:"openrouter_id,omitempty"`
	OpenRouterRefID    string                      `yaml:"openrouter_ref_id,omitempty"`
	Surfaces           map[string]string           `yaml:"surfaces,omitempty"`
	SpeedTokensPerSec  float64                     `yaml:"speed_tokens_per_sec,omitempty"`
	ContextWindow      int                         `yaml:"context_window,omitempty"`
	Power              int                         `yaml:"power,omitempty" json:"power,omitempty"`
	ExactPinOnly       bool                        `yaml:"exact_pin_only,omitempty" json:"exact_pin_only,omitempty"`
	NoTools            bool                        `yaml:"no_tools,omitempty"`
	ReasoningDefault   reasoning.Reasoning         `yaml:"reasoning_default,omitempty"`
	ReasoningMaxTokens int                         `yaml:"reasoning_max_tokens,omitempty"`
	ReasoningBudgets   map[reasoning.Reasoning]int `yaml:"reasoning_budgets,omitempty"`
	ReasoningLevels    []string                    `yaml:"reasoning_levels,omitempty"`
	ReasoningControl   string                      `yaml:"reasoning_control,omitempty"`
	ReasoningWire      string                      `yaml:"reasoning_wire,omitempty"`
	// SamplingControl declares whether catalog sampling_profiles reach the
	// wire for runs that resolve to this model. See ADR-007 §4.
	//   client_settable (default) — provider honors all five sampler fields.
	//   harness_pinned             — wrapped harness pins samplers internally;
	//                                 the resolver short-circuits to a
	//                                 zero-value bundle.
	//   partial                    — provider honors a subset (reserved; not
	//                                 enforced in v1).
	SamplingControl string `yaml:"sampling_control,omitempty"`
}

// EffectiveQuotaPool returns the quota allocation name used for this model.
// Explicit quota_pool values take precedence; otherwise provider_system is the
// default pool.
func (m ModelEntry) EffectiveQuotaPool() string {
	if m.QuotaPool != "" {
		return m.QuotaPool
	}
	return m.ProviderSystem
}

// PowerProvenance records why a model received its catalog power score.
type PowerProvenance struct {
	Method          string             `yaml:"method,omitempty" json:"method,omitempty"`
	Benchmarks      map[string]float64 `yaml:"benchmarks,omitempty" json:"benchmarks,omitempty"`
	Recency         string             `yaml:"recency,omitempty" json:"recency,omitempty"`
	CostInputPerM   float64            `yaml:"cost_input_per_m,omitempty" json:"cost_input_per_m,omitempty"`
	CostOutputPerM  float64            `yaml:"cost_output_per_m,omitempty" json:"cost_output_per_m,omitempty"`
	DeploymentClass string             `yaml:"deployment_class,omitempty" json:"deployment_class,omitempty"`
	OverrideReason  string             `yaml:"override_reason,omitempty" json:"override_reason,omitempty"`
}

// Reasoning capability control values.
const (
	ReasoningControlTunable = "tunable"
	ReasoningControlFixed   = "fixed"
	ReasoningControlNone    = "none"

	ReasoningWireProvider = "provider"
	ReasoningWireModelID  = "model_id"
	ReasoningWireNone     = "none"
	ReasoningWireEffort   = "effort"
	ReasoningWireTokens   = "tokens"
)

// Sampling-control values for ModelEntry.SamplingControl. See ADR-007 §4.
const (
	SamplingControlClientSettable = "client_settable"
	SamplingControlHarnessPinned  = "harness_pinned"
	SamplingControlPartial        = "partial"
)

type manifest struct {
	Version        int                      `yaml:"version"`
	GeneratedAt    string                   `yaml:"generated_at"`
	CatalogVersion string                   `yaml:"catalog_version,omitempty"`
	Models         map[string]ModelEntry    `yaml:"models,omitempty"`
	Policies       map[string]policyEntry   `yaml:"policies,omitempty"`
	Providers      map[string]providerEntry `yaml:"providers,omitempty"`
	// SamplingProfiles is a map from profile name (e.g., "code") to a named
	// bundle of sampling-parameter overrides. Profile bundles are the L1
	// layer of the resolution chain; see ADR-007.
	SamplingProfiles map[string]sampling.Profile `yaml:"sampling_profiles,omitempty"`
}

type policyEntry struct {
	MinPower   int      `yaml:"min_power,omitempty"`
	MaxPower   int      `yaml:"max_power,omitempty"`
	AllowLocal bool     `yaml:"allow_local,omitempty"`
	Require    []string `yaml:"require,omitempty"`
}

type providerEntry struct {
	Type             string `yaml:"type,omitempty"`
	IncludeByDefault bool   `yaml:"include_by_default,omitempty"`
	Billing          string `yaml:"billing,omitempty"`
}

// Default loads the embedded default catalog snapshot.
func Default() (*Catalog, error) {
	return Load(LoadOptions{})
}

// Load loads a catalog from an external manifest or falls back to the embedded snapshot.
func Load(opts LoadOptions) (*Catalog, error) {
	data := embeddedManifest
	source := "embedded"

	if opts.ManifestPath != "" {
		externalData, err := os.ReadFile(opts.ManifestPath)
		if err != nil {
			if opts.RequireExternal {
				return nil, fmt.Errorf("modelcatalog: read manifest %s: %w", opts.ManifestPath, err)
			}
		} else {
			data = externalData
			source = opts.ManifestPath
		}
	}

	return loadManifest(data, source)
}

func loadManifest(data []byte, source string) (*Catalog, error) {
	var m manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("modelcatalog: parse manifest %s: %w", source, err)
	}
	if err := validateManifest(m); err != nil {
		return nil, fmt.Errorf("modelcatalog: validate manifest %s: %w", source, err)
	}
	return &Catalog{
		manifest:    m,
		manifestSrc: source,
	}, nil
}

func validateManifest(m manifest) error {
	if m.Version != maxSchemaVersion {
		return fmt.Errorf("manifest schema v%d required (got v%d); see ADR-009 for v0.11 schema redesign", maxSchemaVersion, m.Version)
	}
	if len(m.Policies) == 0 {
		return fmt.Errorf("policies must not be empty")
	}
	if _, ok := m.Policies["default"]; !ok {
		return fmt.Errorf("default policy must be defined")
	}

	policyNames := make([]string, 0, len(m.Policies))
	for name := range m.Policies {
		policyNames = append(policyNames, name)
	}
	sort.Strings(policyNames)
	for _, name := range policyNames {
		entry := m.Policies[name]
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("policy name must not be empty")
		}
		if entry.MinPower <= 0 {
			return fmt.Errorf("policy %q min_power must be > 0", name)
		}
		if entry.MaxPower <= 0 {
			return fmt.Errorf("policy %q max_power must be > 0", name)
		}
		if entry.MinPower > entry.MaxPower {
			return fmt.Errorf("policy %q min_power must be <= max_power", name)
		}
		if entry.MaxPower > 10 {
			return fmt.Errorf("policy %q max_power must be <= 10", name)
		}
		for _, requirement := range entry.Require {
			if !knownPolicyRequirement(requirement) {
				return fmt.Errorf("policy %q has unknown require invariant %q", name, requirement)
			}
		}
	}

	providerNames := make([]string, 0, len(m.Providers))
	for name := range m.Providers {
		providerNames = append(providerNames, name)
	}
	sort.Strings(providerNames)
	for _, name := range providerNames {
		entry := m.Providers[name]
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("provider name must not be empty")
		}
		if strings.TrimSpace(entry.Type) == "" {
			return fmt.Errorf("provider %q must define type", name)
		}
		if entry.Billing != "" && !knownBillingModel(BillingModel(entry.Billing)) {
			return fmt.Errorf("provider %q has invalid billing %q", name, entry.Billing)
		}
		if !knownProviderSystem(entry.Type) && strings.TrimSpace(entry.Billing) == "" {
			return fmt.Errorf("provider %q type %q requires explicit billing field", name, entry.Type)
		}
	}

	for modelID, model := range m.Models {
		if strings.TrimSpace(modelID) == "" {
			return fmt.Errorf("model ID must not be empty")
		}
		status := normalizedStatus(model.Status)
		switch status {
		case statusActive, statusDeprecated, statusStale:
		default:
			return fmt.Errorf("model %q has invalid status %q", modelID, model.Status)
		}
		if model.CostInputPerM < 0 || model.CostInputPerMTok < 0 {
			return fmt.Errorf("model %q cost_input_per_m must be >= 0", modelID)
		}
		if model.CostOutputPerM < 0 || model.CostOutputPerMTok < 0 {
			return fmt.Errorf("model %q cost_output_per_m must be >= 0", modelID)
		}
		if model.CostCacheReadPerM < 0 {
			return fmt.Errorf("model %q cost_cache_read_per_m must be >= 0", modelID)
		}
		if model.CostCacheWritePerM < 0 {
			return fmt.Errorf("model %q cost_cache_write_per_m must be >= 0", modelID)
		}
		if model.ContextWindow < 0 {
			return fmt.Errorf("model %q context_window must be >= 0", modelID)
		}
		if model.Power < 0 {
			return fmt.Errorf("model %q power must be >= 0", modelID)
		}
		if model.Power > 10 {
			return fmt.Errorf("model %q power must be <= 10", modelID)
		}
		if model.QuotaPool != "" && !validQuotaPoolName(model.QuotaPool) {
			return fmt.Errorf("model %q has invalid quota_pool %q (must contain only lowercase letters, digits, and dash)", modelID, model.QuotaPool)
		}
		if model.ReasoningMaxTokens < 0 {
			return fmt.Errorf("model %q reasoning_max_tokens must be >= 0", modelID)
		}
		for level, budget := range model.ReasoningBudgets {
			if budget < 0 {
				return fmt.Errorf("model %q reasoning_budgets %q must be >= 0", modelID, level)
			}
			if model.ReasoningMaxTokens > 0 && budget > model.ReasoningMaxTokens {
				return fmt.Errorf("model %q reasoning_budgets %q exceeds reasoning_max_tokens", modelID, level)
			}
		}
		switch model.ReasoningDefault {
		case "", reasoning.ReasoningOff, reasoning.ReasoningLow, reasoning.ReasoningMedium, reasoning.ReasoningHigh, reasoning.ReasoningMax, reasoning.ReasoningXHigh:
		default:
			return fmt.Errorf("model %q has invalid reasoning_default %q", modelID, model.ReasoningDefault)
		}
		switch model.ReasoningControl {
		case "", ReasoningControlTunable, ReasoningControlFixed, ReasoningControlNone:
		default:
			return fmt.Errorf("model %q has invalid reasoning_control %q (must be one of tunable, fixed, none)", modelID, model.ReasoningControl)
		}
		switch model.ReasoningWire {
		case "", ReasoningWireProvider, ReasoningWireModelID, ReasoningWireNone, ReasoningWireEffort, ReasoningWireTokens:
		default:
			return fmt.Errorf("model %q has invalid reasoning_wire %q (must be one of provider, model_id, none, effort, tokens)", modelID, model.ReasoningWire)
		}
		switch model.SamplingControl {
		case "", SamplingControlClientSettable, SamplingControlHarnessPinned, SamplingControlPartial:
		default:
			return fmt.Errorf("model %q has invalid sampling_control %q (must be one of client_settable, harness_pinned, partial)", modelID, model.SamplingControl)
		}
	}

	return nil
}

func knownPolicyRequirement(requirement string) bool {
	switch strings.TrimSpace(requirement) {
	case "no_remote":
		return true
	default:
		return false
	}
}

func normalizedStatus(status string) string {
	status = strings.ToLower(strings.TrimSpace(status))
	if status == "" {
		return statusActive
	}
	return status
}

func validQuotaPoolName(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9':
		case r == '-':
		default:
			return false
		}
	}
	return true
}
