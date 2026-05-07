---
ddx:
  id: FEAT-004
  depends_on:
    - helix.prd
    - FEAT-003
---
# Feature Specification: FEAT-004 — Shared Model Catalog, Power-Based Routing, and Provider Configuration

**Feature ID**: FEAT-004
**Status**: Draft
**Priority**: P0 (provider sources/endpoints), P1 (shared catalog), P1 (power-based routing)
**Owner**: Fizeau Team

## Overview

Fizeau keeps the runtime boundary deliberately simple: `agent.Run()` receives
one resolved `Provider`. Configuration declares provider sources and endpoints;
it does not contain complicated route rules. The agent discovers what models
those sources can serve, joins that inventory with catalog power/cost/speed
metadata, tracks usage and availability, and automatically routes to the best
candidate that satisfies the requested power and hard constraints. If the
caller supplies no power bounds, the agent selects the best lowest-cost viable
auto-routable model from the discovered inventory.

This feature therefore has three related but separate responsibilities:

- **Provider sources and endpoints** — concrete transport/auth definitions for
  provider types such as `lmstudio`, `omlx`, `vllm`, `llama-server`,
  `openrouter`, `anthropic`, and subprocess harnesses such as `codex` or
  `claude`
- **Shared model catalog** — agent-owned model metadata, numeric power,
  aliases, surface mappings, deprecations, costs, and benchmark provenance
- **Power-based routing** — generated candidate inventory plus deterministic
  selection within caller-supplied hard constraints and optional power bounds

Prompt presets stay separate from all three. `preset` already means system
prompt behavior and must not be reused for model policy or routing.

## Terminology

- **Provider source** — the implementation/source type used to reach models,
  such as `lmstudio`, `omlx`, `ollama`, `openrouter`, `anthropic`, `codex`, or
  `claude`
- **Endpoint** — a concrete configured transport/auth location for a provider
  source: base URL, headers/credentials, optional host label, and optional
  default model hint. Endpoint identity is diagnostic and filterable; it is not
  the primary routing abstraction.
- **Model catalog** — agent-owned policy/data describing model families,
  numeric power, aliases, canonical policy targets, provider/deployment class,
  deprecation/staleness status, costs, benchmark inputs, and surface mappings
- **Manifest** — the structured model-catalog data file maintained separately
  from Go logic and consumed by the catalog package
- **Model reference** — a user-facing name resolved through the catalog, such
  as a concrete model entry or optional alias
- **Profile** — a named shorthand for a power-policy point or bounded power
  range, with optional ranking preferences. Profiles do not name a closed set
  of concrete models and do not override hard pins.
- **Canonical target** — the stable policy target the catalog wants a given
  reference to resolve to; one target may project to different concrete models
  and reasoning defaults on different consumer surfaces
- **Power** — an integer model-strength score from 1 to 10. Higher means more
  capable for agent tasks. `0` means unknown, missing, or not eligible for
  automatic routing.
- **Provider/deployment class** — catalog provenance describing whether a model
  is a managed cloud frontier model, prepaid harness model, metered API model,
  local/free inference server, community/self-hosted copy, or test model.
  Benchmarks alone do not make different deployment classes equivalent.
- **Route candidate** — one discovered `(harness, provider source, endpoint,
  model)` option after joining live inventory with catalog metadata
- **Server instance** — the concrete inference server reached by a candidate,
  derived from endpoint configuration or normalized host/port, such as
  `grendel`, `vidar`, or `sindri`. Stickiness targets server instances, not
  model strings.
- **Sticky route key** — a request sequence identity, normally the validated
  `CorrelationID` or a future worker/session sequence ID, used to bias related
  requests toward the same server instance
- **Endpoint utilization** — a normalized operational signal for one provider
  endpoint, including active requests, queued/deferred requests, concurrency
  when known, cache pressure when known, freshness, and source
- **Prompt preset** — system prompt selection (`preset`, `--preset`); unrelated
  to model policy and routing

## Problem Statement

- **Current situation**: Fizeau can select configured transport endpoints
  directly, while callers and orchestrators still carry duplicated or
  mismatched routing assumptions above it.
- **Pain points**: Prompt presets already occupy the `preset` naming surface,
  provider configs currently mix transport and model concerns, and route tables
  force operators to encode policy that the agent can derive from live model
  inventory and the catalog.
- **Desired outcome**: Fizeau becomes the reusable source of truth for model
  aliases, numeric power, deprecations, deployment-class-aware model metadata,
  and embedded provider selection, while callers keep retry/escalation policy
  and HELIX keeps stage intent only.

