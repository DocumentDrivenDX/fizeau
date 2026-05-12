---
ddx:
  id: ADR-011
  depends_on:
    - ADR-005
    - ADR-006
    - ADR-007
---
# ADR-011: Cost-Based Routing With Quota Pools

| Date | Status | Deciders | Related | Confidence |
|------|--------|----------|---------|------------|
| 2026-05-12 | Proposed | Fizeau maintainers | `ADR-005`, `ADR-006`, `ADR-007`, `fizeau-c04be6b0`, `fizeau-d18e11f5` | Medium |

## Context

ADR-005 established automatic routing over a joined candidate inventory. ADR-006
framed explicit harness, provider, and model pins as override signals rather
than the normal operating mode. ADR-007 moved generation defaults into the
catalog so callers do not compensate for missing policy with ad hoc request
knobs. ADR-009 later tightened the public surface around policies, numeric
power bounds, and hard pins.

The remaining routing gap is cost. For a given required model power, the router
should prefer the cheapest qualified candidate that is likely to complete the
request. "Cheapest" cannot mean list price alone because Fizeau can dispatch
through several billing shapes:

- subscription harnesses, where marginal cost is effectively prepaid until a
  quota pool is exhausted;
- per-token APIs, where each request burns metered input and output tokens;
- local or fixed-cost providers, where marginal cost is zero but only if the
  provider is eligible for automatic routing;
- provider surfaces excluded from default routing by `IncludeByDefault`, where
  accidental spend or special-purpose endpoints must be impossible unless the
  caller explicitly pins them.

Recent implementation beads provide two pieces this ADR can rely on without
specifying new code in this bead. `fizeau-c04be6b0` added catalog
`quota_pool` schema and the default semantic that missing pool values derive
from the provider system. `fizeau-d18e11f5` wired configured provider
`IncludeByDefault` into routing eligibility, so excluded providers are removed
from default candidate sets and remain reachable only through explicit pins.

Operator direction on 2026-05-11 was: for a given model power, choose
intelligently based on cost, and let remaining usage quota influence that cost.
This ADR defines that policy. Implementation follows in a separate epic after
the ADR is accepted.

## Decision

### Candidate Eligibility Comes Before Cost

The router first builds the joined inventory described by ADR-005 and filters it
before any cost ranking:

1. Apply hard pins for harness, provider, and exact model identity.
2. Drop candidates that fail policy requirements, capability requirements,
   health, context, tool, reasoning, or catalog auto-routing gates.
3. Apply power bounds: `MinPower <= candidate.power <= MaxPower` when a maximum
   is present, or `candidate.power >= MinPower` otherwise.
4. Apply `IncludeByDefault`: a provider or catalog entry with
   `IncludeByDefault: false` is absent from the default candidate set. This is a
   hard filter, not a cost penalty. Explicit pins may bypass this gate exactly
   as specified by ADR-009 and implemented by `fizeau-d18e11f5`.
5. Apply quota-pool exhaustion: when a quota pool is known exhausted, drop every
   candidate in that pool.

Only surviving candidates receive cost scores. This preserves the ADR-006
principle that manual pins are override signals and prevents price scoring from
accidentally making an excluded provider "almost eligible."

### Effective Cost Formula

Effective cost is request-local and expressed in normalized dollars so
subscription, metered, and local candidates can be compared in one ranking.

For per-token APIs:

```text
effective_cost =
  cost_input_per_m  * estimated_input_tokens  / 1_000_000
+ cost_output_per_m * estimated_output_tokens / 1_000_000
```

For local or fixed-cost providers, `effective_cost = 0` after eligibility
filters. They still lose to higher-capability candidates when power bounds or
policy requirements demand that, and they still carry latency, reliability, and
health signals outside this ADR's cost term.

For subscription candidates, the router computes the same nominal per-request
metered cost when catalog price data exists, then discounts it by quota
fraction:

```text
quota_fraction = remaining_quota / quota_limit

if quota_fraction <= 0:
  drop the quota pool
if quota_fraction >= 0.20:
  effective_cost = 0
else:
  effective_cost = nominal_metered_cost * (1 - quota_fraction / 0.20)
```

This quota fraction mapping is deliberately simple. Healthy prepaid quota is
free at the margin. The final 20 percent of a pool is still usable, but it
acquires a linear scarcity cost so another qualified subscription pool, a local
candidate, or a cheap metered API can win before the pool reaches zero. When
the catalog lacks a comparable per-token price for a subscription model, the
router uses the cheapest known per-token model in the same provider family and
power band as the nominal cost proxy; if no proxy exists, the scarcity cost is
`0` until the pool is exhausted.

### Quota Pool Semantics

Models belong to quota pools. A catalog `quota_pool` value names an explicit
pool; otherwise the effective pool is the provider system, matching
`fizeau-c04be6b0`.

Pool exhaustion is a hard candidate-set event. If the main OpenAI Codex pool is
exhausted, every model in that pool is dropped together. A model in a different
pool, such as `gpt-5.3-codex-spark` in `openai-codex-spark`, remains eligible
when it satisfies the caller's power floor and other filters, even if it is not
the newest model in the family. This intentionally enables "use spark when the
main pool is empty" without encoding that fallback as a version-newest rule.

