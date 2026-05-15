# Design Plan: Provider Identity, Routing Policy, and Bash Output Filtering

**Date**: 2026-04-19
**Status**: CONVERGED
**Refinement Rounds**: 5

## Problem Statement

Fizeau currently treats several distinct concerns as one concern:

- The OpenAI-compatible HTTP/SSE protocol is used as if it were a provider identity.
- The routing engine has candidate ranking and quota fields, but the service does not yet expose a caller policy for local versus subscription candidates or provide real quota/burn-rate signals to the engine.
- The bash tool bounds output by tail truncation, which protects the context window only after noisy output has already reached a large fixed limit.

The affected users are agent harness callers, DDx queue execution, benchmark workflows, and telemetry consumers. They need provider attribution that names the actual model source, routing that uses the agent's local quota and availability knowledge, and command output that is compact before it is sent back into the model.

This design intentionally breaks backward compatibility where current public shapes encode the wrong concept. DDx will update to the new agent interface shape when agent publishes.

## Requirements

### Functional

1. Define provider identity as the place the model is obtained from, such as `openai`, `openrouter`, `lmstudio`, `omlx`, `ollama`, or `anthropic`.
2. Move OpenAI-compatible request shaping, SSE decoding, tool-call conversion, and wire debug support into a shared implementation layer that has no provider identity.
3. Keep provider-specific behavior in provider packages: auth defaults, endpoint defaults, model discovery, limit discovery, cost attribution, protocol capability claims, and local endpoint handling.
4. Replace runtime "flavor" detection in provider calls with explicit provider identity. URL/port heuristics may only run during config load as inference for incomplete config.
5. Support local providers such as LM Studio and oMLX with multiple configured host:port endpoints.
6. Expose local/subscription routing policy through the service request and route request.
7. Use live quota and trend signals when scoring subscription candidates, while preserving tier intent.
8. Add opt-in RTK-style bash output filtering that can use an installed `rtk` binary and can later support internal command-aware filters.
9. Preserve bash exit code, timeout behavior, stderr handling, cancellation behavior, and bounded output.
10. Keep built-in `read`, `find`, `grep`, `ls`, and related tools outside the first RTK hook scope.

### Non-Functional

1. Routing and provider construction must remain deterministic under test with injected time and fake provider/harness state.
2. Provider wrappers should be thin enough that conformance tests can cover each provider without duplicating the shared protocol test matrix.
3. Provider discovery and health probes must stay bounded by the existing probe timeout model.
4. Bash filtering must fail closed to today's behavior: if the filter is unavailable or errors, the command result is still returned with normal truncation and a marker.
5. No runtime network access is introduced for RTK support, and agent does not install RTK or mutate user shell configuration.

### Constraints

1. The root `agent` package remains core types only.
2. Event emission remains in `loop.go` and `consumeStream`; bash filtering changes only tool result content before the loop emits `EventToolCall`.
3. `Provider` remains the synchronous provider interface; streaming remains opt-in through `StreamingProvider`.
4. The model catalog remains a static shared artifact. Provider live discovery augments availability and limits, not catalog ownership.
5. The service contract is allowed to break for DDx because the old shapes encode the wrong provider definition.

## Architecture Decisions

### Decision 1: Provider Identity Means Model Source

- **Question**: Should `openai-compat` remain a provider, or should provider identity name the actual source?
- **Alternatives**:
  - Keep `openai-compat` as the provider and add a vendor/flavor field. This preserves some readers but keeps the wrong concept in the primary key.
  - Change provider identities to real model sources and update readers now.
- **Chosen**: Provider identity names the actual source. `openai-compat` is not a provider identity.
- **Rationale**: Telemetry, cost, auth, model listing, quota, local endpoint management, and provider-specific bugs all attach to the real source. The OpenAI-compatible shape is an API protocol concern and should not be the identity used by routing or analytics.

### Decision 2: Shared SDK Without Identity

- **Question**: Where should OpenAI-compatible request/response plumbing live?
- **Alternatives**:
  - Keep it in `internal/provider/openai` and parameterize flavor behavior.
  - Extract it to `internal/sdk/openaicompat` and make concrete providers wrap it.
