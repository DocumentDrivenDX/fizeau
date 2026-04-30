---
ddx:
  id: docs-cleanup-inventory-2026-04-30
  bead: agent-e965bac2
  parent: agent-996dca04
  criteria: cleanup-criteria-2026-04-30
  created: 2026-04-30
---

# CL-003 docs cleanup inventory

This inventory classifies duplicate, stale, and historical documentation before
the Fizeau rename. It is an inventory only: no existing docs are edited,
deleted, or moved by CL-003.

The classification rules come from
`docs/research/cleanup-criteria-2026-04-30.md`: historical HELIX and changelog
evidence is kept by default; docs with active links, tests, or website
inclusion are kept; superseded orphan docs may be deleted; historically useful
orphan docs may be archived.

## Evidence commands

The rows below cite these repository checks, all run from the repository root on
2026-04-30:

- `find docs -type f -name '*.md' | wc -l` -> 76 docs under `docs/`.
- `git ls-files 'website/content/*.md' 'website/content/**/*.md' 'docs/routing*.md' 'docs/routing/**/*.md' 'docs/research/*.md' 'docs/helix/**/*.md' README.md CONTRIBUTING.md bench/README.md scripts/benchmark/README.md scripts/beadbench/README.md scripts/benchmark/tool-evolution-matrix.md | wc -l` -> 86 tracked doc surfaces in this inventory scope.
- `rg -l 'DDX Agent|ddx-agent|AGENT_|\.agent|github.com/DocumentDrivenDX/agent' README.md CONTRIBUTING.md bench/README.md scripts/**/*.md website/content/**/*.md docs/routing/**/*.md docs/routing-profile-*.md` -> active non-HELIX old-name docs: `README.md`, `CONTRIBUTING.md`, `bench/README.md`, `docs/routing/index.md`, `docs/routing/override-precedence.md`, `docs/routing/profiles.md`, `scripts/benchmark/README.md`, `scripts/benchmark/cost-guards/README.md`, `website/content/_index.md`, `website/content/demos/_index.md`, `website/content/docs/getting-started.md`.
- `rg -l 'DDX Agent|ddx-agent|AGENT_|\.agent|github.com/DocumentDrivenDX/agent' docs/helix | wc -l` -> 51 HELIX docs with old-name surface, all treated as historical by CL-001 unless a separate bead proves otherwise.
- `rg -l 'DDX Agent|ddx-agent|Fizeau|fizeau|lucebox|luce' docs/research | wc -l` -> 12 research docs with rename or prior-name surface.

## Inventory

