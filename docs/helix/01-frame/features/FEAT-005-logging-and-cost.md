---
ddx:
  id: FEAT-005
  depends_on:
    - helix.prd
    - FEAT-001
    - FEAT-003
---
# Feature Specification: FEAT-005 — Logging, Replay, and Cost Tracking

**Feature ID**: FEAT-005
**Status**: Draft
**Priority**: P0
**Owner**: DDX Agent Team

## Overview

Every LLM interaction and tool call in DDX Agent is logged to a structured
session log. Sessions can be replayed to understand exactly what happened.
Analytics and cross-tool observability are emitted through OpenTelemetry.
Cost is preserved when a provider, gateway, or explicitly configured runtime
knows it; otherwise it remains unknown. This implements PRD P0 requirements
10-11.

Patterned on DDx's agent session logging (`SessionEntry` JSONL and
`ddx agent usage` reporting) but with deeper granularity — DDx logs
one entry per subprocess invocation; DDX Agent logs every turn within the
conversation loop.

## Problem Statement

- **Current situation**: DDx logs one `SessionEntry` per agent subprocess
  invocation — prompt in, response out, tokens, cost. This is the outer
  envelope. What happened inside the agent (which files it read, what edits
  it made, how many LLM turns it took) is opaque.
- **Pain points**: When an agent task fails or produces unexpected results,
  there's no way to see the intermediate steps. Debugging requires re-running
  the task. Cost and timing analytics are not normalized across providers or
  compatible with external observability tooling.
- **Desired outcome**: A session log that captures every LLM turn and tool
  call with full bodies, enabling replay and debugging. OTel spans and metrics
  expose interoperable analytics. Cost is tracked per-turn when known and
  never guessed when unknown.

## Requirements

### Functional Requirements

#### Session Logging

1. Each `agent.Run()` call creates a session with a unique ID
2. The session log captures events in order:
   - `session.start`: timestamp, config (provider, model, working dir, max
     iterations), prompt
   - `llm.request`: messages sent to provider, tools offered
   - `llm.response`: model response (text and/or tool calls), token usage,
     timing, and cost metadata when known
   - `tool.call`: tool name, inputs, outputs, duration, error (if any)
   - `session.end`: final status, total tokens, total known cost or unknown
     status, total duration, final output
3. Events are written as JSONL — one JSON object per line, appendable
4. Each event includes: `session_id`, `seq` (sequence number), `type`,
   `timestamp`, and type-specific fields
5. Full prompt and response bodies are stored in the event (not external
   files, at least for P0)
6. Log directory is configurable. Default: `.agent/sessions/`
7. The caller can provide correlation metadata (bead_id, workflow, etc.)
   that is stored on `session.start` and `session.end` events

#### Replay

8. Given a session log file, DDX Agent can reconstruct and display the full
   conversation: system prompt, each user/assistant turn, each tool call
   with inputs and outputs, token counts per turn, cost per turn
9. Replay is a read-only operation on the log file
10. Replay output is human-readable text (not JSON) — suitable for terminal
    display or piping to a pager

#### Telemetry and Analytics

11. DDX Agent emits OpenTelemetry spans and metrics alongside JSONL logs for
    analytics and cross-tool observability
12. OTel instrumentation uses OpenTelemetry GenAI semantic conventions for
    token usage, conversation correlation, model calls, and tool execution
    where those conventions exist
13. JSONL is the authoritative replay artifact; OTel is the authoritative
    analytics and aggregation surface
    The normative telemetry schema lives in
    `docs/helix/02-design/contracts/CONTRACT-001-otel-telemetry-capture.md`.
14. Telemetry records broad provider identity and runtime-specific identity so
    local systems such as `bragi`, `vidar`, and `grendel` are distinguishable
    in cost and performance reports
15. LLM timing captures request start, first token when streaming, completion,
    and any provider-specific prefill or cache timing exposed by the backend