- **Chosen**: Extract `internal/sdk/openaicompat`.
- **Rationale**: The shared layer can own Chat Completions request shaping, tool schema serialization, streaming chunk parsing, tool-call delta accumulation, debug wire capture, request timeouts, and generic `/v1/models` discovery. It must not contain `ProviderName`, provider-system naming, URL heuristics, cost attribution, or provider capability tables.

### Decision 3: Concrete Provider Packages Own Idiosyncrasies

- **Question**: How should provider-specific quirks be represented?
- **Alternatives**:
  - One capability table keyed by detected flavor.
  - Concrete provider packages that supply options and hooks to the shared SDK.
- **Chosen**: Provider packages own quirks and pass explicit options/hooks into the SDK.
- **Rationale**: Adding a provider should mean adding a provider package, not editing a shared flavor switch. At minimum:
  - `internal/provider/openai`: api.openai.com defaults, OpenAI API key, OpenAI model/list behavior.
  - `internal/provider/openrouter`: OpenRouter base URL, API key, headers, model limit discovery, and cost attribution.
  - `internal/provider/lmstudio`: endpoint list, local defaults, `/api/v0/models` and `/api/v0/models/{model}` limit discovery, thinking support.
  - `internal/provider/omlx`: endpoint list, local defaults, `/v1/models/status` limit discovery, oMLX stream quirks.
  - `internal/provider/ollama`: local defaults and Ollama capability claims.
  - `internal/provider/anthropic`: remains a native Anthropic provider, not an OpenAI-compatible wrapper.

### Decision 4: Config Uses Provider Type Plus Endpoint Pool

- **Question**: How should config represent LM Studio/oMLX servers that may exist on several host:port pairs?
- **Alternatives**:
  - Require one named provider per endpoint.
  - Allow a provider instance to hold an endpoint pool.
  - Hide endpoint lists behind URL heuristics.
- **Chosen**: Provider config has a concrete `type` and may have an endpoint pool for local providers.
- **Rationale**: The user config names the provider relationship, while endpoints are routable serving locations for that provider. A local provider with several hosts should not require pretending each host is a different provider source.

Example:

```yaml
providers:
  openrouter:
    type: openrouter
    api_key: ${OPENROUTER_API_KEY}

  studio:
    type: lmstudio
    endpoints:
      - name: vidar
        base_url: http://vidar:1234/v1
      - name: eitri
        base_url: http://eitri:1234/v1
    model_pattern: qwen|coder

  mlx:
    type: omlx
    endpoints:
      - name: local
        base_url: http://localhost:1235/v1
```

For non-local cloud providers, `base_url` remains a shorthand for a single endpoint when the provider supports override URLs. Config load rejects `type: openai-compat`; URL inference may map a missing type from well-known hosts/ports to `openrouter`, `lmstudio`, `omlx`, `ollama`, or `openai`.

### Decision 5: Routing Preference Is Explicit Request Policy

- **Question**: How should callers express local/subscription intent without changing model tier semantics?
- **Alternatives**:
  - Encode preference into model profiles like `cheap` and `smart`.
  - Use a new request field that filters or biases candidates within the selected tier.
- **Chosen**: Add `ProviderPreference` to `ExecuteRequest`, `RouteRequest`, and `internal/routing.Request`.
- **Rationale**: Provider preference is orthogonal to model intent. A caller asking for a cheap or standard tier can still choose local-first, subscription-first, or a hard local/subscription filter.

Allowed values:

| Value | Meaning |
| --- | --- |
| `""` | Default. Local-first within the requested tier, subscription fallback when local is unavailable or cooling down. |
| `local-first` | Explicit default. Same as empty. |
| `subscription-first` | Prefer subscription candidates with acceptable quota before local candidates in the same tier. |
| `local-only` | Reject subscription candidates. |
| `subscription-only` | Reject local candidates. |

### Decision 6: Quota Signals Bias, Not Tier Escalation

- **Question**: Should abundant subscription quota automatically escalate a cheap request to a smarter model?
- **Alternatives**:
  - Let quota availability move between tiers.
  - Use quota only among candidates that satisfy the requested model/tier intent.
