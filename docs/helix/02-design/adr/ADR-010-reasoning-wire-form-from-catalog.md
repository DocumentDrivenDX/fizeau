---
ddx:
  id: ADR-010
  depends_on:
    - ADR-005
    - ADR-006
    - ADR-007
---
# ADR-010: Reasoning Wire Form Belongs in the Model Catalog

| Date | Status | Deciders | Related | Confidence |
|------|--------|----------|---------|------------|
| 2026-05-11 | Accepted | Fizeau maintainers | `ADR-005`, `ADR-006`, `ADR-007` | High |

## Context

Fizeau's reasoning policy (`internal/reasoning/reasoning.go`) normalizes
caller intent into one of two kinds:

- `KindNamed` — `low` / `medium` / `high` / etc. (a tier shortcut).
- `KindTokens` — an integer thinking budget.

Each provider's translator emits a wire form for the upstream API. For
OpenRouter that's a nested `reasoning` block with either `effort:
"<tier>"` or `max_tokens: <int>`. For Qwen-native servers
(llama-server, vLLM) it's `enable_thinking: true, thinking_budget:
<int>`. For Anthropic-shaped servers (oMLX, ds4, lucebox) it's
`thinking: {type: enabled, budget_tokens: <int>}`.

Today the OpenRouter translator at
`internal/provider/openai/openai.go:453-473` chooses wire shape from
the policy `Kind` alone:

```go
case KindTokens:
    reasoning["max_tokens"] = policy.Tokens
case KindNamed:
    reasoning["effort"] = effort
