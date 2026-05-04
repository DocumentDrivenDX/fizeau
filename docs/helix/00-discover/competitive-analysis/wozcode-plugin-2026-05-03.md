---
ddx:
  id: helix.discover.competitive.wozcode-plugin
  type: competitive-analysis
  captured: 2026-05-03
---
# Competitive Analysis — WozCode Plugin (Claude Code marketplace)

**Subject**: https://github.com/WithWoz/wozcode-plugin
**Captured by**: Erik LaBianca (research delegated to a coding-agent
sub-agent on 2026-05-03)
**Status**: Initial review. Source not cloned; findings derived from
README, plugin manifests, agent definitions, skills, and hook config
fetched from the public repo. Schema details for the largest tool
(`Search`) are inferred from agent docs because `servers/code-server.js`
(~22 MB single bundled file) exceeded fetch limits.

## Why we looked

Fizeau today exposes its tool registry only to its own native agent
loop. WozCode is a commercial Claude Code plugin selling "smarter tools
for Claude Code that reduce token usage and cost." The shape of how
they reach the host harness is directly relevant to whether fizeau
should grow a similar surface, and the consolidation of their tool set
is directly relevant to FEAT-002's design center.

## TL;DR

- WozCode is a **closed-source, bundled MCP server** loaded by Claude
  Code as a marketplace plugin. It ships a tiny set of "fat"
  replacement tools (`Search`, `Edit`, `Sql`, `Recall`, wrapped `Bash`)
  and uses the host's loop, not its own.
- Integration is pure Claude-Code plugin convention: `.mcp.json` +
  `.claude-plugin/plugin.json` + `agents/*.md` that **`disallowedTools`**
  the host's `Read/Edit/Write/Grep/Glob/NotebookEdit` so the model is
  forced onto the WOZ tools — no skill is required for steering because
  the agent frontmatter does it.
- Most of the realized savings come from a **`PreToolUse` Bash hook**
  that rewrites reflexive shell calls (`rg|grep|find|cat`) to MCP tool
  calls. This is the "where the tokens actually go" insight.

## Tool surface

WozCode collapses Claude Code's six file primitives
(`Read/Edit/Write/Grep/Glob/NotebookEdit`) into a small namespaced set
registered by `servers/code-server.js`:

- **`Search`** — unified read + grep + glob + symbol lookup. Agent doc
  `agents/explore.md` instructs the model to "locate the right starting
  point with glob patterns and regex searches before reading full
  files" and to "launch independent searches in parallel." Inferred
  shape: regex + path glob + optional read-range, returning matched
  chunks rather than whole files.
- **`Edit`** — batched multi-file editor. A `PostToolUse` hook
  (`hooks/hooks.json` → `edit-batching-nudge.js`) actively nudges the
  model to coalesce edits, which only makes sense if `Edit` accepts an
  array of patches per call (or at least benefits from sequential
  coalescing).
- **`Sql`** — SQLite/DB introspection. README claims "up to 10× faster
  on database tasks."
