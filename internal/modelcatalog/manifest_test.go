package modelcatalog

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestManifestPreservesReasoningCapabilityFields(t *testing.T) {
	src := `
version: 5
generated_at: 2026-04-26T00:00:00Z
policies:
  default:
    min_power: 7
    max_power: 8
models:
  tunable-model:
    family: example
    status: active
    power: 8
    reasoning_default: high
    reasoning_levels: [off, low, medium, high, max]
    reasoning_control: tunable
    reasoning_wire: provider
    surfaces:
      agent.anthropic: tunable-model
  fixed-variant:
    family: example
    status: active
    power: 8
    reasoning_levels: [high]
    reasoning_control: fixed
    reasoning_wire: model_id
    surfaces:
      agent.openai: fixed-variant
  no-reasoning:
    family: example
    status: active
    power: 8
    reasoning_default: off
    reasoning_levels: [off]
    reasoning_control: none
    reasoning_wire: none
    surfaces:
      gemini: no-reasoning
`

	path := writeFixtureManifest(t, src)
	catalog, err := Load(LoadOptions{ManifestPath: path, RequireExternal: true})
	require.NoError(t, err)

	models := catalog.AllModels()

	tunable, ok := models["tunable-model"]
	require.True(t, ok, "tunable-model present")
	assert.Equal(t, []string{"off", "low", "medium", "high", "max"}, tunable.ReasoningLevels)
	assert.Equal(t, ReasoningControlTunable, tunable.ReasoningControl)
	assert.Equal(t, ReasoningWireProvider, tunable.ReasoningWire)

	fixed, ok := models["fixed-variant"]
	require.True(t, ok)
	assert.Equal(t, []string{"high"}, fixed.ReasoningLevels)
	assert.Equal(t, ReasoningControlFixed, fixed.ReasoningControl)
	assert.Equal(t, ReasoningWireModelID, fixed.ReasoningWire)

	none, ok := models["no-reasoning"]
	require.True(t, ok)
	assert.Equal(t, []string{"off"}, none.ReasoningLevels)
	assert.Equal(t, ReasoningControlNone, none.ReasoningControl)
	assert.Equal(t, ReasoningWireNone, none.ReasoningWire)

	out, err := yaml.Marshal(map[string]ModelEntry{
		"tunable-model": tunable,
		"fixed-variant": fixed,
		"no-reasoning":  none,
	})
	require.NoError(t, err)

	var roundTrip map[string]ModelEntry
	require.NoError(t, yaml.Unmarshal(out, &roundTrip))
	assert.Equal(t, tunable.ReasoningLevels, roundTrip["tunable-model"].ReasoningLevels)
	assert.Equal(t, tunable.ReasoningControl, roundTrip["tunable-model"].ReasoningControl)
	assert.Equal(t, tunable.ReasoningWire, roundTrip["tunable-model"].ReasoningWire)
	assert.Equal(t, fixed.ReasoningControl, roundTrip["fixed-variant"].ReasoningControl)
	assert.Equal(t, fixed.ReasoningWire, roundTrip["fixed-variant"].ReasoningWire)
	assert.Equal(t, none.ReasoningControl, roundTrip["no-reasoning"].ReasoningControl)
	assert.Equal(t, none.ReasoningWire, roundTrip["no-reasoning"].ReasoningWire)
}

func TestManifestPreservesPowerEligibilityFields(t *testing.T) {
	src := `
version: 5
generated_at: 2026-04-30T00:00:00Z
policies:
  default:
    min_power: 7
    max_power: 8
models:
  route-model:
    family: example
    status: active
    power: 6
    exact_pin_only: true
    surfaces:
      agent.openai: provider/route-model
`

	path := writeFixtureManifest(t, src)
	catalog, err := Load(LoadOptions{ManifestPath: path, RequireExternal: true})
	require.NoError(t, err)

	models := catalog.AllModels()
	entry, ok := models["route-model"]
	require.True(t, ok)
	assert.Equal(t, 6, entry.Power)
	assert.True(t, entry.ExactPinOnly)

	out, err := yaml.Marshal(map[string]ModelEntry{"route-model": entry})
	require.NoError(t, err)

	var roundTrip map[string]ModelEntry
	require.NoError(t, yaml.Unmarshal(out, &roundTrip))
	assert.Equal(t, 6, roundTrip["route-model"].Power)
	assert.True(t, roundTrip["route-model"].ExactPinOnly)
}