```

Empirical finding (2026-05-11 probe against `qwen/qwen3.6-27b` on
OpenRouter): the upstream silently flat-maps `effort` for Qwen3 — `low`,
`medium`, and `high` all yield ~5555 reasoning tokens. The
`reasoning.max_tokens` form is forwarded upstream as Qwen-native
`thinking_budget` and is honored. So *which knob bites* is not a
property of the OR transport; it's a property of the upstream model.
Other OR-hosted models (GPT-5, Claude) honor `effort` because their
upstream vendors interpret it natively.

The principle from ADR-006 ("overrides are routing-failure signals")
and ADR-007 ("sampling is catalog policy, not user configuration")
applies again: a benchmark profile having to specify `reasoning: 4096`
to work around a tier flatten is the catalog failing to carry the
knowledge that OR-Qwen3 needs a token-budget wire. Callers should be
able to write `reasoning: low` and have the right wire shape emitted.

A latent bug accelerates the case: `internal/config/config.go:925-944`
keys the `reasoning_wire` lookup map by **catalog ID** (e.g.
`qwen3.6-27b`), but the openai translator queries with the **surface
ID** (`qwen/qwen3.6-27b` for the OR route). The catalog entry for
`qwen3.6-27b` has `reasoning_wire: model_id`, but the OR provider
never sees it — the lookup misses on key mismatch. Today this is
benign (the lookup returns the default `provider` and the wire path
runs); under any catalog change to the `effort`/`tokens` values, the
mismatch becomes a silent correctness bug.

## Decision

### 1. Wire form is catalog-declared

Extend `ReasoningWire` (`internal/modelcatalog/manifest.go:61`,
`internal/modelcatalog/manifest.go:90-93`) with two new accepted
values:

- `effort` — the upstream honors a discrete effort tier. Wire emits
  `reasoning.effort: "<tier>"` (OpenRouter) or the equivalent for
  other transports.
- `tokens` — the upstream honors only a token budget. Wire emits
  `reasoning.max_tokens: <int>` (OpenRouter), `thinking_budget: <int>`
  (Qwen-native), or `budget_tokens: <int>` (Anthropic-shape).

The full enum becomes `provider | model_id | none | effort | tokens`.
`provider` remains the default and means "use the provider's default
wire form" — preserving today's behavior for any model the catalog
doesn't classify.

### 2. PortableBudgets is the bidirectional conversion table

When the caller's `Kind` and the catalog's wire form disagree, the
translator converts via the existing `PortableBudgets` map
(`low: 2048`, `medium: 8192`, `high: 32768`):

- Caller passes `reasoning: low`, catalog says `tokens` → emit
  `max_tokens: 2048`.
- Caller passes `reasoning: 4096`, catalog says `effort` → snap to
  the nearest tier by PortableBudgets, **rounding up** on ties
  (principle of least surprise: a caller asking for "more thinking"
  gets at least what they asked for, never less).

PortableBudgets stays the single source of truth for the tier↔tokens
relationship. No second mapping table.

### 3. Provider translators receive `model` and consult the catalog

`openRouterReasoningOptions(policy)` becomes
`openRouterReasoningOptions(policy, model, wire)`, where `wire` is
the catalog's reasoning-wire value for `model`. The translator picks
the wire shape from `wire`, applying PortableBudgets if the caller's
form needs converting. When `wire` is empty or `provider`, the
translator falls back to today's "pick wire shape from policy.Kind"
behavior (preserving correctness for ad-hoc model strings not in the
catalog).

Other format-specific paths
(`ThinkingWireFormatQwen`, `ThinkingWireFormatThinkingMap`) get the
same model + wire treatment for symmetry. They're already
budget-only on the wire, so the new `effort` value is meaningless
for them; `effort` on a `Qwen` or `ThinkingMap` provider is treated
as `tokens` (the only thing they can express) with a single
structured-warning log so catalog mis-classifications are visible.

### 4. Lookup is surface-id-aware

Fix `modelReasoningWireMap()` to emit one map per surface ID, not
per catalog ID. The provider system already knows which surface it
serves (e.g. `agent.openai` for OR), so the lookup becomes
`wireMap[surface][surfaceID]`. The fix lands together with the
catalog value updates so no intermediate state regresses any
existing route.

### 5. Probe tooling is the source of truth; catalog is the cached form

The catalog's `reasoning_wire` value for any model is determined by
**measurement**, not by interpretation of the model card prose. The
probe tool (`cmd/fizeau-probe-reasoning`, see beads) sends a small
matrix of requests against the live endpoint and records, per
(provider, model, knob form), whether `usage.completion_tokens_details
.reasoning_tokens` actually moved. The verdict is what gets written
to the catalog.

Model cards remain useful operator context (they tell the human what
to probe and what to expect) but they are not load-bearing for
runtime correctness. We do not attempt to parse cards.

### 6. No runtime probing

The probe is operator tooling, not a startup hook. The runtime path
remains: read catalog (compiled-in or external manifest) → emit
wire. No network IO at request time for catalog purposes. This
preserves cold-start latency and request determinism.

## Consequences

**Positive:**

- Benchmark profiles stop carrying provider-quirk workarounds.
  `reasoning: low` works the same on OR-Qwen3, sindri-llamacpp, and
  vidar-ds4.
- The single "what wire shape does this model honor?" question has
  one home (catalog), populated by one tool (probe), with one
  conversion table (PortableBudgets).
- The latent surface-id lookup bug gets fixed before it bites (today
  it would silently start dropping reasoning entirely if any catalog
  entry's `reasoning_wire` were set to `model_id` for an OR model).
- Adds a forcing function for catalog hygiene without a CI gate:
  when a sweep produces unexpected reasoning behavior, the operator
  re-runs the probe and either confirms or updates the catalog —
  process emerges from data, not policy.

**Negative:**

- One more catalog dimension. Operators authoring new model entries
  must run the probe to populate `reasoning_wire` correctly (or
  leave it as the `provider` default and accept today's behavior).
- The wire-form decision moves from "implicit in the policy `Kind`
  the caller passed" to "looked up in a registry." A new contributor
  reading `openRouterReasoningOptions` has one more indirection to
  follow.
- The probe tool is operator infrastructure that needs maintenance
  separately from runtime code. Operators may forget to re-probe
  when an upstream changes routing; cells will then exhibit the
  pre-fix symptoms (silent flat-mapping) until someone notices.

**Mitigations:**

- The implementation bead pins a backwards-compat test: when
  `reasoning_wire` is unset (i.e. on a manifest predating this
  change), the wire shape is selected from policy `Kind` exactly
  as today. No regression for unaudited models.
- A structured log (`reasoning_wire=provider, model=...`) fires on
  the unset path so post-hoc analysis can spot models that haven't
  been classified yet.
- The probe tool ships with the bead, not as a follow-up. Operator
  hygiene without tooling is aspirational.

## Out of scope

- A catalog refresh tool that pulls upstream `/models` registries
  and diffs against the catalog (pricing, context windows, supported
  parameters). Not blocking; ship when the pain is felt, not before.
- A CI gate on `last_probed_at` staleness. Stale entries produce
  subtly-wrong reasoning, which a date check doesn't catch; the
  control is "re-probe when results look weird," not process.
- Model-card URL or citation fields on catalog entries. The probe
  artifact is the audit object; cards are operator context, not
  schema.
- A startup probe that auto-discovers wire form per session.
  Explicitly rejected: runtime determinism and cold-start latency
  outweigh the convenience.
- Per-`(model, provider, region)` wire-form variation. Today every
  observed case is per-model regardless of OR upstream region;
  recoverable additively if a real case appears.

## References

- ADR-005 (smart routing replaces model routes)
- ADR-006 (overrides are routing-failure signals)
- ADR-007 (sampling profiles in catalog) — same architectural
  principle: behavioral knowledge belongs in the catalog, not in
  caller config.
- `internal/reasoning/reasoning.go` — `Policy`, `Kind`,
  `PortableBudgets`.
- `internal/provider/openai/openai.go:340-435` —
  `reasoningRequestOptions` and `openRouterReasoningOptions`.
- `internal/modelcatalog/manifest.go:59-93` — `ReasoningWire` enum
  and validation.
- `internal/config/config.go:925-944` — `modelReasoningWireMap`
  (the surface-id lookup bug).