## Requirements

### Functional Requirements

#### Phase 1 (P0): Provider Sources and Endpoints

1. `Config` specifies provider sources with concrete `type` values (`openai`,
   `openrouter`, `lmstudio`, `omlx`, `vllm`, `llama-server`, `ollama`,
   `anthropic`, and harness-backed sources where applicable) plus endpoint
   transport/auth data such as base URL, API key, headers, and optional
   endpoint pools. `type: openai-compat` is rejected at config load; use the
   actual model source instead.
2. Endpoint labels may exist for diagnostics, host display, and explicit
   endpoint selection, but stable user-authored endpoint labels are not the
   primary routing API.
3. A provider endpoint may carry a default model hint for direct dispatch, but
   endpoint config is not the canonical source of model aliases, power, or
   routing policy.
4. The CLI can constrain routing by provider source/type and, when necessary,
   by a concrete endpoint selector. Source/type constraints are preferred over
   arbitrary configured endpoint labels.
5. If a caller selects one exact endpoint, all requests go to that endpoint. If
   it fails, the request fails with attempted-route evidence.
6. No retry across endpoints or sources happens inside one agent request.
   Provider SDK retries within one endpoint remain governed by FEAT-003.
7. The harness, provider source, endpoint identity, and model used are recorded
   in the `Result`.

#### Phase 1B (P1): Shared Model Catalog and Manifest

7. Fizeau owns a reusable shared model catalog separate from provider configs
   and prompt presets.
8. The catalog represents:
   - model families
   - numeric power on a 1..10 scale, with `0` reserved for unknown or
     exact-pin-only models
   - optional model aliases for exact model identity and migration
   - canonical policy targets
   - per-model entries for every concrete model eligible for a tier
   - ordered tier candidate lists that can contain multiple concrete models
   - deprecated or stale targets with replacement metadata
   - provider/deployment class and provenance for power assignment
   - consumer-surface mappings where a canonical target needs different
     concrete strings and may carry different reasoning defaults for different
     downstream integrations
   - provider-specific concrete surface IDs on model entries, so a single tier
     can choose among Anthropic, OpenAI-compatible, Codex, or Claude Code model
     strings without duplicating cost and benchmark metadata
   - per-model reasoning capability metadata, including supported named values,
     numeric maximums, and named-to-token maps when a provider/model cannot
     derive safe limits from live metadata
9. Catalog data is stored in a structured manifest maintained separately from
   Go logic inside the agent repo.
10. Fizeau ships an embedded snapshot of that manifest and may also load an
    external manifest override so policy/data can update independently of code
    releases where practical.
11. Fizeau publishes versioned catalog manifests outside normal binary
    releases and exposes a stable machine-readable channel pointer so operators
    and callers can refresh policy more quickly than the binary release cadence.
12. Catalog refresh is explicit. Ordinary request execution must not fetch
    remote manifest data.
13. The Fizeau CLI and any caller can resolve a model reference through the
    catalog to a concrete model string appropriate for the chosen consumer
    surface.
13a. When a profile or canonical target has an ordered candidate list,
     provider-backed routing tests those candidates against the endpoint's
     live model inventory in order. The router chooses the first candidate the
     endpoint advertises instead of failing only because the target's primary
     candidate is absent.
13b. Provider-native model names may include vendor prefixes, casing,
     quantization, accelerator, or packaging suffixes such as
     `Qwen3.6-27B-MLX-8bit`. Catalog metadata lookup must match those names
     back to the intended catalog model when the canonical identity is
     unambiguous, so auto-routing sees the catalog power, context window, tool
     support, and auto-routable status for the selected provider-native ID.
14. Explicit concrete model pins remain supported as hard constraints. If the
    requested model is unavailable, routing fails with detailed evidence rather
    than substituting another model.
15. Ownership split is explicit:
    - agent owns model catalog data/policy and provider-source/endpoint
      selection inside the embedded runtime
    - callers own cross-harness orchestration, semantic retry, and guardrails
    - HELIX owns stage intent only
16. The catalog uses `reasoning_default` for model-reasoning policy.
17. `reasoning_default` is a single scalar using the same value grammar as the
    public CLI/config/API `reasoning` field: `auto`, `off`, `low`, `medium`,
    `high`, supported extended values such as `minimal`, `xhigh` / `x-high`,
    and `max`, or numeric values such as `0`, `2048`, and `8192`.
