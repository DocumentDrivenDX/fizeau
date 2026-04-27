package modelcatalog

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEmbeddedManifestSeedsCodeSamplingProfile pins ADR-007 v1 contract:
// the embedded models.yaml must declare a "code" sampling profile with
// the Qwen3.6-27B "Best Practices: Thinking Mode — Precise Coding"
// values. If this test breaks, the seeded values were edited — verify
// against the Qwen3.6-27B HF model card before changing.
func TestEmbeddedManifestSeedsCodeSamplingProfile(t *testing.T) {
	catalog, err := Load(LoadOptions{})
	require.NoError(t, err)

	p, ok := catalog.SamplingProfile("code")
	require.True(t, ok, "embedded manifest must declare sampling_profiles.code")

	require.NotNil(t, p.Temperature, "code profile must set temperature")
	assert.InDelta(t, 0.6, *p.Temperature, 1e-9)

	require.NotNil(t, p.TopP, "code profile must set top_p")
	assert.InDelta(t, 0.95, *p.TopP, 1e-9)

	require.NotNil(t, p.TopK, "code profile must set top_k")
	assert.Equal(t, 20, *p.TopK)

	// presence_penalty (not currently plumbed) and repetition_penalty are
	// at their no-op defaults for this profile per Qwen3.6 model card —
	// the catalog leaves them nil so the wire omits them.
	assert.Nil(t, p.MinP)
	assert.Nil(t, p.RepetitionPenalty)
	assert.Nil(t, p.Seed)
}