- **Chosen**: Quota affects ranking within the eligible candidate set; it does not change tier intent.
- **Rationale**: FEAT-004 keeps model intent separate from routing mechanics. The new bead is about choosing between local and subscription candidates inside a tier, not automatic quality escalation.

The service should populate routing inputs from harness/provider quota state:

- `QuotaOK`: false when a subscription provider is exhausted or expected to exhaust before useful completion.
- `QuotaPercentUsed`: latest known usage percentage.
- `QuotaTrend`: new internal signal with `unknown`, `healthy`, `burning`, and `exhausting`.
- `QuotaStale`: true when the latest probe is older than the configured freshness window.

The first implementation can encode trend into score adjustments without exposing it on the public `Candidate` type. A later observability pass may expose it as `PerfSignal` or a dedicated route candidate diagnostic.

### Decision 7: Bash Filtering Is a Tool Boundary Feature

- **Question**: Should RTK-style compaction rewrite every tool result or only bash?
- **Alternatives**:
  - Filter all built-in tool outputs.
  - Filter only `bash` output in the first slice.
- **Chosen**: Filter only `bash` output first.
- **Rationale**: RTK's own hook model rewrites Bash tool calls before execution and explicitly does not intercept built-in read/grep/glob-style tools. The current agent has a single bash boundary that already owns command execution, stdout, stderr, exit code, timeout, and truncation semantics.

Filtering has two phases. Before execution, the filter may rewrite an allowlisted command to a proxy command such as `rtk git status`. After execution, the filter annotates the result and may apply internal post-processing for modes that do not proxy execution. The bash tool then applies bounded truncation to the final stdout/stderr. This keeps RTK aligned with its hook model while still leaving room for an internal post-exec compactor.

## Interface Contracts

### Provider Config

New provider config shape:

```go
type ProviderConfig struct {
    Type          string             `yaml:"type"` // openai|openrouter|lmstudio|omlx|ollama|anthropic|virtual
    BaseURL       string             `yaml:"base_url,omitempty"`
    Endpoints     []ProviderEndpoint `yaml:"endpoints,omitempty"`
    APIKey        string             `yaml:"api_key,omitempty"`
    Model         string             `yaml:"model,omitempty"`
    ModelPattern  string             `yaml:"model_pattern,omitempty"`
    Headers       map[string]string  `yaml:"headers,omitempty"`
    Reasoning     reasoning.Reasoning `yaml:"reasoning,omitempty"`
    MaxTokens     int                `yaml:"max_tokens,omitempty"`
    ContextWindow int                `yaml:"context_window,omitempty"`
}

type ProviderEndpoint struct {
    Name    string `yaml:"name,omitempty"`
    BaseURL string `yaml:"base_url"`
}
```

Removed:

- `type: openai-compat`
- `flavor`

`base_url` is normalized to a one-element endpoint list for providers that use HTTP endpoints. Validation is provider-specific:

- `openrouter`: requires `api_key`; default base URL is `https://openrouter.ai/api/v1`.
- `openai`: requires `api_key`; default base URL is `https://api.openai.com/v1`.
- `lmstudio`: requires at least one endpoint or defaults to localhost only when caller asks for the default local config.
- `omlx`: requires at least one endpoint or defaults to localhost only when caller asks for the default local config.
- `ollama`: defaults to `http://localhost:11434/v1` unless configured.
- `anthropic`: requires `api_key`; does not use OpenAI-compatible endpoint config.

### Provider Runtime Identity

Provider responses and attempt metadata should use:

```go
type AttemptMetadata struct {
    Provider      string // configured provider name, e.g. "studio" or "openrouter"
    ProviderType  string // provider package identity, e.g. "lmstudio" or "openrouter"
    ProviderEndpoint string // endpoint name or host:port when applicable
    Model         string
    CostUSD       float64
}
```

If the existing `ProviderName` and `ProviderSystem` fields remain in structs for mechanical migration, their meanings change:

- `ProviderName`: configured provider name.
- `ProviderSystem`: provider type, not protocol shape.

