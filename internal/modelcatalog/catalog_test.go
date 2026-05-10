package modelcatalog

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/easel/fizeau/internal/reasoning"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeFixtureManifest(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "models.yaml")
	require.NoError(t, os.WriteFile(path, []byte(contents), 0o644))
	return path
}

func loadFixtureCatalog(t *testing.T) *Catalog {
	t.Helper()
	catalog, err := Load(LoadOptions{
		ManifestPath: writeFixtureManifest(t, `
version: 5
generated_at: 2026-05-08T00:00:00Z
catalog_version: 2026-05-08.test
policies:
  default:
    min_power: 7
    max_power: 8
    allow_local: true
  cheap:
    min_power: 5
    max_power: 5
    allow_local: true
  smart:
    min_power: 9
    max_power: 10
    allow_local: false
  air-gapped:
    min_power: 5
    max_power: 5
    allow_local: true
    require: [no_remote]
models:
  alpha-10:
    family: alpha
    status: active
    provider_system: anthropic
    deployment_class: managed_cloud_frontier
    power: 10
    cost_input_per_m: 5.00
    cost_output_per_m: 15.00
    context_window: 1000000
    swe_bench_verified: 80.0
    reasoning_default: high
    surfaces:
      agent.openai: alpha-openai-10
      agent.anthropic: alpha-anthropic-10
  alpha-9-local:
    family: alpha
    status: active
    provider_system: openai
    deployment_class: local_free
    power: 9
    cost_input_per_m: 0.10
    cost_output_per_m: 0.30
    context_window: 262144
    surfaces:
      agent.openai: alpha-local-9
  beta-8:
    family: beta
    status: active
    provider_system: openai
    deployment_class: metered_cloud
    power: 8
    cost_input_per_m: 1.00
    cost_output_per_m: 4.00
    context_window: 200000
    reasoning_default: off
    surfaces:
      agent.openai: beta-openai-8
  gamma-5-local:
    family: gamma
    status: active
    provider_system: openai
    deployment_class: local_free
    power: 5
    cost_input_per_m: 0.10
    cost_output_per_m: 0.30
    context_window: 131072
    surfaces:
      agent.openai: gamma-local-5
  delta-5-remote:
    family: delta
    status: active
    provider_system: google
    deployment_class: metered_cloud
    power: 5
    cost_input_per_m: 0.05
    cost_output_per_m: 0.20
    context_window: 131072
    surfaces:
      agent.openai: delta-remote-5
  old-alpha:
    family: alpha
    status: deprecated
    provider_system: anthropic
    deployment_class: managed_cloud_frontier
    power: 0
    exact_pin_only: true
    surfaces:
      agent.openai: old-alpha
`),
		RequireExternal: true,
	})
	require.NoError(t, err)
	return catalog
}

func TestDefault_LoadsEmbeddedManifest(t *testing.T) {
	catalog, err := Default()
	require.NoError(t, err)

	resolved, err := catalog.Current("smart", ResolveOptions{
		Surface: SurfaceAgentAnthropic,
	})
	require.NoError(t, err)
	assert.Equal(t, "smart", resolved.Profile)
	assert.Equal(t, "smart", resolved.CanonicalID)
	assert.Equal(t, "opus-4.7", resolved.ConcreteModel)
	assert.Equal(t, reasoning.ReasoningHigh, resolved.SurfacePolicy.ReasoningDefault)
	assert.Equal(t, "embedded", resolved.ManifestSource)
	assert.Equal(t, 5, resolved.ManifestVersion)
	assert.Equal(t, "2026-05-08.1", resolved.CatalogVersion)
}

func TestEmbeddedCatalogParses(t *testing.T) {
	catalog, err := Default()
	require.NoError(t, err)
	assert.Equal(t, 5, catalog.Metadata().ManifestVersion)
	assert.Equal(t, "2026-05-08.1", catalog.Metadata().CatalogVersion)
}

func TestCatalogPolicyAndPolicies(t *testing.T) {
	catalog, err := Default()
	require.NoError(t, err)

	policies := catalog.Policies()
	require.Len(t, policies, 4)
	assert.Equal(t, []string{"air-gapped", "cheap", "default", "smart"}, []string{
		policies[0].Name,
		policies[1].Name,
		policies[2].Name,
		policies[3].Name,
	})

	defaultPolicy, ok := catalog.Policy("default")
	require.True(t, ok)
	assert.Equal(t, 7, defaultPolicy.MinPower)
	assert.Equal(t, 8, defaultPolicy.MaxPower)
	assert.True(t, defaultPolicy.AllowLocal)

	airGapped, ok := catalog.Policy("air-gapped")
	require.True(t, ok)
	assert.Equal(t, []string{"no_remote"}, airGapped.Require)
}

