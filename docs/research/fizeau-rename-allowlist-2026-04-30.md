---
ddx:
  id: fizeau-rename-allowlist-2026-04-30
  bead: agent-8409c544
  parent: agent-2b694e0e
  base-rev: cd0b14984c175f4a84e3ca3afaa7d9e800445667
  current-rev: cd0b14984c175f4a84e3ca3afaa7d9e800445667
  created: 2026-04-30
---

# Fizeau rename old-name allowlist

This note defines the old-name taxonomy for the Fizeau rename before
mechanical implementation starts. It is a decision artifact only: no code,
configuration, or existing documentation is renamed here.

The active target names come from `docs/research/rename-fizeau-2026-04-27.md`:

- Project, repository, Go module, and root package: `fizeau`
- CLI binary: `fiz`
- Config and state namespace: `fizeau`
- Environment prefix: `FIZEAU_*`

The rename has no compatibility window. Active runtime behavior must not keep
reading old paths, env vars, module paths, or command names as aliases.

## Taxonomy

| classification | meaning for rename beads |
|---|---|
| forbidden | Old product-name surface must not remain in active code, public docs, install/update flows, tests that assert supported behavior, release assets, or examples. Rename it to the Fizeau target or remove it. |
| historical | Old surface may remain only as evidence of what existed at the time: archived research, HELIX history, changelog entries, DDx tracker/audit records, execution artifacts, old release notes, and old command transcripts. Do not rewrite history just to satisfy a grep. |
| domain-allowed | The string may remain in active material only when it is not the product identity, or when it is explicitly negative migration/detection text saying the old surface is unsupported. These uses must not create a working compatibility alias. |
| external-standard | The string belongs to a separate standard or integration namespace that this rename does not own. Preserve it unless a separate governing artifact changes that external contract. |

## Kill list and allowlist

| old-name surface | active classification | forbidden active uses | allowed contexts |
|---|---|---|---|
| `github.com/DocumentDrivenDX/agent` | forbidden | `go.mod`, Go imports, README install snippets, website examples, generated docs, release/update URLs, and new instructions must use the Fizeau module/repository path. | Historical references may remain in archived research, HELIX plans, changelog entries, old release evidence, `.ddx/` audit data, and DDx execution artifacts. Domain-allowed active migration text may cite it only as the unsupported old module path, paired with the Fizeau replacement. |
| root `package agent` | forbidden | Root Go files must move to `package fizeau`. New root-package examples, tests, and godoc must not preserve `package agent` as the supported package name. | Historical code snippets and inventory notes may retain it. Generic prose about "an agent" is domain-allowed, but exact root-package declarations are not. |
| `ddx-agent` | forbidden | Supported CLI command, `cmd/` path, Makefile targets, installer/updater names, release asset names, docs examples, website copy, and active benchmark operator instructions must use `fiz` or the final Fizeau benchmark binary name. | Historical transcripts, old release evidence, archived benchmark comparisons, and cleanup/rename inventories may retain it. The DDx CLI command phrase `ddx agent` with a space is domain-allowed because it names DDx's agent subsystem, not this product binary. |
| `DDX Agent` / `DDx Agent` | forbidden | Active product title, README heading, website content, package comments, UI/log branding, docs that describe current behavior, and release notes for new versions must say `Fizeau`. | Historical docs may keep the old product title. Lower-case generic references such as "a DDx agent run" are domain-allowed when they refer to DDx orchestration rather than the product brand. |
| `.agent` | forbidden | Supported project-local config, session, cache, fixture, or example paths must move to `.fizeau` or the chosen Fizeau path. Active code must not read `.agent` as a fallback alias. | Historical examples may remain. Negative detection or warning text may mention `.agent` only to tell users it is obsolete. `.agents/` is external-standard and is not the same surface; do not mechanically rename `.agents/skills/**`. |
| `~/.config/agent` | forbidden | Supported XDG config paths and docs must use `~/.config/fizeau`. Active config loading must not read `~/.config/agent` as a compatibility path. | Historical docs may remain. Domain-allowed active migration or error text may mention `~/.config/agent` only as an unsupported old path and must point to `~/.config/fizeau`. |
| `AGENT_*` | forbidden | Exported env vars for provider config, debug flags, install options, harness record mode, tests, scripts, docs, and examples must use `FIZEAU_*` when they are owned by this repository. | Historical evidence may remain. Domain-allowed active uses are limited to local, unexported implementation variables that do not document or consume a process env contract, or negative tests/docs proving old env vars are rejected. |
| `DDX_AGENT_*` | forbidden | Fizeau-owned env vars for binary selection, benchmark metadata, harness quota caches, auth/session overrides, tests, scripts, and docs must not remain as supported active env names; use `FIZEAU_*` or a more specific Fizeau-owned prefix. | Historical evidence may remain. External-standard use is allowed only if a separate DDx-owned contract explicitly defines a `DDX_AGENT_*` variable outside this product's namespace; cite that contract at the use site. |

## External standards not in the kill list

These strings look similar to old product names but are not Fizeau product
identity and should not be renamed by mechanical old-name beads:

- `AGENTS.md`: repository instruction file convention for coding agents.
- `.agents/skills/**`: agentskills.io-compatible local skill directory.
- `.claude/agents/**` or other harness-local agent directories.
- Generic nouns and type descriptions such as `agent`, `agent loop`,
  `agent run`, `agent harness`, and `agent service` when they describe the
  domain rather than the product brand.

## Mechanical rename guidance

1. First replace forbidden active surfaces in source, tests, build scripts,
   install/update flows, website content, README/CONTRIBUTING, and routing docs.
2. Exclude `.git/`, `.ddx/`, DDx execution evidence, archived research, and
   explicitly historical HELIX artifacts from mechanical grep gates unless a
   follow-up bead names one of those files.
3. Keep any active old-name mention narrowly worded as negative migration or
   rejection text. The old string should not be accepted as a working alias.
4. Treat `.agent` and `.agents/` as different strings. The former is old
   product config state; the latter is an external skill standard.
5. For env vars, distinguish public process env names from local shell or Go
   variable names. Public env names matching `AGENT_*` or `DDX_AGENT_*` are
   forbidden unless an explicit external DDx contract is cited.

## Acceptance traceback

Bead `agent-8409c544` requires this file to classify:

- `github.com/DocumentDrivenDX/agent`: classified above as forbidden in active
  surfaces, historical in evidence, and domain-allowed only for negative
  migration text.
- root `package agent`: classified above as forbidden for the root package and
  historical/domain-allowed only outside exact active package declarations.
- `ddx-agent`: classified above as forbidden for the product CLI and historical
  or domain-allowed for old evidence and the separate `ddx agent` command.
- `DDX Agent` / `DDx Agent`: classified above as forbidden for product branding
  and historical/domain-allowed for old docs or generic DDx orchestration prose.
- `.agent`: classified above as forbidden for product config state, historical
  for old evidence, and external-standard only for the distinct `.agents/` path.
- `~/.config/agent`: classified above as forbidden as a supported config path
  and historical/domain-allowed only for old evidence or unsupported-path hints.
- `AGENT_*`: classified above as forbidden for Fizeau-owned public env vars and
  historical/domain-allowed only for evidence, local non-env variables, or
  rejection tests.
- `DDX_AGENT_*`: classified above as forbidden for Fizeau-owned public env vars,
  historical for evidence, and external-standard only with an explicit separate
  DDx contract citation.
