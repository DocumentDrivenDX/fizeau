# Changelog

All notable changes to ddx-agent are recorded here.
Dates use the repo convention (`YYYY-MM-DD`); versions follow semver.

## [Unreleased]

## [v0.7.0] — 2026-04-20

### Fixed
- **Route HTTP provider-backed native harnesses by concrete provider type.**
  Service execution now resolves native harness/provider dispatch through the
  concrete provider identity instead of the configured provider name, so
  renamed `lmstudio`, `omlx`, `openrouter`, `ollama`, and `openai` providers
  route correctly after the v0.6.0 provider split. (`agent-117a0868`)

### Changed
- **Profile-owned placement policy replaced public provider preference
  routing.** Service callers use profiles such as `cheap`, `standard`,
  `smart`, or user-defined profiles as the public routing knob. Catalog
  `surface_policy` can carry placement order, cost ceilings, failure policy,
  and reasoning defaults; subscription quota health and burn trend still
  influence same-tier scoring internally. (`agent-117a0868`)

## [v0.6.0] — 2026-04-20

### Breaking
- **Removed runtime provider `flavor` behavior from the OpenAI-compatible
  provider.** Concrete provider packages now own provider identity,
  capabilities, discovery, limit lookup, and cost attribution. Direct
  `openai.Provider` construction defaults to OpenAI identity unless callers
  explicitly pass provider metadata.
- **Removed deprecated prompt preset aliases.** Harness-flavored names such as
  `agent`, `worker`, `cursor`, `claude`, and `codex` now fail clearly instead
  of warning and resolving to canonical presets.

### Added
- **Concrete provider identity split.** `openai`, `openrouter`, `lmstudio`,
  `omlx`, `ollama`, and `anthropic` are provider identities; shared
  OpenAI-compatible protocol plumbing lives below them in
  `internal/sdk/openaicompat`.
- **Provider preference routing.** Service and routing requests can express
  local-first, subscription-first, local-only, and subscription-only policy,
  with subscription quota health and burn trend influencing same-tier scoring.

### Changed
- Provider routing and tool contract docs were refreshed to reflect the
  concrete-provider model and bounded tool-output behavior.

## [v0.5.0] — 2026-04-19

### Breaking
- **Removed the legacy `agent.Run` API from the public module surface.**
  The former `Request`, `Result`, `Provider`, `StreamingProvider`, `Tool`,
  `Message`, `ToolDef`, `Event`, session-log DTO, compaction callback, pricing,
  and provider-conformance exports now live behind `internal/` for agent-owned
  code only. External consumers must use `agent.New(...).Execute(...)` and the
  DdxAgent service contract.
- **Removed public compatibility packages for the old provider/tool/session
  stack.** Provider implementations, compaction, prompt building, built-in
  tools, session replay/logging, and provider conformance helpers are no longer
  importable outside this module; Go `internal/` enforcement now blocks
  consumers that have not migrated.

### Changed
- The standalone `ddx-agent` binary continues to use the internal native loop,
  but that loop is no longer part of the exported Go API.

### Changed
- **Removed harness-flavored prompt preset names.** The old `agent`, `worker`,
  `cursor`, `claude`, and `codex` names now return clear errors. Use the
  canonical names (`default`, `smart`, `cheap`, `minimal`, `benchmark`)
  instead. (`agent-ff9c0289`)
- **Renamed the file-discovery tool from `glob` to `find`.** The built-in tool
  catalog now exposes only `find`; there is no `glob` compatibility alias.
  (`agent-1b00b3ea`)

## [v0.3.14] — 2026-04-18

