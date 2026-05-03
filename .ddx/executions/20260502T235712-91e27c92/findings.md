# Investigation: bench/cobol-modernization — Sonnet PASS vs Vidar tool-call loop

Bead: `fizeau-aedbd35d`
Date: 2026-05-02
Runs:
- PASS — `smoke-cobol-modernization-20260502T202221Z` (Sonnet 4.6 via openrouter)
- FAIL — `smoke-cobol-modernization-20260502T202731Z` (Vidar / Qwen3.5-27B-4bit via omlx)

## 1. Confirmed observations

### Sonnet 4.6 — reward 1.0
`benchmark-results/harbor-jobs/smoke-cobol-modernization-20260502T202221Z/cobol-modernization__numS4Ua/agent/fiz.txt`

- `status: success`, `tokens: 413808 in / 13295 out`, `cost_usd: 1.4408`,
  `duration_ms: 269532000000` (~4:30 wall).
- Final answer: a multi-paragraph summary describing the Python re-implementation
  of the COBOL BOOKFORUM program — fixed-width record sizes (34/28/22 bytes),
  REWRITE-as-seek+overwrite, OPEN EXTEND-as-`ab`, unsigned overflow on PIC 9(10),
  and space-as-zero numeric handling.
- harbor verifier: pass (reward 1.0).

### Vidar / Qwen3.5-27B-4bit — reward 0.0
`benchmark-results/harbor-jobs/smoke-cobol-modernization-20260502T202731Z/cobol-modernization__6f7Mm9r/`

- `agent/fiz.txt` line 1:
  `WARN tool-call loop: identical calls repeated 3 times, aborting`
- Final fiz JSON (lines 2–17):
  `status: failed`, `tokens: 109409 in / 2334 out`,
  `duration_ms: 263098000000` (~4:23 wall),
  `error: "agent: identical tool calls repeated, aborting loop"`,
  `model: Qwen3.5-27B-4bit`.
- harbor: `NonZeroAgentExitCodeError` (exit 1, no retry).
- `exception.txt` shows harbor's wrapper raising on the non-zero exit; the actual
  abort happened inside `fiz` before harbor's verifier ever ran.

> Note on `agent/trajectory.json`: both runs show
> `final_metrics.total_steps: 0` and an empty `steps: []`. This is a
> benchmark-side recording artifact (trajectory shape unchanged across the
> Sonnet pass and the Vidar fail) — not the failure signal. The 109k input /
> 2334 output token totals on the Vidar run prove the model did execute
> multiple turns; trajectory just isn't fed by the fiz harness path.

## 2. Hypothesis: validated

The bead's hypothesis ("Qwen3.5-27B-4bit got stuck in a repetitive tool call
loop — same tool called 3× identically, triggering loop-detection abort")
matches the recorded warning verbatim.

The abort is fired by `internal/core/loop.go:736–744`:

```go
if consecutiveToolCallCount >= toolCallLoopLimit {
    slog.Warn("tool-call loop: identical calls repeated 3 times, aborting")
    result.Status = StatusError
    result.Error = ErrToolCallLoop
    ...
    return result, ErrToolCallLoop
}
```

with the limit set immediately above at `internal/core/loop.go:131`:

```go
const toolCallLoopLimit = 3
```

The fingerprint compared turn-over-turn is `name + "\x00" + arguments` joined
by `\x01` across all calls in the turn (`internal/core/loop.go:911–917`). It
is byte-exact: any change in argument whitespace, ordering, or content resets
the streak. So the abort means the model emitted the *exact same* tool-call
JSON three turns running.

The error type itself is defined at `internal/core/errors.go:69–71`:

```go
// ErrToolCallLoop reports that the agent produced identical tool calls for
// three consecutive turns.
var ErrToolCallLoop = errors.New("agent: identical tool calls repeated, aborting loop")
```

## 3. Why this is a known failure mode for small/quantized Qwen

Two contributing factors lined up:

