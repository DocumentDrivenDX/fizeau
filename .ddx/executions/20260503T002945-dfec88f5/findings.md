# Investigation: bench/log-summary-date-ranges — Sonnet PASS vs Vidar reasoning stall

Bead: `fizeau-ddd80cec`
Date: 2026-05-02
Runs:
- PASS — `smoke-log-summary-date-ranges-20260503T002152Z` (Sonnet 4.6 via openrouter), reward 1.0
- FAIL — `smoke-log-summary-date-ranges-20260503T002327Z` (Vidar / Qwen3.6-27B-MLX-8bit via omlx), reward 0.0, `NonZeroAgentExitCodeError`

## 1. Confirmed observations

### Sonnet 4.6 — reward 1.0
Harbor run dir (host, not in worktree):
`benchmark-results/harbor-jobs/smoke-log-summary-date-ranges-20260503T002152Z/`

Reported by the bead description: status success, reward 1.0.

### Vidar / Qwen3.6-27B-MLX-8bit — reward 0.0
Harbor run dir (host, not in worktree):
`benchmark-results/harbor-jobs/smoke-log-summary-date-ranges-20260503T002327Z/`

Excerpted from the bead description:

```
reasoning stall: tokens: 0 in / 0 out
```

This is the most extreme variant of the stall pattern observed so far: the
MLX server emitted **no usable token deltas at all** before the harness
tripped the reasoning-stall guard. Compare:

| Task                       | Reasoning tokens at abort |
|----------------------------|---------------------------|
| break-filter-js-from-html  | 141                       |
| sqlite-db-truncate         | 4496                      |
| sanitize-git-repo          | 355                       |
| **log-summary-date-ranges**| **0**                     |

A 0/0 stall implies the connection / chat-completions stream opened but
never produced a single content, tool-call, or reasoning delta inside the
5-minute `DefaultReasoningStallTimeout` window. The harness still classifies
it as a reasoning stall because `nonReasoningSeen == false` at timeout —
the absence of *any* delta is treated identically to "only reasoning
deltas" by the guard at `internal/core/stream_consume.go:160-194`.

## 2. Hypothesis: validated, with a new sub-shape

The bead's hypothesis ("4th task showing the same pattern; Qwen3.6-27B-MLX-
8bit thinking mode systematically incompatible with the benchmark harness
timeout") matches the evidence and is now strongly supported.

The 0/0 sub-shape is new and worth flagging:

- Prior three stalls (break-filter, sqlite-db-truncate, sanitize-git-repo)
  produced between 141 and 4496 reasoning tokens before the 5-minute abort,
  i.e. the model was actively streaming a thinking trace that simply never
  transitioned to tool-call output.
- This run produced **zero** of either. Possible explanations, none yet
  ruled out:
  1. MLX server warm-up / model-load latency exceeded 5 minutes on this
     specific job (cold start after the prior smoke batch).
  2. Initial chat-template rendering for the longer log-summary prompt
     hit a slow path on the MLX runtime.
  3. The MLX server entered thinking mode but suppressed all SSE deltas
     until the (advisory) `thinking_budget=8192` was reached — the harness
     timeout fires at 300s, well before that budget is consumed at the
     observed throughput from prior runs (≈1–25 tok/s).
- All three would manifest as 0/0 at the harness boundary. Distinguishing
  among them requires MLX-server-side logs we do not capture in harbor.

## 3. Configuration in scope

Identical to `fizeau-ff3150d4`, `fizeau-cfe8c5af`, and `fizeau-efed53f4`:

- `scripts/benchmark/profiles/vidar-qwen3-6-27b.yaml` →
  `sampling.reasoning: medium` → 8192-token thinking budget
  (`internal/reasoning/reasoning.go`,
  `PortableBudgets[ReasoningMedium] = 8192`).
- Wire format: Qwen `enable_thinking: true` / `thinking_budget: 8192`
  (`internal/provider/openai/openai.go`).