func TestProvidersDeriveBillingModels(t *testing.T) {
	catalog, err := Default()
	require.NoError(t, err)

	byName := make(map[string]Provider)
	for _, provider := range catalog.Providers() {
		byName[provider.Name] = provider
	}
	assert.Equal(t, BillingModelPerToken, byName["openai"].Billing)
	assert.Equal(t, BillingModelFixed, byName["lmstudio"].Billing)
	assert.Equal(t, BillingModelSubscription, byName["codex"].Billing)
}

func TestResolveCompatibilityPolicyNames(t *testing.T) {
	catalog := loadFixtureCatalog(t)

	standard, err := catalog.Resolve("standard", ResolveOptions{Surface: SurfaceAgentOpenAI})
	require.NoError(t, err)
	assert.Equal(t, "standard", standard.Profile)
	assert.Equal(t, "standard", standard.CanonicalID)
	assert.Equal(t, "beta-openai-8", standard.ConcreteModel)

	codeSmart, err := catalog.Resolve("code-smart", ResolveOptions{Surface: SurfaceAgentOpenAI})
	require.NoError(t, err)
	assert.Equal(t, "smart", codeSmart.Profile)
	assert.Equal(t, "alpha-openai-10", codeSmart.ConcreteModel)
}

func TestResolvePolicyHonorsNoRemoteRequirement(t *testing.T) {
	catalog := loadFixtureCatalog(t)

	cheap := catalog.CandidatesFor(SurfaceAgentOpenAI, "cheap")
	assert.Equal(t, []string{"delta-remote-5", "gamma-local-5"}, cheap)

	airGapped := catalog.CandidatesFor(SurfaceAgentOpenAI, "air-gapped")
	assert.Equal(t, []string{"gamma-local-5"}, airGapped)
}

func TestResolvePolicyExcludesLocalWhenDisallowed(t *testing.T) {
	catalog := loadFixtureCatalog(t)

	candidates := catalog.CandidatesFor(SurfaceAgentOpenAI, "smart")
	assert.Equal(t, []string{"alpha-openai-10"}, candidates)
}

func TestResolveCanonicalModel(t *testing.T) {
	catalog := loadFixtureCatalog(t)

	resolved, err := catalog.Resolve("alpha-10", ResolveOptions{Surface: SurfaceAgentAnthropic})
	require.NoError(t, err)
	assert.Equal(t, "alpha-10", resolved.CanonicalID)
	assert.Equal(t, "alpha-anthropic-10", resolved.ConcreteModel)
	assert.Equal(t, "alpha", resolved.Family)
	assert.Equal(t, reasoning.ReasoningHigh, resolved.SurfacePolicy.ReasoningDefault)
}

func TestResolveSurfaceModelID(t *testing.T) {
	catalog := loadFixtureCatalog(t)

	resolved, err := catalog.Resolve("alpha-openai-10", ResolveOptions{Surface: SurfaceAgentOpenAI})
	require.NoError(t, err)
	assert.Equal(t, "alpha-10", resolved.CanonicalID)
	assert.Equal(t, "alpha-openai-10", resolved.ConcreteModel)
}

func TestResolveDeprecatedStrict(t *testing.T) {
	catalog := loadFixtureCatalog(t)

	_, err := catalog.Resolve("old-alpha", ResolveOptions{
		Surface: SurfaceAgentOpenAI,
	})
	require.Error(t, err)

	var deprecatedErr *DeprecatedTargetError
	require.True(t, errors.As(err, &deprecatedErr))
	assert.Equal(t, "old-alpha", deprecatedErr.CanonicalID)
	assert.Equal(t, statusDeprecated, deprecatedErr.Status)
}

func TestResolveDeprecatedAllowed(t *testing.T) {
	catalog := loadFixtureCatalog(t)

	resolved, err := catalog.Resolve("old-alpha", ResolveOptions{
		Surface:         SurfaceAgentOpenAI,
		AllowDeprecated: true,
	})
	require.NoError(t, err)
	assert.True(t, resolved.Deprecated)
	assert.Equal(t, "old-alpha", resolved.ConcreteModel)
}

