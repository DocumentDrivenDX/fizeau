---
ddx:
  id: FEAT-004
  depends_on:
    - helix.prd
    - FEAT-003
    - ADR-009
---
# Feature Specification: FEAT-004 — Shared Model Catalog and Policy Routing

**Feature ID**: FEAT-004
**Status**: Draft
**Priority**: P0 (provider sources/endpoints), P1 (catalog), P1 (routing)
**Owner**: Fizeau Team

## Overview

Fizeau routes requests by evaluating `route(client_inputs, fiz_models_snapshot)`.
Client inputs include policy/profile, pins, `no_remote`, metered opt-in, tools,
context, reasoning needs, and other explicit constraints. The `fiz models`
snapshot is the only source of routing facts: health, quota, model
availability, effective cost, `actual_cash_spend`, billing kind, context/tools/
reasoning support, locality, reliability, latency, utilization, and per-field
freshness.

The snapshot is assembled from configured provider sources and harnesses,
discovered model IDs, catalog metadata, and runtime signals joined into one set
of provider/model facts. The server/refresh coordinator owns background
freshness and refresh coalescing; model discovery and snapshot refresh must not
spawn independent probe storms.

The public v0.11 routing surface is:

- `Policy`: one of `cheap`, `default`, `smart`, or `air-gapped`;
- `MinPower` and `MaxPower`: numeric soft power hints on the 1..10 catalog
  scale;
- hard pins: `Harness`, `Provider`, and exact `Model`.

Deprecated route tables, model reference aliases, compatibility targets, and
surface policy projections are not public routing controls.

This feature spec defines the required routing behavior and public contracts.
`SD-005` owns the implementation sequence, cache mechanics, candidate scoring
formula, and routing trace construction.

## Problem Statement

- Provider config should describe transport/auth, not route policy.
- The catalog should own model metadata, power, cost, deployment class,
  auto-routable status, and provider surface strings.
- Callers need a small, explainable routing vocabulary that avoids accidental
  paid spend but still allows exact pins and explicit escalation.

## Terminology

- **Policy**: a named bundle of power bounds, local allowance, and hard
  requirements.
- **Power**: catalog model-strength integer from 1 to 10. `0` means unknown or
  exact-pin-only.
- **Hard pin**: caller assertion on `Harness`, `Provider`, or exact `Model`.
- **Route candidate**: one `(harness, provider, endpoint, model)` option after
  live discovery and catalog join.
- **Default inclusion**: provider-level `include_by_default`, used only for
  unpinned automatic routing.
- **Metered opt-in**: operator permission for pay-per-token candidates to
  participate in unpinned automatic routing. Provider default inclusion is still
  required.
- **Unpinned request**: a request with no `Harness`, no `Provider`, and no exact
  `Model`. `Policy`, `MinPower`, `MaxPower`, `Reasoning`, capability flags, and
  token estimates do not make a request pinned.
- **Sticky affinity**: a score bonus for reusing a server instance for related
  requests when the candidate is still eligible.

## Requirements

### Catalog and Manifest

1. The manifest schema is v5.
2. Catalog models carry concrete model ID, provider surface strings, power,
   auto-routable/exact-pin-only status, deployment class, cost, context,
   benchmark provenance, capabilities, and reasoning defaults.
3. Catalog policies carry `min_power`, `max_power`, `allow_local`, and
   `require[]`.
4. Catalog providers carry provider `type`, `include_by_default`, and explicit
   billing only when the hardcoded provider-system table cannot infer billing.
5. Removed v0.10 routing concepts (`target`, aliases as routing personas, and
   user-visible `surface_policy`) must not be presented as public routing API.
   Narrow compatibility structs may exist only to keep older internal catalog
   readers working while the primary v5 shape is used.
6. Ordinary execution uses the embedded or configured local manifest. It does
   not fetch manifest updates over the network.

### Canonical Policies

7. The canonical policy set is exactly:

| Policy | MinPower | MaxPower | AllowLocal | Require | Intent |
|--------|----------|----------|------------|---------|--------|
| `cheap` | 5 | 5 | true | none | minimize marginal spend; local/fixed candidates preferred |
| `default` | 7 | 8 | true | none | balanced default; local/fixed or healthy subscription can win |
| `smart` | 9 | 10 | false | none | quality-first; subscription/cloud-capable candidates preferred |
| `air-gapped` | 5 | 5 | true | `no_remote` | local-only execution; remote/account providers rejected |

