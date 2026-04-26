package modelcatalog

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestManifestPreservesReasoningCapabilityFields(t *testing.T) {
	src := `
version: 4
generated_at: 2026-04-26T00:00:00Z
models:
  tunable-model:
    family: example
    status: active
    reasoning_levels: [off, low, medium, high, max]
    reasoning_control: tunable
    reasoning_wire: provider
    surfaces:
      agent.anthropic: tunable-model
  fixed-variant:
    family: example
    status: active
    reasoning_levels: [high]
    reasoning_control: fixed
    reasoning_wire: model_id
    surfaces:
      agent.openai: fixed-variant
  no-reasoning:
    family: example
    status: active
    reasoning_levels: [off]
    reasoning_control: none
    reasoning_wire: none
    surfaces:
      gemini: no-reasoning
profiles:
  default:
    target: example-tier
targets:
  example-tier:
    family: example
    aliases: [ex]
    candidates: [tunable-model, fixed-variant, no-reasoning]
    surfaces:
      agent.anthropic: tunable-model
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

func TestManifestRejectsInvalidReasoningControl(t *testing.T) {
	src := `
version: 4
generated_at: 2026-04-26T00:00:00Z
models:
  bad-control:
    family: example
    status: active
    reasoning_control: bogus
    surfaces:
      agent.anthropic: bad-control
profiles:
  default:
    target: example-tier
targets:
  example-tier:
    family: example
    aliases: [ex]
    candidates: [bad-control]
    surfaces:
      agent.anthropic: bad-control
`
	path := writeFixtureManifest(t, src)
	_, err := Load(LoadOptions{ManifestPath: path, RequireExternal: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reasoning_control")
}

func TestManifestRejectsInvalidReasoningWire(t *testing.T) {
	src := `
version: 4
generated_at: 2026-04-26T00:00:00Z
models:
  bad-wire:
    family: example
    status: active
    reasoning_wire: bogus
    surfaces:
      agent.anthropic: bad-wire
profiles:
  default:
    target: example-tier
targets:
  example-tier:
    family: example
    aliases: [ex]
    candidates: [bad-wire]
    surfaces:
      agent.anthropic: bad-wire
`
	path := writeFixtureManifest(t, src)
	_, err := Load(LoadOptions{ManifestPath: path, RequireExternal: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reasoning_wire")
}
