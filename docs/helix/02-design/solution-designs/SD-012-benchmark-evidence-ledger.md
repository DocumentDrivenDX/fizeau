---
ddx: true
id: SD-012
title: Benchmark Evidence Ledger and Derived Model Power
status: draft
updated: 2026-05-06
---

# Solution Design: SD-012 — Benchmark Evidence Ledger and Derived Model Power

## Summary

Fizeau should treat benchmark observations as raw evidence, not as direct
catalog truth. Every benchmark runner should be able to emit normalized records
keyed by:

- model
- harness
- provider
- benchmark
- benchmark task or subset
- run environment and scoring metadata

Catalog `power` then becomes a derived projection over those records plus
cost, availability, recency, deployment class, and explicit override policy.
This preserves the distinction between model capability and model × harness ×
provider compatibility while still letting benchmark performance drive routing
strength over time.

## Problem

TerminalBench, MHI-style evals, SWE-bench, HumanEval, MMLU, TAU-bench, and
project-local beadbench results all measure different things. Some are close to
model-only capability. Others are explicitly model × harness behavior. Current
Fizeau catalog power stores a single integer with provenance, but the raw
evidence that led to the integer is not represented in a common shape.

Without a raw evidence layer:

- TerminalBench leaderboard rows can be misread as model-only scores even when
  the row is really `Harness__Model`.
- Harness effects such as tool-call parsing, prompt discipline, retry behavior,
  and session logging get folded into model power without attribution.
- Cost, speed, quota, and availability signals are mixed with capability
  judgments in ad hoc ways.
- Recomputing model power after new benchmark runs requires reconstructing
  intent from benchmark-specific report files.

## Decision

Introduce a benchmark evidence ledger format. Existing benchmark-specific
outputs remain the source artifacts, but importers can project them into a
shared JSONL record type described by
`scripts/benchmark/benchmark-evidence.schema.json`.

The ledger is append-only evidence. It does not directly change routing.
Catalog generation or catalog-refresh tooling reads the ledger and computes
model-level derived power plus model × harness capability summaries.

## Record Identity

A single evidence record represents one aggregate or one atomic trial. Atomic
task-level records are preferred when available; aggregate records are allowed
when the upstream source only publishes aggregate scores.

Identity fields:

- `schema_version`: evidence schema version.
- `record_id`: stable content-derived identifier.
- `captured_at`: when Fizeau captured/imported the record.
- `source`: source runner or external publisher.
- `benchmark`: benchmark identity and version.
- `subject`: model, harness, provider, endpoint, and optional surface.
- `scope`: task, subset, split, repetition, and run identifiers.

## Subject Semantics

`subject.model` is the canonical model identity if known. `subject.model_raw`
preserves the model string from the source. `subject.harness` and
`subject.provider` are never optional for agentic/tool benchmarks; if a public
source hides either value, it must be recorded as `unknown`, not omitted.

This prevents the common mistake of treating `ClaudeCode__GLM-4.7` as a GLM-only
score. It is a score for GLM-4.7 through Claude Code, on whatever provider and
version the source used.

## Score Semantics

The normalized `score` block carries the headline metric in a common form:

- `metric`: e.g. `pass_rate`, `reward_mean`, `accuracy`, `mhi`.
- `value`: normalized 0..1 score when possible.
- `raw_value`: original score if different, e.g. `92` for MHI on a 0..100 scale.
- `n`: number of trials or tasks represented.
- `passed`: count of passed trials when applicable.
- `failed`: count of failed trials when applicable.

Additional benchmark-specific dimensions live under `components`; examples:
`tool_call_success`, `terminalbench_pass_rate`, `humaneval_pass_rate`,
`mmlu_accuracy`, `median_wall_seconds`, `tool_calls_per_success`, and
`cost_per_success_usd`.

## Derived Power

Catalog `models.yaml` remains the compact routing surface. `power` is derived
from evidence, but the derivation is intentionally policy-owned:

1. Normalize each benchmark into capability dimensions.
2. Separate model-only evidence from model × harness evidence.
3. Weight reliable, harness-normalized agentic benchmarks heavily for coding
   agent routing. TerminalBench should become a major contributor once we have
   harness-normalized rows.
4. Apply deployment-class and availability guardrails. A local/community model
   should not receive managed-cloud frontier power from one benchmark alone.
5. Apply cost, quota, latency, and recency as routing utility signals or
   bounded power modifiers, not as silent replacements for capability.
6. Emit the resulting `power` and a compact `power_provenance` summary back into
   the catalog.

Power should therefore answer "how strong is this model for automatic routing?"
Model × harness evidence should answer "how strong is this combination in this
benchmark environment?"

## Initial Import Targets

Near-term importers should cover:

- `cmd/bench matrix` TerminalBench reports (`matrix.json`).
- Harbor job `result.json` and verifier reward files.
- beadbench `report.json`.
- public TerminalBench leaderboard reward cache.
- MHI-style local eval reports when available.

Each importer should preserve source artifact paths and source hashes so a
ledger record can be traced back to the original run.

## Open Questions

- Whether the first ledger implementation should live under `benchmark-results/`
  only, or whether curated evidence snapshots should be checked in under
  `docs/research/` or `scripts/benchmark/evidence/`.
- Exact power derivation weights. These should be data-driven once we have
  enough harness-normalized TerminalBench and local-model runs.
- Whether latency/cost should affect catalog `power` directly or only routing
  score. The current default should be conservative: keep power capability-led
  and use routing score for cost/latency preference.
