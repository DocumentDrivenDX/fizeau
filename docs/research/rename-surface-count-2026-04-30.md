---
ddx:
  id: rename-surface-count-2026-04-30
  bead: agent-71632f5a
  parent: agent-996dca04
  base-rev: 10446491543d694cecf0ade4a542980f305e5ed9
  current-rev: f6139871ddf1ed97011a77d7bec17d96028b995b
  created: 2026-04-30
---

# CL-008 post-cleanup rename-surface count

This is a measurement note only. It records the remaining old-name surface
after the cleanup delete/archive beads landed so Fizeau rename beads can work
from a bounded file set. No source, docs, config, or command names are renamed
by this bead.

No `docs/research/rename-surface-inventory-2026-04-30.md` file exists in this
worktree, so the comparison baseline is the cleanup-inventory baseline commit
`10446491543d694cecf0ade4a542980f305e5ed9`, cited by
`docs/research/cleanup-go-inventory-2026-04-30.md`. Cleanup inventory fallback
context also comes from:

- `docs/research/docs-cleanup-inventory-2026-04-30.md`, whose evidence section
  records active old-name docs, HELIX old-name docs, and research old-name docs.
- `docs/research/scripts-fixtures-assets-cleanup-inventory-2026-04-30.md`,
  whose CL-007 rows record the archive targets that have since landed.

## Scope

Primary Fizeau target scope excludes DDx tracker/audit files and historical
evidence docs:

```
--hidden
-g '!.git/**'
-g '!.ddx/**'
-g '!.agents/**'
-g '!.claude/**'
-g '!docs/research/**'
-g '!docs/helix/**'
-g '!docs/research/rename-surface-count-2026-04-30.md'
```

This leaves active source, tests, scripts, website, root docs, routing docs,
fixtures, and workflow/config files as the practical rename target surface.
The root `package agent` count is intentionally scoped to `./*.go`.

## Commands

Baseline counts were collected by extracting the baseline revision and running
the same commands inside that temporary tree:

```
BASE=10446491543d694cecf0ade4a542980f305e5ed9
BASE_TMP=$(mktemp -d /tmp/rename-surface-baseline.XXXXXX)
git archive "$BASE" | tar -x -C "$BASE_TMP"
```

Current counts were run from repository root at
`f6139871ddf1ed97011a77d7bec17d96028b995b`. Occurrence counts use `rg -o` and
file counts use `rg -l` with the same pattern and scope.

Old module path:

```bash
rg --hidden -g '!.git/**' -g '!.ddx/**' -g '!.agents/**' -g '!.claude/**' -g '!docs/research/**' -g '!docs/helix/**' -g '!docs/research/rename-surface-count-2026-04-30.md' -F -o 'github.com/DocumentDrivenDX/agent' | wc -l
rg --hidden -g '!.git/**' -g '!.ddx/**' -g '!.agents/**' -g '!.claude/**' -g '!docs/research/**' -g '!docs/helix/**' -g '!docs/research/rename-surface-count-2026-04-30.md' -F -l 'github.com/DocumentDrivenDX/agent' | wc -l
```

Root `package agent`:

```bash
rg -n '^package agent$' ./*.go | wc -l
rg -l '^package agent$' ./*.go | wc -l
```

`ddx-agent`:

```bash
rg --hidden -g '!.git/**' -g '!.ddx/**' -g '!.agents/**' -g '!.claude/**' -g '!docs/research/**' -g '!docs/helix/**' -g '!docs/research/rename-surface-count-2026-04-30.md' -F -o 'ddx-agent' | wc -l
rg --hidden -g '!.git/**' -g '!.ddx/**' -g '!.agents/**' -g '!.claude/**' -g '!docs/research/**' -g '!docs/helix/**' -g '!docs/research/rename-surface-count-2026-04-30.md' -F -l 'ddx-agent' | wc -l
```

`DDX Agent` / `DDx Agent`:

```bash
rg --hidden -g '!.git/**' -g '!.ddx/**' -g '!.agents/**' -g '!.claude/**' -g '!docs/research/**' -g '!docs/helix/**' -g '!docs/research/rename-surface-count-2026-04-30.md' --pcre2 -o 'DDX Agent|DDx Agent' | wc -l
rg --hidden -g '!.git/**' -g '!.ddx/**' -g '!.agents/**' -g '!.claude/**' -g '!docs/research/**' -g '!docs/helix/**' -g '!docs/research/rename-surface-count-2026-04-30.md' --pcre2 -l 'DDX Agent|DDx Agent' | wc -l
```

`.agent`:

