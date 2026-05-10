# FZ-040c Telemetry Dashboard Impact Note

Scope: operator-facing dashboard, query, and alert impact from the telemetry
identity changes made during the rename from `agent` to `fizeau`.

## Impact Summary

External observability consumers must update selectors that hard-code the old
product identity. The telemetry schema did not change: custom `ddx.*`
attribute keys, standard OTel / GenAI attribute keys, and emitted metric names
remain stable.

## Changed Service Names and Identity Values

| Surface | Previous product identity | Current Fizeau identity | Dashboard impact |
|---|---|---|---|
| OTel `service.name` attribute emitted on spans and chat metrics | not emitted by the core runtime | `fizeau` | Prefer this selector for new Fizeau dashboards. Update any new dashboards copied from pre-rename queries to include `service.name="fizeau"` rather than older harness-name-only filters. |
| OTel tracer / meter instrumentation scope name | `github.com/easel/fizeau/telemetry` after the module rename; earlier `github.com/DocumentDrivenDX/agent/telemetry` | `fizeau` | Update backend queries or processors that filter by instrumentation scope name. |
| Root run span name | `invoke_agent agent` | `invoke_agent fizeau` | Update span-name filters that match the full root span name. Prefer filtering on `gen_ai.operation.name="invoke_agent"` plus `service.name="fizeau"` going forward. |
| `ddx.harness.name` runtime value for Fizeau runs | `agent` | `fizeau` | Update dashboards grouping or filtering by harness name. |
| `gen_ai.agent.name` runtime value for Fizeau runs | not emitted by the core runtime | `Fizeau` | Add or update display-name filters and labels. Keep the key name unchanged because `gen_ai.agent.name` is the OTel GenAI semantic-convention key. |

## Unchanged Keys and Metrics

No dashboard migration is required for key names or metric names.

- `ddx.*` custom key strings remain in the DDX namespace.
- OTel standard keys remain unchanged, including `service.name`,
  `gen_ai.agent.name`, `gen_ai.operation.name`, `gen_ai.provider.name`,
  `gen_ai.request.model`, and `gen_ai.response.model`.
- DDX runtime identity extension keys remain unchanged, including
  `ddx.provider.system`, `ddx.provider.route`, and
  `ddx.provider.model_resolved`.
- Cost, timing, routing, token usage, and tool-execution key strings remain
  unchanged.
- Metric names remain `gen_ai.client.operation.duration` and
  `gen_ai.client.token.usage`.

## Required Operator Updates

Update any external dashboard, alert, SLO, collector processor, or saved query
that matches old product identity values. The minimum value update for
runtime-harness selectors is:

```text
ddx.harness.name = "agent"  ->  ddx.harness.name = "fizeau"
```

Also review queries for:

- instrumentation scope names
  `github.com/easel/fizeau/telemetry` or
  `github.com/DocumentDrivenDX/agent/telemetry`
- full span-name matches for `invoke_agent agent`
- old queries that should now add `service.name = "fizeau"`
- display labels that should now use `gen_ai.agent.name = "Fizeau"`
- manually normalized dashboard labels such as `ddx-agent` or `DDX Agent`

Dashboards that group by provider, model, route, cost, token usage, error type,
tool name, or timing keys do not need schema changes. They only need value-level
updates if they also filter to the old product service or harness identity.

## Verification References

- `telemetry/telemetry.go` defines `ServiceName` and `InstrumentationName`
  from `productinfo.ConfigDir`, currently `fizeau`, and `ProductName` from
  `productinfo.Name`, currently `Fizeau`.
- `internal/core/telemetry_contract_test.go` asserts runtime root spans emit
  `service.name="fizeau"`, `ddx.harness.name="fizeau"`, and
  `gen_ai.agent.name="Fizeau"`.
- `.ddx/notes/FZ-040b-telemetry-key-audit.md` confirms the custom key strings
  themselves do not encode the old product name.