8. `ListPolicies` returns these canonical entries and manifest metadata. It
   does not list dropped compatibility names.
9. `allow_local=false` disallows local/fixed candidates for that policy unless
   the caller changes policy or requirements.
10. `require[]` currently supports `no_remote`. Unknown requirements fail
    validation instead of being ignored.
11. `no_remote` rejects remote or account-billed candidates even when the
    caller pins a provider or harness.

### Assembled Routing Inventory

12. `ResolveRoute`, `RouteStatus`, and `fiz models` use the same assembled
    snapshot as their routing inventory contract. The router must not maintain a
    second discovery view that can diverge from operator-visible model facts.
    `ResolveRoute` is the public `route(client_inputs, fiz_models_snapshot)`
    contract.
13. The assembled snapshot contains one identity per discovered
    `(provider, model_id)` pair, including harness-as-provider identities for
    subscription harnesses. Catalog-only models do not appear as available
    models unless a configured source actually serves them.
14. Live discovery wins over configured model hints. A configured default model
    is a fallback hint when discovery is unavailable, not a closed inventory.
15. Discovered model IDs may be matched to catalog metadata when the mapping is
    unambiguous. Unknown models remain inspectable and exact-pinnable, but are
    not eligible for unpinned automatic routing.
16. The route decision trace records selected, eligible, and rejected
    candidates with typed reasons. Consumers must use typed fields, not parse
    human-readable reason strings.
17. Test-only harnesses never leak into policy-based routing unless explicitly
    requested.

### Eligibility and Pins

18. Hard pins narrow the candidate set before scoring:
    - `Harness` means only that harness may be used.
    - `Provider` means only that provider source or selected endpoint may be
      used.
    - `Model` means only that exact model identity may be used.
19. Unpinned automatic routing excludes pay-per-token candidates unless the
    provider is included by default and metered routing is explicitly opted in
    by user config.
20. Pins override provider `include_by_default` and metered opt-in: a
    deliberately pinned default-deny pay-per-token provider can be considered.
21. Pins do not override policy `require[]`; `air-gapped` plus a remote
    provider pin fails.
22. Missing-power, inactive, deprecated, and exact-pin-only models are excluded
    from unpinned automatic routing. Exact model pins may still use them when
    the selected harness/provider can serve the model.
23. Hard gates are limited to explicit user constraints and dispatchability:
    pins, `require[]`, `no_remote`, metered opt-in, exact-pin support, and
    whether the candidate can actually be dispatched. Cost, quality, health
    risk, latency, utilization, and power fit are scoring inputs, not broad
    vetoes.

### Power Scoring

24. `MinPower` and `MaxPower` are soft scoring hints, not closed candidate
    lists, once a model has passed auto-routable eligibility.
25. A candidate below `MinPower` receives a stronger penalty than a candidate
    above `MaxPower`. This asymmetric scoring reflects failure risk: too weak
    is more likely to fail the task, while too strong is primarily a cost and
    latency concern.
26. If no power hints are supplied, model power contributes positively to the
    score alongside policy cost/placement preferences.
27. Exact `Model` pins keep exact identity. Policy-derived power bounds are
    still reported as evidence, but they do not substitute a different model.

### Ranking

28. Ranking considers:
    - policy baseline (`cheap`, `default`, `smart`, `air-gapped`);
    - catalog power;
    - provider billing and effective marginal cost;
    - subscription shadow cost using PAYG-equivalent effective cost while
      retaining `actual_cash_spend=false`;
    - subscription quota health and burn-rate prediction;
    - route-health cooldown and observed reliability;
    - context headroom and required capabilities;
    - observed latency/speed;
    - endpoint utilization and saturation;
    - sticky affinity.
29. A qualified candidate is one that passes hard constraints, policy
    requirements, default-inclusion and metered opt-in gates, auto-routability,
    liveness, quota, and dispatchability. Power hints shape ranking inside that
    qualified set rather than replacing exact pins.
