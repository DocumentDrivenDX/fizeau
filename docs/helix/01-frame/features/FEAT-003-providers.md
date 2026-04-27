---
ddx:
  id: FEAT-003
  depends_on:
    - helix.prd
---
# Feature Specification: FEAT-003 — LLM Providers

**Feature ID**: FEAT-003
**Status**: Draft
**Priority**: P0
**Owner**: DDX Agent Team

## Overview

DDX Agent supports multiple LLM backends through a common interface, with two
built-in groups: a set of concrete OpenAI-compatible providers (LM Studio,
omlx, Ollama, OpenAI, OpenRouter) and an Anthropic provider (Claude). This
implements PRD P0 requirements 3-4.

Provider identity names the **actual model source** (e.g. `lmstudio`, `openrouter`,
`ollama`, `anthropic`). The OpenAI-compatible HTTP/SSE protocol is a shared
API shape; it is not a provider identity. OpenAI-compatible request shaping,
SSE decoding, tool-call conversion, and wire debug support live in a shared
`internal/sdk/openaicompat` layer that has no provider identity. Concrete
provider packages wrap this shared layer and own auth defaults, endpoint
defaults, model discovery, limit discovery, cost attribution, and local
endpoint handling.

**Provider identity is not `openai-compat`.** Telemetry, cost, auth, model
listing, quota, local endpoint management, and provider-specific bugs all
attach to the real source. The OpenAI-compatible shape is an API protocol
concern and must not be used as the routing key or analytics label.

## Problem Statement

- **Current situation**: DDx harnesses each implement provider-specific CLI
  invocation. Adding a new provider means modifying the registry and writing
  invocation glue.
- **Pain points**: No unified Go API for calling different providers. LM Studio
  and Ollama both speak OpenAI-compatible API but DDx treats them as separate
  harnesses.
- **Desired outcome**: A `Provider` interface in Go with concrete implementations
  that each wrap the shared OpenAI-compatible SDK. Configure by type — same SDK
  talks to LM Studio, Ollama, and OpenAI through their respective wrappers.

## Requirements

### Functional Requirements

#### Provider Interface

1. Common interface: `Chat(ctx, []Message, []Tool, Options) (Response, error)`
2. Messages include role (system/user/assistant/tool), content, and optional
   tool-call metadata
3. Tools are described as JSON Schema function definitions
4. Options include: model name, temperature, max tokens, stop sequences
5. Response includes: content (text), tool calls (if any), token usage
   (input/output), model ID, finish reason

#### Concrete OpenAI-Compatible Providers

6. Each concrete provider (`lmstudio`, `omlx`, `ollama`, `openai`,
   `openrouter`) wraps a shared `internal/sdk/openaicompat` layer that owns:
   Chat Completions request shaping, tool schema serialization, streaming chunk
   parsing, tool-call delta accumulation, debug wire capture, request timeouts,
   and generic `/v1/models` discovery. The shared layer contains no
   `ProviderName`, provider-system naming, URL heuristics, cost attribution,
   or provider capability tables.
7. Provider packages own these idiosyncrasies and pass explicit options into
   the shared layer:
   - `lmstudio`: LM Studio API key (often `lmstudio`), endpoint list,
     `/api/v0/models` and `/api/v0/models/{model}` limit discovery, thinking
     support.
   - `omlx`: endpoint list, `/v1/models/status` limit discovery, oMLX stream
     quirks.
   - `ollama`: local defaults, Ollama capability claims.
   - `openai`: api.openai.com defaults, OpenAI API key, OpenAI model/list
     behavior.
   - `openrouter`: OpenRouter base URL, API key, headers, model limit
     discovery, and cost attribution.
8. Adding a new provider means adding a provider package, not editing a shared
   flavor switch.

#### Provider Config and Endpoint Pools

9. Provider config has a concrete `type` and may have an endpoint pool for
   local providers:

```yaml
providers:
  studio:
    type: lmstudio
    endpoints:
      - name: vidar
        base_url: http://vidar:1234/v1
      - name: eitri
        base_url: http://eitri:1234/v1
    model_pattern: qwen|coder

  openrouter:
    type: openrouter
    api_key: ${OPENROUTER_API_KEY}
```

10. Supported provider `type` values: `openai`, `openrouter`, `lmstudio`,
    `omlx`, `ollama`, `anthropic`, `virtual`.
    `type: openai-compat` is rejected at config load. URL inference maps
    well-known hosts/ports to concrete types at config load only.