18. Catalog defaults are keyed to model metadata and numeric power. Profiles
    are allowed only as named shorthands for specific points on the power curve
    or bounded power-policy ranges. They are not routing personas and do not
    resolve to a closed list of models.
    - Explicit caller `reasoning` always wins over tier defaults, including
      supported requests above high such as `xhigh`, `x-high`, or `max`, and
      explicit numeric values.
19. Catalog candidates for numeric-only reasoning providers must publish
    per-model maximums or named-value maps unless the provider can derive safe
    limits from live metadata. The router must fail clearly on explicit
    unsupported or over-limit requests and may drop only auto/default reasoning
    controls for unsupported candidates.
20. Manifest schema v4 stores cost, context, benchmark, OpenRouter ID, and
    surface model strings on top-level `models` entries. Target entries retain
    tier policy only: family, aliases, status/replacement metadata,
    `context_window_min`, `swe_bench_min`, ordered `candidates`, and
    `surface_policy`.
21. Catalog power synthesis uses benchmark inputs when available, but
    benchmark numbers are not sufficient on their own. Cost, recency,
    capability metadata, provider/deployment class, and explicit override
    reason all contribute to power. A local/community/self-hosted copy such as
    `gpt-oss-120b` or `minimax2.7` must not receive the same power as a managed
    cloud frontier Sonnet/Opus/GPT model solely because one benchmark is high.
22. Models with missing or zero power remain inspectable and may be used by
    exact pin when available, but are excluded from unpinned automatic routing.

#### Phase 2A (P1): Candidate Inventory and Power-Based Routing

23. Fizeau builds the candidate set by enumerating available harnesses,
    provider sources, endpoints, and discovered model IDs, then joining that
    live inventory with catalog metadata.
23a. A native embedded-provider harness with configured provider sources must
     not synthesize a providerless fallback candidate when every live provider
     probe fails or returns no models. It must surface no eligible provider
     candidates with rejection evidence instead.
24. The CLI exposes the joined inventory through `fiz --list-models`,
    with JSON support. Rows include model, harness, provider, endpoint/host,
    server instance, power, provider/deployment class, cost, speed/perf signal,
    maximum configured context length, context source,
    availability, catalog reference, auto-routable status, and exact-pin-only
    status.
25. Routing filters candidates by hard caller constraints first:
    - `Model` means only that model identity may be used.
    - `Provider` means only that provider source/type or explicit endpoint
      selector may be used, depending on the request surface.
    - `Harness` means only that harness may be used.
26. `Profile`, `MinPower`, and `MaxPower` filter only unpinned automatic
    routing. A profile expands to an effective power policy before candidate
    filtering. Numeric bounds supplied with a profile further constrain that
    policy. None of these fields widen or override hard model/provider/harness
    pins.
27. When no `MinPower` or `MaxPower` is supplied, automatic routing still uses
    the discovered inventory and selects the best lowest-cost viable
    auto-routable model according to power, cost, availability, speed, context,
    and capability.
28. Automatic routing excludes inactive, deprecated, exact-pin-only, and
    unknown-power models unless the caller explicitly pins that model and live
    discovery confirms it is available.
29. The router ranks survivors by model power, provider/deployment placement,
    effective marginal cost, prepaid quota, availability, maximum configured
    context length, context/capability fit, observed speed/latency, normalized
    utilization, and sticky server-instance affinity.
30. When unpinned local/free LLMs are available and satisfy requested power,
    tools, context, and health constraints, they are preferred over paid cloud
    candidates. This local/free preference never overrides hard pins,
    `MinPower`/`MaxPower`, or capability requirements.
30a. For multiple eligible local/free server instances, routing assigns each
     new sticky route key to the least-loaded eligible server instance. Load
     combines service-owned in-flight route leases with fresh provider-owned
     utilization probes when available. Requests with the same sticky route key
     receive a ranking bonus for that server instance so worker-local cache
     behavior stays stable across related requests, including requests that use
     a different model on the same server.
30b. Sticky affinity never bypasses eligibility. A sticky server instance may
     lose when it no longer satisfies hard pins, power policy, context,
     capability, health, quota, or hard saturation constraints. A merely better
     score on another server instance is not enough to move an existing sticky
     key; severe utilization or performance pressure is enough when the scoring
     policy says the affinity bonus is outweighed.