Quota signals are consumed in this priority order:

1. explicit subscription usage endpoints, when available;
2. HTTP rate-limit or quota-exhaustion response headers, including the
   `quota_exhausted` signal already plumbed by commit `7776890e`;
3. in-memory recent-rejection rate for the same quota pool.

Known exhaustion wins over stale positive data. Unknown quota is not treated as
exhausted; it is scored as scarce only when recent rejection evidence indicates
probable depletion.

### Quota Window Handling

Quota window handling is v1 local and greedy. The router uses quota available
now and does not reserve quota for later requests, future sessions, or
higher-value work. Reset time may appear in trace output and may clear a stale
exhaustion state when observed, but it does not create a reservation policy.
The quota window rule is therefore "available now," not "save for later."

This keeps routing deterministic and avoids a scheduler hidden inside the
router. Per-tenant, per-key, and priority reservation behavior is future scope.

### Token Estimate Source

The token estimate source for v1 is the request itself plus a coarse heuristic:

- use caller-provided `EstimatedPromptTokens` when present;
- otherwise estimate input tokens from system prompt, retained transcript,
  user prompt, and tool schemas using the same tokenizer when a provider
  tokenizer is available;
- fall back to `ceil(bytes / 4)` when no tokenizer is available.

Estimated output tokens come from the request's explicit max-output setting
when present. Otherwise v1 uses a fixed default budget by power band:

| Candidate power | Estimated output tokens |
|---|---:|
| 1-4 | 2,048 |
| 5-7 | 4,096 |
| 8-10 | 8,192 |

The estimate is intentionally conservative enough for ranking and not intended
to predict final billing exactly. Actual usage continues to be recorded from
provider responses for reporting and future tuning.

### Ranking and Tie-Breaking

Among eligible candidates, choose the minimum effective cost that satisfies the
caller power floor. This ADR makes "cheapest qualified candidate" the primary
rule, not "most powerful available candidate."

Tie-breaking is deterministic:

1. prefer subscription candidates over pay-as-you-go candidates when effective
   cost is equal;
2. prefer local or fixed-cost candidates over pay-as-you-go candidates when
   effective cost is equal and capability requirements are still satisfied;
3. prefer the lower-power candidate when both exceed `MinPower`;
4. prefer healthier and lower-latency candidates using ADR-005 route-health
   signals;
5. preserve stable catalog order only as the final tie-break.

### Fallback Chain On Mid-Request Exhaustion

The fallback chain is caller-owned retry, consistent with ADR-005. The service
dispatches the top-ranked candidate once. If dispatch fails because the quota
pool becomes exhausted mid-request or the provider returns a quota/rate-limit
error, the service records the pool exhaustion signal, emits the failed
attempt's trace, and returns the error.

The caller may issue a new request. On that next routing pass, the exhausted
quota pool is filtered out and the next cheapest eligible pool can win. The
router does not silently retry an alternate model inside the same service
request because that would hide cost, duplicate side effects, and blur the
operator-visible route decision.

## Consequences

### Positive

- Cost routing now reflects the real marginal economics of subscriptions:
  healthy quota is cheap, scarce quota is protected, and exhausted pools are
  removed.
- Quota pools provide an explicit mechanism for fallback across independent
  subscription allocations without relying on model recency.
- `IncludeByDefault` composes cleanly with cost ranking because excluded
  providers never enter the default ranked set.
- The routing trace can explain cost decisions with concrete inputs:
  estimated tokens, price data, quota fraction, quota pool, and filter reason.

### Negative

- The quota fraction threshold is a policy choice, not an empirical optimum.
  It should be tuned after route traces show real depletion behavior.
- Coarse token estimates can mis-rank close candidates. This is acceptable for
  v1 because actual billing remains visible and estimates can improve without
  changing the public contract.
- Subscription models without comparable catalog pricing cannot express
  scarcity cost until proxy data exists; they remain zero-cost until exhausted.
- No within-request retry means callers may see one quota error before fallback
  takes effect on the next request.

## Out of scope

- Runtime implementation of the scorer, candidate trace fields, or quota-pool
  filters. Those belong to the follow-up implementation epic.
- New quota signal infrastructure beyond existing rate-limit/header plumbing
  and future beads for subscription usage endpoints.
- Per-tenant, per-key, or priority quota reservations.
- Learning the quota fraction mapping from historical usage.
- Counterfactual dispatch to measure whether a more expensive candidate would
  have completed better.

## References

- `ADR-005` — power-based automatic routing, candidate inventory, scoring, and
  caller-owned retry.
- `ADR-006` — manual pins are override signals, not the primary routing mode.
- `ADR-007` — catalog-owned generation policy; this ADR follows the same
  catalog-policy principle for cost and quota metadata.
- `ADR-009` — v0.11 routing surface, billing classification, and
  `IncludeByDefault` composition.
- `fizeau-c04be6b0` — catalog `quota_pool` field schema and effective-pool
  default semantics.
- `fizeau-d18e11f5` — provider `IncludeByDefault` routing eligibility filter.
- Commit `7776890e` — existing `quota_exhausted` signal plumbing from HTTP
  rate-limit response handling.
