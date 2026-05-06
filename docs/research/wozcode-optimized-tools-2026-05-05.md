---
ddx:
  id: wozcode-optimized-tools-2026-05-05
  created: 2026-05-05
  reviewed_by: pending
status: DRAFT v1
exit_criterion: Use this note to shape follow-up beads for Fizeau's third-party harness tool exposure plan.
---

# WozCode Optimized Tools Architecture

WozCode is a useful comparator for Fizeau's "advanced tools for third-party
harnesses" work. It is not merely an instruction file, and it is not merely an
MCP server. Public artifacts show a layered Claude Code plugin that combines
custom tools, agent defaults, hooks, skills, telemetry, and benchmarking.

## Sources Checked

- Product site: <https://www.wozcode.com/>
- Docs: <https://www.wozcode.com/docs>
- Public plugin repository: <https://github.com/WithWoz/wozcode-plugin>
- Plugin manifest: <https://raw.githubusercontent.com/WithWoz/wozcode-plugin/main/.claude-plugin/plugin.json>
- MCP config: <https://raw.githubusercontent.com/WithWoz/wozcode-plugin/main/.mcp.json>
- Agent profiles:
  - <https://raw.githubusercontent.com/WithWoz/wozcode-plugin/main/agents/code.md>
  - <https://raw.githubusercontent.com/WithWoz/wozcode-plugin/main/agents/explore.md>
  - <https://raw.githubusercontent.com/WithWoz/wozcode-plugin/main/agents/code-free.md>
- Hooks config: <https://raw.githubusercontent.com/WithWoz/wozcode-plugin/main/hooks/hooks.json>

## Observed Shape

WozCode installs as a Claude Code plugin:

```text
claude plugin marketplace add WithWoz/wozcode-plugin
claude plugin install woz@wozcode-marketplace
```

The plugin manifest describes the product as "enhanced coding tools" with
smart search, batch editing, SQL introspection, and cost-optimized subagent
delegation. The repo ships:

- `.claude-plugin/` metadata for Claude's plugin marketplace.
- `.mcp.json` with an always-loaded stdio MCP server named `code`, running
  `servers/code-server.js`.
- `agents/` profiles for `woz:code`, `woz:explore`, and a free-plan fallback.
- `hooks/hooks.json` with Claude hook wiring.
- `skills/` commands for login, savings, benchmark, settings, update, and
  related user workflows.
- `scripts/` hook and CLI helpers.

The public server bundle is minified/obfuscated and large, so this note does
not treat its internal implementation as reviewed. The plugin-level files are
enough to infer the product architecture.

## Tool Economy Pattern

The main `woz:code` agent disables Claude Code's built-in file tools:

```yaml
disallowedTools: Read, Edit, Write, Grep, Glob, NotebookEdit
```

That is the strongest architectural signal. WozCode does not rely on the model
choosing better tools opportunistically; it removes the lower-efficiency tools
from the main agent and forces the replacement surface.

The `woz:explore` agent is a read-only subagent configured on a cheaper model:

```yaml
model: haiku
tools: mcp__plugin_woz_code__Search, mcp__plugin_woz_code__Sql, Bash
disallowedTools: mcp__plugin_woz_code__Edit, Agent, Edit, Write, Read, Grep, Glob
```

Its prompt tells the agent to use search before full reads, parallelize
independent searches, and reserve Bash for shell-only tasks. This is close to
Fizeau's benchmark-preset and navigation-tool work, but with a dedicated
subagent role and tool allowlist.

## Hooks Pattern

The plugin uses hooks as guardrails and observability points:

- `SessionStart`: checks runtime prerequisites and injects action-required
  context if Node is unavailable.
- `PreToolUse` on `Bash`, the plugin Bash wrapper, and `Agent`: runs a
  redirect hook.
- `PostToolUse` on the Woz edit tool: runs an edit-batching nudge.
- `PostToolUse`, `SubagentStop`, `Stop`, `PreCompact`, and `PostCompact`: run
  session telemetry.

This suggests hooks are most useful as:

- redirects away from inefficient fallback behavior,
- post-tool nudges after non-optimal use,
- telemetry and savings accounting,
- prerequisite checks.

Hooks are not the whole product. They work because the plugin also supplies
replacement tools and agent profiles that make the desired behavior the default.

## Product Claims

The WozCode site claims:

- 30-40% faster on most tasks.
- Up to 10x faster on database tasks.
- Lower token and cost usage than Claude Code alone.
- TerminalBench 2.0 improvement over Claude Code alone.
- Local operation without proxying source code or prompts, with operational
  telemetry for usage and tool counts.