30c. On a single machine, in-process route leases provide the authoritative
     sticky assignment and fallback load count. On multiple machines, correct
     cross-process stickiness and fair distribution require a shared lease
     backend; server metrics are advisory and racy, not a replacement for
     shared leases. See
     [plan-2026-05-05-shared-lease-backend.md](../../02-design/plan-2026-05-05-shared-lease-backend.md)
     for the lease-record and atomic acquire/refresh/release contract.
30d. Local provider utilization is provider-owned and type-derived where the
     server exposes it. `vllm` uses root `/metrics`. `llama-server` uses root
     `/metrics` when started with `--metrics` and root `/slots` as fallback.
     `omlx` uses root `/api/status` and may optionally consume admin-only
     `/admin/api/stats` when explicitly configured. `rapid-mlx` uses its
     documented status or metrics surface when available and otherwise reports
     unknown utilization with service-owned lease fallback. A configured
     OpenAI-compatible `base_url` ending in `/v1` is converted to the server
     root for these probes. Ollama's documented native surfaces expose request
     timing on `POST /api/chat` or `POST /api/generate` via
     `total_duration`, `load_duration`, `prompt_eval_count`,
     `prompt_eval_duration`, `eval_count`, and `eval_duration`; loaded-model
     inventory and allocated `context_length` come from `GET /api/ps`.
     Ollama's native surfaces do not expose a verified cache-pressure counter,
     so cache remains unknown unless a future probe adds it. LM Studio
     currently has verified native chat performance and model-management
     surfaces, but no live utilization probe, so it stays on the
     unknown/service-owned fallback path until a follow-up probe bead lands.
31. `agent.Run()` still receives one concrete `Provider` per attempt. `Execute`
    selects and dispatches the top candidate once.
32. The selected concrete harness, provider source, endpoint, model, requested
    model input, resolved model reference, profile, effective power policy,
    server instance, maximum configured context length, sticky assignment
    status, endpoint utilization source/freshness, performance signal, and
    score components are recorded in the result/session artifacts when known.
33. Existing `backends`, `default_backend`, `--backend`, and user-authored
    route-table surfaces are deprecated compatibility inputs during migration
    and must emit warnings if still parsed.

#### Phase 2B (P1): Availability Feedback Without Agent Retry

34. Fizeau may track recent provider/endpoint/model availability failures
    and temporarily back off unhealthy candidates using a bounded cooldown
    window.
35. Availability feedback is keyed at least by `(harness, provider source,
    endpoint, model)` so one bad endpoint or model does not poison a whole
    provider source, family, or power band.
36. `Execute` does not try a second candidate after dispatching the selected
    candidate. It returns attempted-route identity and failure class so callers
    can decide whether to issue a new request.
37. Prompt-shape, tool-schema, semantic task failures, test failures, review
    blocks, or low-quality output do not trigger agent-owned retry. Those are
    caller-owned retry/escalation signals.
38. Callers continue to pass model intent (`MinPower`, `MaxPower`, `model_ref`,
    or exact pin) into the embedded harness. Callers must not duplicate inner
    provider-selection logic.

#### Phase 2C (P1): Placement and Quota-Aware Scoring

39. Placement and `surface_policy` catalog data may carry placement order, cost
    ceilings, failure policy, and reasoning defaults. These fields refine
    scoring and filtering within the numeric power/pin constraints.
40. The routing engine uses live quota signals to influence subscription candidate scoring:
    - `QuotaOK` — false when a subscription provider is exhausted.
    - `QuotaPercentUsed` — known usage percentage; applies a penalty when high (>= 80%).
    - `QuotaStale` — applies a penalty when the latest quota probe is older than the configured freshness window.
    - `QuotaTrend` — biases score based on burn rate (`healthy`, `burning`, `exhausting`).
41. Quota and placement signals only affect ranking and filtering within the
    eligible candidate set that satisfies requested power bounds and hard pins.
    They do not trigger automatic semantic escalation.

### Non-Functional Requirements

- **Simplicity**: library users can still pass a concrete `agent.Provider`
  directly with no YAML, catalog, or routing machinery.
- **Clarity**: prompt presets, provider config, model policy, and provider
  routing each use distinct terminology.
- **Boundary safety**: Callers may depend on agent-owned routing for the embedded
  harness, but they express power bounds and hard pins rather than reproducing
  provider candidate logic.
- **Updateability**: rapidly changing model policy/data can be refreshed via an
  external manifest without requiring every consumer to wait for a new Go
  release.
- **Compatibility**: legacy configured endpoint names and route-table inputs,
  when present, are migration-only and not the target architecture.

