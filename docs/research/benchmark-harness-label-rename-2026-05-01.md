---
ddx:
  id: benchmark-harness-label-rename-2026-05-01
  bead: agent-b69923cf
  parent: agent-2b694e0e
  created: 2026-05-01
---

# FZ-041a benchmark harness label rename decision

This is a decision artifact only. It decides how benchmark labels that still
say `ddx-agent` map into the Fizeau rename.

## Decision

Active benchmark harness labels for this product become `fiz`.

`ddx-agent` is not retained as the active matrix harness label. In the benchmark
matrix, rows compare CLI harnesses such as `pi`, `opencode`, and this product's
CLI. The current product CLI is `fiz`, so `fiz` is the stable product-facing
benchmark label.

Do not add a supported `ddx-agent` compatibility alias for new benchmark runs.
This follows the Fizeau rename policy: no compatibility window, and old command
names are historical evidence rather than active supported names.

## Product-facing labels

The following active surfaces are product-facing and should use `fiz` in the
benchmark implementation and active benchmark docs:

| Surface | Current old label | Target label | Reason |
|---|---|---|---|
| `fiz-bench matrix --harnesses` value for this product | `ddx-agent` | `fiz` | Users type this value and it selects the product row. |
| Matrix defaults and examples | `ddx-agent,pi,opencode` | `fiz,pi,opencode` | Defaults and examples define current operator behavior. |
| `matrix.json` / `report.json` `harness` values | `ddx-agent` | `fiz` | These artifacts are published and compared across runs. |
| `matrix.md` row and cell labels | `ddx-agent` | `fiz` | Report readers see these as benchmark result labels. |
| New result directory path segments derived from harness labels | `ddx-agent` | `fiz` | Paths are part of the active result contract and should match the row label. |
| TerminalBench / Harbor `AgentInfo.Name` for new runs | `ddx-agent` | `fiz` | Harbor reporters use this name as the leaderboard/executor label. |
| The product adapter's active Python module path used by new matrix invocations | `ddx_agent.py` / `ddx-agent` | `fiz.py` / `fiz` | SD-010 maps harness labels to adapter modules; the visible active adapter should match the canonical harness ID. |
| Installed in-container executable name for new benchmark cells | `/installed-agent/ddx-agent` | `/installed-agent/fiz` | The cell executes the product CLI binary, whose active name is `fiz`. |
| Active benchmark operator docs and procedure snippets | `ddx-agent` / `ddx-agent-bench` | `fiz` / `fiz-bench` | These instruct current use rather than recording past runs. |

## Domain and implementation terms that remain

The rename does not change generic benchmark or Harbor terminology:

| Surface | Decision | Reason |
|---|---|---|
| `AgentInfo` Go type and JSON object name | Keep | TerminalBench / Harbor schema term for the executor recorded in a trajectory. |
| `BaseInstalledAgent` and Harbor "agent" vocabulary | Keep | Third-party benchmark domain terminology, not product branding. |
| `HARBOR_AGENT_ARTIFACT` | Keep | Harbor adapter artifact input name; it is not a Fizeau-owned public env var. |
| Go/Python field names such as `Harness`, `harness`, `adapter`, `agent` | Keep | Generic benchmark model fields. Only their product string values change. |
| Generic prose such as "agent harness" or "agent exits" | Keep when generic | Describes the benchmark domain, not this product's old name. |

## Historical labels

`ddx-agent` remains allowed only where it records old evidence:

- Archived benchmark baselines, old matrix memos, and old command transcripts
  produced before the rename decision.
- `.ddx/` tracker records, execution evidence, and review logs.
- HELIX or research documents that explicitly describe pre-rename history.
- Cleanup inventories that cite old strings for audit purposes.

Historical material should not be mechanically rewritten just to satisfy a
grep. If a document is an active operator procedure or an active normative
benchmark spec, it is not historical for this purpose and should use the target
labels above.

## Evidence used

- `docs/research/fizeau-rename-allowlist-2026-04-30.md` classifies `ddx-agent`
  as forbidden in active CLI commands, examples, release assets, and active
  benchmark operator instructions, while allowing historical evidence.
- `.ddx/notes/FZ-029-user-facing-runtime-audit.md` deferred the benchmark
  harness adapter label to the dedicated benchmark rename beads.
- `docs/helix/02-design/solution-designs/SD-010-harness-matrix-benchmark.md`
  defines the matrix `--harnesses` input and output artifacts; those labels are
  visible in active reports rather than purely private implementation details.
- `internal/benchmark/external/termbench/trajectory.go` records `AgentInfo.Name`
  as the label Harbor reporters use for benchmark output.

## Acceptance traceback

Bead `agent-b69923cf` requires a decision note stating label policy and which
labels are product-facing versus historical.

- Label policy: active benchmark product labels become `fiz`; no new
  `ddx-agent` compatibility alias.
- Product-facing labels: enumerated in "Product-facing labels".
- Historical labels: enumerated in "Historical labels".