func TestResolveMissingSurface(t *testing.T) {
	catalog := loadFixtureCatalog(t)

	_, err := catalog.Resolve("default", ResolveOptions{Surface: SurfaceGemini})
	require.Error(t, err)

	var missingSurfaceErr *MissingSurfaceError
	require.True(t, errors.As(err, &missingSurfaceErr))
	assert.Equal(t, SurfaceGemini, missingSurfaceErr.Surface)
}

func TestResolveUnknownReference(t *testing.T) {
	catalog := loadFixtureCatalog(t)

	_, err := catalog.Resolve("does-not-exist", ResolveOptions{Surface: SurfaceAgentOpenAI})
	require.Error(t, err)

	var unknownErr *UnknownReferenceError
	require.True(t, errors.As(err, &unknownErr))
	assert.Equal(t, "does-not-exist", unknownErr.Ref)
}

func TestLoad_ExternalOverride(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "models.yaml")
	require.NoError(t, os.WriteFile(manifestPath, []byte(`
version: 5
generated_at: 2026-04-09T00:00:00Z
catalog_version: 2026-04-10.1
policies:
  default:
    min_power: 9
    max_power: 10
models:
  gpt-4.1:
    family: gpt-4.1
    status: active
    provider_system: openai
    deployment_class: metered_cloud
    power: 9
    reasoning_default: medium
    cost_input_per_m: 1.00
    cost_output_per_m: 4.00
    surfaces:
      agent.openai: gpt-4.1
`), 0o644))

	catalog, err := Load(LoadOptions{ManifestPath: manifestPath})
	require.NoError(t, err)

	resolved, err := catalog.Resolve("default", ResolveOptions{Surface: SurfaceAgentOpenAI})
	require.NoError(t, err)
	assert.Equal(t, "default", resolved.CanonicalID)
	assert.Equal(t, "gpt-4.1", resolved.ConcreteModel)
	assert.Equal(t, reasoning.ReasoningMedium, resolved.SurfacePolicy.ReasoningDefault)
	assert.Equal(t, "2026-04-10.1", resolved.CatalogVersion)
	assert.Equal(t, manifestPath, resolved.ManifestSource)
	assert.Equal(t, 5, resolved.ManifestVersion)
}

func TestLoad_FallbackToEmbedded(t *testing.T) {
	catalog, err := Load(LoadOptions{
		ManifestPath: filepath.Join(t.TempDir(), "missing.yaml"),
	})
	require.NoError(t, err)

	resolved, err := catalog.Resolve("smart", ResolveOptions{
		Surface: SurfaceAgentAnthropic,
	})
	require.NoError(t, err)
	assert.Equal(t, "embedded", resolved.ManifestSource)
	assert.Equal(t, "smart", resolved.CanonicalID)
}

