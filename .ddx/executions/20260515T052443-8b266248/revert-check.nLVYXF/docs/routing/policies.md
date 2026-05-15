# Routing Policies and Power Bounds

Policies are named routing-intent bundles. They expand to effective power
policy and hard requirements, not to a closed set of concrete models.

Canonical policies:

| Policy | MinPower | MaxPower | AllowLocal | Require | Intent |
|--------|----------|----------|------------|---------|--------|
| `cheap` | 5 | 5 | true | none | minimize effective cost; local/fixed candidates preferred |
| `default` | 7 | 8 | true | none | balanced default; local/fixed or healthy subscription can win |
| `smart` | 9 | 10 | false | none | quality-first; subscription/cloud-capable candidates preferred |
| `air-gapped` | 5 | 5 | true | `no_remote` | local-only execution; remote/account providers rejected |

Use `--policy` when the caller wants a project-maintained routing intent. Use
`--min-power` and `--max-power` for explicit numeric power hints. Use
`--model`, `--provider`, and `--harness` only as hard pins.

`fiz policies` lists the canonical policies and their manifest metadata.
`fiz models --format json` shows live model inventory, power, billing, availability,
auto-routable state, and exact-pin-only state.

Power hints are soft once a model has passed automatic-routing eligibility.
Undershooting `MinPower` carries a larger score penalty than overshooting
`MaxPower`; a too-weak model is more likely to fail, while a too-strong model is
mainly a cost/latency tradeoff. Routing also considers the snapshot's
effective_cost, actual_cash_spend, quota, reliability, latency, and utilization.
Subscription candidates score with a PAYG-equivalent effective cost while
retaining `actual_cash_spend=false`.

`cheap` is not implemented as a list of frontier-model exclusion gates. It
scores all dispatchable candidates and selects the lowest effective-cost model
with enough expected capability. A maximum-quality frontier subscription model
should lose to nano, mini, local, or fixed-cost candidates when those cheaper
candidates satisfy the request constraints and policy intent.

Provider `include_by_default` only affects unpinned automatic routing. A request
is unpinned when it has no `--harness`, no `--provider`, and no exact `--model`;
policy, power, reasoning, capability, and token-estimate fields are not pins.
Pay-per-token providers are excluded from unpinned automatic routing unless the
provider is included by default and metered routing is explicitly enabled, for
example with `routing.allow_metered: true`, because unpinned dispatch would
otherwise create actual metered spend without consent. Pins can consider
excluded or metered providers, but pins do not bypass policy requirements. For
example, `--policy air-gapped --provider openrouter` fails because the policy
requires `no_remote`.

Sticky affinity is keyed by validated `CorrelationID` and targets the server
instance, such as `grendel` or `vidar`, rather than a model string. Related
requests bias toward the same server instance to preserve cache locality, but
that affinity is a score component, not a pin.
