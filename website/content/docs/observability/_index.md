---
title: Performance tracking
linkTitle: Performance tracking
weight: 21
description: "Per-turn timing, token usage, cost, and the OpenTelemetry surface that downstream tools read back."
---

## What it solves

Fizeau treats round-trip timing as a first-class output. Every turn
emits a structured record — request, response, tool call, session start
and end — with token counts, latency, cost, and (for streaming
providers) per-chunk delta timing. Downstream code reads these records
to compute throughput, attribute cost, and feed signals back into
[auto-routing](../routing/) so the next decision incorporates the last
one's outcome.

The point is not "we collect metrics." The point is that the *measurement
chain* is the public surface — public events, public projections, an
OpenTelemetry semantic-convention span — so embedders never need to
parse harness-native streams or raw JSONL to learn what happened.

## Public surface

### Per-turn JSONL session log

The session logger
([`internal/session/logger.go`](https://github.com/easel/fizeau/blob/master/internal/session/logger.go#L56))
writes one JSONL line per event into `<SessionLogDir>/<session-id>.jsonl`.
Event payloads are stable, versioned types defined in
[`internal/session/event.go`](https://github.com/easel/fizeau/blob/master/internal/session/event.go):

| Event | Fields that matter for performance |
|-------|------------------------------------|
| `session.start` | `selected_provider`, `selected_endpoint`, `resolved_model`, `sticky.*`, `utilization.*` ([source](https://github.com/easel/fizeau/blob/master/internal/session/event.go#L11-L33)) |
| `llm.request`   | `temperature`, `top_p`, `top_k`, `min_p`, `seed`, `cache_policy`, `sampling_source` ([source](https://github.com/easel/fizeau/blob/master/internal/session/event.go#L36-L56)) |
| `llm.response`  | `usage.{input,output,total}`, `cost_usd`, `latency_ms`, `finish_reason` ([source](https://github.com/easel/fizeau/blob/master/internal/session/event.go#L59-L67)) |
| `tool.call`     | `duration_ms`, `output`, `error` ([source](https://github.com/easel/fizeau/blob/master/internal/session/event.go#L70-L76)) |
| `session.end`   | aggregated `tokens`, `cost_usd`, `duration_ms`, full route/sticky/utilization snapshot ([source](https://github.com/easel/fizeau/blob/master/internal/session/event.go#L79-L102)) |

The lifecycle wrapper
[`serviceSessionLog`](https://github.com/easel/fizeau/blob/master/service_session_log.go#L20)
writes `session.end` exactly once even when the run fails partway
through.

### Aggregated projection

The service-owned projection
[`UsageReport`](https://github.com/easel/fizeau/blob/master/service_session_projection.go#L34)
folds historical session logs into per-(provider, model) rows. Rows
expose computed accessors so callers don't re-derive them:

- `SuccessRate()` — per-provider reliability rate
- `CostPerSuccess()` — known cost ÷ successful sessions
- `InputTokensPerSecond()` / `OutputTokensPerSecond()` — throughput
- `CacheHitRate()` — cached-input / total-input fraction

The report also carries a
[`RoutingQuality`](https://github.com/easel/fizeau/blob/master/service_routing_quality.go#L19)
block (auto-acceptance rate, override-class breakdown), so a single
`UsageReport` covers both *what happened* and *how the routing
performed* over the same window.

### OpenTelemetry surface

When `telemetry:` is configured, every chat call produces an
[`invoke_agent` / `chat` / `execute_tool`](https://github.com/easel/fizeau/blob/master/telemetry/telemetry.go#L25-L29)
span tagged with stable semantic-convention keys:

- Standard GenAI keys (`gen_ai.usage.input_tokens`,
  `gen_ai.usage.output_tokens`, `gen_ai.request.model`,
  `gen_ai.response.model`, …).
- DDX timing keys
  ([`telemetry.go`](https://github.com/easel/fizeau/blob/master/telemetry/telemetry.go#L56-L61)):
  `ddx.timing.first_token_ms` (TTFT),
  `ddx.timing.queue_ms`,
  `ddx.timing.prefill_ms`,
  `ddx.timing.generation_ms`,
  `ddx.timing.cache_read_ms`,
  `ddx.timing.cache_write_ms`.
- DDX cost keys
  ([`telemetry.go`](https://github.com/easel/fizeau/blob/master/telemetry/telemetry.go#L46-L55)):
  `ddx.cost.amount`, `ddx.cost.input_amount`,
  `ddx.cost.output_amount`, `ddx.cost.cache_read_amount`,
  `ddx.cost.cache_write_amount`, `ddx.cost.pricing_ref`,
  `ddx.cost.source`.

### Feedback into routing

Token counts also flow through
[`observeTokenUsage`](https://github.com/easel/fizeau/blob/master/provider_burn_rate.go#L94)
into the
[`ProviderBurnRateTracker`](https://github.com/easel/fizeau/blob/master/provider_burn_rate.go#L28).
When projected end-of-day usage exceeds the configured
`daily_token_budget`, the tracker pre-emptively transitions the
provider to `quota_exhausted` — without waiting for the upstream 429.
This loop makes performance tracking actionable rather than purely
diagnostic.

## Operator surface

### Config (`.fizeau/config.yaml`)

| Key | Effect |
|-----|--------|
| `session_log_dir` | Where per-session JSONL is written. Defaults to `.fizeau/sessions/`. |
| `telemetry.enabled` | Toggle OTel span emission ([source](https://github.com/easel/fizeau/blob/master/internal/config/config.go#L280)). |
| `telemetry.pricing.*` | Per-(provider, model) pricing for cost attribution when the provider doesn't return cost. |
| `providers.<name>.daily_token_budget` | Arms predictive burn-rate exhaustion for that provider. |

### CLI

- [`fiz log [session-id]`](../cli/fiz_log/) — pretty-print one session
  log, or list recent sessions when no id is given.
- [`fiz replay <session-id>`](../cli/fiz_replay/) — re-render the public
  event stream from a session log.
- [`fiz usage`](../cli/fiz_usage/) — `UsageReport` projection over a
  window (`--since today`, `--since 7d`, …) including the
  `routing_quality` block.

## Examples

Stream tokens from a recent session as ND-JSON:

```
$ fiz log 2026-05-09T14-32-08Z --json | \
    jq -c 'select(.type=="llm.response") | {latency_ms: .data.latency_ms, in: .data.usage.input, out: .data.usage.output, cost: .data.cost_usd}'
{"latency_ms":847,"in":1240,"out":312,"cost":0.0019}
{"latency_ms":621,"in":1583,"out":89,"cost":0.0012}
```

7-day report with provider reliability and cost per success:

```
$ fiz usage --since 7d --json | \
    jq '.rows[] | {provider, model, sessions, success_rate: (.success_sessions / .sessions), cost_per_success: .known_cost_usd}'
```

## Where to look next

- Source of truth: [`AGENTS.md`](https://github.com/easel/fizeau/blob/master/AGENTS.md)
  package layout § *Cross-cutting* (`internal/session/`, `telemetry/`).
- Schema: [`internal/session/event.go`](https://github.com/easel/fizeau/blob/master/internal/session/event.go)
  is the authoritative shape of every JSONL line.
- Session lifecycle:
  [`service_session_log.go`](https://github.com/easel/fizeau/blob/master/service_session_log.go),
  [`service_session_projection.go`](https://github.com/easel/fizeau/blob/master/service_session_projection.go).
- OTel keys: [`telemetry/telemetry.go`](https://github.com/easel/fizeau/blob/master/telemetry/telemetry.go)
  is the canonical list of attribute names.
- Sibling page: [Auto-routing](../routing/) — what consumes these
  signals and turns them into the next decision.