30. For a given policy and qualified set, Fizeau prefers the lowest effective
    marginal cost candidate whose power fit is sufficient for the policy intent.
    A zero-cost but substantially underpowered candidate should not beat an
    in-band candidate for routine `default` work solely because it is free. A
    subscription model may have `actual_cash_spend=false` and still carry a
    PAYG-equivalent effective cost for scoring.
31. Local/fixed candidates are preferred by `cheap` and `default` when they are
    eligible and capable. This preference never beats hard pins or
    `require[]`.
32. `smart` prefers higher-capability subscription/cloud routes when healthy
    and allowed.
33. `air-gapped` is local-only through `require=["no_remote"]`.
34. The router dispatches one selected candidate per request. Semantic retry or
    escalation belongs to the caller.

### Status and Evidence

35. `ResolveRoute` returns the selected candidate plus the full candidate
    trace, power policy, sticky evidence, utilization evidence, and the selected
    model's catalog-projected power.
36. `RouteStatus` reports recent decisions, cooldowns, provider reliability,
    sticky assignments, and routing-quality metrics. Routing quality is
    distinct from provider reliability.
37. Session logs and final events record the actual attempted route and failure
    class. They use v0.11 `policy` / `power_policy` fields.
38. When a route succeeds, fails, or rejects candidates, the evidence must be
    explainable from the same assembled snapshot facts exposed by `fiz models`
    plus request-local constraints.

## Acceptance Criteria

| ID | Criterion | Suggested Verification |
|----|-----------|------------------------|
| AC-FEAT-004-01 | The embedded manifest is schema v5 and validates models, policies, providers, billing, and supported requirements. | `go test ./internal/modelcatalog ./...` |
| AC-FEAT-004-02 | `ListPolicies` returns exactly `air-gapped`, `cheap`, `default`, and `smart` with power bounds, `allow_local`, `require[]`, and manifest metadata. | `go test ./... -run ListPolicies` |
| AC-FEAT-004-03 | `cheap`, `default`, `smart`, and `air-gapped` produce the documented local/subscription/remote behavior under representative inventories. | `go test ./internal/routing ./... -run Policy` |
| AC-FEAT-004-04 | Pay-per-token providers are skipped in unpinned automatic routing unless provider default inclusion and metered opt-in both allow them, while explicit pins can select them. | `go test ./... -run 'IncludeByDefault|Metered'` |
| AC-FEAT-004-05 | Pins override default inclusion and metered opt-in but not `require[]`; `air-gapped` plus a remote pin returns `ErrPolicyRequirementUnsatisfied`. | `go test ./internal/routing ./... -run Policy` |
| AC-FEAT-004-06 | Soft power scoring penalizes undershooting `MinPower` more than overshooting `MaxPower` and does not replace an exact model pin. | `go test ./internal/routing ./... -run Power` |
| AC-FEAT-004-07 | Route decisions consume the assembled snapshot, expose typed candidate rejection reasons, score components, selected endpoint/server instance, sticky evidence, and utilization evidence. | `go test ./... -run 'ResolveRoute|RouteStatus|routing_decision|ModelSnapshot'` |
| AC-FEAT-004-08 | Removed v0.10 names are not advertised by policy listing, CLI help, or public service fields. | `go test ./agentcli ./cmd/fiz ./...` |

## Constraints and Assumptions

- Fizeau owns routing inside the embedded runtime; callers own semantic retry
  and cross-harness orchestration strategy.
- Provider configs remain transport/auth definitions.
- Catalog data can be refreshed explicitly, but normal request execution is
  offline with respect to manifest fetching.
- Benchmark inputs inform power, but deployment class and cost prevent local
  community copies from tying managed frontier models solely on one benchmark.

## Dependencies

- `FEAT-003` for provider identity, billing, and default inclusion.
- `FEAT-005` for cost/session projections.
- `FEAT-006` for the CLI surface.
- `ADR-009` for the v0.11 naming and migration decision.
- `ADR-012` for assembled snapshot cache and harness-as-provider identity.

## Out of Scope

- User-authored route tables or per-request fallback chains.
- Automatic learning from routing-quality metrics.
- Network manifest refresh during ordinary execution.