1. **Context size.** The session reached **109,409 input tokens** before the
   abort. That is well into the regime where a 4-bit quantized 27B model
   degrades on long-context recall — it loses track of what tool calls it has
   already made and what the previous tool results said, so it re-issues the
   same call expecting a different answer.

2. **Output ratio.** Only **2,334 output tokens** across the entire session
   for ~4 minutes of wall time (vs. Sonnet's 13,295 output for the same task)
   indicates the model was producing thin tool-call JSON each turn with little
   reasoning content — a classic signature of a stuck loop where the model
   keeps re-emitting one short call rather than rethinking.

Both factors are intrinsic to the model+quant choice; the loop-detector did
its job by aborting cleanly instead of letting the session burn through more
tokens.

## 4. The fizeau pivot work already targets this exact failure

`docs/research/tool-call-loop-pivot-2026-05-01.md` (DRAFT v1) is the strategy
pivot design that was filed *for* this failure mode. Its problem statement
(quoted):

> When `internal/core/loop.go` detects 3 consecutive identical tool-call
> fingerprints it immediately sets `result.Status = StatusError`,
> `result.Error = ErrToolCallLoop`, and returns. This hard-abort causes 100%
> of tool-call-loop events to produce `graded_fail` on terminal-bench tasks.
> The model is often in a fixable state: it has hit an error it cannot
> interpret and needs a redirect, not a crash.

The Vidar cobol-modernization run is exactly the situation that document was
written to address: a 4-bit Qwen on a long-context task that *might* be
recoverable with a "your last 3 tool calls were identical, try a different
strategy" pivot message rather than a hard-abort.

## 5. Scoped follow-ups

These are not part of this bead — list them so they can be filed separately.

1. **Land the PIVOT-1 epic** described in
   `docs/research/tool-call-loop-pivot-2026-05-01.md`:
   - Add `ToolCallLoopPivotLimit` and `ToolCallLoopPivotMessage` to
     `core.Request` (default 0 → preserves current hard-abort).
   - On the third identical fingerprint, when pivot limit > 0: append a
     `RoleUser` recovery message, reset counters, increment `pivotCount`,
     emit `EventToolCallLoopPivot`, continue the loop; hard-abort only when
     `pivotCount` reaches the configured limit.
   - Re-run cobol-modernization on Vidar with `ToolCallLoopPivotLimit ≥ 2`
     to measure recovery rate.

2. **Surface the looping tool-call payload in `fiz.txt`** when the abort
   fires. Today the warning line tells us the *kind* of failure but not
   *which* call repeated. A one-line dump of the fingerprint (or the last
   tool name + first ~120 bytes of arguments) would let future investigations
   skip straight to root cause without needing the in-memory message log.
   Touch point: the `slog.Warn` at `internal/core/loop.go:737`.

3. **Trajectory recording for the fiz harness path.** Both the Sonnet pass
   and the Vidar fail wrote `total_steps: 0` to `agent/trajectory.json`. The
   benchmark loses per-step granularity, which makes triage harder than it
   needs to be. Audit whether `scripts/benchmark/harbor_agent.py` and the
   agent base in harbor are wired to consume fiz's per-step events.

4. **Context-budget heuristic for small quantized models.** A 4-bit 27B
   model at 109k input tokens is operating outside its reliable regime.
   Consider a profile-level `MaxContextTokens` soft cap that triggers
   compaction earlier on `vidar-qwen3-5-27b-4bit` than on
   `vidar-qwen3-6-27b-mlx-8bit`. This is a separate axis from the pivot
   work and should not block it.

## 6. Disposition

This is an investigation bead. The hypothesis is **confirmed**; the code path
that fired the abort is `internal/core/loop.go:736–744` driven by the
fingerprint check at `internal/core/loop.go:728–735`; the underlying model
behavior (long-context, low-output, repeating calls) is consistent with a
4-bit 27B Qwen on a 100k-token context. No code changes are made in this
bead — see `no_changes_rationale.txt` for why, and the PIVOT-1 design doc
for the planned remediation.