func TestLoad_RequireExternal(t *testing.T) {
	_, err := Load(LoadOptions{
		ManifestPath:    filepath.Join(t.TempDir(), "missing.yaml"),
		RequireExternal: true,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "read manifest")
}

func TestLoad_InvalidManifest(t *testing.T) {
	manifestPath := writeFixtureManifest(t, `
version: 4
generated_at: 2026-04-09T00:00:00Z
policies:
  default:
    min_power: 7
    max_power: 8
`)

	_, err := Load(LoadOptions{ManifestPath: manifestPath, RequireExternal: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "manifest schema v5 required")
}

func TestResolveEmptyReference(t *testing.T) {
	catalog := loadFixtureCatalog(t)

	_, err := catalog.Resolve("", ResolveOptions{Surface: SurfaceAgentOpenAI})
	require.Error(t, err)

	var unknownErr *UnknownReferenceError
	require.True(t, errors.As(err, &unknownErr))
}

func TestCurrentEmptyProfile(t *testing.T) {
	catalog := loadFixtureCatalog(t)

	_, err := catalog.Current("", ResolveOptions{Surface: SurfaceAgentOpenAI})
	require.Error(t, err)

	var unknownErr *UnknownReferenceError
	require.True(t, errors.As(err, &unknownErr))
}

func TestLookupModel_KnownModel(t *testing.T) {
	catalog := loadFixtureCatalog(t)

	entry, ok := catalog.LookupModel("alpha-10")
	require.True(t, ok)
	assert.Equal(t, "alpha", entry.Family)
	assert.Equal(t, "anthropic", entry.ProviderSystem)
	assert.Equal(t, 5.00, entry.CostInputPerM)
	assert.Equal(t, 15.00, entry.CostOutputPerM)
	assert.Equal(t, 80.0, entry.SWEBenchVerified)
}

func TestLookupModel_UnknownModel(t *testing.T) {
	catalog := loadFixtureCatalog(t)

	_, ok := catalog.LookupModel("does-not-exist")
	assert.False(t, ok)
}

func TestModelEligibility_AutoRoutableAndExactPinOnly(t *testing.T) {
	catalog, err := Load(LoadOptions{
		ManifestPath: writeFixtureManifest(t, `
version: 5
generated_at: 2026-04-30T00:00:00Z
catalog_version: 2026-04-30.1
policies:
  default:
    min_power: 7
    max_power: 8
models:
  routable-model:
    family: alpha
    status: active
    power: 7
    surfaces:
      agent.openai: provider/routable-model
  missing-power-model:
    family: alpha
    status: active
    surfaces:
      agent.openai: provider/missing-power-model
  exact-only-model:
    family: alpha
    status: active
    power: 8
    exact_pin_only: true
    surfaces:
      agent.openai: provider/exact-only-model
  stale-model:
    family: alpha
    status: stale
    power: 6
    surfaces:
      agent.openai: provider/stale-model
`),
		RequireExternal: true,
	})
	require.NoError(t, err)

	routable, ok := catalog.ModelEligibility("routable-model")
	require.True(t, ok)
	assert.Equal(t, 7, routable.Power)
	assert.False(t, routable.ExactPinOnly)
	assert.True(t, routable.AutoRoutable)

	bySurface, ok := catalog.ModelEligibility("provider/routable-model")
	require.True(t, ok)
	assert.Equal(t, routable, bySurface)

	byMixedCaseSurface, ok := catalog.ModelEligibility("Provider/Routable-Model")
	require.True(t, ok)
	assert.Equal(t, routable, byMixedCaseSurface)

	missingPower, ok := catalog.ModelEligibility("missing-power-model")
	require.True(t, ok)
	assert.Equal(t, 0, missingPower.Power)
	assert.False(t, missingPower.ExactPinOnly)
	assert.False(t, missingPower.AutoRoutable)

	exactOnly, ok := catalog.ModelEligibility("provider/exact-only-model")
	require.True(t, ok)
	assert.Equal(t, 8, exactOnly.Power)
	assert.True(t, exactOnly.ExactPinOnly)
	assert.False(t, exactOnly.AutoRoutable)

	stale, ok := catalog.ModelEligibility("stale-model")
	require.True(t, ok)
	assert.Equal(t, 6, stale.Power)
	assert.False(t, stale.ExactPinOnly)
	assert.False(t, stale.AutoRoutable)

	_, ok = catalog.ModelEligibility("does-not-exist")
	assert.False(t, ok)
}

func TestContextWindowForModel_KnownModel(t *testing.T) {
	catalog := loadFixtureCatalog(t)

	assert.Equal(t, 200000, catalog.ContextWindowForModel("beta-openai-8"))
}

func TestContextWindowForModel_UnknownModel(t *testing.T) {
	catalog := loadFixtureCatalog(t)

	assert.Equal(t, 0, catalog.ContextWindowForModel("does-not-exist"))
}

func TestContextWindowForModel_EmbeddedCatalogHasQwenWindow(t *testing.T) {
	catalog, err := Default()
	require.NoError(t, err)
	assert.Equal(t, 262144, catalog.ContextWindowForModel("qwen3.5-27b"))
}

func TestModelEligibility_EmbeddedCatalogMatchesProviderNativeQwenVariant(t *testing.T) {
	catalog, err := Default()
	require.NoError(t, err)

	eligibility, ok := catalog.ModelEligibility("Qwen3.6-27B-MLX-8bit")
	require.True(t, ok)
	assert.Equal(t, 5, eligibility.Power)
	assert.True(t, eligibility.AutoRoutable)
	assert.Equal(t, 262144, catalog.ContextWindowForModel("Qwen3.6-27B-MLX-8bit"))
}

func TestModelEligibility_EmbeddedCatalogMatchesClaudeFamilyDisplayVariants(t *testing.T) {
	catalog, err := Default()
	require.NoError(t, err)

	tests := []struct {
		name      string
		wantPower int
	}{
		{name: "Claude-Opus-4.6", wantPower: 10},
		{name: "claude-opus-4-6", wantPower: 10},
		{name: "Opus-4.6", wantPower: 10},
		{name: "Claude-4.6-opus", wantPower: 10},
		{name: "Claude-4.5-sonnet", wantPower: 8},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eligibility, ok := catalog.ModelEligibility(tt.name)
			require.True(t, ok)
			assert.Equal(t, tt.wantPower, eligibility.Power)
			assert.True(t, eligibility.AutoRoutable)
			assert.Equal(t, 1000000, catalog.ContextWindowForModel(tt.name))
		})
	}
}