Treat these as vendor claims until reproduced under Fizeau's harness matrix.
The useful point for Fizeau is not the exact number; it is the optimization
strategy.

## Implications For Fizeau

The earlier "MCP vs CLI vs AGENTS.md" framing is too narrow. WozCode's lesson
is that optimized-tool products win by controlling the tool economy end to end:

1. Provide composite tools that collapse common multi-turn loops.
2. Make those tools the default through agent profiles or tool allowlists.
3. Disable or discourage lower-efficiency built-ins where the host allows it.
4. Use hooks to redirect bad behavior and measure savings.
5. Use skills/commands for user-facing workflows.
6. Benchmark savings locally against the vanilla harness.

For Fizeau, this argues for a portable core plus host-specific adapters:

- A durable `fiz tool` CLI should be the cross-harness base for Codex, Claude,
  Gemini, pi, and opencode.
- A Claude plugin adapter may be worthwhile because Claude's plugin system can
  supply always-loaded tools, agent profiles, and hooks.
- AGENTS.md and skills remain important, but they should teach and activate the
  optimized loop rather than carry the whole mechanism.
- Hooks should be optional accelerators and guardrails, not the only integration
  path.

## Candidate Fizeau Slices

### 1. Durable CLI Tool Surface

Expose Fizeau's optimized operations as shell-native commands:

```text
fiz tool search ...
fiz tool read --anchors ...
fiz tool anchor-edit ...
fiz tool edit --old ... --new ...
fiz tool patch ...
```

The key missing requirement for third-party harnesses is durable state. Native
anchor mode currently stores anchors in memory for one Fizeau run; external
harnesses invoke separate commands. A CLI anchor loop needs a session store
with file path, content hash, line count, anchor mapping, and invalidation.

### 2. Composite Search

WozCode's public materials emphasize search that combines globbing, regex, and
ranked snippets in one tool call. Fizeau already has `find`, `grep`, `ls`, and
`read`; the next efficient surface is a composite search command that returns
ranked, bounded snippets and avoids the `find -> grep -> read` round-trip
chain.

### 3. Batch Editing

Fizeau has exact edit, patch, and anchor edit. WozCode's differentiator is
batched multi-file editing. A Fizeau analogue should support a JSON edit plan
containing multiple file edits, apply them transactionally where possible, and
run local syntax validation for common formats.

### 4. Claude Plugin Adapter

A Claude-specific adapter can mirror the WozCode architecture:

- MCP server or plugin tool wrapper for Fizeau composite tools.
- Main agent profile that disallows built-in `Read/Edit/Write/Grep/Glob` when
  Fizeau tools are active.
- Read-only exploration subagent using cheaper model settings when available.
- PreToolUse hooks that redirect shell navigation and broad file rewrites.
- PostToolUse hooks that nudge batching and record savings metrics.

This adapter should be downstream of the CLI/core tools, not the primary
implementation.

### 5. Cross-Harness Skills And AGENTS.md

For Codex and other harnesses without the same plugin surface, install concise
instructions:

- Prefer `fiz tool search` over ad hoc `find`, `grep`, `rg`, `cat`, and `sed`
  exploration.
- Prefer anchored or batch edits for targeted changes.
- Use standard shell only for commands that Fizeau tools do not cover.
- Re-read or refresh anchors after any non-anchor edit.

## Open Questions

- Can Codex be configured with a per-run tool allowlist or hook equivalent
  similar to Claude plugin agents?
- Should Fizeau implement a Claude plugin, or generate a minimal project-local
  Claude configuration that references installed `fiz` commands?
- What is the smallest composite search result format that improves benchmark
  behavior without hiding too much file context?
- Should batch edit be exact-match, fuzzy-match, anchor-based, or a layered
  strategy?
- What local benchmark should compare vanilla Claude/Codex against
  Fizeau-instructed and Fizeau-plugin runs?

## Provisional Recommendation

Use WozCode as evidence for a layered integration strategy:

1. Build optimized Fizeau tools as reusable core/CLI operations first.
2. Add skills and AGENTS.md snippets for broad harness compatibility.
3. Add a Claude plugin adapter only after the CLI tools prove useful.
4. Use hooks to redirect and measure, not to carry core functionality.
5. Add local side-by-side benchmarking as a first-class command or bead.

The product thesis is: reduce turns and context growth by replacing primitive
file operations with composite, validated, batchable operations, then make the
model use them by default.
