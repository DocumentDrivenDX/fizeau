---
ddx:
  id: matrix-baseline-phase-a1-2026-05-04
  bead: agent-5b6f5872
  depends_on:
    - SD-010
    - agent-d7d2e4dd
    - agent-73f90363
---

# Phase A.1 Matrix Baseline — 2026-05-04

Status: terminal run completed; grading blocked by missing Terminal-Bench task
bundle.

This memo publishes the Phase A.1 matrix attempt for bead `agent-5b6f5872`.
The shipped matrix runner and aggregator were used. All 27 planned runs reached
a terminal `final_status`, but none reached Harbor grading because the
documented task directory did not contain the canary task folders.

## Caveat: same-model-different-harness comparison

Cells in this matrix that share a model column and differ only by harness row
are not a clean control of model capability. Each harness ships its own system
prompt, tool schema, retry policy, reasoning effort, context compaction
strategy, and default sampling. The numbers below compare (harness scaffolding
+ policy) over a shared model API, not pure harness skill, and not pure model
skill. Differences in scaffolding, prompt template, tool surface, and turn
budget account for an unknown share of any observed delta. See SD-010 §2 D4
(telemetry schema), §5 (failure taxonomy), and §7 for the full obligations.

## Pinning

| Field | Value |
| --- | --- |
| Repository revision | `8519dcb402b629bfa4fb3166aecd11781675a1f1` |
| Runner | `ddx-agent-bench matrix`; `ddx-agent-bench matrix-aggregate` |
| Subset | `scripts/beadbench/external/termbench-subset-canary.json` |
| Task IDs | `fix-git`, `log-summary-date-ranges`, `git-leak-recovery` |
| Tasks dir passed to runner | `scripts/benchmark/external/terminal-bench-2/tasks` |
| Profile | `gpt-5-mini` |
| Profile pricing hash | `scripts/benchmark/profiles/gpt-5-mini.yaml#sha256=e53a2c3d627730eaf80e1d7173a14027395109a03f31ee90e564edcfd6112421` |
| Harnesses | `ddx-agent`, `pi`, `opencode` |
| Reps | `3` |
| Per-run cap | `$1.00` |
| Matrix cap | `$32.40` |

The bead text still names stale profile `gpt-5-3-mini`. The prerequisite note
`docs/research/phase-a1-live-matrix-preflight-2026-04-30.md` records the
current anchor rename to `gpt-5-mini` and the move to `OPENROUTER_API_KEY`.

## Commands

```sh
ddx-agent-bench matrix \
  --subset=scripts/beadbench/external/termbench-subset-canary.json \
  --profiles=gpt-5-mini \
  --harnesses=ddx-agent,pi,opencode \
  --reps=3 \
  --tasks-dir=scripts/benchmark/external/terminal-bench-2/tasks \
  --per-run-budget-usd=1.00 \
  --budget-usd=32.40 \
  --out=benchmark-results/matrix-20260504T044909Z

ddx-agent-bench matrix-aggregate benchmark-results/matrix-20260504T044909Z
```

## Published Artifacts

| Artifact | SHA-256 |
| --- | --- |
| `benchmark-results/matrix-20260504T044909Z/matrix.json` | `7d3e6c83bfeada693bd3736e793fe3f8e8fa04fe5589bdcd3f48c2475d8656ff` |
| `benchmark-results/matrix-20260504T044909Z/matrix.md` | `9343c53d4607c941b4271ef57c7006c757eba614245c62c2344819a40ab880d6` |
| `benchmark-results/matrix-20260504T044909Z/costs.json` | `fb102d9b720c0d466e5f45464f3b904081c26183d48d60427c365ec4c1f846e5` |

## Final Status

| final_status | Count |
| --- | ---: |
| `install_fail_permanent` | 27 |

All 27 runs reached terminal status. The common cause was:

```text
task directory not found: .../scripts/benchmark/external/terminal-bench-2/tasks/<task-id>
```

Root-cause follow-up is filed as bead `fizeau-01248b3d`:
`Provide Terminal-Bench canary task bundle for Phase A.1 matrix`.

## Rewards And SD

| Harness | Profile | Runs | Reported rewards | Mean reward | SD |
| --- | --- | ---: | ---: | --- | --- |
| `ddx-agent` | `gpt-5-mini` | 9 | 0 | n/a | n/a |
| `opencode` | `gpt-5-mini` | 9 | 0 | n/a | n/a |
| `pi` | `gpt-5-mini` | 9 | 0 | n/a | n/a |

No reward or SD is reported because every run failed before Harbor grading.
The generated `matrix.md` therefore reports `n/a` for the reward table and
lists all 27 runs under non-graded runs.

## Cost Reconciliation

| Cell | Input tok | Output tok | Cached tok | Retried tok | Cost |
| --- | ---: | ---: | ---: | ---: | ---: |
| `ddx-agent / gpt-5-mini` | 0 | 0 | 0 | 0 | `$0.000000` |
| `opencode / gpt-5-mini` | 0 | 0 | 0 | 0 | `$0.000000` |
| `pi / gpt-5-mini` | 0 | 0 | 0 | 0 | `$0.000000` |
| **Matrix total** | 0 | 0 | 0 | 0 | **`$0.000000`** |

The per-run and matrix caps were supplied as `$1.00` and `$32.40`,
respectively, matching the SD-010 cost-guard floor and the 27-run matrix safety
formula (`$1.00 * 27 * 1.2`). The final cost is zero because the runner stopped
at task-directory validation before invoking Harbor, the harnesses, or the
provider. `costs.json` records the same caps, zero token streams, and the
profile pricing hash above.
