// Package pi implements the pi subprocess harness.
//
// CONTRACT-004: pi.Runner implements harnesses.Harness and
// harnesses.ModelDiscoveryHarness. It does not implement QuotaHarness or
// AccountHarness — pi has no provider-side subscription quota or
// account/auth probe to expose.
//
// SupportedAliases() returns an empty slice: pi requires exact provider/model
// identifiers (resolved via --list-models or --model) and recognizes no
// family alias. ResolveModelAlias rejects every family with
// harnesses.ErrAliasNotResolvable.
package pi
