---
ddx:
  id: tb2-baseline-qwen3-27b-2026-05-01
  created: 2026-05-01
  status: FINAL
---

# TB-2 Baseline: Qwen3.6-27B via fizeau — 2026-05-01

This is the v1 benchmark baseline for fizeau on terminal-bench-2.
All improvement beads (PIVOT, PRESET, PLAN, SKILL, ANC) gate against these numbers.

## Run conditions

| Field | Value |
|---|---|
| Matrix dir | `benchmark-results/matrix-20260501T035821Z` |
| Agent | fizeau (`fiz`) at commit `2e87466` |
| Preset | `benchmark` |
| Model | `Qwen3.6-27B-MLX-8bit` via omlx @ vidar:1235 |
| Profile | `vidar-qwen3-6-27b` |
| Tasks | 89 TB-2 tasks × 3 reps = 267 cells |
| TB-2 pin | `53ff2b87d621bdb97b455671f2bd9728b7d86c11` |
| Sampling | T=0.6, top_p=0.95, top_k=20, reasoning=medium |
| Concurrency | --jobs=1 (sequential) |
| Date | 2026-05-01 |

## Results

| Metric | Value |
|---|---|
| **Pass rate** | **0 / 267 (0%)** |
| graded_pass | 0 |
| graded_fail | 265 |
| harness_crash | 1 |
| ran (ungraded) | 1 |

### By difficulty

| Difficulty | Pass | Total | Rate |
|---|---|---|---|
| easy | 0 | 12 | 0% |
| medium | 0 | 165 | 0% |
| hard | 0 | 90 | 0% |

## Agent failure modes (from fiz.txt analysis)

Of the 265 graded_fail cells, fiz.txt was present in ~80; the remainder had no fiz.txt (Docker pull failures that were later cleaned up and re-run). Agent-level outcomes:

| fiz status | Count | % |
|---|---|---|
| failed | 246 | 96% |
| stalled | 4 | 2% |
| success (but verifier failed) | 5 | 2% |

### Error breakdown within "failed"

| Error | Count | % of failed |
|---|---|---|
| `agent: identical tool calls repeated, aborting loop` | 156 | 63% |
| `agent: reasoning stall: model produced only reasoning tokens` | 87 | 35% |
| `no_progress_tools_exceeded` | 4 | 2% |
| provider connectivity errors | 3 | 1% |

### "success but verifier failed" analysis

Five cells where fiz reported `status: success` but reward=0:
- `mteb-leaderboard` (3 reps): agent ran to completion but never wrote `/app/result.txt`
- `polyglot-c-py` (2 reps): agent ran to completion but output incorrect

## Bragi run (contaminated — excluded from baseline)

`matrix-20260501T040121Z` (bragi lmstudio, Qwen3.6-27B Q4_K_M) produced 0/267 passes,
but **83% of failures were provider connectivity errors** (`POST \` network failures).
lmstudio was crashing under even 1-concurrent-request load. This run is not a valid
model performance baseline — it measures lmstudio server stability, not agent quality.
Exclude from all comparison tables until bragi is re-tested with a stable server.

## Interpretation

The 0% pass rate is real, not a harness bug. The agent is running on the correct model
and reaching the verifier — it just produces wrong or missing output. Two root causes
account for 98% of failures:

1. **Tool call loop (63%)**: Model issues the same failing bash command repeatedly.
   Current loop detector aborts after 3 identical calls with no recovery.
   → Fix: PIVOT beads (strategy pivot recovery).

2. **Reasoning stall (35%)**: Model enters extended thinking and never calls a tool.
   Current stall detector fires but there is no recovery prompt.
   → Fix: PRESET beads (anti-stall guideline) + PLAN beads (force plan-then-act).

3. **No output written (2%)**: Agent reports success internally but never calls `write`.
   → Fix: PRESET beads (OUTPUT VERIFICATION guideline).

## Improvement targets (gates for feature beads)

| After shipping | Target pass rate | Rationale |
|---|---|---|
| PIVOT + PRESET only | ≥ 5% (13/267) | Recovery from loops; should rescue at least the easy tasks |
| + PLAN | ≥ 10% (27/267) | Planning pass reduces stalls on medium tasks |
| + SKILL | ≥ 15% | Progressive context reduces mid-session context rot |
| Full stack | ≥ 20% (53/267) | Still below Dirac (65%) — model quality is the ceiling |

These are minimum thresholds. If a batch of changes does not lift pass rate,
the changes must be investigated before filing the next batch.

## Rerun recipe

```bash
cd /home/erik/Projects/agent
OMLX_API_KEY=local HARBOR_AGENT_ARTIFACT=$(pwd)/fiz-linux-amd64 \
  /tmp/bench-fixed matrix \
  --profiles=vidar-qwen3-6-27b --harnesses=ddx-agent --reps=3 \
  --subset=scripts/beadbench/external/termbench-full.json \
  --tasks-dir=scripts/benchmark/external/terminal-bench-2 \
  --out=benchmark-results/matrix-<ts>/ --jobs=1
```
