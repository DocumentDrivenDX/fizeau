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

TerminalBench, MHI-style evals, SkillsBench, SWE-bench, HumanEval, MMLU,
TAU-bench, and project-local beadbench results all measure different things.
The concrete MHI source captured for this design is Rapid-MLX's Model-Harness
Index resource in `docs/resources/rapid-mlx-mhi-2026-05-06.md`; SkillsBench,
SWE-bench, and HumanEval are captured in sibling files under `docs/resources/`.
Some benchmarks are close to model-only capability. Others are explicitly model
× harness behavior. Current Fizeau catalog power stores a single integer with
provenance, but the raw evidence that led to the integer is not represented in a
common shape.

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

## Derived Reports and Claim Grammar

The ledger is successful only if it can produce strong, reproducible claims from
raw evidence. Reports should support two families of statements:

1. Benchmark-specific comparative claims.
2. Cross-benchmark Fizeau Harness Intelligence claims.

Example benchmark-specific claim:

> fiz native with Opus 4.7 scores 81.0 on TerminalBench, 0.7 points below Claude
> Code with Opus 4.7 on the same subset.

Example FHI claim:

> Using fiz, the most effective model is Opus 4.7, with FHI 56.

These claims require more than headline scores. A claim generator must include
or be able to trace:

- exact Fizeau version or git commit
- exact harness name and harness version
- exact provider name, provider endpoint/API surface, and provider version or
  capture timestamp when no version is available
- exact model raw name, canonical model id when known, and resolved model
  snapshot/version when available
- benchmark name, benchmark version, dataset commit, subset id/version, scorer,
  repetition count, and run timestamps
- score metric, normalized score, raw score, confidence interval or
  uncertainty note, and denominator
- invalid-run counts/classes and denominator handling
- source artifact paths and hashes, including session logs and upstream
  verifier outputs when available

### Benchmark-Specific Claims

Benchmark-specific reports compare rows only when the benchmark, scorer,
dataset/subset, model, and provider surface are intentionally controlled or when
the report states which axis is being varied.

For example, a valid TerminalBench harness comparison may vary only
`subject.harness` while holding model/provider/benchmark/subset constant:

```text
TerminalBench tb2-wide@<dataset_commit>, REPS=3
model=opus-4.7, provider=anthropic, benchmark=terminal-bench

fiz-native       81.0 ± 2.1
claude-code      81.7 ± 1.8
delta            -0.7
```

The report must not claim that a harness is better or worse when the rows also
vary model, provider, subset, scorer, or benchmark version unless the claim text
names those confounds explicitly.

### FHI Claims

Fizeau Harness Intelligence is a derived model × harness × provider metric. FHI
answers "how effective is this combination when driven through Fizeau's
execution surface?" It is separate from catalog model `power`, which answers
"how strong is this model for automatic routing?"

An FHI report may rank models within a fixed harness/provider surface:

```text
FHI for harness=fiz-native, provider=anthropic, evidence window=2026-Q2

opus-4.7         56
sonnet-4.6       49
gpt-5.4-mini     43
```

It may also rank harnesses within a fixed model/provider surface:

```text
FHI for model=opus-4.7, provider=anthropic, evidence window=2026-Q2

claude-code      57
fiz-native       56
fiz-claude       55
```

FHI derivation is policy-owned and must be versioned. Every FHI output includes
the FHI formula version, benchmark weights, included evidence window, included
benchmarks, excluded evidence with reasons, and confidence/coverage notes.

HumanEval, MMLU, and MHI-style components may contribute to FHI, but they must
not dominate long-horizon agentic evidence. TerminalBench, beadbench,
SkillsBench, and SWE-bench-style task outcomes should carry the primary weight
once enough harness-normalized rows exist.

## Initial Import Targets

Near-term importers should cover:

- `cmd/bench matrix` TerminalBench reports (`matrix.json`).
- Harbor job `result.json` and verifier reward files.
- beadbench `report.json`.
- public TerminalBench leaderboard reward cache.
- SkillsBench public rows or local SkillsBench reports.
- SWE-bench family leaderboard rows or task-level reports.
- HumanEval pass@k reports as low-cost coding/model-power components.
- MHI-style local eval reports when available.

The first MHI-style source to support is Rapid-MLX commit
`903487e82ad1998f0c20b721a7df66ec815ea673`, documented in
`docs/resources/rapid-mlx-mhi-2026-05-06.md`.

Benchmark-specific resource notes:

- `docs/resources/skillsbench-2026-05-06.md`
- `docs/resources/swebench-2026-05-06.md`
- `docs/resources/humaneval-2026-05-06.md`

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
