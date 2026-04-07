---
ddx:
  id: SD-006
  depends_on:
    - FEAT-001
    - SD-001
---
# Solution Design: SD-006 — Conversation Compaction

## Problem

The agent loop appends every message and tool result to the conversation
history. For tasks requiring many tool-call rounds, the history will exceed
the model's context window and the provider will return an error. Local models
have especially small windows (8K-32K) making this a practical blocker.

## Research Summary

### Pi's Approach
- **Trigger**: `contextTokens > contextWindow - reserveTokens` (reserve: 16K default)
- **Keep recent**: ~20K tokens of recent messages preserved verbatim
- **Summarize**: Everything before the cut point summarized by the LLM
- **Format**: Structured markdown — Goal, Progress (checkboxes), Key Decisions, Next Steps, Critical Context
- **Update mode**: When prior summary exists, merges new info rather than re-summarizing
- **File tracking**: Accumulates read/modified file lists in XML tags on the summary
- **Tool result truncation**: 2K chars max in summarization input
- **Split turn handling**: Separate "turn prefix summary" when cut falls mid-turn

### Codex's Approach
- **Trigger**: `total_usage_tokens >= auto_compact_token_limit` (per-model, e.g., 200K)
- **Two modes**: Local (LLM-based) and remote (OpenAI server-side `/responses/compact` API)
- **Prompt**: Short — "Create a handoff summary for another LLM that will resume the task"
- **Summary injection**: "Another language model started to solve this problem..."
- **User messages**: Keeps up to 20K tokens of recent user messages alongside summary
- **Timing**: Pre-turn (before user input) and mid-turn (between tool call rounds)
- **Fallback**: Trims oldest history items if compaction prompt itself exceeds context
- **Warning**: Alerts user that multiple compactions degrade accuracy
- **Cache preservation**: When trimming oversize compaction input, trims from the
  *beginning* to preserve prefix-based prompt cache hits. The comment explicitly
  states: "Trim from the beginning to preserve cache (prefix-based) and keep
  recent messages intact."
- **Initial context reinjection**: Two modes — `DoNotInject` (pre-turn: clears
  `reference_context_item` so the next regular turn reinjects system context
  fresh) and `BeforeLastUserMessage` (mid-turn: injects system context into the
  replacement history just above the last user message, because the model expects
  the compaction summary as the last item)
- **Window generation**: After compaction, advances a `window_generation` counter
  that invalidates the websocket session / prompt cache, forcing the provider to
  re-process the compacted history from scratch
- **Ghost snapshots**: Preserves undo/redo state snapshots across compaction by
  copying them into the replacement history

## Design: Forge Compaction

### Strategy

Forge follows pi's structured approach (richer summaries, file tracking) with
Codex's pragmatism (mid-turn compaction, configurable thresholds, graceful
fallback). The compaction is a library feature — not just CLI — so embedders
can control it.

### Configuration

```go
type CompactionConfig struct {
    // Enabled controls whether automatic compaction runs. Default: true.
    Enabled bool

    // ContextWindow is the model's context window in tokens. If zero,
    // the provider is queried or a conservative default (8192) is used.
    ContextWindow int

    // ReserveTokens is the token budget reserved for the model's response
    // and the next prompt. Compaction triggers when conversation tokens
    // exceed ContextWindow - ReserveTokens. Default: 8192.
    ReserveTokens int

    // KeepRecentTokens is how many tokens of recent messages to preserve
    // verbatim after compaction. Default: 8192.
    KeepRecentTokens int

    // MaxToolResultTokens is the max tokens per tool result included in
    // the summarization input. Longer results are truncated. Default: 500.
    MaxToolResultTokens int

    // SummarizationModel overrides the model used for summarization.
    // If empty, uses the same model as the agent loop. Useful for using
    // a faster/cheaper model for compaction (e.g., local model for
    // summarization even when the agent uses a cloud model).
    SummarizationModel string

    // SummarizationProvider overrides the provider for summarization.
    // If nil, uses the same provider as the agent loop.
    SummarizationProvider Provider
}
```

Added to `Request`:

```go
type Request struct {
    // ...existing fields...

    // Compaction configures automatic conversation compaction.
    // If nil, compaction is enabled with defaults.
    Compaction *CompactionConfig
}
```

### Trigger Logic

```
shouldCompact = estimatedTokens > (contextWindow - reserveTokens)
```

