---
title: Auto-routing
linkTitle: Auto-routing
weight: 20
description: "How fiz picks a (harness, provider, model) per call, fails over, and recovers."
---

## What it solves

A single `fiz run` request usually under-specifies the route. The caller
asks for a *policy* (cheap / default / smart / air-gapped) or pins one
axis (`--model`, `--provider`, `--harness`) and expects the runtime to
fill in the rest, avoid providers that just timed out, exclude providers
that are quota-exhausted, and reuse a previously-good choice when a
correlation key is present. That's auto-routing: the engine that
collapses an under-specified request into a concrete
`(harness, provider, endpoint, model)` decision against live signals.

The engine is **model-first**: it ranks concrete models against the
caller's policy/power bounds and capability requirements, then picks the
best provider that can serve the chosen model. Provider preference
(`local-first`, `subscription-first`) is a tiebreaker, not a primary
axis. See [ADR-006](../architecture/adr/ADR-006/) for the rationale.

## Public surface

### Resolve one request

The single public entry point is
[`FizeauService.ResolveRoute`](https://github.com/easel/fizeau/blob/master/service_routing.go#L24-L130),
returning a
[`RouteDecision`](https://github.com/easel/fizeau/blob/master/service.go#L415-L451)
with the chosen `Harness`, `Provider`, `Endpoint`, `ServerInstance`,
`Model`, plus the full ranked
[`Candidates`](https://github.com/easel/fizeau/blob/master/service.go#L450)
trace (including rejected candidates and their typed
[`FilterReason`](https://github.com/easel/fizeau/blob/master/internal/routing/engine.go#L204)).

Internally it delegates to
[`internal/routing.Resolve`](https://github.com/easel/fizeau/blob/master/internal/routing/engine.go),
which is the single ranking engine; everything else (cooldowns, lease
reuse, escalation) is plumbing around it.

### Failure modes

The engine returns typed errors so callers can branch precisely:

- [`ErrUnknownProvider`](https://github.com/easel/fizeau/blob/master/routing_errors.go#L26),
  [`ErrUnknownPolicy`](https://github.com/easel/fizeau/blob/master/routing_errors.go#L201),
  [`ErrHarnessModelIncompatible`](https://github.com/easel/fizeau/blob/master/routing_errors.go#L144) —
  configuration mistakes the operator must fix.
- [`ErrModelConstraintAmbiguous`](https://github.com/easel/fizeau/blob/master/routing_errors.go#L56) /
  [`ErrModelConstraintNoMatch`](https://github.com/easel/fizeau/blob/master/routing_errors.go#L85) —
  the `--model` pin matched zero or many concrete models.
- [`ErrNoLiveProvider`](https://github.com/easel/fizeau/blob/master/routing_errors.go#L254) —
  the entire policy ladder (cheap → default → smart) lacked any live
  provider that supports the prompt size + tool requirement.
- [`*NoViableProviderForNow`](https://github.com/easel/fizeau/blob/master/routing_errors.go#L288) —
  every otherwise-eligible candidate is *currently* quota-exhausted.
  Carries a `RetryAfter` so DDx-style supervisors pause work instead of
  treating the request as a permanent failure.

### Quota state machine

[`ProviderQuotaStateStore`](https://github.com/easel/fizeau/blob/master/provider_quota_state.go#L31)
tracks each provider as `available` or `quota_exhausted` with a
`RetryAfter` instant. Transitions:

```
available     --MarkQuotaExhausted--> quota_exhausted
quota_exhausted --MarkAvailable----> available
quota_exhausted --(now >= retry_after)--> available  // auto-decay
```

[`ProviderBurnRateTracker`](https://github.com/easel/fizeau/blob/master/provider_burn_rate.go#L28)
maintains a per-provider rolling daily-token window and *predictively*
transitions a provider to `quota_exhausted` before the upstream quota
error fires, when a `daily_token_budget` is configured. This is the
hook that turns observed token usage (see
[Performance tracking](../observability/)) into routing pressure.

### Per-attempt feedback

After every dispatch, the service records the outcome via
[`RecordRouteAttempt`](https://github.com/easel/fizeau/blob/master/service_route_attempts.go#L12),
which feeds
[`internal/routehealth.Store`](https://github.com/easel/fizeau/blob/master/internal/routehealth/store.go).
Failed attempts cool down the (provider, model, endpoint) tuple for
`routing.health_cooldown` (default 60s) so the next ResolveRoute skips
it. This is what makes auto-routing *adaptive* rather than purely
configuration-driven.

### Routing-quality ring

[`internal/routingquality.Store`](https://github.com/easel/fizeau/blob/master/internal/routingquality/store.go)
is a 1024-entry in-memory ring of recent Execute calls and their
overrides. The aggregator
[`computeRoutingQualityMetrics`](https://github.com/easel/fizeau/blob/master/service_routing_quality.go#L132)
produces three first-class numbers (ADR-006 §5):

- **AutoAcceptanceRate** — fraction of requests with no override. The
  headline routing-health number.
- **OverrideDisagreementRate** — fraction of overrides where the user pin
  actually differed from auto's choice on the overridden axis.
- **OverrideClassBreakdown** — pivot of (prompt-feature bucket, axis,
  match) so operators can see *which* requests humans keep overriding.

## Operator surface

### Config (`.fizeau/config.yaml`)

The
[`routing:` block](https://github.com/easel/fizeau/blob/master/internal/config/config.go#L145-L180)
exposes:

| Key | Effect |
|-----|--------|
| `default_model` | Default model-route key when caller passes neither `--model` nor `--provider`. |
| `health_cooldown` | How long a failed candidate is deprioritized (default `60s`). |
| `history_window` | Lookback for scoring healthy candidates. |
| `probe_timeout` | Timeout for provider availability/model probes. |
| `reliability_weight` | Score weight for recent success rate. |
| `performance_weight` | Score weight for observed latency/throughput. |
| `load_weight` | Score weight for recent selection volume (load-balancing). |
| `cost_weight` | Score weight for known cost. |
| `capability_weight` | Score weight for benchmark capability (`swe_bench_verified`). |

Per-provider:
[`daily_token_budget`](https://github.com/easel/fizeau/blob/master/internal/config/config.go#L73)
arms the predictive burn-rate tracker for that provider.

### Env-var overrides

`FIZEAU_PROVIDER`, `FIZEAU_BASE_URL`, `FIZEAU_API_KEY`, `FIZEAU_MODEL`,
plus the sampling pin overrides
`FIZEAU_TEMPERATURE` / `FIZEAU_TOP_P` / `FIZEAU_TOP_K` / `FIZEAU_MIN_P`
([source](https://github.com/easel/fizeau/blob/master/internal/config/config.go#L421-L466))
override config-file values for one process. The bench harness uses
these to inject per-trial samplers without rewriting `config.yaml`.

### CLI

The operator surface for routing lives in three subcommands — see the
auto-generated CLI reference:

- [`fiz route-status`](../cli/fiz_route-status/) — live cooldowns,
  per-candidate health, last decisions, plus routing-quality metrics
  (`--overrides` for the override-class pivot).
- [`fiz providers`](../cli/fiz_providers/) — every configured provider
  and its current quota state.
- [`fiz check`](../cli/fiz_check/) — probe one or all providers (forces
  a `MarkAvailable` on success).
- [`fiz models`](../cli/fiz_models/) — list discovered models with
  routing metadata.

## Examples

Configure two providers with a daily budget on the cloud one:

```yaml
# .fizeau/config.yaml
providers:
  local:
    type: lmstudio
    base_url: http://127.0.0.1:1234/v1
  cloud:
    type: openrouter
    api_key: ${OPENROUTER_API_KEY}
    daily_token_budget: 2000000
routing:
  default_model: qwen3-coder-30b
  health_cooldown: 90s
  reliability_weight: 1.0
  performance_weight: 0.5
  capability_weight: 0.5
```

Inspect live state:

```
$ fiz route-status --json | jq '.routing_quality'
{
  "auto_acceptance_rate": 0.94,
  "override_disagreement_rate": 0.21,
  "total_requests": 312,
  "total_overrides": 19
}
```

A 0.94 acceptance rate over the recent window means humans accepted the
auto choice 94% of the time. The 0.21 disagreement rate inside the
overrides says ~80% of those overrides were redundant (the human pinned
the same thing auto would have picked).

## Where to look next

- Source of truth: [`AGENTS.md`](https://github.com/easel/fizeau/blob/master/AGENTS.md)
  package layout § *Routing, quota, and modeling*.
- Engine internals: [`internal/routing/engine.go`](https://github.com/easel/fizeau/blob/master/internal/routing/engine.go),
  [`internal/routing/score.go`](https://github.com/easel/fizeau/blob/master/internal/routing/score.go),
  [`internal/routing/gating.go`](https://github.com/easel/fizeau/blob/master/internal/routing/gating.go).
- ADRs: [ADR-006](../architecture/adr/ADR-006/) (routing-quality is the
  primary user surface), [ADR-007](../architecture/adr/ADR-007/) (sampling
  resolution chain).
- Sibling page: [Performance tracking](../observability/) — the per-turn
  signals that auto-routing reads back.
