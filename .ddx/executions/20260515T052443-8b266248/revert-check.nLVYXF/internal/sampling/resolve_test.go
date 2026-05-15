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
	res := Resolve(cat, "any-model", "code", nil)
	assert.Equal(t, Profile{}, res.Profile, "no layer set anything → all-nil profile")
	assert.Empty(t, res.Sources, "no fields → no source records")
	assert.True(t, res.MissingProfile, "catalog had no 'code' → MissingProfile should fire so CLI nudges")
}

func TestResolve_CatalogOnly(t *testing.T) {
	cat := stubCatalog{
		profiles: map[string]Profile{
			"code": {Temperature: ptrF64(0.6), TopP: ptrF64(0.95), TopK: ptrInt(20)},
		},
	}
	res := Resolve(cat, "any-model", "code", nil)

	require.NotNil(t, res.Profile.Temperature)
	assert.InDelta(t, 0.6, *res.Profile.Temperature, 1e-9)
	require.NotNil(t, res.Profile.TopP)
	assert.InDelta(t, 0.95, *res.Profile.TopP, 1e-9)
	require.NotNil(t, res.Profile.TopK)
	assert.Equal(t, 20, *res.Profile.TopK)
	assert.Nil(t, res.Profile.MinP, "min_p stays nil → wire-omit")
	assert.Nil(t, res.Profile.RepetitionPenalty)
	assert.Nil(t, res.Profile.Seed)
	assert.False(t, res.MissingProfile, "profile present → no nudge")

	expected := []FieldSource{
		{Field: "temperature", Source: SourceCatalog},
		{Field: "top_k", Source: SourceCatalog},
		{Field: "top_p", Source: SourceCatalog},
	}
	assert.Equal(t, expected, res.Sources)
	assert.Equal(t, "catalog", SourceSummary(res.Sources))
}

func TestResolve_ProviderOverrideOnly(t *testing.T) {
	cat := stubCatalog{}
	override := Profile{Temperature: ptrF64(0.0)} // explicit greedy
	res := Resolve(cat, "any-model", "code", &override)

	require.NotNil(t, res.Profile.Temperature)
	assert.InDelta(t, 0.0, *res.Profile.Temperature, 1e-9, "T=0 is a meaningful value, not unset")

	require.Len(t, res.Sources, 1)
	assert.Equal(t, FieldSource{Field: "temperature", Source: SourceProviderConfig}, res.Sources[0])
	assert.Equal(t, "provider_config", SourceSummary(res.Sources))
	assert.True(t, res.MissingProfile, "catalog still missing 'code' even when override fills fields")
}

func TestResolve_PerFieldStomping(t *testing.T) {
	cat := stubCatalog{
		profiles: map[string]Profile{
			"code": {Temperature: ptrF64(0.6), TopP: ptrF64(0.95), TopK: ptrInt(20)},
		},
	}
	override := Profile{Temperature: ptrF64(0.0)} // override only T
	res := Resolve(cat, "any-model", "code", &override)

	// T comes from override, top_p and top_k come from catalog
	require.NotNil(t, res.Profile.Temperature)
	assert.InDelta(t, 0.0, *res.Profile.Temperature, 1e-9)
	require.NotNil(t, res.Profile.TopP)
	assert.InDelta(t, 0.95, *res.Profile.TopP, 1e-9)
	require.NotNil(t, res.Profile.TopK)
	assert.Equal(t, 20, *res.Profile.TopK)

	want := []FieldSource{
		{Field: "temperature", Source: SourceProviderConfig},
		{Field: "top_k", Source: SourceCatalog},
		{Field: "top_p", Source: SourceCatalog},
	}
	assert.Equal(t, want, res.Sources)
	assert.Equal(t, "catalog,provider_config", SourceSummary(res.Sources))
	assert.False(t, res.MissingProfile)
}

func TestResolve_HarnessPinnedShortCircuits(t *testing.T) {
	cat := stubCatalog{
		profiles: map[string]Profile{
			"code": {Temperature: ptrF64(0.6), TopP: ptrF64(0.95)},
		},
		control: map[string]string{"wrapped-model": "harness_pinned"},
	}
	override := Profile{Seed: ptrI64(42)}
	res := Resolve(cat, "wrapped-model", "code", &override)

	assert.Equal(t, Profile{}, res.Profile, "harness_pinned forces zero-value profile")
	assert.Empty(t, res.Sources, "harness_pinned emits no source attribution")
	assert.False(t, res.MissingProfile, "harness_pinned is a deliberate skip, not a missing-data condition")
}

func TestResolve_UnknownProfileNameSetsMissingProfile(t *testing.T) {
	cat := stubCatalog{
		profiles: map[string]Profile{
			"code": {Temperature: ptrF64(0.6)},
		},
	}
	override := Profile{TopP: ptrF64(0.5)}
	res := Resolve(cat, "any-model", "no-such-profile", &override)

	// Catalog skip; only override fires.
	assert.Nil(t, res.Profile.Temperature)
	require.NotNil(t, res.Profile.TopP)
	assert.InDelta(t, 0.5, *res.Profile.TopP, 1e-9)

	require.Len(t, res.Sources, 1)
	assert.Equal(t, FieldSource{Field: "top_p", Source: SourceProviderConfig}, res.Sources[0])
	assert.True(t, res.MissingProfile, "named-but-undeclared profile → MissingProfile fires")
}

func TestResolve_EmptyProfileNameDoesNotFireMissing(t *testing.T) {
	cat := stubCatalog{}
	res := Resolve(cat, "any-model", "", nil)
	assert.False(t, res.MissingProfile, "no profile requested → no missing-profile nudge")
}

func TestResolve_NilCatalogToleratedForOverrideOnly(t *testing.T) {
	override := Profile{Temperature: ptrF64(0.7)}
	res := Resolve(nil, "", "code", &override)

	require.NotNil(t, res.Profile.Temperature)
	assert.InDelta(t, 0.7, *res.Profile.Temperature, 1e-9)
	require.Len(t, res.Sources, 1)
	assert.Equal(t, SourceProviderConfig, res.Sources[0].Source)
	assert.True(t, res.MissingProfile, "nil catalog with named profile → MissingProfile fires")
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

	res := Resolve(cat, "m", "code", nil)
	require.NotNil(t, res.Profile.Temperature)
	*src.Temperature = 99.0
	assert.InDelta(t, 0.6, *res.Profile.Temperature, 1e-9, "catalog mutation must not bleed into resolved profile")
}
