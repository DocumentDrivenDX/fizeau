---
ddx:
  id: terminalbench-fiz-wrapper-comparison-2026-05-06
  created: 2026-05-06
  extends:
    - external-benchmarks
    - routing
---

# TerminalBench Fiz-Wrapper Comparison

## Problem

The medium-model TerminalBench comparison attempted to compare native Claude
Code, native Codex, and fiz by installing separate Harbor agents for each
harness. That duplicates fiz's routing and harness-normalization job inside
benchmark glue. It also creates false failures from Harbor/container details:
TerminalBench images commonly run as root, prebuilt task images may be
cross-architecture, and Claude/Codex permission/auth flags differ.

The benchmark should not hand-roll Claude Code or Codex CLI semantics. Fizeau
already owns those wrappers through its harness registry, permission policy,
model aliasing, session logging, and quota/account interpretation.

## Decision

TerminalBench matrix runs must use one Harbor installed agent:
`scripts/benchmark/harbor_agent.py:FizeauAgent`.

Benchmark profiles select the execution target by passing explicit fiz hard
pins into that single agent:

- `FIZEAU_HARNESS=claude` for fiz-wrapped Claude Code.
- `FIZEAU_HARNESS=codex` for fiz-wrapped Codex.
- `FIZEAU_PROVIDER=openrouter` for fiz's provider path.
- `FIZEAU_MODEL`, `FIZEAU_MODEL_REF`, and `FIZEAU_REASONING` retain their
  existing meanings.

Raw Harbor Claude/Codex adapters may remain as diagnostics, but they are not
part of the official medium-model or frontier-reference TerminalBench
comparison.

## Benchmark Lanes

The medium-model comparison uses these cells:

| Cell | Meaning |
| --- | --- |
| `fiz-harness-claude-sonnet-4-6` | Fizeau pinned to the Claude Code harness. |
| `fiz-harness-codex-gpt-5-4-mini` | Fizeau pinned to the Codex harness. |
| `fiz-openrouter-claude-sonnet-4-6` | Fizeau provider path through OpenRouter to Sonnet. |
| `fiz-openrouter-gpt-5-4-mini` | Fizeau provider path through OpenRouter to GPT mini. |

These lanes separate two questions:

1. Harness path: how well does fiz normalize subscription harnesses when the
   underlying model family is held near constant?
2. Provider path: how well do the same model families perform through fiz's
   direct provider/tool loop?

Published memos must state that identical model names across lanes are not a
pure model control. Harnesses still differ in prompt scaffolding, tool schema,
permission semantics, context handling, and quota surface.

## Native Architecture

On arm64 hosts, TerminalBench task images must be built for the native
architecture. The medium comparison defaults `HARBOR_FORCE_BUILD=1` so Harbor
does not reuse amd64 upstream images with arm64 binaries. This is a
reproducibility requirement, not an optimization.

## Invalid Run Classification

Capability aggregates must exclude runs that never reached a meaningful model
attempt. The matrix report must classify and surface these as invalid rather
than as graded failures:

- `invalid_quota` — rate limit, usage exhausted, credits exhausted, quota
  window closed.
- `invalid_auth` — missing or rejected credentials.
- `invalid_setup` — harness installation, binary architecture, permission-mode,
  or task environment failure before agent work.
- `invalid_provider` — provider transport failure before a response is
  produced.

Only verifier failures after a real agent attempt are `graded_fail`.

Invalid runs still appear in `matrix.md` with cause and log path. They are
excluded from mean reward denominators and cost/capability comparisons.

## Implementation Shape

1. The fiz CLI exposes `--harness` as a hard pin on `fiz run`, matching the
   routing docs.
2. `FizeauAgent` forwards `FIZEAU_HARNESS` into the fiz invocation and records
   the resolved harness/provider/model in its trajectory metadata.
3. Benchmark profiles encode lanes; scripts invoke only `HARNESSES=fiz`.
4. Aggregation classifies invalid runs from report fields and known log
   signatures, including Claude Code `api_error_status: 429` and
   `out_of_credits`.
5. Tests prove the official comparison script does not call raw Harbor
   Claude/Codex adapters.

## Out Of Scope

- Making raw Harbor Claude/Codex adapters production quality.
- Reimplementing upstream TerminalBench scoring.
- Treating OpenRouter Sonnet and Claude Code Sonnet as the same provider
  surface.
- Introducing concurrent matrix execution.
