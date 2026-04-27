package sampling

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubCatalog implements CatalogLookup for the table-driven tests.
type stubCatalog struct {
	profiles map[string]Profile
	control  map[string]string
}

func (s stubCatalog) SamplingProfile(name string) (Profile, bool) {
	p, ok := s.profiles[name]
	return p, ok
}

func (s stubCatalog) ModelSamplingControl(modelID string) string {
	return s.control[modelID]
}

func ptrF64(v float64) *float64 { return &v }
func ptrInt(v int) *int         { return &v }
func ptrI64(v int64) *int64     { return &v }

func TestResolve_EmptyAllLayersOmitsAllFields(t *testing.T) {
	cat := stubCatalog{}
	got, sources := Resolve(cat, "any-model", "code", nil)
	assert.Equal(t, Profile{}, got, "no layer set anything → all-nil profile")
	assert.Empty(t, sources, "no fields → no source records")
}

func TestResolve_CatalogOnly(t *testing.T) {
	cat := stubCatalog{
		profiles: map[string]Profile{
			"code": {Temperature: ptrF64(0.6), TopP: ptrF64(0.95), TopK: ptrInt(20)},
		},
	}
	got, sources := Resolve(cat, "any-model", "code", nil)

	require.NotNil(t, got.Temperature)
	assert.InDelta(t, 0.6, *got.Temperature, 1e-9)
	require.NotNil(t, got.TopP)
	assert.InDelta(t, 0.95, *got.TopP, 1e-9)
	require.NotNil(t, got.TopK)
	assert.Equal(t, 20, *got.TopK)
	assert.Nil(t, got.MinP, "min_p stays nil → wire-omit")
	assert.Nil(t, got.RepetitionPenalty)
	assert.Nil(t, got.Seed)

	expected := []FieldSource{
		{Field: "temperature", Source: SourceCatalog},
		{Field: "top_k", Source: SourceCatalog},
		{Field: "top_p", Source: SourceCatalog},
	}
	assert.Equal(t, expected, sources)
	assert.Equal(t, "catalog", SourceSummary(sources))
}

func TestResolve_ProviderOverrideOnly(t *testing.T) {
	cat := stubCatalog{}
	override := Profile{Temperature: ptrF64(0.0)} // explicit greedy
	got, sources := Resolve(cat, "any-model", "code", &override)

	require.NotNil(t, got.Temperature)
	assert.InDelta(t, 0.0, *got.Temperature, 1e-9, "T=0 is a meaningful value, not unset")

	require.Len(t, sources, 1)
	assert.Equal(t, FieldSource{Field: "temperature", Source: SourceProviderConfig}, sources[0])
	assert.Equal(t, "provider_config", SourceSummary(sources))
}

func TestResolve_PerFieldStomping(t *testing.T) {
	cat := stubCatalog{
		profiles: map[string]Profile{
			"code": {Temperature: ptrF64(0.6), TopP: ptrF64(0.95), TopK: ptrInt(20)},
		},
	}
	override := Profile{Temperature: ptrF64(0.0)} // override only T
	got, sources := Resolve(cat, "any-model", "code", &override)

	// T comes from override, top_p and top_k come from catalog
	require.NotNil(t, got.Temperature)
	assert.InDelta(t, 0.0, *got.Temperature, 1e-9)
	require.NotNil(t, got.TopP)
	assert.InDelta(t, 0.95, *got.TopP, 1e-9)
	require.NotNil(t, got.TopK)
	assert.Equal(t, 20, *got.TopK)

	want := []FieldSource{
		{Field: "temperature", Source: SourceProviderConfig},
		{Field: "top_k", Source: SourceCatalog},
		{Field: "top_p", Source: SourceCatalog},
	}
	assert.Equal(t, want, sources)
	assert.Equal(t, "catalog,provider_config", SourceSummary(sources))
}

func TestResolve_HarnessPinnedShortCircuits(t *testing.T) {
	cat := stubCatalog{
		profiles: map[string]Profile{
			"code": {Temperature: ptrF64(0.6), TopP: ptrF64(0.95)},
		},
		control: map[string]string{"wrapped-model": "harness_pinned"},
	}
	override := Profile{Seed: ptrI64(42)}
	got, sources := Resolve(cat, "wrapped-model", "code", &override)

	assert.Equal(t, Profile{}, got, "harness_pinned forces zero-value profile")
	assert.Empty(t, sources, "harness_pinned emits no source attribution")
}

func TestResolve_UnknownProfileNameSilentSkip(t *testing.T) {
	cat := stubCatalog{
		profiles: map[string]Profile{
			"code": {Temperature: ptrF64(0.6)},
		},
	}
	override := Profile{TopP: ptrF64(0.5)}
	got, sources := Resolve(cat, "any-model", "no-such-profile", &override)

	// Catalog skip; only override fires.
	assert.Nil(t, got.Temperature)
	require.NotNil(t, got.TopP)
	assert.InDelta(t, 0.5, *got.TopP, 1e-9)

	require.Len(t, sources, 1)
	assert.Equal(t, FieldSource{Field: "top_p", Source: SourceProviderConfig}, sources[0])
}

func TestResolve_NilCatalogToleratedForOverrideOnly(t *testing.T) {
	override := Profile{Temperature: ptrF64(0.7)}
	got, sources := Resolve(nil, "", "code", &override)

	require.NotNil(t, got.Temperature)
	assert.InDelta(t, 0.7, *got.Temperature, 1e-9)
	require.Len(t, sources, 1)
	assert.Equal(t, SourceProviderConfig, sources[0].Source)
}

func TestSourceSummary_OrderingAndDedup(t *testing.T) {
	in := []FieldSource{
		{Field: "top_p", Source: SourceProviderConfig},
		{Field: "temperature", Source: SourceCatalog},
		{Field: "min_p", Source: SourceCatalog}, // duplicate catalog
	}
	assert.Equal(t, "catalog,provider_config", SourceSummary(in))

	assert.Equal(t, "", SourceSummary(nil))
	assert.Equal(t, "", SourceSummary([]FieldSource{}))
}

func TestMergeFrom_DoesNotShareUnderlyingPointer(t *testing.T) {
	// Regression guard: a mutation on the catalog profile after Resolve
	// must not mutate the resolved profile.
	src := Profile{Temperature: ptrF64(0.6)}
	cat := stubCatalog{profiles: map[string]Profile{"code": src}}

	got, _ := Resolve(cat, "m", "code", nil)
	require.NotNil(t, got.Temperature)
	*src.Temperature = 99.0
	assert.InDelta(t, 0.6, *got.Temperature, 1e-9, "catalog mutation must not bleed into resolved profile")
}
