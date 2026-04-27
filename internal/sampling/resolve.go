package sampling

import (
	"sort"
	"strings"
)

// Source identifies which layer of the resolution chain supplied a non-nil
// field on the resolved profile. See ADR-007 §2.
type Source string

const (
	// SourceCatalog is the L1 layer: a manifest sampling_profiles entry.
	SourceCatalog Source = "catalog"
	// SourceProviderConfig is the L2 layer: a providers.<name>.sampling
	// override block in user config.
	SourceProviderConfig Source = "provider_config"
	// SourceCLI is the L3 layer: an explicit per-request override (CLI
	// flags). Reserved; not emitted by the v1 resolver.
	SourceCLI Source = "cli"
)

// FieldSource records the layer that supplied a single non-nil sampler
// field on the resolved profile.
type FieldSource struct {
	Field  string
	Source Source
}

// CatalogLookup is the narrow interface the resolver needs from the model
// catalog. It exists so test code can supply a stub without depending on
// the full Catalog type.
type CatalogLookup interface {
	// SamplingProfile returns the named profile bundle, or false if the
	// catalog does not declare it.
	SamplingProfile(name string) (Profile, bool)
	// ModelSamplingControl returns the SamplingControl declared on the
	// ModelEntry for the given concrete model ID, or "" if the model is
	// not declared in the catalog or carries no explicit value.
	ModelSamplingControl(modelID string) string
}

// ResolveResult is the full output of Resolve: the merged Profile, the
// per-field origin record, and a flag the caller uses to drive the
// catalog-stale nudge per ADR-007 §7 rule 4.
type ResolveResult struct {
	// Profile is the per-field-merged sampling bundle to send on the wire.
	Profile Profile
	// Sources records which layer supplied each non-nil field.
	Sources []FieldSource
	// MissingProfile is true when the caller asked for a named catalog
	// profile (profileName != "") but the catalog did not declare it. The
	// CLI uses this to emit a single first-use nudge pointing at
	// "ddx-agent catalog update". MissingProfile is false when
	// profileName is empty (no L1 lookup was attempted) or when the
	// resolver short-circuited via harness_pinned (deliberate skip, not
	// a missing-data condition).
	MissingProfile bool
}

// Resolve walks the precedence chain (catalog profile → provider override)
// and produces a per-field-merged Profile plus a record of which layer
// supplied each non-nil field. nil at every layer means the wire field is
// omitted and the server's own default applies — a first-class outcome per
// ADR-007 §2.
//
// modelID may be empty if no model has been resolved yet; in that case the
// catalog's sampling_control is unknown and treated as the default
// (client_settable). When sampling_control is "harness_pinned" the resolver
// short-circuits to a zero-value profile regardless of layer inputs:
// wrapped subprocess harnesses pin samplers internally and ignore wire
// fields, so emitting them would only mislead telemetry.
//
// profileName is the name of the catalog profile to seed L1 from (e.g.,
// "code"). Empty names skip L1 silently. Unknown names (caller asked for a
// profile the catalog does not declare) set ResolveResult.MissingProfile
// so the CLI can emit an ADR-007 §7 catalog-stale nudge.
//
// providerOverride is the user-config L2 override; nil means none.
func Resolve(catalog CatalogLookup, modelID string, profileName string, providerOverride *Profile) ResolveResult {
	// Harness-pinned short-circuit: catalog says wire-side fields are
	// ignored for this model. Per ADR-007 §4 the resolver returns the
	// zero-value profile and emits no source attribution; the caller will
	// also typically skip the resolver entirely for wrapped harnesses,
	// but we preserve correctness even if it does not.
	if catalog != nil && modelID != "" {
		if catalog.ModelSamplingControl(modelID) == "harness_pinned" {
			return ResolveResult{}
		}
	}

	resolved := Profile{}
	sources := map[string]Source{}
	missingProfile := false

	// L1 — catalog profile.
	if profileName != "" {
		var found bool
		if catalog != nil {
			if cat, ok := catalog.SamplingProfile(profileName); ok {
				mergeFrom(&resolved, cat, sources, SourceCatalog)
				found = true
			}
		}
		if !found {
			missingProfile = true
		}
	}

	// L2 — provider override.
	if providerOverride != nil {
		mergeFrom(&resolved, *providerOverride, sources, SourceProviderConfig)
	}

	return ResolveResult{
		Profile:        resolved,
		Sources:        sortedFieldSources(sources),
		MissingProfile: missingProfile,
	}
}

// mergeFrom copies any non-nil field from src into dst, recording the
// supplying source for each copied field. Per-field merge — a non-nil src
// field stomps the dst regardless of whether dst already had a value.
func mergeFrom(dst *Profile, src Profile, sources map[string]Source, layer Source) {
	if src.Temperature != nil {
		v := *src.Temperature
		dst.Temperature = &v
		sources["temperature"] = layer
	}
	if src.TopP != nil {
		v := *src.TopP
		dst.TopP = &v
		sources["top_p"] = layer
	}
	if src.TopK != nil {
		v := *src.TopK
		dst.TopK = &v
		sources["top_k"] = layer
	}
	if src.MinP != nil {
		v := *src.MinP
		dst.MinP = &v
		sources["min_p"] = layer
	}
	if src.RepetitionPenalty != nil {
		v := *src.RepetitionPenalty
		dst.RepetitionPenalty = &v
		sources["repetition_penalty"] = layer
	}
	if src.Seed != nil {
		v := *src.Seed
		dst.Seed = &v
		sources["seed"] = layer
	}
}

func sortedFieldSources(m map[string]Source) []FieldSource {
	if len(m) == 0 {
		return nil
	}
	out := make([]FieldSource, 0, len(m))
	for f, s := range m {
		out = append(out, FieldSource{Field: f, Source: s})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Field < out[j].Field })
	return out
}

// SourceSummary collapses a FieldSource list into the comma-separated layer
// list that LLMRequestData.SamplingSource records. Layers appear in the
// chain order (catalog before provider_config before cli); a layer is
// included once if it supplied any non-nil field. Empty input → "".
func SourceSummary(sources []FieldSource) string {
	seen := map[Source]bool{}
	for _, s := range sources {
		seen[s.Source] = true
	}
	var parts []string
	for _, layer := range []Source{SourceCatalog, SourceProviderConfig, SourceCLI} {
		if seen[layer] {
			parts = append(parts, string(layer))
		}
	}
	return strings.Join(parts, ",")
}