Token estimation: use the provider's reported usage from the last response
(accurate), plus chars/4 heuristic for messages added since.

Checked at two points:
1. **Pre-iteration**: Before sending the next prompt to the model
2. **Mid-iteration**: After tool results are appended (a large bash output
   can push over the limit between iterations)

### What Gets Compacted

1. Walk backwards from newest messages, accumulating token estimates
2. Stop when `keepRecentTokens` is reached — everything after this point is kept
3. Everything before the cut point is serialized and summarized
4. The cut point must be at a message boundary (never mid-tool-call)

### Serialization for Summarization

Tool calls serialized compactly:
```
[User]: Read main.go and fix the bug
[Assistant → read(path="main.go")]: package main...
[Assistant → edit(path="main.go", old="bug", new="fix")]: Replaced 1 occurrence
[Assistant]: Fixed the bug by replacing...
```

Tool results truncated to `MaxToolResultTokens`.

### Summarization Prompt

Forge uses pi's structured format (more useful for spec-driven work) with
Codex's framing (handoff to another LLM):

```
You are performing a CONTEXT CHECKPOINT COMPACTION. Create a structured
handoff summary for another LLM that will resume this task.

Use this EXACT format:

## Goal
[What the user is trying to accomplish]

## Constraints & Preferences
- [Requirements, conventions, or preferences mentioned]

## Progress
### Done
- [x] [Completed work with file paths]

### In Progress
- [ ] [Current work]

## Key Decisions
- **[Decision]**: [Brief rationale]

## Next Steps
1. [What should happen next]

## Files
### Read
- [Files that were read]

### Modified
- [Files that were created or edited]

## Critical Context
- [Data, error messages, or references needed to continue]

Keep each section concise. Preserve exact file paths, function names,
and error messages. Do not continue the conversation — only output
the summary.
```

### Update Mode

When a previous compaction summary exists, the prompt changes to:

```
The messages above are NEW conversation since the last compaction.
Update the existing summary (provided in <previous-summary> tags)
by merging new information.

RULES:
- PRESERVE all existing information from the previous summary
- ADD new progress, decisions, and context
- UPDATE Progress: move completed items from In Progress to Done
- UPDATE Next Steps based on what was accomplished
- PRESERVE exact file paths and error messages
```

### Summary Injection

The summary replaces all compacted messages as a user-role message:

```
The conversation history before this point was compacted into the
following summary:

<summary>
{structured summary}
</summary>
```

### File Tracking

Like pi, forge tracks which files were read and modified across compactions.
The file lists are appended to the summary in XML tags and carried forward
through subsequent compactions.

### Prompt Cache Preservation

Both Anthropic and OpenAI support prefix-based prompt caching — the provider
caches the tokenized prefix of the conversation, and subsequent requests that
share the same prefix get a cache hit (faster, cheaper). Compaction destroys
the prefix, but we can minimize the damage.

**Post-compaction message ordering** (cache-optimized):

```
1. System prompt                    ← stable prefix, always cached
2. Compaction summary (user msg)    ← new after compaction, but stable until next compaction
3. Initial context (if mid-turn)    ← system context reinjected for mid-turn compaction
4. Recent user messages (preserved) ← kept verbatim, extends the cacheable prefix
5. Recent assistant/tool messages   ← the tail that changes each turn
```

The system prompt is always first and never changes — this maximizes the
stable prefix for caching. The compaction summary is second and stays
stable until the next compaction, giving it time to warm the cache.

**Key rules** (learned from Codex):

1. **Trim from the front when compaction overflows.** If the compaction
   prompt itself exceeds the context window, trim the oldest messages from
   the summarization input (not the newest). This preserves the prefix
   cache for the summarization call itself.

2. **System prompt reinjection.** After compaction replaces the history,
   the system prompt must remain at position 0. Two strategies:
   - **Pre-iteration compaction**: Replace history with
     `[summary, recent_messages]`. The system prompt is injected by the
     agent loop as usual on the next `Chat()` call — it's not part of
     the history.
   - **Mid-iteration compaction**: The loop is already mid-conversation.
     Inject system context above the last user message in the replacement
     history so the model sees it in the expected position.

3. **Invalidate provider-side cache after compaction.** The conversation
   prefix has fundamentally changed. If the provider uses sticky sessions
   or incremental request tracking, signal that the window changed.
   Forge exposes this via `EventCompactionEnd` — providers that maintain
   session state should listen for it.

