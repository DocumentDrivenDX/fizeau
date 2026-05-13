# Design Plan: Auto-Routing SD-005 Implementation Gap

**Date**: 2026-05-12
**Status**: CONVERGED
**Refinement Rounds**: 4

**2026-05-13 amendment:** Freshness policy changed after this plan converged.
Fizeau does not have a required daemon. Autorouting now refreshes stale
routing-relevant snapshot fields synchronously through the ADR-012
lock-coordinated refresh path before scoring. `fiz models` remains quick by
default; DDx server freshness is an optional heartbeat over the same Fizeau
refresh API, not a dependency for route correctness.

## Problem Statement

SD-005 now defines auto-routing as a service-owned selection process over the
same assembled available-model snapshot exposed by `fiz models`. The current
implementation has most of the routing engine mechanics, but it does not use one
canonical snapshot end to end. `fiz models` reads `internal/modelregistry`
through ADR-012 cache semantics, while `ResolveRoute`, `RouteStatus`, and root
`ListModels` still assemble their own views from service registry metadata,
direct probes, and configured provider defaults.

The user-visible risk is explainability drift: an operator can inspect one model
inventory in `fiz models`, while routing may have accepted, rejected, or ranked
candidates from a different inventory. Closing the gap makes routing decisions
auditable from one source of truth.

## Requirements

### Functional

- `ResolveRoute`, `Execute`, `RouteStatus`, root `ListModels`, and `fiz models`
  must consume the same assembled model snapshot for provider/model inventory.
- Snapshot rows must contain enough identity to expand into dispatchable
  candidates: harness, provider source, endpoint/server instance, model ID, and
  catalog/runtime metadata.
- Hard pins must remain constraints: `Harness`, `Provider`, and exact `Model`
  are never relaxed by scoring.
- `Policy`, `MinPower`, and `MaxPower` must shape eligibility and score as
  SD-005 describes, with power bounds treated as scoring hints rather than hard
  range filters for otherwise eligible candidates.
- Rejected candidates must carry typed filter reasons and the facts needed to
  explain the rejection through route evidence plus `fiz models`.
- Dispatch remains single-attempt per request. Retry/escalation stays caller-owned.

### Non-Functional

- `fiz models` must keep the quick stale-while-revalidate inspection behavior
  and avoid blocking on slow model discovery by default.
- `ResolveRoute` and `Execute` must call `ensureFreshEnough(client_inputs,
  snapshot)` and may block on routing-relevant stale fields before scoring.
  Force-refresh behavior is still explicit for CLI inspection through
  `fiz models --refresh` and `fiz models --refresh-all`.
- The adapter from snapshot to routing candidates must be deterministic and
  tested without real provider credentials.
- Existing public request fields and session/event contracts must remain
  backward compatible unless a governing contract explicitly changes them.

### Constraints

- Keep root package public contracts in `fizeau`; concrete discovery/cache
  mechanics remain under `internal/`.
- Use Go stdlib plus existing project packages.
- Use `testing` stdlib and existing focused tests; broaden to integration-style
  tests only where crossing service, cache, and CLI boundaries.

## Current Gap

### Already Implemented

- `ResolveRoute` delegates to `internal/routing.Resolve` and emits a full
  candidate trace with typed filter reasons.
- The routing engine already supports hard provider/harness/model constraints,
  policy requirements, IncludeByDefault gates, context/tool/reasoning gates,
  route-attempt cooldowns, quota signals, endpoint utilization, sticky affinity,
  and score components.
- `fiz models` uses `internal/modelregistry.AssembleWithOptions` with
  discovery/runtime cache semantics.
- `KnownModel` already carries catalog enrichment, auto-routable/exact-pin-only
  state, runtime status, quota remaining, and recent latency.

### Missing Or Divergent

- `ResolveRoute` builds `routing.Inputs` through `service_routing.go` instead of
  reading `modelregistry.ModelSnapshot`.
- Root `ListModels` still performs inline discovery and rendering-specific
  enrichment, separate from the `fiz models` snapshot path.
- `RouteStatus` still groups configured provider defaults, not the assembled
  snapshot rows or the router's expanded candidates.
- `KnownModel` is only `(provider, model)` identity today. SD-005 needs endpoint
  and server-instance identity for multi-endpoint routing and evidence.
- Snapshot assembly handles CLI `internal/config.Config`, while service routing
  consumes the public `ServiceConfig` interface. There is no shared adapter that
  lets root service code assemble the same snapshot without importing CLI-only
  concerns.
- The routing engine's score component names are close but not yet aligned to
  SD-005's public evidence vocabulary. `power_hint_fit`, effective cost,
  availability/staleness, and latency are present only indirectly through
  existing `power`, `cost`, `quota_health`, and `performance` buckets.
- `RouteStatus` and error evidence do not yet prove that every route decision can
  be explained from the same snapshot facts exposed by `fiz models`.

## Architecture Decisions

### Decision 1: Snapshot Is The Routing Inventory Boundary