11. For cloud providers, `base_url` is a shorthand for a single endpoint when
    the provider supports URL override.

#### Provider Runtime Identity

12. Provider responses and attempt metadata use:

```go
type AttemptMetadata struct {
    Provider         string // configured provider name, e.g. "studio" or "openrouter"
    ProviderType     string // provider package identity, e.g. "lmstudio" or "openrouter"
    ProviderEndpoint string // endpoint name or host:port when applicable
    Model            string
    CostUSD          float64
}
```

13. No code emits `openai-compat` as a provider name after the cutover.
    `ProviderType` replaces flavor-based naming in telemetry and cost
    attribution.

#### Context and Token Limit Discovery

14. `LookupModelLimits` resolves context window size and max output tokens for
    the active model via a three-step cascade:
    a. Explicit config fields (`context_window` / `max_tokens`) — used directly
       if non-zero
    b. Live API probe against the provider's type-specific endpoint (see
       below) — used if the probe succeeds and returns non-zero values
    c. Zero — caller uses compaction defaults
15. Per-provider-type probe endpoints:
    - `lmstudio`: `GET /api/v0/models/{model}` → `loaded_context_length`
      (prefers loaded context over theoretical maximum)
    - `omlx`: `GET /v1/models/status` → `max_context_window` and `max_tokens`
      per model entry
    - `openrouter`: `GET https://openrouter.ai/api/v1/models` →
      `context_length` and `top_provider.max_completion_tokens`
    - Other types: no probe; falls through to zero

#### Reasoning Configuration

16. `reasoning` is the single public model-reasoning control for provider
    configuration, CLI execution, service requests, and embedding callers.
    Provider-specific terms such as `thinking`, `effort`, `variant`, and token
    budgets are adapter terminology only.
17. `reasoning` accepts one scalar value:
    - Named values: `auto`, `off`, `low`, `medium`, `high`
    - Extended named values when the selected provider or harness advertises
      support, including `minimal`, `xhigh` / `x-high`, and `max`
    - Numeric values such as `0`, `2048`, or `8192`
18. Normalization and tri-state semantics:
    - Empty or unset means no caller preference.
    - `auto` means resolve model, catalog, or provider defaults.
    - `off`, `none`, `false`, and numeric `0` mean explicit reasoning off.
    - Positive integers mean an explicit max reasoning-token budget, or a
      documented provider-equivalent numeric value.
    - Spelling variants may normalize where safe, for example `x-high` to
      `xhigh`, but explicit extended requests must not be silently downgraded.
19. Portable named-to-token defaults are `low=2048`, `medium=8192`, and
    `high=32768`. Provider, model, or catalog metadata may override these
    defaults with a more specific map.
20. Providers that only accept numeric reasoning controls must map named values
    to numeric budgets using capability-aware model metadata and must enforce
    model-specific maximum reasoning-token limits. `max` resolves at the
    provider or harness boundary to the selected model/provider maximum, and is
    accepted only when that maximum is known.
21. Unsupported providers or models may drop reasoning controls that came from
    `auto` or default policy. Explicit unsupported reasoning values and
    explicit over-limit numeric values fail clearly rather than silently
    downgrading.
22. Provider configuration exposes only `reasoning`; older split provider
    config names are rejected with a clear error.

#### Sampling Defaults

22a. Sampling parameters (`temperature`, `top_p`, `top_k`, `min_p`,
     `repetition_penalty`) are **catalog policy**, not user configuration.
     The model catalog carries named `sampling_profiles` bundles; the active
     profile resolves through a precedence chain (catalog → per-provider
     config → CLI) with per-field merge. Any field unset at every layer is
     omitted from the wire so the server's own default applies — this is a
     first-class outcome, not a fallback. See
     [ADR-007](../../02-design/adr/ADR-007-sampling-profiles-in-catalog.md)
     for the full design.
22b. `ModelEntry.sampling_control` records whether the catalog values reach
     the wire: `client_settable` (native agent path; values flow),
     `harness_pinned` (subprocess harnesses pi/codex/claude-code; values are
     metadata only), or `partial` (provider honors a subset; reserved).
22c. The native agent default avoids greedy decoding (`temperature=0`) for
     reasoning-capable models; the Qwen3 model cards explicitly warn that
     greedy decoding under `enable_thinking=True` causes endless repetitions
     and is the failure mode catalog-driven sampling exists to prevent.
