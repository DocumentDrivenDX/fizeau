---
ddx:
  id: SD-005
  depends_on:
    - FEAT-003
    - FEAT-004
    - FEAT-006
    - SD-001
---
# Solution Design: SD-005 — Provider Sources, Model Catalog, and Power Routing

## Problem

Fizeau started with a single flat provider config (`provider`, `base_url`,
`api_key`, `model`). That shape is sufficient for one local inference server,
but real use has separate concerns:

1. **Provider source and endpoint setup** — concrete transport/auth data for
   model discovery and dispatch.
2. **Shared model policy** — one agent-owned catalog for model aliases, numeric
   power, per-surface projections, deprecations, cost, context, and benchmark
   provenance.
3. **Automatic selection** — one routing decision per request based on live
   inventory, catalog metadata, availability, usage, cost, speed, and caller
   constraints.

Prompt presets remain a separate concern for system prompt behavior only.

## Design

Fizeau keeps two layers above the runtime boundary:

- **Provider sources and endpoints** declare transport/auth locations. Endpoint
  labels may exist for diagnostics, host display, and explicit endpoint
  selection, but stable user-authored endpoint labels are not the primary
  routing abstraction.
- **Model catalog** owns reusable policy/data loaded from an embedded snapshot
  plus an optional external manifest override, with published manifest bundles
  distributed outside binary releases. It owns model power, cost, context
  window, capability metadata, provider/deployment class, benchmark provenance,
  and reasoning defaults per model.

There is no user-authored routing-rule layer in the target design. Per-request
routing follows ADR-005: the service discovers what each configured source can
serve, joins that inventory with the catalog, applies hard caller constraints
and optional power bounds, scores survivors, dispatches the top candidate once,
and reports the attempted route outcome.

After resolution, the service builds exactly one concrete native provider
adapter and executes it internally. Consumers do not receive provider
instances.

Caller boundary (see CONTRACT-003):

- Callers choose a harness only when they need to constrain the execution
  surface. Otherwise the service may consider all eligible harnesses.
- Callers pass routing intent through public request fields (`Provider`,
  `Model`, `ModelRef`, `MinPower`, `MaxPower`) plus optional auto-selection
  inputs (`EstimatedPromptTokens`, `RequiresTools`, `Reasoning`).
- Explicit model, provider-source/endpoint, and harness pins always win over
  automatic selection. If a hard pin cannot be satisfied, routing fails with
  detailed no-candidate evidence and never substitutes a broader model, source,
  endpoint, or harness.
- Embedded `fiz` chooses the concrete provider candidate, constructs the
  adapter, dispatches exactly one candidate, and reports the attempted route
  outcome.
- Callers receive attribution facts from the embedded run: selected candidate,
  rejected candidates, filter reasons, score components, and the actual
  harness/provider-source/endpoint/model used.
- Callers own all retry and task-level escalation. The agent reports route
  evidence but does not dispatch another candidate in the same request.

## Config Format

The target config declares sources/endpoints for discovery and dispatch. It
does not encode route order, model-strength policy, or fallback chains.

```yaml
# .fizeau/config.yaml
model_catalog:
  manifest: ~/.config/fiz/models.yaml   # optional local override

provider_sources:
  - type: lmstudio
    endpoints:
      - label: vidar
        base_url: http://vidar:1234/v1
        api_key: lmstudio
        reasoning: off
      - label: grendel
        base_url: http://grendel:1234/v1
        api_key: lmstudio

  - type: omlx
    endpoints:
      - label: vidar-omlx
        base_url: http://vidar:1235/v1
        api_key: omlx
        model: Qwen3.5-27B-4bit
        reasoning: off

  - type: vllm
    endpoints:
      - label: bragi-vllm
        base_url: http://bragi:8000/v1

  - type: llama-server
    endpoints:
      - label: local-llama
        base_url: http://localhost:8080/v1

  - type: openrouter
    endpoints:
      - label: openrouter
        base_url: https://openrouter.ai/api/v1
        api_key: ${OPENROUTER_API_KEY}
        headers:
          HTTP-Referer: https://github.com/easel/fizeau
          X-Title: Fizeau

  - type: anthropic
    endpoints:
      - label: anthropic
        api_key: ${ANTHROPIC_API_KEY}

routing:
  health_cooldown: 30s

preset: default
max_iterations: 20
session_log_dir: .fizeau/sessions
```

