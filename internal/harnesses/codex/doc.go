// Package codex implements the CONTRACT-004 harness contracts for
// OpenAI's codex CLI: Harness, QuotaHarness, AccountHarness, and
// ModelDiscoveryHarness.
//
// SupportedLimitIDs (emitted by Windows[].LimitID on QuotaStatus):
//
//   - "codex"         — primary (5h) rolling quota window
//   - "codex-weekly"  — weekly rolling quota window
//
// SupportedAliases (recognized by ResolveModelAlias):
//
//   - "gpt"    — resolves to the highest-version concrete gpt-* model
//     in the discovery snapshot
//   - "gpt-5"  — resolves to the highest concrete gpt-5.x model in the
//     discovery snapshot
//
// These sets are the stable public contract for this harness. The
// programmatic source of truth is (*Runner).SupportedLimitIDs() and
// (*Runner).SupportedAliases(); this doc comment mirrors them for
// human readers and must be kept in sync.
package codex