22d. Sampling profiles ship through the catalog distribution channel
     (see plan-2026-04-10-catalog-distribution-and-refresh) and reach users
     via `ddx-agent catalog update`, not via binary upgrades. Existing
     installations on stale manifests degrade gracefully — server defaults
     apply, and the agent emits a single first-use nudge pointing at the
     refresh command. ADR-007 §7 covers the schema-evolution rules.

#### Model Auto-Discovery

23. When `model` is empty in config, the provider queries `GET /v1/models`,
    ranks the returned IDs, and auto-selects the top-ranked one.
24. Ranking tiers (highest first):
    - Tier 3: catalog-recognized model IDs
    - Tier 2: pattern-matched via `model_pattern` regex config field
    - Tier 1: uncategorized (any remaining model)
    Within a tier, selection is deterministic (e.g., lexicographic) so the
    chosen model does not change across restarts unless the server's model list
    changes.
25. Local providers with multiple endpoints may select a different endpoint
    per discovery call based on health state.

#### Public Model Listing

26. `DdxAgent.ListModels` is the public interface for listing configured
    provider models. It must list OpenRouter, LM Studio, and oMLX models by
    querying each configured endpoint's OpenAI-compatible models endpoint
    (`<base_url>/models`, typically `/v1/models`).
27. `ModelInfo` results for provider-backed models include the configured
    provider name, concrete provider type (`openrouter`, `lmstudio`, or
    `omlx`), endpoint name, endpoint base URL, model ID, availability, ranking,
    context/cost/catalog metadata when known, and route/default markers.
28. Endpoint-pool behavior is additive and deterministic: a reachable endpoint
    contributes its discovered models; an unreachable endpoint contributes no
    models and does not prevent other endpoints or providers from being listed.

#### Protocol Capability Introspection

29. Providers expose protocol-capability accessors that report what the
    server+type combination can actually honor, so callers can gate dispatch
    on supported features rather than dispatch-and-fail:
    - `SupportsTools() bool` — `/v1/chat/completions` accepts a `tools` field
      and returns structured `tool_calls`
    - `SupportsStream() bool` — `stream: true` returns a well-formed SSE
      stream with incremental `choices[0].delta` chunks
    - `SupportsStructuredOutput() bool` — honors `response_format: json_object`
      or equivalent JSON-mode / tool-use-required semantics
30. Capability flags are type-keyed (`lmstudio` / `omlx` / `openrouter` /
    `ollama` / `openai`). Unknown types return `false` conservatively so
    routing rejects rather than dispatches-and-fails.
31. Protocol capability is distinct from routing capability (the benchmark-
    quality score used by smart-routing scoring). These axes do not interact.

#### Debug and Observability

32. A process-wide opt-in debug mode (`AGENT_DEBUG_WIRE=1`) dumps every HTTP
    request and response at the openai-go transport boundary to stderr (or a
    file via `AGENT_DEBUG_WIRE_FILE=<path>`). Default off, zero cost when
    disabled. Authorization Bearer tokens are redacted before any event is
    written. Complements session events (`EventLLMRequest`/`EventLLMResponse`)
    which capture the logical view; wire dump captures the HTTP view.
33. The shared `internal/sdk/openaicompat` layer owns debug wire capture. No
    provider identity strings appear in captured wire logs.

#### Anthropic Provider

34. Connects to Anthropic's Messages API.
35. Sends tools in Anthropic's tool-use format.
36. Handles Anthropic-specific response structure (content blocks).
37. Reports token usage from response.
38. Uses `github.com/anthropics/anthropic-sdk-go` and is not an
    OpenAI-compatible wrapper.

### Non-Functional Requirements

- **Performance**: Provider overhead (request serialization, response parsing)
  < 10ms beyond network round-trip
- **Reliability**: The runtime owns retry with exponential backoff for
  transient errors (429, 500, 503). Providers execute one request attempt per
  call and surface enough metadata for attempt-scoped observability. Max 3
  runtime retries.
- **Observability**: Each provider call logs model, token counts, latency,
  and any error

## Edge Cases and Error Handling

- **Local server not running**: Return clear error with URL attempted — don't
  hang. Connection timeout of 5s.
- **Model not loaded**: Return provider error as-is (LM Studio and Ollama
  return meaningful errors for this)
- **Tool calling not supported by model**: Let it fail naturally — model will
  return text instead of tool calls, agent loop handles it
- **Streaming interrupted**: Return partial response with error
- **API key missing for cloud provider**: Return error at call time, not at
  provider construction (allows constructing providers speculatively)

