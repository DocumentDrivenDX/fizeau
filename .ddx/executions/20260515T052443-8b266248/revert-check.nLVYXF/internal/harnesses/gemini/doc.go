// Package gemini implements the CONTRACT-004 harness contracts for
// Google's gemini CLI: Harness, QuotaHarness, AccountHarness, and
// ModelDiscoveryHarness.
//
// Quota and account refresh cadences are independent — quota is a
// 15-minute window driven by the /model manage probe, account/auth is
// a 7-day window driven by ~/.gemini and environment evidence. The
// service-level refresh scheduler runs them on separate tickers.
//
// SupportedLimitIDs (emitted by Windows[].LimitID on QuotaStatus, one
// per tier surfaced by /model manage):
//
//   - "gemini-pro"
//   - "gemini-flash"
//   - "gemini-flash-lite"
//
// Tier names (Flash / Flash Lite / Pro) appear only in QuotaWindow.Name
// and via the LimitID; they MUST NOT be carried in QuotaStatus.Detail.
//
// SupportedAliases (recognized by ResolveModelAlias; concrete tier
// names like "gemini-2.5-flash" are not aliases — they pass through
// unchanged when requested directly):
//
//   - "gemini"
//   - "gemini-2"
//   - "gemini-2.5"
//
// These sets are the stable public contract for this harness. The
// programmatic source of truth is (*Runner).SupportedLimitIDs() and
// (*Runner).SupportedAliases(); this doc comment mirrors them for
// human readers and must be kept in sync.
package gemini