```bash
rg --hidden -g '!.git/**' -g '!.ddx/**' -g '!.agents/**' -g '!.claude/**' -g '!docs/research/**' -g '!docs/helix/**' -g '!docs/research/rename-surface-count-2026-04-30.md' -F -o '.agent' | wc -l
rg --hidden -g '!.git/**' -g '!.ddx/**' -g '!.agents/**' -g '!.claude/**' -g '!docs/research/**' -g '!docs/helix/**' -g '!docs/research/rename-surface-count-2026-04-30.md' -F -l '.agent' | wc -l
```

`AGENT_*`:

```bash
rg --hidden -g '!.git/**' -g '!.ddx/**' -g '!.agents/**' -g '!.claude/**' -g '!docs/research/**' -g '!docs/helix/**' -g '!docs/research/rename-surface-count-2026-04-30.md' --pcre2 -o '(?<![A-Z0-9_])AGENT_[A-Z0-9_]+' | wc -l
rg --hidden -g '!.git/**' -g '!.ddx/**' -g '!.agents/**' -g '!.claude/**' -g '!docs/research/**' -g '!docs/helix/**' -g '!docs/research/rename-surface-count-2026-04-30.md' --pcre2 -l '(?<![A-Z0-9_])AGENT_[A-Z0-9_]+' | wc -l
```

`DDX_AGENT_*`:

```bash
rg --hidden -g '!.git/**' -g '!.ddx/**' -g '!.agents/**' -g '!.claude/**' -g '!docs/research/**' -g '!docs/helix/**' -g '!docs/research/rename-surface-count-2026-04-30.md' --pcre2 -o '(?<![A-Z0-9_])DDX_AGENT_[A-Z0-9_]+' | wc -l
rg --hidden -g '!.git/**' -g '!.ddx/**' -g '!.agents/**' -g '!.claude/**' -g '!docs/research/**' -g '!docs/helix/**' -g '!docs/research/rename-surface-count-2026-04-30.md' --pcre2 -l '(?<![A-Z0-9_])DDX_AGENT_[A-Z0-9_]+' | wc -l
```

## Primary counts

| surface | baseline occurrences | baseline files | current occurrences | current files | occurrence delta | file delta |
|---|---:|---:|---:|---:|---:|---:|
| old module path `github.com/DocumentDrivenDX/agent` | 528 | 251 | 525 | 249 | -3 | -2 |
| root `package agent` | 49 | 49 | 49 | 49 | 0 | 0 |
| `ddx-agent` | 186 | 69 | 184 | 68 | -2 | -1 |
| `DDX Agent` / `DDx Agent` | 32 | 18 | 29 | 17 | -3 | -1 |
| `.agent` | 104 | 30 | 104 | 30 | 0 | 0 |
| `AGENT_*` | 105 | 21 | 105 | 21 | 0 | 0 |
| `DDX_AGENT_*` | 68 | 14 | 68 | 14 | 0 | 0 |

The cleanup deletes reduced the actionable target set only where stale files
carried old module, CLI, or product-name strings. Root package names and
environment-variable surfaces are unchanged; those remain real Fizeau work.

## Audit-scope check

For traceability, the same count was also run across all tracked project
content excluding only `.git`, `.ddx`, `.agents`, `.claude`, and this note. That
scope includes HELIX and research history. Its counts increase in several rows
because the cleanup inventory notes themselves contain old-name evidence.

| surface | baseline occurrences | baseline files | current occurrences | current files | occurrence delta | file delta |
|---|---:|---:|---:|---:|---:|---:|
| old module path `github.com/DocumentDrivenDX/agent` | 541 | 256 | 608 | 256 | +67 | 0 |
| root `package agent` | 49 | 49 | 49 | 49 | 0 | 0 |
| `ddx-agent` | 606 | 117 | 612 | 119 | +6 | +2 |
| `DDX Agent` / `DDx Agent` | 212 | 55 | 215 | 56 | +3 | +1 |
| `.agent` | 133 | 42 | 135 | 43 | +2 | +1 |
| `AGENT_*` | 129 | 27 | 131 | 28 | +2 | +1 |
| `DDX_AGENT_*` | 68 | 14 | 68 | 14 | 0 | 0 |

This audit-scope result is not the practical rename queue; it confirms why
Fizeau beads should keep historical evidence docs out of the mechanical rename
target unless a governing follow-up explicitly requires updating one of them.

## Acceptance traceback

Bead `agent-71632f5a` AC: *"A docs/research note records counts for old module
path, package agent at root, ddx-agent, DDX Agent/DDx Agent, .agent, AGENT_*,
DDX_AGENT_* and compares them to the pre-cleanup baseline."*

- Counts are recorded in the primary table for every named surface.
- Exact `rg` commands and path scope are recorded in the commands section.
- The comparison baseline is the cleanup-inventory baseline revision
  `10446491543d694cecf0ade4a542980f305e5ed9`; the current revision is
  `f6139871ddf1ed97011a77d7bec17d96028b995b`.
- The audit-scope table explains the difference between historical evidence
  growth and the smaller practical Fizeau target set.