## Success Metrics

- Same prompt completes successfully via LM Studio, omlx, Ollama, and
  Anthropic providers
- Token counts are accurately reported for all providers
- Provider swap is a type/base-URL change — no code changes
- When `model` is unset, auto-discovery selects a working model without
  operator intervention
- `LookupModelLimits` returns non-zero values for LM Studio, omlx, and
  OpenRouter when their servers are reachable

## Acceptance Criteria

| ID | Criterion | Suggested Verification |
|----|-----------|------------------------|
| AC-FEAT-003-01 | OpenAI-compatible and Anthropic providers each perform exactly one upstream request attempt per `Chat()` call, return token usage and response model data, and surface attempt metadata needed by runtime retries and telemetry. | `go test ./provider/... ./...` |
| AC-FEAT-003-02 | Streaming provider paths assemble partial text and tool-call fragments into the same logical response shape as synchronous calls, and interrupted streams preserve any partial response while still surfacing an error. | `go test ./provider/... ./...` |
| AC-FEAT-003-03 | Unreachable local endpoints fail within the documented bounded timeout and include the attempted endpoint/base URL in the surfaced error so operators can distinguish routing from model behavior problems. | `go test ./provider/... ./...` |
| AC-FEAT-003-04 | Missing cloud credentials fail at call time rather than constructor time, and default local base URLs remain constructible without extra configuration. | `go test ./provider/... ./...` |
| AC-FEAT-003-05 | Build-tagged integration coverage exercises the same prompt path against LM Studio, omlx, OpenAI, OpenRouter, and Anthropic providers when the corresponding test environment is available. | `go test -tags=integration ./...`; `go test -tags=e2e ./...` |
| AC-FEAT-003-06 | `LookupModelLimits` returns the correct context window and max-token values for LM Studio (via `/api/v0/models/{model}`), omlx (via `/v1/models/status`), and OpenRouter (via their public models endpoint); explicit config fields override live probe results; unreachable endpoints fall through to zero without error. | `go test ./provider/... ./...` |
| AC-FEAT-003-07 | Provider type resolution: explicit `type` config skips all probes; URL inference maps well-known hosts/ports to concrete types at config load only; ambiguous URLs fire concurrent probes and resolve to the first responding type within the 3-second timeout. | `go test ./provider/... ./...` |
| AC-FEAT-003-08 | When `model` is empty, auto-discovery selects the highest-ranked available model according to the three-tier ranking (catalog → pattern → uncategorized) and the selection is deterministic across repeated calls with the same model list. | `go test ./provider/... ./...` |
| AC-FEAT-003-09 | Provider config rejects `type: openai-compat` and `flavor` at config load; concrete provider types are accepted; `base_url` is expanded to a single endpoint for cloud providers. | `go test ./provider/... ./...` |
| AC-FEAT-003-10 | No code emits `openai-compat` as a provider name or telemetry label; `ProviderType` (e.g. `lmstudio`, `openrouter`) is used instead for cost attribution and routing keys. | `go test ./provider/... ./...` |
| AC-FEAT-003-11 | `DdxAgent.ListModels` lists models from OpenRouter, LM Studio, and oMLX through the public service API, includes provider type and endpoint identity in each `ModelInfo`, and continues listing healthy endpoints when another endpoint in the same pool fails. | `go test ./... -run TestListModels` |

## Constraints and Assumptions

- LM Studio and Ollama both speak OpenAI-compatible API well enough for a
  single shared SDK. Edge cases handled by provider wrappers if needed.
- Anthropic needs its own provider due to fundamentally different wire format.
- Models are pre-loaded in LM Studio/Ollama — DDX Agent does not manage model
  lifecycle.

## Dependencies

- **Other features**: FEAT-001 (agent loop uses providers)
- **Governing design**: [Provider Identity, Routing Policy, and Bash Output Filtering](./../../02-design/plan-2026-04-19-provider-routing-tool-output.md)
- **External services**: LM Studio, omlx, Ollama, Anthropic API, OpenAI API
- **PRD requirements**: P0-3, P0-4

## Out of Scope

- Google Gemini native API (use via OpenAI-compatible wrapper or OpenRouter)
- Provider-side prompt caching
- Model lifecycle management (load/unload/pull)
- Availability health checking (e.g., readiness/liveness polling — caller's
  responsibility); type-detection probes are one-shot identification, not
  ongoing health monitoring