No code should emit `openai-compat` as a provider name after the cutover.

### Service Routing Contract

Add:

```go
type ProviderPreference string

const (
    ProviderPreferenceLocalFirst        ProviderPreference = "local-first"
    ProviderPreferenceSubscriptionFirst ProviderPreference = "subscription-first"
    ProviderPreferenceLocalOnly         ProviderPreference = "local-only"
    ProviderPreferenceSubscriptionOnly  ProviderPreference = "subscription-only"
)
```

Add `ProviderPreference ProviderPreference` to:

- `ExecuteRequest`
- `RouteRequest`
- `internal/routing.Request`

`ResolveRoute` must validate the value. Empty normalizes to `local-first`.

### Bash Output Filtering Config

Service/config surface:

```yaml
tools:
  bash:
    output_filter:
      mode: off        # off|rtk|internal|auto
      rtk_binary: rtk
      max_bytes: 51200
      raw_output_dir: "" # optional recovery path; empty disables raw artifact writing
```

Go shape:

```go
type BashOutputFilterConfig struct {
    Mode         string
    RTKBinary    string
    MaxBytes     int
    RawOutputDir string
}

type BashOutputFilter interface {
    Plan(ctx context.Context, in BashFilterPlanInput) (BashFilterPlan, error)
    Finalize(ctx context.Context, in BashFilterResultInput) (BashFilterOutput, error)
}

type BashFilterPlanInput struct {
    Command string
}

type BashFilterPlan struct {
    Command       string // original or rewritten command
    Filter        string // rtk|internal|none
    Rewritten     bool
    FallbackNote  string
}

type BashFilterResultInput struct {
    OriginalCommand string
    ExecutedCommand string
    Stdout          string
    Stderr          string
    ExitCode        int
    Elapsed         time.Duration
    TimedOut        bool
    Filter          string
    Rewritten       bool
}

type BashFilterOutput struct {
    Stdout   string
    Stderr   string
    Summary  string
    Filter   string // rtk|internal|none
    Fallback bool
}
```

For RTK mode, the first implementation should execute `rtk` as the command proxy for allowlisted commands rather than post-process arbitrary output. Example transformation:

- `git status` -> `rtk git status`
- `go test ./...` -> `rtk go test ./...`

Commands that are not recognized or are unsafe to rewrite run normally. If `rtk` is missing or cannot plan the rewrite, the original command path is used with a marker. If the rewritten command starts and then exits nonzero or times out, agent preserves that result and does not re-run the original command.

## Data Model

Provider data separates four concepts:

1. **Configured provider name**: user's logical entry, such as `studio` or `openrouter`.
2. **Provider type**: concrete package identity, such as `lmstudio`, `omlx`, or `openrouter`.
3. **API protocol**: implementation shape, such as OpenAI-compatible Chat Completions or Anthropic Messages.
4. **Endpoint**: serving address for providers that can have multiple host:port locations.

Routing candidate keys should include endpoint identity when endpoint pools are enabled:

```text
harness/provider/endpoint/model
agent/studio/vidar/qwen3-coder
agent/openrouter/default/openai/gpt-4o-mini
claude//default/sonnet
```

Provider cooldowns currently keyed only by provider name should move to provider plus endpoint for local endpoint failures. Cloud provider auth/quota failures remain provider-scoped.

Pricing maps should key by provider type and concrete model, not configured provider name:

```yaml
telemetry:
  pricing:
    openrouter:
      openai/gpt-4o-mini: ...
    openai:
      gpt-4o-mini: ...
```

## Error Handling

Provider config errors:

- Unknown provider type: fail config load and list supported types.
- Missing API key for providers that require one: fail config load unless the provider is not selected by the active route and lazy validation is explicitly implemented later.
- Missing endpoints for local providers: fail config load, except generated default local config may fill localhost defaults.
- Ambiguous URL inference: fail with a message asking for `type`.

Provider runtime errors:

- Endpoint unavailable: mark provider endpoint cooldown and try the next candidate if route policy permits.
- Auth failure: mark provider-scoped failure and do not retry other endpoints for the same cloud provider.
- Discovery failure: keep the provider available if a configured default model exists, but mark discovered model list stale/empty.
- Limit discovery failure: use catalog/default limits and emit a diagnostic.