Current implementation may still load older `providers:` entries during the
migration window. Those entries should be treated as endpoint definitions under
their declared source type, not as the primary routing API.

the removed route-table field was deprecated by ADR-005 and its compatibility parser has now
been removed. Automatic routing covers the same intent by combining provider
source discovery, endpoint health, catalog power, and score components without
per-candidate route order in YAML.

### Provider Source and Endpoint Fields

Source fields:

| Field | Type | Description |
|---|---|---|
| `type` | enum | Provider source type such as `lmstudio`, `omlx`, `vllm`, `llama-server`, `ollama`, `openrouter`, `anthropic`, or harness-backed sources where applicable |
| `endpoints` | list | Concrete transport/auth locations for this source |

Endpoint fields:

| Field | Type | Description |
|---|---|---|
| `label` | string | Optional diagnostic and explicit endpoint selector |
| `base_url` | string | API base URL when the source uses HTTP |
| `api_key` | string | Secret reference or literal token for the endpoint |
| `headers` | map | Optional endpoint-specific HTTP headers |
| `model` | string | Optional default model hint for direct dispatch, not catalog policy |
| `reasoning` | scalar string/int | Public reasoning control for this endpoint |
| `placement` | enum | Optional override for placement metadata: local/free, prepaid, metered, or test |
| `max_tokens` | int | Max output tokens per turn; `0` = use provider default |
| `context_window` | int | Explicit context window override; `0` = attempt live discovery |

Provider-specific wire terms such as `thinking`, `effort`, `variant`, and token
budgets are adapter implementation details, not public config.

### Reasoning Values

`reasoning` is one scalar rather than separate public level and budget fields.

- Empty or unset means no caller preference.
- `auto` means resolve model, catalog, or provider defaults.
- `off`, `none`, `false`, and numeric `0` mean explicit reasoning off.
- `low`, `medium`, and `high` use portable fallback budgets of 2048, 8192, and
  32768 tokens when provider/catalog metadata does not publish a better map.
- Extended names such as `minimal`, `xhigh`, `x-high`, and `max` are accepted
  only when the selected provider or harness advertises support. `x-high`
  normalizes to `xhigh`; explicit extended requests are never silently
  downgraded.
- Positive integers mean an explicit max reasoning-token budget, or a
  documented provider-equivalent numeric value.

Providers that only accept numeric reasoning controls must map named values to
numeric budgets with capability-aware model metadata and must enforce
model-specific maximum reasoning-token limits. `max` resolves at the provider
or harness boundary to the selected model/provider maximum and is accepted only
when that maximum is known. Auto/default reasoning controls may be dropped for
unsupported providers/models, but explicit unsupported or over-limit values
fail clearly.

Model catalog metadata uses `reasoning_default`. Explicit caller values always
win when supported, including numeric values and values above `high` such as
`xhigh`, `x-high`, or `max`.

## Model Catalog and Power

Power is the canonical routing strength axis. Higher values mean stronger
models for agent tasks. Every catalog model must have power from 1..10 to be
eligible for automatic routing; power `0` means unknown, missing, or
exact-pin-only.

The catalog manifest stores concrete model metadata at the model entry level:
family, display name, status, cost, cache cost, context window, benchmark
metadata, OpenRouter ID, reasoning metadata, provider/deployment class, power,
power provenance, and consumer surface strings. Target entries define policy
and ordered candidates, not duplicated model metadata.

```yaml
version: 4
models:
  qwen3.5-27b:
    family: qwen
    display_name: Qwen3.5 27B
    status: active
    power: 5
    power_provenance:
      method: benchmark_cost_recency
      inputs: [swe_bench, context_window, cost, recency, deployment_class]
    deployment_class: local_free
    cost_input_per_m: 0.10
    cost_output_per_m: 0.30
    context_window: 262144
    surfaces:
      agent.openai: qwen3.5-27b
targets:
  code-work:
    family: coding
    status: active
    context_window_min: 131072
    candidates: [qwen3.5-27b]
    surface_policy:
      agent.openai:
        reasoning_default: off
```

