# Investigation: bench/vidar — systemic Qwen3.6-27B-MLX-8bit reasoning stall on medium tasks

Bead: `fizeau-413f8448`
Date: 2026-05-02
Scope: synthesis across the smoke-medium task set, not a single task probe.
Predecessors (per-task divergence beads, all merged):
- `fizeau-ff3150d4` — break-filter-js-from-html (141 reasoning tokens at abort)
- `fizeau-cfe8c5af` — sqlite-db-truncate (4496)
- `fizeau-efed53f4` — sanitize-git-repo (355)
- `fizeau-ddd80cec` — log-summary-date-ranges (0/0 sub-shape)
- `fizeau-c0c39e55` — largest-eigenval (0/0 sub-shape, recurrence)

This bead promotes those individual confirmations to a profile-level
finding and records the consolidated recommendation set. No source code
is changed by this bead — it is the investigation/synthesis artifact
required by the bead contract.

## 1. Confirmed observations

### 1.1 Failure population

Every medium-difficulty smoke task run with profile
`scripts/benchmark/profiles/vidar-qwen3-6-27b.yaml`
(`sampling.reasoning: medium`, omlx provider, model
`Qwen3.6-27B-MLX-8bit`) failed with
`agent: reasoning stall: model produced only reasoning tokens past stall
timeout (timeout=5m0s)` or its 0/0 (no-delta) variant:

| Task                       | Tokens in / out at abort | Stall sub-shape          |
|----------------------------|--------------------------|--------------------------|
| break-filter-js-from-html  | 2738 / 141               | reasoning-only           |
| sqlite-db-truncate         | 16150 / 4496             | reasoning-only           |
| sanitize-git-repo          | 2856 / 355               | reasoning-only           |
| log-summary-date-ranges    | 0 / 0                    | no-delta (0/0)           |
| largest-eigenval           | 0 / 0                    | no-delta (0/0)           |
| vulnerable-secret          | 17682 / 302              | reasoning-only           |
| distribution-search        | unknown                  | NonZeroAgentExitCodeError |

n = 7 medium tasks, 7 failures. The 0/0 sub-shape has reproduced on a
second task, ruling out a one-off MLX warm-up race. The
`distribution-search` `NonZeroAgentExitCodeError` is a different surface
form but, per prior bead context, lands in the same medium-task bucket
under the same profile and is consistent with a downstream effect of
the stall (agent process exits non-zero after the harness reports a
stall).

### 1.2 Vidar passes are consistent with "short-task escape"

The same profile passed three short / non-medium tasks in the same
batch:

| Task                       | Result | Wall clock |
|----------------------------|--------|------------|
| nginx-request-logging      | PASS   | ~7 min     |
| openssl-selfsigned-cert    | PASS   | ~6 min     |
| git-multibranch            | PASS (reward=1.0; late `AgentTimeoutError`, reward written first) | ~21 min |

These are all short prompts whose first agent step is unambiguous — the
model emits a tool call before the 5-minute reasoning-stall guard
fires. They do not contradict the medium-task divergence; they are
consistent with the "stall window vs. throughput" framing from the
predecessor beads (`fizeau-ff3150d4`, `fizeau-cfe8c5af`,
`fizeau-efed53f4`, `fizeau-ddd80cec`, `fizeau-c0c39e55`).

### 1.3 Sonnet 4.6 cross-check

Sonnet 4.6 (openrouter) ran the same medium tasks to reward 1.0 in
adjacent smoke batches (documented per-task in the predecessor
beads). The harness, prompt template, and verifier are therefore not
the cause — the divergence is local to the
`vidar-qwen3-6-27b.yaml` × `Qwen3.6-27B-MLX-8bit` × `reasoning=medium`
combination.

## 2. Hypothesis: validated at profile level

The bead's hypothesis ("Qwen3.6-27B-MLX-8bit's extended thinking/CoT
mode generates long internal reasoning chains without emitting tool
calls or content tokens; the fiz reasoning-stall detector correctly
triggers after 5 minutes") is supported by:

1. **7/7 medium-task failure rate** in the same profile, across three
   distinct task families (text manipulation, sqlite, git, mathematics,
   security/secrets, statistics).
2. **Two stall sub-shapes** (reasoning-only, 0/0) both reproducing,
   ruling out a single-cause flake.
3. **Short-task pass rate** confirming the harness, transport, and
   verifier work end-to-end against the same provider/model/profile.
4. **Sonnet pass rate on the same tasks** confirming the tasks
   themselves are achievable inside the harness budget on a frontier
   model.

The detection itself is correct: `internal/core/stream_consume.go:160-194`
classifies "only reasoning deltas seen, no content/tool deltas, past
`DefaultReasoningStallTimeout`" as `ErrReasoningStall`, and treats the
0/0 case identically via the `nonReasoningSeen == false` branch. The
guard is not the bug — it is reporting a real failure mode of the
model under this configuration.

## 3. Configuration in scope

Identical to all five predecessor beads:

- Profile: `scripts/benchmark/profiles/vidar-qwen3-6-27b.yaml`,
  `sampling.reasoning: medium` → 8192-token thinking budget
  (`internal/reasoning/reasoning.go`, `PortableBudgets[ReasoningMedium]`).
- Wire format: Qwen `enable_thinking: true` /
  `thinking_budget: 8192` (`internal/provider/openai/openai.go`).