Routing errors:

- Invalid `ProviderPreference`: reject at `ResolveRoute`.
- `local-only` with no viable local candidate: return no viable candidate without subscription fallback.
- `subscription-only` with exhausted quota: return no viable candidate unless the candidate is unknown-quota and policy allows optimistic use.
- Stale quota: conservative for `cheap` and `standard`, less conservative for `smart` only when no local candidate satisfies the same request.

Bash filter errors:

- RTK not found: run original command, include `[output filter unavailable: rtk not found]`.
- RTK returns an execution error before running the underlying command: run original command and include fallback marker.
- Original command nonzero: preserve nonzero exit code and filtered output.
- Timeout/cancellation: preserve timeout error behavior; do not re-run original command after timeout because that can double side effects.

## Security

1. API keys stay in provider config and are only sent to the provider package that requires them.
2. URL inference must not probe arbitrary external services during validation unless the config path already opts into live probing. Heuristics can inspect hostname/port; live provider probes remain bounded route/discovery operations.
3. Bash filtering must not hide exit codes, stderr, or timeout markers.
4. Raw output recovery, if enabled, writes only under an explicitly configured session/work directory, with `0600` file permissions and no automatic inclusion in telemetry.
5. RTK mode must not install RTK, run RTK init hooks, or modify shell rc files.
6. Command rewriting must be allowlisted. Unknown commands run through the existing shell path.

## Test Strategy

- **Unit**:
  - Config rejects `openai-compat` and `flavor`, accepts concrete provider types, and expands `base_url` to a single endpoint.
  - URL inference maps known ports/hosts to concrete provider types only at config load.
  - `internal/sdk/openaicompat` conformance tests cover non-streaming, streaming, tool calls, seed/temperature, reasoning serialization, and debug capture without provider identity.
  - Provider packages set expected provider type/name/capabilities and own discovery hooks.
  - Routing validates `ProviderPreference` and filters/ranks local/subscription candidates correctly.
  - Bash filter tests use a fake `rtk` binary for `git status` and `go test`, missing-rtk fallback, nonzero exit, stderr, oversized output, and timeout.
- **Integration**:
  - Existing provider conformance suite runs against `openai`, `openrouter`, `lmstudio`, `omlx`, and `ollama` wrappers with fake servers.
  - `ListProviders`, `ListModels`, and `ResolveRoute` show provider type and endpoint metadata consistently.
  - Session usage/pricing tests verify `openrouter` cost attribution no longer depends on `openai-compat`.
- **E2E**:
  - A native `Execute` with `ProviderPreference=local-only` never dispatches to subscription.
  - A native `Execute` with local provider in cooldown and default/local-first preference falls back to a same-tier subscription candidate.
  - Bash command output for a noisy `go test` fixture is compacted when filtering is enabled and unchanged except for bounded truncation when disabled.

## Implementation Plan

### Dependency Graph

1. Contract and config surface updates.
2. Shared `internal/sdk/openaicompat` extraction with no behavior change.
3. Concrete provider wrapper packages and config factory cutover.
4. Provider discovery, limit discovery, cost attribution, and endpoint pool routing.
5. Routing `ProviderPreference` plus quota/trend scoring.
6. Bash output filter config, fake-RTK tests, and implementation.
7. Documentation refresh: FEAT-003, FEAT-004, FEAT-002, SD-005, CONTRACT-003, architecture.

Provider split can start before routing preference work, but endpoint cooldown keys should be aligned before quota-aware routing lands. Bash filtering is independent after config plumbing is agreed.

### Issue Breakdown

Existing beads to refine:

- `agent-05d81638`: provider split. Update acceptance to remove backward-compat options, reference this plan, and split implementation into provider SDK/config/wrapper sub-slices.
- `agent-4c482f60`: routing preference and quota. Update acceptance to include `ProviderPreference` contract values and quota stale/trend behavior.
- `agent-d540f5dd`: RTK-style bash output filtering. Update acceptance to reference the bash-only tool boundary, fake RTK binary tests, and fallback markers.