| id | path | type | status | cleanup classification | reason | evidence | superseder / replacement | follow-up |
|---|---|---|---|---|---|---|---|---|
| `CL-003.01` | `docs/routing-profile-catalog.md` | docs-redirect | delete | delete | Superseded redirect stub; no active inbound link remains. | `rg -n -e 'routing-profile-catalog\.md' -e 'docs/routing-profile-catalog' -e 'routing-profile-catalog' docs README.md AGENTS.md CONTRIBUTING.md website .github scripts bench cmd internal --glob '!docs/routing-profile-catalog.md'` -> no matches; `rg -n -e 'docs/routing/index\.md' -e 'routing/index\.md' ...` -> `README.md:182` and this stub; not in `website/content/`. | `docs/routing/index.md` | CL-006 may delete this file; no link updates needed. |
| `CL-003.02` | `docs/routing-profile-override-precedence.md` | docs-redirect | delete | delete | Superseded redirect stub; no active inbound link remains. | `rg -n -e 'routing-profile-override-precedence\.md' -e 'docs/routing-profile-override-precedence' -e 'routing-profile-override-precedence' docs README.md AGENTS.md CONTRIBUTING.md website .github scripts bench cmd internal --glob '!docs/routing-profile-override-precedence.md'` -> no matches; `rg -n -e 'docs/routing/override-precedence\.md' -e 'routing/override-precedence\.md' -e 'override-precedence\.md' ...` -> `docs/routing/index.md:45` and this stub; not in `website/content/`. | `docs/routing/override-precedence.md` | CL-006 may delete this file; no link updates needed. |
| `CL-003.03` | `docs/routing/index.md`, `docs/routing/best-provider.md`, `docs/routing/override-precedence.md` | routing-reference | active | keep | Current routing reference set. | `README.md:182` links `docs/routing/index.md`; `docs/routing/index.md:44-45` links `best-provider.md` and `override-precedence.md`; old-name examples are current until the rename lands. | None. | Rename bead should update product and CLI names in place. |
| `CL-003.04` | `docs/routing/profiles.md` | routing-reference | active | keep | Legacy-name page is intentionally retained and tested. | `docs/routing/index.md:46` links it as "Legacy name surface"; `internal/modelcatalog/profile_docs_test.go:26-43` reads and asserts the file contents. | None. | Keep through cleanup; rename/follow-up routing work may revisit the compatibility page later. |
| `CL-003.05` | `README.md` | root-doc | active | keep | Primary public entry point, even though it contains old product and CLI names. | `README.md:1` is the project title; `README.md:181-183` links website and routing docs; `docs/helix/02-design/plan-2026-04-08-rename-agent.md:83,179` explicitly lists README as a rename target. | None. | FZ rename updates in place. |
| `CL-003.06` | `CONTRIBUTING.md` | root-doc | active | keep | Contributor setup and project structure doc. | `README.md:187` links `CONTRIBUTING.md`; `docs/helix/02-design/plan-2026-04-08-rename-agent.md:84,179` lists it as a rename target. | None. | FZ rename updates in place. |
| `CL-003.07` | `website/content/_index.md`, `website/content/docs/getting-started.md`, `website/content/demos/_index.md` | website-content | active | keep | Built Hugo site content; old names are live website surface. | `.github/workflows/website.yml:57` runs `hugo --gc --minify`; `website/hugo.yaml` defines the Hextra site; `website/content/docs/_index.md:7` links `getting-started`; README links the built site at `README.md:181,183`. | None. | FZ rename updates in place and should run the website build. |
| `CL-003.08` | `bench/README.md` | benchmark-doc | active | keep | Current corpus/result layout for the `cmd/bench` runner. | `cmd/bench/runner.go:173-179` defaults to `bench/corpus` and `bench/results`; `cmd/bench/bench_test.go:11-19` walks to `bench/corpus`; doc old name matches current `ddx-agent-bench` binary surface. | None. | FZ rename updates binary examples after productinfo changes. |
| `CL-003.09` | `scripts/benchmark/README.md` | benchmark-doc | active | keep | Current Terminal-Bench/Harbor operator guide. | `scripts/benchmark/run_benchmark.sh:23` points readers to this README; `docs/helix/02-design/epic-validation-e8c1f21c.md:56` lists it as benchmark documentation. | None. | FZ rename updates examples after CLI/binary decisions land. |
| `CL-003.10` | `scripts/beadbench/README.md` | benchmark-doc | active | keep | Current Beadbench operator guide and scoring caveat source. | `scripts/beadbench/run_beadbench.py:1258` cites README scoring caveats; `scripts/beadbench/test_run_beadbench.py:4` directs maintainers to run the script; multiple research notes cite `scripts/beadbench/run_beadbench.py`. | None. | FZ rename updates command names after bench binary rename. |
| `CL-003.11` | `scripts/benchmark/cost-guards/README.md` | benchmark-doc | active | keep | Current matrix cost-guard procedure. | `cmd/bench/matrix_aggregate.go:167` records that observation-derived values come from `scripts/benchmark/cost-guards`; the README documents live budget procedure and no-API verification. | None. | FZ rename updates `ddx-agent-bench` examples after bench binary rename. |
| `CL-003.12` | `scripts/benchmark/tool-evolution-matrix.md` | benchmark-history | archive | archive | Historical checkpoint/run-order note with no active inbound references; active `scripts/benchmark/` placement can imply current instructions. | `rg -n -e 'tool-evolution-matrix\.md' -e 'Tool Evolution Benchmark Matrix' -e '3dd0d4d01fb23c67b194f1f315ce8c4faaa2df75' -e '401db8659f4c18fbfacee55c53bbc7c2c143b40a' . --glob '!scripts/benchmark/tool-evolution-matrix.md'` -> no matches; not under `website/content/`. | No replacement; history should be preserved. | CL-006 may `git mv` to an archive location such as `docs/research/archive/tool-evolution-matrix.md`. |
| `CL-003.13` | `docs/helix/**` | helix-artifacts | historical | keep | HELIX plans, ADRs, solution designs, and alignment reviews are historical/governing evidence. | 53 markdown files under `docs/helix`; 51 contain old-name surface; CL-001 explicitly marks `docs/helix/**` retrospectives, ADRs, and design notes as non-delete historical artifacts. | None. | Do not delete/archive for CL-000; FZ rename should decide which active governing docs get in-place wording updates. |
| `CL-003.14` | `docs/research/rename-fizeau-2026-04-27.md` | rename-research | active | keep | Current pending Fizeau naming research for this cleanup epic. | Parent bead `agent-996dca04` states cleanup is before the Fizeau rename; this file documents the pending project/repo/package `fizeau` and CLI `fiz` candidate. | None. | Keep as governing rename context until FZ adoption decision is superseded. |
| `CL-003.15` | `docs/research/cleanup-criteria-2026-04-30.md` | cleanup-policy | active | keep | Governing cleanup criteria for CL-002 through CL-007. | Frontmatter names bead `agent-235efb9a`; text says it defines criteria for CL-002/CL-003/CL-004 inventory beads and CL-005/CL-006/CL-007 deletion beads. | None. | Keep through CL-000; later archive only after cleanup epic closes. |
| `CL-003.16` | Current benchmark research under `docs/research/` (`harness-matrix-plan-2026-04-29.md`, `model-census-2026-04-29.md`, `oss-harness-install-2026-04-29.md`, `phase-a1-live-matrix-preflight-2026-04-30.md`, `matrix-baseline-phase-a1-2026-04-30.md`) | research-docs | active | keep | Current matrix/benchmark work still references these notes. | `docs/helix/02-design/solution-designs/SD-010-harness-matrix-benchmark.md:9,47,62,529` cites the matrix plan and model census; `scripts/beadbench/external/termbench-subset-canary.json:7` cites the plan; `scripts/benchmark/profiles/gpt-5-3-mini.yaml:2` cites the model census. | None. | FZ rename updates command/product examples only where the docs remain active. |
| `CL-003.17` | Older experiment/research notes under `docs/research/` (`lucebox-tool-support-2026-04-27.md`, `qwen3.6-27b-cross-provider-2026-04-27.md`, `external-benchmark-baseline-2026-04-27.md`, `beadbench-omlx-qwen-reasoning-sweep-2026-04-24.md`, parity/alignment notes) | research-docs | historical | keep | Historical experiment evidence; old names document what was tested at the time. | `docs/research/qwen3.6-27b-cross-provider-2026-04-27.md:129,168,174` cites `lucebox-tool-support-2026-04-27.md`; `CHANGELOG.md:61,137,179` cites historical research notes; CL-001 defaults historical evidence to keep when uncertain. | None. | Do not delete for CL-000; archive only with a future research-doc retention policy. |

## Summary for CL-006

Rows `CL-003.01` and `CL-003.02` are deletion candidates. Row `CL-003.12` is
an archive candidate. All other rows are keep classifications: active docs must
be renamed in place by FZ work, while historical HELIX/research artifacts should
not be deleted just because they contain old names.