- Provider capability: `internal/provider/omlx/omlx.go` →
  `ProtocolCapabilities` with `ThinkingFormat: ThinkingWireFormatQwen`,
  `StrictThinkingModelMatch: true`.
- Stall guard: `internal/core/stream_consume.go:21` →
  `DefaultReasoningStallTimeout = 300s`.
- Task corpus: `scripts/beadbench/external/termbench-full.json` —
  every failing task carries a `medium` difficulty label.

## 4. Why this is a divergence, not a flake

- Sonnet 4.6 ran every failing medium task to reward 1.0 in adjacent
  smoke batches. The tasks are achievable in budget on a frontier
  model with the same harness and prompt.
- The Vidar arm fails before producing a tool call on every medium
  task — there is no trajectory to grade. The verifier never runs.
- The same Vidar profile passes short tasks in the same batch, so the
  failure is correlated with task class, not transport health.
- Two distinct stall sub-shapes (reasoning-only and 0/0) both
  reproduce, arguing against an MLX cold-start race or a one-off
  network glitch.
- This is the **sixth synthesis-level confirmation** (5 per-task beads
  + this aggregate) of the same configuration's failure mode in the
  same smoke regime.

## 5. Recommendations

The recommendations from the predecessor beads stand and are
consolidated here. The "lower reasoning effort" item is now the
gating action — additional smoke evidence at `medium` is no longer
informative.

1. **Lower reasoning effort on the vidar smoke profile (BLOCKING)**.
   Flip `scripts/benchmark/profiles/vidar-qwen3-6-27b.yaml`
   `sampling.reasoning: medium` → `low` (8192 → 2048 token budget).
   This recommendation has been open since `fizeau-efed53f4`
   (sanitize-git-repo), was marked overdue in `fizeau-ddd80cec`, and
   was promoted to "blocking further smoke evidence" in
   `fizeau-c0c39e55`. With 7/7 medium-task failures it is now the
   single most informative experiment available on this profile and
   should be filed as its own change-bead and re-run against the full
   medium-task set.
2. **First-token latency telemetry in harbor results**. Surface
   stream-open timestamp, first-byte latency, and first-delta latency
   so the 0/0 (no-delta) sub-shape can be split between (a) MLX
   cold-start / chat-template slow path and (b) suppressed deltas
   while thinking. Justified by recurrence on `largest-eigenval`
   after first appearing on `log-summary-date-ranges`.
3. **Do not raise the 5-minute stall timeout**. It would mask the
   symptom on this profile and silently break stall detection
   everywhere else. Carried over from all five predecessor beads.
4. **Defer broader routing conclusions** to
   `docs/research/AR-2026-04-26-agent-vs-pi-omlx-vidar-qwen36.md`
   (agent-vs-pi parity for OMLX/Vidar/Qwen3.6-27B). Until the
   reasoning-effort downgrade is exercised, smoke evidence cannot
   distinguish "Qwen3.6-27B-MLX-8bit is unfit for these tasks" from
   "the medium reasoning budget is unfit for this throughput".
5. **Investigate `distribution-search`'s `NonZeroAgentExitCodeError`
   separately** if it does not collapse to a stall under the
   `low` budget re-run. It is the only failure that did not surface
   the stall string and may be a distinct failure mode hiding under
   the same profile.
6. **Sonnet `AgentTimeoutError`-after-pass classification**. Three
   Sonnet passes in this batch (`git-multibranch`, plus the per-task
   `largest-eigenval` and `log-summary-date-ranges` cross-checks)
   wrote a reward-1.0 verdict before the harness wall-clock cap
   fired. The smoke summary should distinguish "timeout after reward
   written" from "timeout before reward written" and not flag the
   former as an error. Out of scope for this bead; carried as a
   harness-side follow-up.

## 6. Evidence pointers

- `internal/core/stream_consume.go:21` — `DefaultReasoningStallTimeout = 300s`
- `internal/core/stream_consume.go:160-194` — stall detection branch
  (covers both reasoning-only and 0/0 sub-shapes via
  `nonReasoningSeen == false`)
- `internal/core/errors.go` — `ErrReasoningStall`
- `internal/reasoning/reasoning.go` — `PortableBudgets`
- `internal/provider/openai/openai.go` — Qwen thinking wire format
- `internal/provider/omlx/omlx.go` — `ProtocolCapabilities`
- `scripts/benchmark/profiles/vidar-qwen3-6-27b.yaml` —
  `sampling.reasoning: medium`
- `scripts/beadbench/external/termbench-full.json` — task difficulty
  labels (every failing task is `medium`)
- Predecessor findings:
  - `.ddx/executions/20260502T231952-3d7f445d/findings.md`
    (`fizeau-ff3150d4`, break-filter-js-from-html)
  - `.ddx/executions/20260503T001340-2daf470c/findings.md`
    (`fizeau-cfe8c5af`, sqlite-db-truncate)
  - `.ddx/executions/20260503T002325-c7a9fe81/findings.md`
    (`fizeau-efed53f4`, sanitize-git-repo)
  - `.ddx/executions/20260503T002945-dfec88f5/findings.md`
    (`fizeau-ddd80cec`, log-summary-date-ranges)
  - `.ddx/executions/20260503T014110-b5dfbab6/findings.md`
    (`fizeau-c0c39e55`, largest-eigenval)
- Research note (deferred conclusions):
  `docs/research/AR-2026-04-26-agent-vs-pi-omlx-vidar-qwen36.md`
