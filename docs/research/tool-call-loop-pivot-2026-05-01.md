---
ddx:
  id: tool-call-loop-pivot-2026-05-01
  created: 2026-05-01
  reviewed_by: pending
status: DRAFT v1
exit_criterion: Filing of epic bead (PIVOT-1). Subsequent revisions live in child beads.
---

# Tool-Call Loop Strategy Pivot Recovery

## Problem Statement

When `internal/core/loop.go` detects 3 consecutive identical tool-call fingerprints it immediately sets `result.Status = StatusError`, `result.Error = ErrToolCallLoop`, and returns. This hard-abort causes 100% of tool-call-loop events to produce `graded_fail` on terminal-bench tasks. The model is often in a fixable state: it has hit an error it cannot interpret and needs a redirect, not a crash.

## Desired Behavior

On reaching `toolCallLoopLimit` identical consecutive turns, before hard-aborting, the loop should:

1. Inject a recovery ("pivot") message into the conversation as a `RoleUser` message.
2. Reset `consecutiveToolCallCount` to 0 and `lastToolCallFingerprint` to `""`.
3. Increment a `pivotCount` session counter.
4. Continue the agent loop normally from the next iteration.
5. If `pivotCount` reaches the configured `ToolCallLoopPivotLimit`, perform the existing hard-abort.

The no-pivot path (default pivot limit = 0) must be unchanged for callers that rely on the current abort-immediately behavior.

## Exact File Changes

### `internal/core/agent.go`

New fields on `Request`:

```go
// ToolCallLoopPivotLimit is the maximum number of strategy-pivot recoveries
// allowed per session. Zero means no pivot attempts (hard-abort, legacy behavior).
ToolCallLoopPivotLimit int

// ToolCallLoopPivotMessage is the user message injected on each pivot attempt.
// When empty, DefaultToolCallLoopPivotMessage is used.
ToolCallLoopPivotMessage string
```

New constant:

```go
const DefaultToolCallLoopPivotMessage = "Your last 3 tool calls were identical and produced the same result. This approach isn't working. Analyze the error you've been getting, then try a completely different strategy."
```

New EventType:

```go
// EventToolCallLoopPivot fires when the agent detects a tool-call loop and
// injects a pivot recovery message instead of aborting.
// Data: "pivot_count" (int), "pivot_limit" (int), "fingerprint" (string).
EventToolCallLoopPivot EventType = "tool_call_loop.pivot"
```

New Result field:

```go
// ToolCallLoopPivots records how many pivot recoveries were injected.
ToolCallLoopPivots int `json:"tool_call_loop_pivots,omitempty"`
```

### `internal/core/loop.go`

New local state at top of `Run()`:

```go
pivotCount := 0
pivotLimit := req.ToolCallLoopPivotLimit
pivotMsg := req.ToolCallLoopPivotMessage
if pivotMsg == "" {
    pivotMsg = DefaultToolCallLoopPivotMessage
}
```

Replace the unconditional hard-abort block (currently lines 727–743) with:

```go
if consecutiveToolCallCount >= toolCallLoopLimit {
    if pivotLimit > 0 && pivotCount < pivotLimit {
        pivotCount++
        result.ToolCallLoopPivots = pivotCount
        slog.Warn("tool-call loop: injecting strategy pivot",
            "pivot", pivotCount, "pivot_limit", pivotLimit)
        emitCallback(req.Callback, Event{
            SessionID: sessionID, Seq: seq,
            Type: EventToolCallLoopPivot, Timestamp: time.Now().UTC(),
            Data: mustMarshal(map[string]any{
                "pivot_count": pivotCount,
                "pivot_limit": pivotLimit,
                "fingerprint": fp,
            }),
        })
        seq++
        messages = append(messages, Message{Role: RoleUser, Content: pivotMsg})
        consecutiveToolCallCount = 0
        lastToolCallFingerprint = ""
        // continue to next iteration
    } else {
        slog.Warn("tool-call loop: identical calls repeated 3 times, aborting",
            "pivot_count", pivotCount)
        result.Status = StatusError
        result.Error = ErrToolCallLoop
        result.Duration = time.Since(start)
        snapshotMessages()
        emitFinalSessionEnd(req.Callback, sessionID, &seq, req.Provider, &result, req.Metadata)
        return result, ErrToolCallLoop
    }
}
```

### `internal/core/errors.go`

Append to the `ErrToolCallLoop` doc comment:

> When Request.ToolCallLoopPivotLimit > 0, the loop first attempts up to that many strategy-pivot recoveries before returning this error.

### `internal/core/loop_test.go`

Three new test functions (package `core`, using existing `mockProvider`/recording-callback conventions):

- `TestRun_ToolCallLoopPivot_SinglePivotSucceeds`: `PivotLimit=1`, loop fires once, model recovers → `StatusSuccess`, `ToolCallLoopPivots==1`, one `EventToolCallLoopPivot`.
- `TestRun_ToolCallLoopPivot_ExhaustedPivotsAborts`: `PivotLimit=2`, model never escapes → `ErrToolCallLoop`, `ToolCallLoopPivots==2`, two `EventToolCallLoopPivot`.
- `TestRun_ToolCallLoopPivot_ZeroPivotLimitPreservesLegacyAbort`: `PivotLimit=0` → identical behavior to pre-change, zero pivot events.

Existing `TestRun_ToolCallLoopDetection` must continue to pass unchanged.

## Configuration Summary

| Field | Default | Location |
|---|---|---|
| `ToolCallLoopPivotLimit` | 0 (no pivots, legacy) | `Request` in `agent.go` |
| `ToolCallLoopPivotMessage` | `""` → resolved to constant | `Request` in `agent.go` |
| `DefaultToolCallLoopPivotMessage` | see text | `const` in `agent.go` |
| `toolCallLoopLimit` | 3 (unchanged) | `const` in `loop.go` |

## Bead Breakdown

| Bead | Title | Deps | Size |
|---|---|---|---|
| PIVOT-1 (epic) | Tool-Call Loop Strategy Pivot Recovery | — | M |
| PIVOT-2 | Add new types, fields, constants to `agent.go` | — | S |
| PIVOT-3 | Implement pivot branch in `loop.go` | PIVOT-2 | S |
| PIVOT-4 | Three pivot test cases in `loop_test.go` | PIVOT-3 | S |

## Risks

| Risk | Mitigation |
|---|---|
| Pivot loop becomes infinite | `ToolCallLoopPivotLimit` caps count; hard-abort fires after limit |
| Pivot message too generic | `ToolCallLoopPivotMessage` is configurable per `Request` |
| Unknown event type breaks consumers | Consumers ignore unknown `EventType` strings (opaque) |
| Cost overrun from extra pivot turns | Bounded by `PivotLimit × MaxIterations`; default 0 = no change |