Bootstrap power from normalized benchmark evidence, model capabilities
(context, tools, reasoning), recency, cost, and provider/deployment class. When
benchmark coverage is missing, cost times recency is the first-order proxy:
within a provider/model family, the newest and most expensive model is assumed
strongest unless the catalog explicitly overrides power or marks an older model
as a useful cost/power exception. Older family members are exact-pin-only for
automatic routing without that override. Keep raw benchmark inputs beside the
derived power value so catalog updates can evolve scores quantitatively as new
models and measurements arrive.

Provider/deployment class prevents benchmark-only equivalence across unlike
surfaces. A local/community/self-hosted copy must not receive the same power as
a managed cloud frontier model solely because one benchmark is high.

## Resolution Model

Per request, the service:

1. Loads provider source config and the agent model catalog.
2. Builds an available-model inventory:
   1. Enumerates every configured harness, provider source, endpoint, and
      discovered concrete model.
   2. Joins each concrete model to the model catalog. Matched entries provide
      power, family, status, context window, reasoning capability, tool
      support, list price, deployment class, and benchmark quality. Unknown
      models remain inspectable but are not eligible for automatic routing
      unless explicitly pinned.
   3. Joins live operational signals: source/endpoint health, endpoint
      cooldown, observed latency, prepaid quota remaining/reset time, and known
      marginal cost.
3. Applies caller intent:
   - `--min-power` and `--max-power` select the allowed catalog power range.
     If unset, there is no power bound and the router selects the best
     lowest-cost viable auto-routable model from discovered inventory.
   - `--model-ref` resolves through the catalog. A reference to a concrete
     model entry is an exact model constraint. Catalog aliases are for exact
     model identity and migration, not routing personas.
   - `--model` is an exact concrete model constraint. If the caller asks for
     `qwen-3.6-27b`, the router may choose among provider sources/endpoints
     that serve that model, but it must not substitute a different model.
   - `--provider` is a hard provider-source or endpoint constraint, depending
     on the request surface. `--provider lmstudio` means only the LM Studio
     source is considered; an endpoint selector means only that endpoint is
     considered.
   - `--harness` is a hard harness constraint.
   - `--harness + --provider + --model` bypasses scoring after validation,
     except for multiple endpoints under the same constrained source that can
     satisfy the same concrete model.
4. Filters candidates:
   1. Hard constraints remove all candidates outside requested harness,
      provider-source/endpoint, and exact-model axes. These constraints are
      never relaxed by power scoring.
   2. Power bounds remove models outside `MinPower..MaxPower` when either bound
      is set. Models without catalog power are removed unless exactly pinned.
   3. Liveness/model-discovery removes endpoints that are down or do not serve
      the candidate model.
   4. Capability removes candidates with too-small context windows, missing
      tool support for `RequiresTools`, unsupported explicit reasoning, or
      stale/deprecated catalog status when not explicitly allowed.
5. Applies sticky endpoint assignment for equivalent local/free endpoints:
   1. If the request has a live sticky route key with a valid lease, reuse that
      `(provider source, endpoint, model)` assignment before new load balancing.
   2. If no valid lease exists, use normalized endpoint utilization plus
      service-owned in-flight lease counts to prefer the least-loaded equivalent
      local endpoint.
   3. Existing sticky assignments move only when the endpoint disappears, stops
      serving the model, enters cooldown, or crosses a hard saturation threshold.
6. Scores survivors with explicit components:

   ```text
   score = power_weighted_capability
         + latency_weight
         + placement_bonus
         + quota_bonus
         - marginal_cost_penalty
         - availability_penalty
         - stale_signal_penalty
   ```

7. Dispatches the top candidate exactly once. On provider/harness failure, the
   service records the attempted route outcome and returns the full ranked
   trace. It does not try the next eligible candidate and it does not widen
   power bounds inside the same request.

The full ranked candidate trace and per-candidate score components are emitted
as part of the routing-decision event (CONTRACT-003). Operators explain a
decision through `route-status` and `fiz --list-models`, not by reading
route order in config.

## Failure Evidence and Retry Boundary

The router does not recover by retrying. It has one selection mechanism and one
reporting mechanism:

1. **In-request selection** is service-owned. The service ranks candidates,
   dispatches the top candidate once, and returns the ordered trace.
2. **Retry and escalation** are caller-owned. The caller issues a second
   request with a higher `MinPower`, a different `MaxPower`, or different hard
   pins when its task policy says the extra cost/time is justified.

Every failed routed `Execute` returns enough structured evidence for that
caller decision:

