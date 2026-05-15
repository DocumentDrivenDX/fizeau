// Package sampling owns the sampling-profile data type and the resolution
// chain that decides which sampler fields land on the wire for a given
// (model, profile, provider-override) tuple. See ADR-007.
//
// Slice 4 of the ADR-007 implementation will add Resolve(); this file
// currently carries only the data type so the model catalog and config
// packages can share it without a circular import.
package sampling

// Profile is a named bundle of sampling-parameter overrides. Each field is a
// pointer so nil means "leave-unset — fall through to lower layers, then to
// the server default" — distinct from any concrete value (notably 0). Stored
// on the model catalog (per-profile bundles), in user provider config (the
// providers.<name>.sampling override block), and as the resolved bundle the
// resolver returns to the CLI.
type Profile struct {
	Temperature       *float64 `yaml:"temperature,omitempty"`
	TopP              *float64 `yaml:"top_p,omitempty"`
	TopK              *int     `yaml:"top_k,omitempty"`
	MinP              *float64 `yaml:"min_p,omitempty"`
	RepetitionPenalty *float64 `yaml:"repetition_penalty,omitempty"`
	Seed              *int64   `yaml:"seed,omitempty"`
}