## Edge Cases and Error Handling

- **Unknown provider source or endpoint selector**: route resolution returns an
  error before dispatch.
- **Unknown model reference**: catalog resolution returns an error before the
  run.
- **Unsatisfied exact model/provider-source/endpoint/harness pin**: route
  resolution returns a no-candidate error with rejected candidates and reasons;
  it does not choose a broader model, source, endpoint, or harness.
- **Unknown or missing-power model**: excluded from unpinned automatic routing;
  exact pins may use it when live discovery confirms availability.
- **Deprecated or stale model reference**: resolution returns metadata that the
  caller may surface as a warning or block according to policy.
- **Manifest missing or unreadable**: fall back to the embedded snapshot unless
  the caller explicitly required the external manifest.
- **Selected provider not reachable**:
  return an attempted-route error containing harness, provider source,
  endpoint, model, and failure class. The agent does not try another candidate
  in that request.
- **Utilization probe unavailable or stale**: keep the endpoint eligible if
  normal model discovery and health checks pass. Use service-owned route leases
  as the load signal and mark utilization source/freshness as unknown or stale
  in route evidence.
- **Sticky server instance unavailable or ineligible**: invalidate or demote
  that sticky assignment and resolve from the currently eligible candidate set.
  Record the reason.
- **Semantic task failure**: caller-owned. DDx or another caller may retry with
  a stronger `MinPower`, capped `MaxPower`, or different pins, but the agent
  does not infer semantic failure or escalate automatically.

## Success Metrics

- Provider source/endpoint config works with LM Studio, Ollama, OpenRouter, and
  Anthropic without making arbitrary configured names the primary route
  primitive.
- Callers can consume agent-owned catalog data without maintaining duplicate alias
  and power tables.
- Operators can run `fiz --list-models` to inspect the same joined model
  inventory the router scores.
- Prompt preset docs and model-policy docs stay terminology-safe and do not
  overload `preset`.
- Power-based routing selects one candidate deterministically and records the
  actual harness/provider-source/endpoint/model used.

## Acceptance Criteria