func TestCandidatesFor_ModelReference(t *testing.T) {
	catalog := loadFixtureCatalog(t)

	candidates := catalog.CandidatesFor(SurfaceAgentAnthropic, "alpha-10")
	assert.Equal(t, []string{"alpha-anthropic-10"}, candidates)
}

func TestCandidatesFor_MissingReference(t *testing.T) {
	catalog := loadFixtureCatalog(t)

	candidates := catalog.CandidatesFor(SurfaceAgentAnthropic, "no-such-policy")
	assert.Nil(t, candidates)
}

func TestPricingForPreservesCacheFieldsFromManifest(t *testing.T) {
	manifestPath := writeFixtureManifest(t, `
version: 5
generated_at: 2026-04-25T00:00:00Z
catalog_version: 2026-04-25.1
policies:
  default:
    min_power: 7
    max_power: 8
models:
  cache-priced-model:
    family: cached
    status: active
    provider_system: anthropic
    power: 8
    cost_input_per_m: 3.00
    cost_output_per_m: 15.00
    cost_cache_read_per_m: 0.30
    cost_cache_write_per_m: 3.75
    surfaces:
      agent.anthropic: cache-priced-model
`)
	catalog, err := Load(LoadOptions{ManifestPath: manifestPath, RequireExternal: true})
	require.NoError(t, err)

	pricing := catalog.PricingFor()
	p, ok := pricing["cache-priced-model"]
	require.True(t, ok, "expected cache-priced-model in pricing")
	assert.Equal(t, 3.00, p.InputPerMTok)
	assert.Equal(t, 15.00, p.OutputPerMTok)
	assert.Equal(t, 0.30, p.CacheReadPerM, "PricingFor must preserve cost_cache_read_per_m")
	assert.Equal(t, 3.75, p.CacheWritePerM, "PricingFor must preserve cost_cache_write_per_m")
}

func TestUpdateManifestPricing_UpdatesModelEntries(t *testing.T) {
	oldFetch := fetchOpenRouterPricing
	t.Cleanup(func() { fetchOpenRouterPricing = oldFetch })
	fetchOpenRouterPricing = func(time.Duration) (map[string]openrouterModelEntry, error) {
		return map[string]openrouterModelEntry{
			"provider/alpha": {
				ID:            "provider/alpha",
				ContextLength: 123456,
				Pricing: openrouterPricing{
					Prompt:          "0.000002",
					Completion:      "0.000006",
					InputCacheRead:  "0.0000005",
					InputCacheWrite: "0.0000025",
				},
			},
		}, nil
	}

	manifestPath := writeFixtureManifest(t, `
version: 5
generated_at: 2026-04-13T00:00:00Z
policies:
  default:
    min_power: 7
    max_power: 8
models:
  alpha-model:
    family: alpha
    status: active
    openrouter_id: provider/alpha
    power: 8
    cost_input_per_m: 1
    cost_output_per_m: 2
    surfaces:
      agent.openai: alpha-model
  missing-model:
    family: alpha
    status: active
    openrouter_id: provider/missing
    power: 8
    surfaces:
      agent.openai: missing-model
`)

	updated, notFound, err := UpdateManifestPricing(manifestPath, time.Second)
	require.NoError(t, err)
	assert.Equal(t, 1, updated)
	assert.Equal(t, []string{"missing-model"}, notFound)

	catalog, err := Load(LoadOptions{ManifestPath: manifestPath, RequireExternal: true})
	require.NoError(t, err)
	model, ok := catalog.LookupModel("alpha-model")
	require.True(t, ok)
	assert.Equal(t, 2.0, model.CostInputPerM)
	assert.Equal(t, 6.0, model.CostOutputPerM)
	assert.Equal(t, 0.5, model.CostCacheReadPerM)
	assert.Equal(t, 2.5, model.CostCacheWritePerM)
	assert.Equal(t, 123456, model.ContextWindow)
}