- requested power bounds, hard constraints, and exact pins
- selected candidate, rejected candidates, and filter reasons
- score components and the live/cost/quota facts used for ranking
- final failure class: `setup/config`, `no-candidate`, `provider-transient`,
  `capability`, `cancelled`, or `timeout`
- attempted route outcome for the single dispatched candidate

Hard pins do not suggest broader alternatives. If `--model qwen-3.6-27b`
cannot be satisfied, the error explains that exact constraint and the inspected
provider sources/endpoints rather than recommending an unrelated model.

## Available Model Inventory

The service exposes the joined inventory through `FizeauService.ListModels`. The CLI
exposes the operator-facing equivalent as `fiz --list-models`; JSON
output is the contract and text output is a rendering.

Each row contains:

- identity: harness, provider source, endpoint label/base URL, model ID,
  catalog ID
- policy: power, family, provider/deployment class, deprecation status,
  auto-routable status, exact-pin-only status
- capability: context window, tool support, reasoning support, streaming and
  structured-output support when known
- economics: placement (local/free, prepaid, metered, test), marginal cost,
  cost source, prepaid quota remaining/reset time
- operations: health, cooldown, recent latency
- routing: power filter reasons and score components for supplied power bounds

This surface is the debugging contract for routing. If `route-status` says a
candidate lost, `fiz --list-models --min-power <n> --json` must show the
raw facts that caused the loss.

## Key Design Decisions

**D1: Provider sources and endpoints are transport setup.** They hold endpoint
URLs, credentials, headers, and optional model hints. They are not the
canonical source of power, alias, or route-order policy.

**D2: The model catalog is a first-class layer.** The catalog is loaded from an
embedded manifest snapshot with an optional external override, and it owns
model power, exact identity aliases, deprecations, benchmark inputs,
provider/deployment class, and per-surface projections.

**D2A: Publish catalog bundles independently of binary releases.** The embedded
snapshot remains the safe default, but operators and callers can install a
newer shared manifest from a versioned published bundle via an explicit update
flow.

**D2B: Manifest v4 separates concrete models from target policy.** Top-level
model entries carry concrete model metadata. Target entries define policy,
replacement metadata, ordered candidates, and per-surface defaults.

**D3: Preserve prompt preset terminology for prompts only.** The top-level
`preset` field and CLI `--preset` flag refer to system prompt presets defined
in SD-003. Model policy uses `model_ref`, numeric power bounds, exact model
pins, or catalog entries, never `preset`.

**D4: Power routing replaces the removed route-table field.** Per ADR-005, the service
combines catalog power, provider/harness model inventory, placement, cost,
context, capability, liveness, and usage/quota to pick the best candidate per
request. Users do not author per-candidate route order. the removed route-table field config
is rejected as a removed legacy surface.

**D5: Power is routing intent; model/provider/harness are constraints.**
`--min-power` and `--max-power` select the model-strength range. `--model-ref`
is exact when it names a concrete catalog model. `--model`, provider
source/endpoint selection, and `--harness` are hard constraints. Routing may
optimize cost and availability inside those constraints but must fail with a
detailed candidate trace when they cannot be met.

**D6: Auto-selection inputs are deterministic.** Auto-selection signals are
`EstimatedPromptTokens` (filter by context window), `RequiresTools` (filter by
tool support), and `Reasoning` (filter by reasoning support). No prose
heuristic complexity classifier. `RequiresTools` is explicit caller intent, or
derived only when a request surface has unambiguously enabled tool execution.

**D7: No agent-owned retry.** The routing engine ranks candidates with explicit
components. `Execute` dispatches the top candidate once and returns the ranked
trace plus attempted-route outcome. DDx or another caller owns any follow-up
request with a stronger `MinPower`, capped `MaxPower`, or different hard pins.
Per-(harness, provider source, endpoint, model) availability/latency replaces
coarser health memory.

**D7A: Placement is provider-candidate metadata.** `agent` as a native harness
may front local/free, prepaid, and metered providers. Routing placement filters
operate on the provider-source/endpoint candidate, not the harness. Default
placement comes from source type and catalog deployment class, with endpoint
override available only as metadata.

**D8: Environment variable expansion still applies to values.** `${VAR}` is
expanded at config load time. No shell evaluation.

