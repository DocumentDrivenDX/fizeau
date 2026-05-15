// Package opencode implements the opencode subprocess harness.
//
// CONTRACT-004: opencode.Runner implements harnesses.Harness and
// harnesses.ModelDiscoveryHarness. It does not implement QuotaHarness or
// AccountHarness — opencode has no provider-side subscription quota or
// account/auth probe to expose.
//
// SupportedAliases() returns an empty slice: opencode requires exact
// provider/model identifiers (e.g. "opencode/gpt-5.4") and recognizes no
// family alias such as "claude" or "gpt". Callers must pin a concrete
// model; ResolveModelAlias rejects every family with
// harnesses.ErrAliasNotResolvable.
package opencode