- **`Recall`** — semantic search over prior Claude Code session
  transcripts. Single SKILL.md (`skills/woz-recall/SKILL.md`) teaches
  the model trigger phrases ("remember when", "last time", "how did
  we") and one canonical invocation:
  `mcp__plugin_woz_code__Recall({ query: "..." })`.
- **`Bash`** — wrapped Bash, intercepted by a `PreToolUse` hook
  (`hooks/tool-redirect-hook.js`, matchers `Bash`, the namespaced WOZ
  Bash, and `Agent` in `hooks/hooks.json`) that rewrites common shell
  shapes to call `Search` instead.

Why: README is explicit — *"Smarter tools for Claude Code that reduce
token usage and cost… fewer tokens per tool call means cheaper
sessions."* The `/woz-savings` command (`scripts/savings-report.js`)
reports calls / time / tokens saved, and `standalone/savings-check.js`
scans `~/.claude/projects/*.jsonl` to compute the metric.

## Host-harness integration shape

Not in-process, no CLI subprocess RPC. It is the standard Claude Code
plugin layout:

- `.claude-plugin/plugin.json` and `.claude-plugin/marketplace.json`
  register the plugin (`plugins[0].source: "./"`).
- `.mcp.json` declares one MCP server: `node servers/code-server.js`,
  `alwaysLoad: true`, with PostHog telemetry env vars baked in.
- `hooks/hooks.json` registers `SessionStart`, `PreToolUse` (matchers
  `Bash`, `mcp__plugin_woz_code__Bash`, `Agent`), `PostToolUse`,
  `SubagentStop`, `Stop`, `PreCompact`, `PostCompact` — all Node
  scripts under `${CLAUDE_PLUGIN_ROOT}/scripts/`.
- `settings.json` ships `{"agent": "woz:code"}` so the plugin
  auto-selects its own subagent as the main thread.
- A **Conductor adapter** is the only non-Claude-Code surface:
  `wozcode conductor` prints the path of a `claude-woz` shim
  executable that Conductor uses as its "Claude Code executable" — i.e.
  they wrap the host CLI rather than expose tools to a different
  harness.

Permissions/approvals are **delegated entirely to Claude Code** — no
custom approval UI. Auth to WozCode's backend is handled out-of-band
via `/woz-login` (browser OAuth or `--token` paste), credentials cached
by `scripts/wozcode-cli.js`. A "free plan fallback" agent
(`agents/code-free.md`) re-enables the built-in tools and disallows
`Search/Edit/Sql` when the user's quota is exhausted — a clean
degradation pattern.

## Steering the host model

There is **no SKILL.md that promotes the search/edit tools**. Steering
is done purely through agent frontmatter:

- `agents/code.md` — `description: "WozCode enhanced coding agent with
  smart search, batch editing, and SQL introspection. Use as the
  default main thread agent."`, `model: inherit`,
  **`disallowedTools: [Read, Edit, Write, Grep, Glob, NotebookEdit]`**.
- `agents/explore.md` — restricted to `Search, Sql, Bash`, runs on
  Haiku for cheap multi-call scans.
- `agents/code-free.md` — inverse list (disallows `Search, Edit, Sql`)
  for the free-plan fallback.

Because `settings.json` pins `agent: woz:code` and the WOZ tools are
namespaced (`mcp__plugin_woz_code__*`), the model literally cannot see
the built-in equivalents — no prompting required. The `skills/`
directory is reserved for *operational* concerns
(`woz-login`, `woz-recall`, `woz-savings`, `woz-share`, `woz-status`,
`woz-update`, `woz-benchmark`, `woz-settings`); each is a thin SKILL.md
with trigger phrases and a single shell command to
`scripts/wozcode-cli.js`. Only `woz-recall/SKILL.md` actually teaches
the model to call an MCP tool.

Why: agent frontmatter is a hard constraint enforced by the harness; a
skill is just advisory text the model may ignore. Disabling competing
tools is more reliable than persuading.

## Lessons for fizeau (ranked by expected impact)

1. **Fat tools beat thin tools.** Collapse Read+Grep+Glob into one
   `search` (regex + glob + slice) and Edit+Write+NotebookEdit into one
   batched `edit`. Single biggest token-saving lever WozCode pulls;
   harness-agnostic.
2. **Ship a `fizeau mcp` (stdio MCP server) entry point.** Mirror the
   `.mcp.json` pattern. Fizeau stays an embedded library for its own
   loop *and* gains a server mode for free.
3. **Generate a host installer bundle** (`.claude-plugin/` +
   `agents/fizeau.md` + a couple of SKILL.md files). Use
   `disallowedTools` to *force* the host model onto fizeau's tools
   rather than hoping prompting works.
4. **Add a Bash-call interceptor** equivalent to
   `hooks/tool-redirect-hook.js` that rewrites `rg|grep|find|cat`
   invocations to the native search/read tool. Models reach for shell
   reflexively; this is where the realized savings actually land.
   Keep it inside fizeau's tool implementation (not a host hook) so it
   works in MCP, native loop, and any future host integration.
5. **Instrument and surface savings.** WozCode's `/woz-savings` and
   `standalone/savings-check.js` (read-only scan of
   `~/.claude/projects/*.jsonl`) double as a marketing tool *and* a
   feedback loop for tool design. A `fizeau metrics` subcommand
   counting tool calls / tokens / round-trips per session would let
   fizeau justify and tune the consolidation.

## Non-applicable items

- **License**: WozCode is proprietary ("© WozCode. All rights
  reserved."). Copy *patterns*, not code.
- **Language mismatch**: `code-server.js` is JavaScript using
  `@napi-rs/canvas` (image rendering for status badges?) and Zod.
  Fizeau is Go — reimplement, don't port. `mark3labs/mcp-go` covers
  the MCP server surface.
- **Auth/SaaS coupling**: `/woz-login`, PostHog telemetry tokens
  hardcoded in `.mcp.json`, gated free vs. paid agents. Fizeau is an
  embedded runtime — no backend account, no telemetry-by-default.
- **`spinnerVerbs`, `attribution`, status-line cosmetics**: Claude
  Code-only chrome. Skip.
- **Conductor adapter**: only relevant if a third-party harness expects
  to *launch a CLI binary that pretends to be Claude Code*. Probably
  not on fizeau's roadmap.
- **Single-agent-pinning via `settings.json`**: WozCode owns the
  user's main thread by default. Fizeau likely shouldn't — it should
  coexist as one MCP server among many.

## Open questions

- **Exact `Search`/`Edit` schemas.** The 22 MB `code-server.js`
  exceeded fetch limits, so the precise input shapes (does `Search`
  return ranked snippets? line ranges? embeddings via `Recall`?) are
  inferred from agent docs and savings claims, not read directly. To
  confirm: clone locally and
  `rg -n 'name:\s*"(Search|Edit|Sql|Recall|Bash)"' servers/code-server.js`.
- **Is `Recall` server-side (WozCode SaaS embeddings) or local?**
  `/woz-login` requirement and PostHog wiring suggest server-side.
  Fizeau's local equivalent could be backed by `.fizeau/sessions/*.jsonl`
  (which already exist).
- **How does `Edit` batch?** A single tool call with N patches, or
  sequential with hook-driven coalescing? `edit-batching-nudge.js`
  hints at the latter, but a true batched schema would be stronger.
- **Hook architecture portability.** Claude Code hooks
  (`SessionStart`, `PreToolUse`, `PreCompact`, …) are harness-specific.
  Codex and other harnesses don't have an equivalent — fizeau's
  MCP-side tool interception (e.g. the Bash redirect) would have to
  live inside the MCP server itself, not in a host hook.

## Suggested next moves

- Frame phase: open a feature spec for "host harness tool surface"
  drawing on lessons 1–4 once the open questions on `Search`/`Edit`
  schemas are resolved.
- Discover phase follow-up: pull the WozCode source locally and
  confirm the inferred tool schemas; capture any concrete schema as an
  appendix to this note.
- Tactical: prototype the Bash-call interceptor inside fizeau's
  existing `bash` tool — it's the lowest-risk lesson and validates the
  "where the tokens actually go" thesis without committing to MCP work.

## Key files referenced

All in `WithWoz/wozcode-plugin@main`:

- `.mcp.json`, `.claude-plugin/plugin.json`,
  `.claude-plugin/marketplace.json`
- `settings.json`
- `hooks/hooks.json` (and the per-hook scripts under
  `${CLAUDE_PLUGIN_ROOT}/scripts/`)
- `agents/code.md`, `agents/explore.md`, `agents/code-free.md`
- `skills/woz-recall/SKILL.md`, `skills/woz-savings/SKILL.md`,
  `skills/woz-benchmark/SKILL.md`
- `servers/code-server.js`
- `standalone/savings-check.js`, `scripts/wozcode-cli.js`,
  `scripts/savings-report.js`