**D9: Backwards compatible with legacy flat format during migration.** Old flat
config still maps to one endpoint under the declared provider source.
the removed route-table field parsing has been removed after its deprecation cycle. A
boundary test forbids re-introduction of that parser.

**D10: Provider limit discovery is live and type-gated.** When
`context_window` or `max_tokens` are zero, the CLI calls `LookupModelLimits`
against the provider's API to discover them. Explicit config values always win.
Discovery is keyed by server type:

- **LM Studio** — `GET /api/v0/models/{model}`; prefers
  `loaded_context_length`
- **omlx** — `GET /v1/models/status`; returns `max_context_window` and
  `max_tokens` per model
- **OpenRouter** — `GET /api/v1/models` (public list)

Undiscoverable values stay zero and the compaction layer uses its own defaults.

**D11: Provider type replaces flavor heuristics for limit discovery.**
Port-based provider detection fails when servers run on non-default ports. The
explicit `type` field lets operators declare the server type. When type is
absent the system:

1. Tries URL-based detection first.
2. Fires concurrent probes to `/v1/models/status` and `/api/v0/models` with a
   3-second timeout to distinguish omlx vs LM Studio on ambiguous ports.
3. Falls back to port heuristics as a last resort.

**D12: omlx is a first-class supported provider source.** omlx is a local
inference runtime that speaks the OpenAI-compatible chat API and exposes
additional endpoints: `GET /v1/models/status` returns per-model
`max_context_window` and `max_tokens`. Set `type: omlx` to use dedicated limit
discovery and avoid probe ambiguity.

**D12A: vLLM and llama-server are first-class local provider sources with
provider-owned utilization probes.** Set `type: vllm` for a vLLM OpenAI server
and `type: llama-server` for llama.cpp `llama-server`. Their `base_url` remains
the OpenAI-compatible API base, usually `/v1`; utilization probes derive the
server root by removing a trailing `/v1`.

- **vLLM** — `GET /metrics` on the server root; normalize
  `vllm:num_requests_running`, `vllm:num_requests_waiting`, and cache pressure
  from `vllm:kv_cache_usage_perc` or legacy `vllm:gpu_cache_usage_perc`.
- **llama-server** — `GET /metrics` on the server root when the process is
  started with `--metrics`; normalize `llamacpp:requests_processing`,
  `llamacpp:requests_deferred`, and `llamacpp:kv_cache_usage_ratio`. If metrics
  are unavailable, fall back to `GET /slots` and count `is_processing` slots.

Provider utilization is not a route table and not a user-authored policy block.
It is an operational input for choosing among otherwise equivalent eligible
local endpoints.

**D12B: Sticky route leases preserve worker affinity.** A long-running worker
sequence with the same sticky route key reuses its assigned local endpoint.
New sticky keys are assigned to the least-loaded equivalent endpoint. On a
single machine, in-process leases are authoritative and provider utilization is
an advisory refinement. Across multiple Fizeau processes, correct stickiness and
fair balancing require a shared lease backend; raw server metrics alone are
sampled and racy.

**D13: Protocol capabilities are type-keyed and conservative.** The provider
exposes `SupportsTools()`, `SupportsStream()`, and
`SupportsStructuredOutput()` accessors that return the effective capability for
the resolved type. Downstream routing consults these before dispatch to avoid
dispatch-and-fail on mismatched prompts. Unknown types return `false` for all
protocol flags so routing rejects rather than dispatches. This surface is
distinct from benchmark-based capability scoring.

**RequiresTools filter scope.** `RequiresTools=true` filters candidates at the
`(harness, provider source, endpoint, model)` level via an OR-permissive gate:
a candidate passes when either `routing.HarnessEntry.SupportsTools` or
`routing.ProviderEntry.SupportsTools` is `true`, and the catalog's per-model
override is not set.

**D14: `DetectedType()` layers on top of `providerSystem` without replacing
it.** `providerSystem` remains the source of truth for per-response telemetry
and cost attribution because those fire on every response and cannot afford a
network probe. `DetectedType()` is the probe-confirmed accessor used for
pre-dispatch gating. It runs the probe at most once per provider via
`sync.Once`, caches the result, and falls back to `providerSystem` when the
probe is inconclusive.