| ID | Criterion | Suggested Verification |
|----|-----------|------------------------|
| AC-FEAT-004-01 | Provider source/endpoint resolution selects the configured transport before the run starts, and unknown provider source or endpoint selectors fail during config/CLI resolution rather than inside `agent.Run()`. | `go test ./internal/config ./cmd/fiz ./...` |
| AC-FEAT-004-02 | Model references resolve through the embedded or external manifest to the correct consumer-surface model string and per-surface reasoning metadata, and missing references/surfaces fail deterministically before the run. | `go test ./internal/modelcatalog ./internal/config ./cmd/fiz ./...` |
| AC-FEAT-004-03 | Deprecated or stale model references are rejected by default, surface replacement metadata, and can be explicitly allowed only when the caller opts in. | `go test ./internal/modelcatalog ./internal/config ./cmd/fiz ./...` |
| AC-FEAT-004-04 | An explicit concrete `--model`, provider-source/endpoint constraint, or `--harness` is a hard constraint. If it cannot be satisfied, routing returns detailed no-candidate evidence and never substitutes a different model/source/endpoint/harness. | `go test ./internal/routing ./cmd/fiz ./...` |
| AC-FEAT-004-05 | `fiz --list-models` exposes the joined available-model inventory with model, harness, provider, endpoint/host, server instance, power, provider/deployment class, cost, speed/perf, maximum configured context length, context source, availability, catalog reference, auto-routable, and exact-pin-only fields. | `go test ./cmd/fiz ./... -run 'ListModels|Models'` |
| AC-FEAT-004-06 | Automatic routing excludes unknown-power and exact-pin-only models, honors `MinPower`/`MaxPower`, and uses only models with catalog power unless the caller made an exact model pin. | `go test ./internal/modelcatalog ./internal/routing ./...` |
| AC-FEAT-004-07 | The selected harness, provider, endpoint, requested model input, resolved model reference, resolved concrete model, power, and score components are recorded in run result and session artifacts so callers and downstream analytics can attribute the actual embedded-provider choice without reproducing route logic. | `go test ./cmd/fiz ./internal/session ./...` |
| AC-FEAT-004-08 | Deprecated `backends`, `default_backend`, `--backend`, and user-authored route-table inputs still resolve during the migration window if supported, emit a deprecation warning, and do not define the target architecture. | `go test ./internal/config ./cmd/fiz ./...` |
| AC-FEAT-004-09 | Catalog publication produces an immutable versioned manifest bundle plus a stable channel pointer, and ordinary request execution never fetches remote manifest data implicitly. | `go test ./internal/modelcatalog ./cmd/fiz ./...` |
| AC-FEAT-004-10 | The starter shared catalog publishes concrete model entries with 1..10 power, provider/deployment class, power provenance, costs, context, benchmark inputs, surface projections, and profile definitions that expand only to effective power policy. Legacy `code-medium` and `code-high` names remain exact compatibility aliases or are deprecated; they do not define target routing policy. | `go test ./internal/modelcatalog ./internal/config ./cmd/fiz ./...` |
| AC-FEAT-004-11 | Manifest schema uses top-level concrete `models` entries and target-level ordered `candidates`; pricing, OpenRouter refresh, context windows, benchmarks, power, and deployment-class provenance are model-scoped while target entries remain policy. Older manifests load through a compatibility upgrade path. | `go test ./internal/modelcatalog ./...` |
| AC-FEAT-004-12 | Routing policy has statement-backed tests for: local/free preference when constraints are satisfied; hard pins overriding local preference; power bounds overriding local preference; unknown-power exact-pin-only behavior; provider/deployment-class power separation; and no retry of candidate 2 after dispatch failure. | `go test ./internal/routing ./... -run 'Policy|Invariant|Routing'` |
| AC-FEAT-004-13 | Profile and target routing use the ordered catalog candidate list against live provider discovery; if the primary candidate is absent but a later candidate is advertised, the later candidate remains eligible and is scored with catalog metadata. | `go test ./internal/routing ./... -run 'CatalogCandidates|LiveDiscovery|Routing'` |
| AC-FEAT-004-14 | Provider-native model IDs with case, vendor-prefix, quantization, accelerator, or packaging suffix differences, such as `Qwen3.6-27B-MLX-8bit`, fuzzy-match to the intended catalog model for power, context, tool support, and auto-routable metadata without treating unrelated short names as equivalent. | `go test ./internal/modelcatalog ./internal/routing ./... -run 'Fuzzy|ProviderNative|ModelEligibility'` |
| AC-FEAT-004-15 | If all configured provider-backed endpoints are absent from live discovery, the native embedded-provider harness does not produce an empty-provider route candidate and cannot win automatic routing as `provider=""`. | `go test ./... -run 'RouteStatus|Routing|Provider'` |
| AC-FEAT-004-16 | The same validated `CorrelationID` repeatedly receives sticky affinity for the same server instance, even when related requests use different models on that server, until the lease expires or the server instance becomes ineligible. | `go test ./internal/routing ./... -run 'Sticky|Lease|Correlation|Routing'` |
| AC-FEAT-004-17 | Different sticky route keys are distributed across eligible local server instances by normalized load; a saturated server is avoided for new keys, while an existing key stays biased to its server unless eligibility or score pressure outweighs the affinity bonus. | `go test ./internal/routing ./... -run 'Utilization|Sticky|RouteStatus'` |
| AC-FEAT-004-18 | vLLM, llama-server, oMLX, and Rapid-MLX utilization probes feed normalized endpoint utilization into routing where available; stale, missing, or failed probes fall back to service-owned in-flight lease counts without making otherwise healthy endpoints unavailable. | `go test ./internal/provider/... ./internal/routing/... -run 'Utilization|VLLM|Llama|OMLX|RapidMLX|Routing'` |
| AC-FEAT-004-19 | `route-status --json` and session artifacts expose selected endpoint, server instance, sticky assignment state, utilization source/freshness, active/queued counts when known, performance signal, maximum configured context length, and score components used for candidate selection. | `go test ./cmd/fiz ./internal/session ./... -run 'RouteStatus|RoutingDecision|Session'` |

## Dependencies

- **Other features**: FEAT-003 (providers)
- **Governing design**: [Provider Identity, Routing Policy, and Bash Output Filtering](./../../02-design/plan-2026-04-19-provider-routing-tool-output.md)
- **PRD requirements**: P0-3, P1-1, P1-10, P2-4

## Out of Scope

- ML-style prompt classification beyond deterministic inputs such as
  `EstimatedPromptTokens`, `RequiresTools`, and explicit reasoning request.
- Agent-owned semantic retry or automatic model-quality escalation.
- Concurrent multi-model execution (multi-harness quorum is a caller concern).
- Model hosting or lifecycle management.
- HELIX stage-to-model resolution logic.