func TestRejectsLegacySchemaV4(t *testing.T) {
	path := writeFixtureManifest(t, `
version: 4
generated_at: 2026-04-30T00:00:00Z
policies:
  default:
    min_power: 7
    max_power: 8
`)

	_, err := Load(LoadOptions{ManifestPath: path, RequireExternal: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "manifest schema v5 required")
}

func TestRejectsCatalogWithoutDefaultPolicy(t *testing.T) {
	path := writeFixtureManifest(t, `
version: 5
generated_at: 2026-04-30T00:00:00Z
policies:
  smart:
    min_power: 9
    max_power: 10
`)

	_, err := Load(LoadOptions{ManifestPath: path, RequireExternal: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "default policy")
}

func TestRejectsUnknownRequireInvariant(t *testing.T) {
	path := writeFixtureManifest(t, `
version: 5
generated_at: 2026-04-30T00:00:00Z
policies:
  default:
    min_power: 5
    max_power: 5
    require: [no_remote, no_network]
`)

	_, err := Load(LoadOptions{ManifestPath: path, RequireExternal: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown require invariant")
}

func TestUserProviderUnknownTypeRequiresBillingField(t *testing.T) {
	path := writeFixtureManifest(t, `
version: 5
generated_at: 2026-04-30T00:00:00Z
policies:
  default:
    min_power: 7
    max_power: 8
providers:
  custom:
    type: privately-hosted
`)

	_, err := Load(LoadOptions{ManifestPath: path, RequireExternal: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "requires explicit billing field")
}

func TestUserProviderUnknownTypeAllowsExplicitBillingField(t *testing.T) {
	path := writeFixtureManifest(t, `
version: 5
generated_at: 2026-04-30T00:00:00Z
policies:
  default:
    min_power: 7
    max_power: 8
providers:
  custom:
    type: privately-hosted
    billing: fixed
`)

	_, err := Load(LoadOptions{ManifestPath: path, RequireExternal: true})
	require.NoError(t, err)
}

func TestManifestRejectsInvalidPolicyBounds(t *testing.T) {
	tests := []struct {
		name    string
		policy  string
		wantErr string
	}{
		{name: "zero min", policy: "min_power: 0\n    max_power: 8", wantErr: "min_power"},
		{name: "inverted", policy: "min_power: 9\n    max_power: 8", wantErr: "<= max_power"},
		{name: "above scale", policy: "min_power: 8\n    max_power: 11", wantErr: "<= 10"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeFixtureManifest(t, `
version: 5
generated_at: 2026-04-30T00:00:00Z
policies:
  default:
    `+tt.policy+`
`)
			_, err := Load(LoadOptions{ManifestPath: path, RequireExternal: true})
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestManifestRejectsNegativePower(t *testing.T) {
	path := writeFixtureManifest(t, `
version: 5
generated_at: 2026-04-30T00:00:00Z
policies:
  default:
    min_power: 7
    max_power: 8
models:
  bad-power:
    family: example
    status: active
    power: -1
    surfaces:
      agent.openai: bad-power
`)
	_, err := Load(LoadOptions{ManifestPath: path, RequireExternal: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "power")
}

func TestManifestRejectsPowerAboveScale(t *testing.T) {
	path := writeFixtureManifest(t, `
version: 5
generated_at: 2026-04-30T00:00:00Z
policies:
  default:
    min_power: 7
    max_power: 8
models:
  bad-power:
    family: example
    status: active
    power: 11
    surfaces:
      agent.openai: bad-power
`)
	_, err := Load(LoadOptions{ManifestPath: path, RequireExternal: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "power")
}

func TestManifestRejectsInvalidReasoningDefault(t *testing.T) {
	path := writeFixtureManifest(t, `
version: 5
generated_at: 2026-04-30T00:00:00Z
policies:
  default:
    min_power: 7
    max_power: 8
models:
  bad-default:
    family: example
    status: active
    power: 8
    reasoning_default: sideways
    surfaces:
      agent.openai: bad-default
`)
	_, err := Load(LoadOptions{ManifestPath: path, RequireExternal: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported value")
}

func TestManifestRejectsInvalidReasoningControl(t *testing.T) {
	path := writeFixtureManifest(t, `
version: 5
generated_at: 2026-04-26T00:00:00Z
policies:
  default:
    min_power: 7
    max_power: 8
models:
  bad-control:
    family: example
    status: active
    power: 8
    reasoning_control: bogus
    surfaces:
      agent.anthropic: bad-control
`)
	_, err := Load(LoadOptions{ManifestPath: path, RequireExternal: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reasoning_control")
}

func TestManifestRejectsInvalidReasoningWire(t *testing.T) {
	path := writeFixtureManifest(t, `
version: 5
generated_at: 2026-04-26T00:00:00Z
policies:
  default:
    min_power: 7
    max_power: 8
models:
  bad-wire:
    family: example
    status: active
    power: 8
    reasoning_wire: bogus
    surfaces:
      agent.anthropic: bad-wire
`)
	_, err := Load(LoadOptions{ManifestPath: path, RequireExternal: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reasoning_wire")
}

func TestManifestPreservesSamplingProfilesAndControl(t *testing.T) {
	src := `
version: 5
generated_at: 2026-04-27T00:00:00Z
sampling_profiles:
  code:
    temperature: 0.6
    top_p: 0.95
    top_k: 20
policies:
  default:
    min_power: 7
    max_power: 8
models:
  client-settable-default:
    family: example
    status: active
    power: 8
    surfaces:
      agent.anthropic: client-settable-default
  pinned-by-harness:
    family: example
    status: active
    power: 8
    sampling_control: harness_pinned
    surfaces:
      claude-code: pinned-by-harness
`
	path := writeFixtureManifest(t, src)
	catalog, err := Load(LoadOptions{ManifestPath: path, RequireExternal: true})
	require.NoError(t, err)

	codeProfile, ok := catalog.SamplingProfile("code")
	require.True(t, ok, "code profile present")
	require.NotNil(t, codeProfile.Temperature)
	assert.InDelta(t, 0.6, *codeProfile.Temperature, 1e-9)
	require.NotNil(t, codeProfile.TopP)
	assert.InDelta(t, 0.95, *codeProfile.TopP, 1e-9)
	require.NotNil(t, codeProfile.TopK)
	assert.Equal(t, 20, *codeProfile.TopK)
	assert.Nil(t, codeProfile.MinP, "min_p unset is nil, distinct from 0")
	assert.Nil(t, codeProfile.RepetitionPenalty, "rep_penalty unset is nil")

	_, ok = catalog.SamplingProfile("nonexistent")
	assert.False(t, ok)

	models := catalog.AllModels()
	defaulted := models["client-settable-default"]
	assert.Equal(t, "", defaulted.SamplingControl, "field unset on YAML preserves empty string default")

	pinned := models["pinned-by-harness"]
	assert.Equal(t, SamplingControlHarnessPinned, pinned.SamplingControl)
}

func TestManifestAcceptsReasoningWireEffortAndTokens(t *testing.T) {
	src := `
version: 5
generated_at: 2026-05-11T00:00:00Z
policies:
  default:
    min_power: 7
    max_power: 8
models:
  effort-model:
    family: example
    status: active
    power: 8
    reasoning_wire: effort
    surfaces:
      agent.openai: effort-model
  tokens-model:
    family: example
    status: active
    power: 8
    reasoning_wire: tokens
    surfaces:
      agent.openai: tokens-model
`

	path := writeFixtureManifest(t, src)
	catalog, err := Load(LoadOptions{ManifestPath: path, RequireExternal: true})
	require.NoError(t, err)

	models := catalog.AllModels()

	effortModel, ok := models["effort-model"]
	require.True(t, ok, "effort-model present")
	assert.Equal(t, ReasoningWireEffort, effortModel.ReasoningWire)

	tokensModel, ok := models["tokens-model"]
	require.True(t, ok, "tokens-model present")
	assert.Equal(t, ReasoningWireTokens, tokensModel.ReasoningWire)

	out, err := yaml.Marshal(map[string]ModelEntry{
		"effort-model": effortModel,
		"tokens-model": tokensModel,
	})
	require.NoError(t, err)

	var roundTrip map[string]ModelEntry
	require.NoError(t, yaml.Unmarshal(out, &roundTrip))
	assert.Equal(t, ReasoningWireEffort, roundTrip["effort-model"].ReasoningWire)
	assert.Equal(t, ReasoningWireTokens, roundTrip["tokens-model"].ReasoningWire)
}

func TestManifestRejectsInvalidSamplingControl(t *testing.T) {
	path := writeFixtureManifest(t, `
version: 5
generated_at: 2026-04-27T00:00:00Z
policies:
  default:
    min_power: 7
    max_power: 8
models:
  bad-control:
    family: example
    status: active
    power: 8
    sampling_control: nonsense
    surfaces:
      agent.anthropic: bad-control
`)
	_, err := Load(LoadOptions{ManifestPath: path, RequireExternal: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sampling_control")
}
