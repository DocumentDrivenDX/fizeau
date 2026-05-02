---
ddx:
  id: planning-mode-2026-05-01
  created: 2026-05-01
  reviewed_by: pending
status: DRAFT v1
exit_criterion: Filing of epic bead (PLAN-000). Subsequent revisions live in child beads.
---

# Planning Mode — Pre-Execution Task Decomposition

Inspired by Dirac's Plan mode.

## Recommendation: Variant B + thin slice of A

Auto-enable in the `benchmark` preset **plus** a `--plan` CLI flag for ad-hoc runs.

- Variant C (config-driven) rejected: config is per-project, not per-run. Planning is a run-level quality knob.
- Variant A alone rejected: lets benchmark runs silently omit planning. Since planning targets terminal-bench-2, it should be on by default in that context.

Combined trigger: `planning := req.ToolPreset == "benchmark" || req.PlanningMode`

## What Planning Mode Does

One lightweight LLM call (no tools) before the main tool loop:

1. Receives task prompt.
2. Makes ONE no-tool `Provider.Chat` call asking the model to analyze the task.
3. Plan response is injected as an `RoleAssistant` message wrapped in `<plan>` tags.
4. Normal tool loop begins with the plan as context.

This is NOT a two-phase "plan then act" system. It is a single injected thinking step.

## Planning Prompt Text

```
You are about to begin a coding task. Before using any tools, produce a concise plan.

Task:
{{PROMPT}}

Think through:
(a) Which files or directories you need to read to understand the current state.
(b) What changes are required, and in which files.
(c) What could go wrong, and how you will recover.
(d) How you will verify that your changes are correct.

Be concise — 150 words maximum. Do not ask questions. Do not start implementing yet.
```

## Exact Code Changes

### `internal/core/agent.go`

```go
// PlanningMode, when true, performs one no-tool LLM call before the main
// tool loop. The plan response is injected as an assistant message.
PlanningMode bool

// EventPlanningTurn fires after the planning LLM call completes.
// Data: "plan" (string), "usage" (TokenUsage), "model" (string).
EventPlanningTurn EventType = "planning.turn"
```

### `internal/core/loop.go`

Insertion point: after `opts := Options{...}` is initialized **and** after `EventSessionStart` is emitted, before the main `for iteration` loop. (Codex review flagged that `opts` is needed for the planning call — do not insert before `opts` is built.)

```go
if req.PlanningMode {
    planMessages := []Message{
        {Role: RoleSystem, Content: req.SystemPrompt},
        {Role: RoleUser,   Content: planningPromptFor(req.Prompt)},
    }
    // opts must be initialized before this block; insertion point is
    // "after opts := Options{...} is built, before the main for-loop".
    emitCallback(req.Callback, Event{Type: EventLLMRequest, ...})
    planResp, err := req.Provider.Chat(ctx, planMessages, nil /* no tools */, opts)
    if err != nil {
        slog.Warn("planning call failed, continuing without plan", "error", err)
    } else {
        // Emit the standard paired response event first (AGENTS.md convention:
        // every provider call gets EventLLMRequest then EventLLMResponse).
        emitCallback(req.Callback, Event{Type: EventLLMResponse, ...})
        // Then emit the planning-specific metadata event.
        emitCallback(req.Callback, Event{
            Type: EventPlanningTurn,
            Data: mustMarshal(map[string]any{
                "plan":  planResp.Content,
                "usage": planResp.Usage,
                "model": planResp.Model,
            }),
        })
        result.Tokens.Add(planResp.Usage)
        messages = append(messages, Message{
            Role:    RoleAssistant,
            Content: "<plan>\n" + planResp.Content + "\n</plan>",
        })
    }
}
```

Key decisions:
- `nil` tools → no tools available → forces no tool calls in planning turn.
- Failure is **non-fatal**: degrades gracefully to normal operation.
- `req.SystemPrompt` forwarded so model has same behavioral framing.
- Tokens accumulated into `result.Tokens` for complete cost accounting.
- Plan wrapped in `<plan>` tags for human/code identification.

### `service_execute.go`

In `loopReq := agentcore.Request{...}`:

```go
PlanningMode: req.PlanningMode || req.ToolPreset == "benchmark",
```

### `agentcli/run.go`

```go
planFlag := fs.Bool("plan", false, "Enable planning mode: one no-tool thinking pass before the agent loop")
// ... wired through serviceExecuteRequestParams → buildServiceExecuteRequest
```

## Session Log Representation

`EventPlanningTurn` JSONL line:

```json
{
  "session_id": "svc-...",
  "seq": 2,
  "type": "planning.turn",
  "ts": "2026-05-01T12:00:00.123Z",
  "data": {
    "plan": "I will start by reading main.go...",
    "usage": {"input": 312, "output": 87, "total": 399},
    "model": "Qwen3.6-27B-MLX-8bit"
  }
}
```

Preceding `EventLLMRequest` carries `"tools": null` — distinguishes planning call in log replay.

## Test Strategy

**`internal/core/loop_test.go`:**
- `TestPlanningMode`: verify first provider call has nil tools + planning prompt; plan injected as assistant message; `EventPlanningTurn` fires before main loop's first `EventLLMRequest`; token accumulation correct.
- `TestPlanningModeFailure`: provider errors on first call; run continues without plan; no `EventPlanningTurn` emitted.

**`service_execute_test.go`:**
- `TestRunNative_BenchmarkPresetEnablesPlanning`: `ToolPreset="benchmark"`, `PlanningMode=false` → planning fires.
- `TestRunNative_PlanningModeFlag`: `ToolPreset="default"`, `PlanningMode=true` → planning fires.

**`agentcli` tests:** verify `--plan` flag appears in `--help` and passes through to params.

## Bead Breakdown

| Bead | Title | Deps | Size |
|---|---|---|---|
| PLAN-000 (epic) | Planning mode for fizeau native agent loop | — | M |
| PLAN-001 | Core: `PlanningMode` field, planning call, `EventPlanningTurn`, tests | — | M |
| PLAN-002 | Service wiring: `ServiceExecuteRequest.PlanningMode`, benchmark auto-enable | PLAN-001 | S |
| PLAN-003 | CLI `--plan` flag + param plumbing | PLAN-002 | S |

Sequencing: PLAN-001 → PLAN-002 → PLAN-003.

Estimated implementation: ~2–3 hours executed sequentially by an agent worker.