- **Question**: Should routing keep assembling `routing.Inputs` directly, or
  consume `modelregistry.ModelSnapshot`?
- **Alternatives**: Keep direct assembly and duplicate fixes; move all route
  metadata into `routing.Inputs`; make `modelregistry` the inventory boundary.
- **Chosen**: Make `modelregistry.ModelSnapshot` the inventory boundary and add
  a narrow adapter to `routing.Inputs`.
- **Rationale**: This is the simplest way to satisfy the SD-005 invariant that
  operators and routing inspect the same available-model facts.

### Decision 2: Enrich Snapshot Identity Before Rewiring Routing

- **Question**: Should `ResolveRoute` consume the current `(provider, model)`
  snapshot immediately?
- **Alternatives**: Adapt now and patch endpoint gaps later; first add endpoint
  identity to `KnownModel`.
- **Chosen**: Extend the snapshot identity first.
- **Rationale**: Routing candidates are dispatch tuples, not just model rows.
  Adapting too early would either lose multi-endpoint behavior or keep a second
  endpoint-discovery path alive.

### Decision 3: Preserve The Existing Routing Engine

- **Question**: Does the engine need a rewrite?
- **Alternatives**: Rewrite around snapshot rows; keep engine and add a snapshot
  adapter.
- **Chosen**: Keep `internal/routing.Resolve` and feed it from a new adapter.
- **Rationale**: The engine already owns the correct hard-pin/filter/rank shape.
  Replacing its input source is lower risk than reimplementing routing policy.

### Decision 4: Root Service Gets A Snapshot Provider Abstraction

- **Question**: How should root service code call modelregistry without tying
  itself to `agentcli`?
- **Alternatives**: Import CLI helpers; duplicate config conversion; add a small
  internal service snapshot helper.
- **Chosen**: Add an internal helper that accepts `ServiceConfig`, catalog, cache
  root, and refresh policy, then returns a `ModelSnapshot`.
- **Rationale**: This keeps public contracts stable while removing the duplicate
  discovery paths.

## Interface Contracts

- `ModelFilter` should map to `modelregistry.AssembleWithOptions` refresh modes:
  default stale-while-revalidate, explicit refresh, and no-refresh.
- `ModelInfo` should be rendered from `KnownModel` rather than inline discovery.
- Routing evidence should expose both candidate score components and candidate
  snapshot facts: provider, endpoint/server instance, status, auto-routable
  state, exclusion reason, power, cost, context, quota, latency, and utilization.
- RouteStatus should group candidates by route-relevant model identity from the
  snapshot, not by configured provider defaults alone.

## Data Model

Extend `modelregistry.KnownModel` with endpoint and dispatch identity:

- `Harness`
- `ProviderType`
- `EndpointName`
- `EndpointBaseURL`
- `ServerInstance`
- `Billing`
- `IncludeByDefault`
- optional capability fields not already represented by catalog/runtime metadata

Keep canonical identity as `(provider, model_id)` for the operator-facing
snapshot, but allow multiple endpoint-backed rows when a provider has multiple
server instances serving the same model. The routing adapter expands each row
into the exact `(harness, provider source, endpoint/server instance, model)`
candidate that can be dispatched.

## Error Handling

- Snapshot assembly failures are source-local whenever possible. A failed source
  should produce source metadata and unavailable candidates rather than aborting
  all routing.
- Force-refresh failures should be visible in `fiz models --refresh` and route
  evidence. Autorouting should only use stale routing-relevant data after the
  coordinated refresh path has failed or timed out and the route evidence
  records that freshness state.
- Hard-pin misses return exact-constraint errors and candidate evidence; they do
  not suggest unrelated models.
- No-candidate errors must report whether candidates were rejected by policy,
  default inclusion, auto-routability, liveness, capability, quota, or hard pins.

## Security

- Snapshot rows must not expose API keys, raw auth headers, or secret-bearing
  endpoint URLs beyond the already configured display/base URL behavior.
- Cache files must continue to use existing safe filesystem and atomic-write
  behavior; no route decision should write arbitrary caller-controlled paths.
- Error messages should include provider/model identities and source status, not
  credential material or full request payloads.

## Test Strategy

- **Unit**:
  - modelregistry endpoint/server-instance assembly from multi-endpoint config.
  - snapshot-to-routing adapter preserves hard pins, IncludeByDefault, power,
    context, cost, runtime quota/latency, endpoint load, and status.
  - power hints affect score asymmetrically without filtering otherwise eligible
    candidates.
  - typed filter reasons are assigned at the rejection sites.
- **Integration**:
  - root `ListModels` and `fiz models --json` produce equivalent rows from a
    fixture config/cache.
  - `ResolveRoute` and `RouteStatus` consume the same fixture snapshot and
    expose explainable candidate evidence.
  - `fiz models` default mode returns stale cache quickly; route-time freshness
    waits for routing-relevant fields through the coordinated refresh path.
- **E2E**:
  - with two local OpenAI-compatible fixture endpoints serving overlapping
    models, `fiz models`, `ResolveRoute`, `RouteStatus`, and `Execute` agree on
    selected provider/endpoint/model and rejection evidence.