- The MLX server treats `thinking_budget` as advisory; the harness 5-minute
  reasoning-stall guard fires before the budget runs out at this model's
  throughput.

## 4. Why this is a divergence, not a flake

- Sonnet ran the same log-summary-date-ranges task to reward 1.0 in the
  adjacent smoke batch — the task is achievable in budget on a frontier
  model with the same harness/prompt.
- The Vidar arm did not fail on verifier output; it failed before producing
  any agent step (no trajectory, no tool calls, **no tokens at all** on
  this run).
- This is the **fourth independent confirmation** of the Qwen3.6-27B-MLX-
  8bit reasoning-stall signature in the same smoke regime
  (`break-filter-js-from-html`, `sqlite-db-truncate`, `sanitize-git-repo`,
  `log-summary-date-ranges`). The pattern is profile-level, not task-
  specific. The 0/0 variant on this run further widens the failure surface:
  it is not even gated on the model emitting *some* reasoning first.

## 5. Recommendations (no code changed in this bead)

Recommendations from prior beads (`fizeau-ff3150d4`, `fizeau-cfe8c5af`,
`fizeau-efed53f4`) stand. With a fourth confirmation — and a 0/0 sub-shape
— recommendation #1 is now overdue and recommendation #2 has new urgency:

1. **Lower reasoning effort for vidar smoke (overdue)** — flip
   `scripts/benchmark/profiles/vidar-qwen3-6-27b.yaml` `sampling.reasoning`
   from `medium` (8192) → `low` (2048). File as its own bead (was
   recommended top-priority in `fizeau-efed53f4`); this bead supplies the
   fourth data point. Re-run all four failing tasks under the lower budget
   and confirm the tighter budget allows the model to enter the tool loop
   within the 5-minute stall window.
2. **Distinguish 0/0 stalls from non-zero stalls in harbor results** —
   the new sub-shape suggests an MLX warm-up / chat-template path that the
   current `reasoning_tail` / `model` payload at
   `internal/core/stream_consume.go:181-187` cannot characterize. Surface
   first-token latency and stream-open timestamp in the harbor result so
   "model never spoke" is separable from "model thought too long".
3. **Do not raise the 5-minute stall timeout** — masks the symptom.
4. **Defer to AR-2026-04-26**
   (`docs/research/AR-2026-04-26-agent-vs-pi-omlx-vidar-qwen36.md`) on
   agent-vs-pi parity for OMLX/Vidar/Qwen3.6-27B before drawing routing
   conclusions.

## 6. Evidence pointers

- `internal/core/stream_consume.go:21` — `DefaultReasoningStallTimeout = 300s`
- `internal/core/stream_consume.go:160-194` — stall detection branch
- `internal/core/errors.go` — `ErrReasoningStall`
- `internal/reasoning/reasoning.go` — `PortableBudgets`
- `internal/provider/openai/openai.go` — Qwen thinking wire format
- `internal/provider/omlx/omlx.go` — `ProtocolCapabilities` with
  `ThinkingFormat: ThinkingWireFormatQwen`, `StrictThinkingModelMatch: true`
- `scripts/benchmark/profiles/vidar-qwen3-6-27b.yaml` —
  `sampling.reasoning: medium`
- Prior identical divergences:
  - bead `fizeau-ff3150d4`, findings at
    `.ddx/executions/20260502T231952-3d7f445d/findings.md`
    (break-filter-js-from-html)
  - bead `fizeau-cfe8c5af`, findings at
    `.ddx/executions/20260503T001340-2daf470c/findings.md`
    (sqlite-db-truncate)
  - bead `fizeau-efed53f4`, findings at
    `.ddx/executions/20260503T002325-c7a9fe81/findings.md`
    (sanitize-git-repo)
- Harbor run dirs (host, not in worktree):
  - `benchmark-results/harbor-jobs/smoke-log-summary-date-ranges-20260503T002152Z/`
  - `benchmark-results/harbor-jobs/smoke-log-summary-date-ranges-20260503T002327Z/`