### Fixed
- **Filter SSE comment frames before the `ssestream` decoder.**
  `openai-go`'s SSE decoder dispatches an event on any blank line —
  including the terminator of a comment-only frame like
  `: keep-alive\n\n`. `Stream.Next` then `json.Unmarshal`s empty bytes
  and surfaces `unexpected end of JSON input`, aborting the stream.
  `omlx` and other servers emit these comment frames during
  reasoning-model warmup. Per the WHATWG SSE spec, empty-data events
  must be silently ignored. Fix adds `sseCommentFilter` +
  `sseFilterMiddleware` to `provider/openai` that strips comment lines
  and suppresses the blank-line dispatch when the current frame has not
  yet seen a field line, so the decoder never observes an empty-event
  dispatch. Flavor-agnostic — applies to all openai-compat
  counterparties. Upstream removal triggers (`openai/openai-go` PRs
  #555 / #643, issues #556 / #618) are referenced in the filter source
  so the shim can be deleted once the SDK ships a fix.
  (`agent-f237e07b`)

### Added
- **`AGENT_DEBUG_WIRE_STREAM_FULL=1`** — opt-in env var that disables
  the 64 KB cumulative cap on `teeBody`, so the entire SSE stream is
  captured for post-mortem analysis. Default behavior unchanged.
  (`agent-f237e07b`, acceptance item 5)

### Tests
- `TestChatStream_SurvivesSSECommentFramesAndLongSilence` —
  regression test asserting that a frame sequence of (keep-alive
  comment, role delta, keep-alive comment, content delta, done)
  completes without error and delivers content.

## [v0.3.13] — 2026-04-18

### Fixed
- **Strip `thinking` field from non-Anthropic openai-compat requests.**
  Wire capture (via `AGENT_DEBUG_WIRE=1`) from DocumentDrivenDX/ddx
  `ddx-6a5dfe35` confirmed that `provider/openai/openai.go` was
  unconditionally injecting the non-standard `thinking` body field
  whenever a provider-level positive reasoning budget was configured,
  regardless of destination flavor.
  omlx silently terminates the SSE stream after the first delta when
  `thinking` is present — client-side OpenAI Go SDK then surfaces
  `unexpected end of JSON input`. Fix gates the field injection on
  a new `Provider.SupportsThinking()` capability flag, backed by a
  flavor-keyed table in `protocol_support.go`. Stripping is automatic
  for `omlx`, `openrouter`, `openai`, and `ollama`; `lmstudio`
  (original target of the field) is unchanged. (`agent-04639431`)

### Added
- **`Provider.SupportsThinking()`** — capability accessor matching the
  existing `SupportsTools` / `SupportsStream` /
  `SupportsStructuredOutput` pattern. Returns `false` for unknown
  flavors (conservative). Extends `protocolCapabilities` with a
  `Thinking bool` field. (`agent-04639431`)

### Specs evolved
- SD-005 (flavor-keyed protocol capability) — add `Thinking` to the
  capability matrix.

## [v0.3.12] — 2026-04-18

### Added
- **`AGENT_DEBUG_WIRE` env-var** — opt-in HTTP request/response dump at the
  openai-go transport boundary, for diagnosing integration defects at the
  `ddx-agent ↔ provider` boundary. Default off. Authorization Bearer tokens
  redacted. `AGENT_DEBUG_WIRE_FILE=<path>` routes JSONL output to a file.
  (`agent-941e7e42`)
- **`Provider.DetectedFlavor()`** — cached accessor that returns the effective
  server flavor (`lmstudio` / `omlx` / `openrouter` / `ollama`). Uses
  `Config.Flavor` when set, otherwise runs a one-time probe, otherwise falls
  back to the URL-heuristic `providerSystem`. (`agent-92f0f324`)
- **Protocol capability flags** — `Provider.SupportsTools()`,
  `SupportsStream()`, `SupportsStructuredOutput()`. Flavor-keyed; unknown
  flavors return `false` conservatively. Consumed by downstream routing (DDx
  `ddx-4817edfd`) to gate dispatch on what the provider+flavor can honor.
  (`agent-767549c7`)

### Notes
- `DetectedFlavor()` does **not** replace the existing `providerSystem` field
  used on the per-response telemetry hot path. See SD-005 D14 for the
  intentional layering.
- `SupportsTools` for omlx is set to `true` per vendor docs. If
  `ddx-6a5dfe35` produces wire evidence showing otherwise, the flavor table in
  `provider/openai/protocol_support.go` will be revised.

### Specs evolved
- FEAT-003 requirements 24–27 (protocol capability, debug observability)
- SD-005 decisions D13 (flavor-keyed protocol capability) and D14
  (`DetectedFlavor` vs `providerSystem` layering)

## [v0.3.11] — 2026-04-17

### Added
- **omlx provider support** — new flavor recognized by the OpenAI-compatible
  provider. Uses `GET /v1/models/status` for per-model context window and
  output token limits. `flavor: omlx` config field plus port-1235 URL
  heuristic.
- **`Flavor` config field** — explicit server-type hint on `ProviderConfig`
  (`lmstudio` / `omlx` / `openrouter` / `ollama`). Bypasses URL-based
  detection and probing when set.
- **Catalog `context_window` fallback** — `ModelEntry.ContextWindow` is now
  parsed from the v4 manifest; `Catalog.ContextWindowForModel(id)` exposes it
  for the CLI's three-tier limit cascade (explicit config → live API →
  catalog → package default). Fixes LM Studio servers that omit
  `context_length` from `/v1/models`.

### Specs evolved
- FEAT-003 requirements 14–23 + ACs 06–08
- SD-005 decisions D10 (live flavor-gated limit discovery), D11 (flavor
  replaces port heuristics), D12 (omlx as first-class provider)
- SD-006 CLI Context Window Resolution section