**D15: `reasoning` is the public model-reasoning control.** The public surface
uses one scalar (`reasoning`) for named and numeric values. Config uses
`reasoning`; catalog metadata uses `reasoning_default`; the CLI uses
`--reasoning`. Provider and harness adapters may translate the resolved value
to wire or subprocess knobs named `thinking`, `effort`, `variant`, or numeric
budgets, but those names are not preferred public controls. Unsupported
auto/default controls may be dropped; explicit unsupported or over-limit
values fail clearly.

**D16: Provider model listing is public and endpoint-aware.** `FizeauService.ListModels`
is the public service surface consumers use to list configured
provider-backed models. For OpenRouter, LM Studio, and oMLX, the service
queries each configured endpoint's `<base_url>/models` endpoint and returns one
result per discovered model per endpoint. Source type and endpoint identity are
explicit `ModelInfo` fields so consumers do not read provider config or infer
type from URLs. Endpoint failures are local to that endpoint during listing;
status diagnostics remain in `ListProviders` and `HealthCheck`.

**D17: Provider observability cassettes come from real servers.** vLLM and
llama-server provider compatibility tests use the established `go-vcr` library.
Replay mode is the default test path. Record mode is opt-in and owns the full
server lifecycle: install or pull the runtime, start a trivial CPU model on
temporary ports, wait for readiness, record `/v1/models`, provider
observability endpoints, minimal chat, and under-load utilization evidence, then
stop the server. The acceptance path must not require a developer to manually
start servers.

## CLI UX

### Prompt Preset Selection

The `--preset` flag (or `preset` in config) selects the system prompt style.
Built-in preset details are defined by SD-003 and implemented in
`prompt/presets.go`. This surface is intentionally unrelated to routing.

### Direct Source / Model Selection

```bash
fiz run --provider lmstudio "prompt"
fiz run --provider anthropic --model opus-4.7 "prompt"
fiz run --model-ref code-high "prompt"
fiz run --model-ref code-high --reasoning max "prompt"
fiz run --model qwen-3.6-27b "prompt"
fiz run --provider lmstudio --reasoning 8192 "prompt"
```

The public CLI flag is `--reasoning <value>`. Do not introduce alternate public
reasoning flags.

### Power-Routed Selection

```bash
fiz run --model qwen3.5-27b "prompt"  # pin a concrete model
fiz run --min-power 5 "prompt"        # request stronger automatic candidates
fiz run --min-power 8 "prompt"        # retry with a stronger floor
fiz run "prompt"                      # automatic routing over eligible candidates
fiz --list-models --json              # inspect joined inventory
```

Compatibility:

```bash
fiz -p "prompt" --backend code-work-local
```

The compatibility flag remains temporarily, but it is not the preferred UX.

## Library and Package Boundaries

The library runtime boundary does not change: `agent.Run()` still takes a
single `Provider` in the `Request`.

Config and CLI code grow a catalog-aware layer above that boundary. Expected
package split:

- `internal/config/` — load provider source/endpoint config, routing defaults,
  and optional manifest override path
- `internal/modelcatalog/` — load, validate, and resolve shared model policy
- `internal/reasoning/` — shared leaf package for the Reasoning scalar,
  parser, normalization, constants, `ReasoningTokens(n)`, and resolved policy
  representation
- `cmd/fiz/` — resolve hard pins and power bounds into one concrete
  provider/model/reasoning policy

## Traceability

- FEAT-004 defines the ownership split and terminology.
- ADR-005 defines the power-routing decision model and retry boundary.
- CONTRACT-003 defines the public service surface and routing attribution
  events.
- SD-003 reserves `preset` for system prompt behavior.
- `plan-2026-04-08-shared-model-catalog.md` defines the catalog package/API,
  manifest format, and consumer examples.
- `plan-2026-04-10-catalog-distribution-and-refresh.md` defines published
  manifest bundles, explicit update flow, and the initial reasoning baseline.
- D10-D12 (provider limit discovery, flavor detection, omlx support) are
  implemented in `internal/config/config.go`, provider adapters, and the
  `LookupModelLimits` call-site in the CLI layer.
- D15 (reasoning contract) is implemented through `reasoning`,
  `reasoning_default`, and CLI `--reasoning`.
- D16 (endpoint-aware provider model listing) is implemented through
  `FizeauService.ListModels` and the exported `ModelInfo` provider/endpoint fields.
- D12A-D12B and D17 govern local endpoint utilization, sticky route leases, and
  real-server VCR acceptance for vLLM and llama-server.