16. Throughput metrics such as output tok/s, input tok/s, and cached tok/s are
    derived only when the matching timing window exists; missing timing data is
    omitted rather than inferred
17. Project-specific telemetry fields not covered by standard OTel GenAI
    conventions use a DDX Agent namespace (for example `ddx.cost.*` and
    `ddx.provider.*`)

#### Cost Tracking

18. Provider- or gateway-reported cost is recorded per `llm.response` event
    when available
19. If no reported cost exists, DDX Agent may compute cost only from explicit
    pricing configuration for the exact runtime/provider system and resolved
    model
20. If neither reported cost nor explicit runtime pricing exists, cost remains
    unknown and is never guessed from generic pricing tables
21. Local inference runtimes are not implicitly free; `$0` cost must come from
    reported billing or explicit configuration
22. Session totals are accumulated only when all contributing turn costs are
    known; otherwise the session total is unknown
23. `Result.CostUSD` reflects the known total session cost or `-1` when
    unknown
24. Usage reporting aggregates token and timing data for all sessions, known
    costs for priced sessions, and reports unknown-cost session counts

#### Usage Reporting (P1 — Standalone CLI)

25. `ddx-agent usage` aggregates session logs and telemetry: per-provider/model
    token counts, known cost, and throughput summaries, with time-window
    filtering (today, 7d, 30d, date range)
26. Output formats: table (default), JSON, CSV — patterned on
    `ddx agent usage`

### Non-Functional Requirements

- **Performance**: Replay logging overhead < 1ms per event; telemetry emission
  must remain low enough that per-tool-call spans are practical in normal use
- **Reliability**: Log writes are best-effort — logging failure must not
  block the agent loop. Telemetry export is also best-effort. Partial logs are
  still useful.
- **Storage**: Session logs grow at ~10-100KB per session. No automatic
  rotation in P0.
- **Compatibility**: Log format should be forward-compatible — new event
  types can be added without breaking replay of old logs

## Edge Cases and Error Handling

- **Log directory not writable**: Warn once, continue without logging.
  `Result` still has token counts and cost.
- **Session interrupted (context cancelled)**: Write `session.end` with
  status=cancelled and whatever data was collected
- **Provider/runtime does not expose cost and no explicit pricing exists**:
  CostUSD = -1 (unknown), not 0 (free)
- **Very large tool output (>1MB)**: Truncate in log with marker, store
  byte count of original
- **Timing breakdown missing**: Emit only available timestamps and durations;
  do not synthesize prefill or cache throughput

## Success Metrics

- Every completed session has a log with all events
- `ddx-agent replay <id>` reproduces the conversation accurately
- Provider- or runtime-reported costs are preserved exactly when available
- Unknown-cost sessions are surfaced explicitly rather than assigned guessed values
- Log files are valid JSONL readable by `jq`
- OTel spans and metrics can be consumed by the same analytics tooling used
  for Codex and Claude Code

## Constraints and Assumptions

- JSONL remains the replay and forensic artifact
- OTel is the canonical analytics surface for cross-tool comparison and usage
  reporting
- Standard OTel GenAI fields are preferred; DDX Agent uses `ddx.*` fields only
  for gaps such as cost source and runtime-specific timing
- `CONTRACT-001` is authoritative for telemetry field names, formulas, and
  capture semantics
- Log format is DDX Agent-specific but designed to be consumable by DDx's
  session inspection tooling with a thin adapter
- No log rotation or retention policy in P0

## Dependencies

- **Other features**: FEAT-001 (loop emits events), FEAT-003 (provider
  reports token usage)
- **External services**: None (logging is local)
- **PRD requirements**: P0-10, P0-11

## Out of Scope

- Log shipping to external systems (Grafana, DataDog, etc.)
- Real-time log streaming to a UI
- Automatic log rotation or retention policies
- Budget enforcement (stopping the agent when cost exceeds a threshold) —
  the caller can do this via context cancellation based on streaming
  cost callbacks