## Implementation Plan

### Dependency Graph

1. Snapshot identity and service adapter
2. Root ListModels migration
3. Routing input adapter
4. RouteStatus/evidence migration
5. Score vocabulary and acceptance hardening
6. Real-install/concurrency verification

### Issue Breakdown

#### 1. Snapshot Identity: Endpoint And Harness Metadata

Add endpoint/server-instance/harness/provider-type fields to `KnownModel` and
teach `modelregistry.AssembleWithOptions` to emit rows for every configured
endpoint that serves a model.

Acceptance:
- Multi-endpoint config yields distinct rows for the same `(provider, model)`
  with different endpoint/server-instance fields.
- Subprocess harness rows carry harness-as-provider identity.
- Existing `fiz models --json` includes the new fields without losing current
  runtime quota/latency fields.

#### 2. Service Snapshot Assembly Helper

Add an internal helper that assembles `ModelSnapshot` from root `ServiceConfig`
without importing `agentcli`.

Acceptance:
- Fixture `ServiceConfig` and equivalent `internal/config.Config` produce the
  same snapshot rows.
- Cache root and refresh policy are injectable.
- No API key/header values appear in serialized snapshot or errors.

#### 3. Root ListModels Uses ModelSnapshot

Replace inline root `ListModels` discovery with snapshot-backed rendering.

Acceptance:
- Existing root `ListModels` tests pass after updating expected metadata.
- A regression test proves root `ListModels`, `agentcli models --json`, and
  `modelregistry.AssembleWithOptions` agree on provider/model/endpoint identity.
- Inline `discoverAndRankModels` in root service is deleted or reduced to a
  compatibility wrapper around the shared path.

#### 4. ResolveRoute Consumes Snapshot-Derived Inputs

Build `routing.Inputs` from `ModelSnapshot` rows plus existing service-local
signals for route attempts, endpoint utilization, sticky leases, and quota state.

Acceptance:
- `ResolveRoute` no longer calls live provider discovery directly in default
  mode when fresh cache exists.
- Unknown snapshot models are exact-pinnable but excluded from unpinned routing.
- Provider/model/harness pins still validate and constrain without substitution.
- Candidate traces include snapshot-derived status/exclusion facts.

#### 5. RouteStatus Uses The Same Candidate Inventory

Rebuild `RouteStatus` from the snapshot-derived candidate inventory and cached
last decisions.

Acceptance:
- `RouteStatus` candidates match the same providers/endpoints/models visible in
  `fiz models --json`.
- Cooldown, quota, and last-decision fields still appear.
- RouteStatus has a regression test for multi-endpoint provider evidence.

#### 6. Score Evidence Vocabulary Alignment

Align score component names and public evidence with SD-005:
`power_weighted_capability`, `power_hint_fit`, `latency_weight`,
`placement_bonus`, `quota_bonus`, `marginal_cost_penalty`,
`availability_penalty`, and `stale_signal_penalty`.

Acceptance:
- Candidate score components sum to the candidate score.
- Below-`MinPower` penalty is stronger than above-`MaxPower` penalty.
- Existing policy behavior is preserved unless tests explicitly document a
  designed change.

#### 7. End-To-End Routing Snapshot Contract

Add a focused contract suite proving `fiz models`, `ResolveRoute`,
`RouteStatus`, and `Execute` agree over one snapshot.

Acceptance:
- Fixture with overlapping multi-endpoint models proves selected route evidence
  is explainable from `fiz models --json`.
- Stale-while-revalidate and force-refresh behavior are covered.
- `go test ./internal/modelregistry/... ./internal/routing/... ./agentcli/... .`
  passes.

## Risk Register

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Root service and CLI config models diverge | Medium | High | Build a small adapter with fixture parity tests before rewiring routing |
| Multi-endpoint identity breaks existing provider pins | Medium | High | Preserve provider source name and endpoint selector separately; add pin tests |
| Cache staleness causes surprising routing | Medium | Medium | Surface source freshness in candidate evidence and keep explicit refresh mode |
| Score renaming changes route order accidentally | Medium | High | First add component aliases/tests, then refactor names with golden route-order tests |
| Live integrations are flaky or credential-dependent | High | Medium | Use local fixture servers for contract tests; keep real-install verification separate |

## Observability

- Routing-decision events should include snapshot generation time and source
  staleness/error metadata for rejected and selected candidates.
- RouteStatus should expose whether evidence came from fresh cache, stale cache,
  configured fallback, or live runtime signals.
- Tests should assert the evidence fields rather than parsing free-form reasons.

## Governing Artifacts

- `docs/helix/01-frame/features/FEAT-004-model-routing.md`
- `docs/helix/02-design/solution-designs/SD-005-provider-config.md`
- `docs/helix/02-design/adr/ADR-012-model-discovery-runtime-cache.md`
- `docs/helix/02-design/contracts/CONTRACT-003-routing.md`