Recommended follow-on beads:

1. **provider: extract OpenAI-compatible SDK**
   - Acceptance: `internal/sdk/openaicompat` contains request/response/stream/tool/debug plumbing; no provider identity strings; existing OpenAI-compatible tests pass through a wrapper.
2. **provider: concrete provider config and wrappers**
   - Acceptance: provider types `openai`, `openrouter`, `lmstudio`, `omlx`, and `ollama` construct successfully; `openai-compat` and `flavor` are rejected; endpoint pools are represented in provider/model listing.
3. **provider: move discovery and cost ownership**
   - Acceptance: OpenRouter cost attribution lives in `provider/openrouter`; LM Studio and oMLX limit discovery live in their packages; conformance tests cover all wrappers.
4. **routing: ProviderPreference and quota scoring**
   - Acceptance: service request/route request carry preference; routing filters/biases local/subscription candidates; quota fields are populated from harness/provider state.
5. **tool: opt-in bash output filter**
   - Acceptance: fake RTK tests cover rewrite and fallback; disabled mode matches current behavior; docs state built-in read/find/grep are not filtered.
6. **docs: provider/routing/tool contract refresh**
   - Acceptance: FEAT-003, FEAT-004, FEAT-002, SD-005, architecture, and CONTRACT-003 use provider identity consistently and remove `openai-compat` as a provider.

## Risk Register

| Risk | Likelihood | Impact | Mitigation |
| --- | --- | --- | --- |
| Telemetry/pricing readers still expect `openai-compat` | H | H | Break intentionally in one cutover, update session usage, replay, benchscore, and docs in the same provider identity slice. |
| Provider endpoint pools make candidate identity ambiguous | M | M | Include endpoint in candidate/cooldown keys and expose endpoint in diagnostics. |
| URL inference guesses the wrong provider | M | M | Restrict inference to config load, prefer explicit type, fail on ambiguity, and avoid runtime behavior changes based on heuristics. |
| Quota trend data is stale or missing | H | M | Add stale state and conservative scoring; keep explicit preference filters deterministic. |
| RTK rewrite changes command semantics | M | H | Use an allowlist, preserve shell fallback, and never retry original command after timeout/cancellation. |
| Filtered output hides useful failing test context | M | M | Keep stderr, exit code, timeout marker, and raw output recovery path; use command-specific tests for failures. |

## Observability

Provider/session events should report:

- configured provider name
- provider type
- endpoint name or host:port
- model
- route preference
- quota state summary when known
- cooldown reason when a candidate is demoted

Route diagnostics should include rejected candidates and reasons. If public `Candidate` remains intentionally small, session logs should still capture candidate metadata for debugging.

Bash tool result text should include filter status only when filtering changed behavior or fell back:

```text
[output filter: rtk git status]
[output filter unavailable: rtk not found; used raw output]
[filtered output truncated: 1800 lines omitted]
```

No RTK analytics or `rtk gain` data is collected by agent.

## Governing Artifacts

- `agent-05d81638` provider split bead
- `agent-4c482f60` quota/local-subscription routing bead
- `agent-d540f5dd` RTK-style bash output filtering bead
- `docs/helix/01-frame/prd.md`
- `docs/helix/01-frame/features/FEAT-002-tools.md`
- `docs/helix/01-frame/features/FEAT-003-providers.md`
- `docs/helix/01-frame/features/FEAT-004-model-routing.md`
- `docs/helix/02-design/contracts/CONTRACT-003-fizeau-service.md`
- `docs/helix/02-design/solution-designs/SD-005-provider-config.md`
- `docs/helix/02-design/plan-2026-04-08-shared-model-catalog.md`
- `docs/helix/02-design/plan-2026-04-10-model-first-routing.md`
- `docs/helix/02-design/plan-2026-04-10-catalog-distribution-and-refresh.md`
- `docs/helix/02-design/architecture.md`
- `docs/helix/02-design/adr/ADR-001-observability-surfaces-and-cost-attribution.md`
- RTK README: https://github.com/rtk-ai/rtk
