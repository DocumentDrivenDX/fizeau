# Routing Profiles

Profiles are named shorthands for points on the routing power curve. They
expand to effective power policy, not to a closed set of concrete models.

Use `--profile` when the caller wants the project-maintained default for a
power point. Use `--min-power` and `--max-power` when the caller needs an
explicit numeric bound. Use `--model`, `--provider`, and `--harness` only as
hard pins.

`ModelRef` is separate from profiles. It resolves catalog references for exact
model identity and migration compatibility. Legacy names such as
`code-medium` and `code-high` are compatibility model references or deprecated
aliases; they are not target routing profiles.

Routing first applies eligibility: hard pins, profile/power policy, context
fit, required capabilities, health, and quota. It then ranks eligible
candidates by power, cost, deployment placement, utilization, performance,
context headroom, and sticky affinity.

Sticky affinity is keyed by validated `CorrelationID` and targets the server
instance, such as `grendel` or `vidar`, rather than a model string. Related
requests bias toward the same server instance to preserve cache locality, but
that affinity is a score component, not a pin.