### Token Counting — Cache-Aware

Pi counts `cacheRead` and `cacheWrite` tokens in its usage calculation:
```
totalTokens = input + output + cacheRead + cacheWrite
```

Forge should do the same. The `TokenUsage` type already has `Input` and
`Output` — add `CacheRead` and `CacheWrite` fields so compaction triggers
account for the full context footprint, not just the billed tokens.

```go
type TokenUsage struct {
    Input      int `json:"input"`
    Output     int `json:"output"`
    CacheRead  int `json:"cache_read,omitempty"`
    CacheWrite int `json:"cache_write,omitempty"`
    Total      int `json:"total"`
}
```

For compaction trigger purposes:
```
effectiveTokens = usage.Input + usage.CacheRead
```

The `CacheRead` tokens represent context the model processed from cache —
they still count against the context window even though they're cheaper.

### Token Counting

Three approaches, in preference order:
1. **Provider-reported usage**: From the last `Response.Usage` — most accurate
2. **Chars/4 heuristic**: For messages added since last response — conservative
3. **Configured context window**: From `CompactionConfig.ContextWindow` or
   provider metadata

### Events

New event types:
```go
EventCompactionStart EventType = "compaction.start"
EventCompactionEnd   EventType = "compaction.end"
```

The `compaction.end` event data includes the summary text, tokens before/after,
and file lists.

### Split Turn Handling

Following pi: if the cut point falls in the middle of a multi-message turn
(e.g., between a user message and its assistant response with tool calls),
generate a separate **turn prefix summary** with a smaller token budget.
This summary is appended to the main compaction summary as:

```
---

**Turn Context (split turn):**

## Original Request
[What the user asked]

## Early Progress
[Work done in the prefix]

## Context for Suffix
[Info needed to understand the kept suffix]
```

### Quality Degradation Warning

Following Codex: after every compaction, emit a warning event:

```
"Long conversations and multiple compactions can cause the model to be
less accurate. Consider starting a new session when possible."
```

This is emitted via `EventCallback` as an `EventCompactionEnd` with a
`warning` field, not printed to stderr (library, not CLI concern).

### Graceful Degradation

If the compaction prompt itself exceeds the context window:
1. Trim oldest messages from the summarization input **(from the front,
   to preserve prefix cache)**
2. If still too large, fall back to aggressive truncation (keep only the
   most recent messages, drop the summarization attempt)
3. Log a warning via callback

## Implementation Plan

| # | Bead | Depends |
|---|------|---------|
| 1 | Token estimation (chars/4 + provider usage, cache-aware) | — |
| 2 | Conversation serialization for summarization | — |
| 3 | Compaction config types and trigger logic | 1 |
| 4 | Summarization prompt and summary injection (cache-optimized ordering) | 2, 3 |
| 5 | File tracking across compactions | 4 |
| 6 | Mid-turn compaction in agent loop (with system context reinjection) | 4 |
| 7 | Update mode (merge with previous summary) + split turn handling | 4 |
| 8 | Integration test: multi-round task with compaction | 6 |

## Design Decisions Not Taken

- **Remote server-side compaction** (Codex has this for OpenAI's `/responses/compact`
  endpoint). Not included — forge is provider-agnostic and shouldn't depend on
  one provider's server-side API. If OpenAI or others expose this, it can be added
  as a provider-specific optimization later.
- **Branch summarization** (pi has this for conversation tree navigation). Not
  included — forge is headless with linear conversations, no branching.
- **Ghost snapshots / undo** (Codex preserves these across compaction). Not
  included — forge has no undo mechanism. If added later, the compaction should
  preserve snapshot items similarly to Codex.

## Risks

| Risk | Prob | Impact | Mitigation |
|------|------|--------|------------|
| Local model produces poor summaries | M | H | Allow dedicated summarization model; structured format constrains output |
| Token estimation inaccurate | M | M | Conservative estimate (chars/4 overestimates); triggers early rather than late |
| Multiple compactions degrade quality | M | M | Warn after compaction; update mode preserves prior summary content |
| Summarization adds latency | L | M | Use faster model for summarization; only triggers when needed |
| Compaction destroys prompt cache | H | M | Cache-optimized ordering (system prompt first, summary second); accepted cost |
| Cache token accounting wrong | M | M | Include CacheRead in effective token count; use provider-reported usage |
| Split turn summary inaccurate | L | L | Smaller token budget; separate focused prompt |
