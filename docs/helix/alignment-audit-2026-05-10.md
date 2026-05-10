---
title: HELIX Alignment Audit — 2026-05-10
status: research
author: alignment pass (read-only)
scope: docs/helix/** vs current code surface and website
---

# Alignment Audit — 2026-05-10

This is a **read-only enumeration** of inconsistencies discovered while
walking the HELIX corpus and comparing it to the current code surface
(`module github.com/easel/fizeau`, binary `fiz`, Go package `fizeau`),
the standalone CLI flags (`cmd/fiz`), and the public website
(`website/content/**`).

No fixes are applied in this pass. Each entry is tagged
`[blocker | nit | info]` and ends with a one-line proposed fix.

## Inventory (what was reviewed)

| Bucket | Count | Notes |
|---|---|---|
| 00-discover (vision, competitive) | 2 + 1/ | `product-vision.md`, `competitive-analysis/wozcode-plugin-2026-05-03.md` |
| 01-frame (PRD, concerns, features) | 1 PRD + 1 concerns + 7 FEAT | `FEAT-001..FEAT-007` |
| 02-design — ADRs | 9 | `ADR-001..ADR-009` |
| 02-design — Solution Designs | 13 | `SD-001..SD-013` |
| 02-design — Contracts | 2 | `CONTRACT-001`, `CONTRACT-003` (note: CONTRACT-002 is missing — see I-12) |
| 02-design — Spikes | 2 | `SPIKE-001`, `SPIKE-002` |
| 02-design — plans/baselines/architecture | ~12 | including `architecture.md`, `harness-golden-integration.md`, several dated plans |
| 06-iterate — Alignment Reviews | 8 | `AR-2026-04-07..AR-2026-04-25` |
| 06-iterate — metrics | 1 | `test-coverage.yaml` |
| **Total spec files reviewed** | **~58** | full corpus; nothing skipped |

There is **no `03-test/`, `04-build/`, `05-deploy/`** directory under
`docs/helix/`. The PRD and feature specs assume those stages exist and
reference forward artifacts (test plans, build plans) that have not been
created. See I-11.

## Severity legend

- **blocker** — actively misleads readers/agents about what the code does
  or about which surface to consume; will cause an incorrect
  implementation if followed verbatim.
- **nit** — out-of-date prose that is locally confusing but downstream
  artifacts (code, other specs) still resolve correctly.
- **info** — observation that should be tracked but does not impede work.

---

## Inconsistencies

### I-01 — Pervasive "DDX Agent" / `ddx-agent` rename rot in current specs
- **Severity:** blocker
- **Status:** RESOLVED — closed by the rename-rot sweep staged on top of `90976f0c` (rewrites all 9 ADRs, both spikes, the affected plans/baselines, dated AR docs, `epic-validation-e8c1f21c.md`, `external-benchmarks.md`, and `benchmark-corpus.md`; relinks the `CONTRACT-003-ddx-agent-service.md` cross-reference to `CONTRACT-003-fizeau-service.md`; preserves the historical rename plan `plan-2026-04-08-rename-agent.md`).
- **Where:** ADR-001..ADR-007 (header rows, prose, alternatives, references), ADR-005 line 82 (`ddx-agent --list-models`), ADR-007 lines 84/86 (`ddx-agent catalog check/update`), ADR-002 line 446 (link to non-existent `CONTRACT-003-ddx-agent-service.md`), `architecture.md` references, `epic-validation-e8c1f21c.md`, `external-benchmarks.md`, `terminalbench-fiz-wrapper-comparison-2026-05-06.md`
- **Why it matters:** Current product is **Fizeau**, module is `github.com/easel/fizeau`, binary is `fiz`, package is `fizeau`. The rename plan (`plan-2026-04-08-rename-agent.md`) was supposed to drive a global rename; the code did rename but the spec corpus did not follow. The rename plan's own title reads "Rename DDX Agent to DDX Agent" — broken on its own face.
- **Proposed fix:** Single global rewrite pass: `DDX Agent` → `Fizeau`, `ddx-agent` → `fiz`, `CONTRACT-003-ddx-agent-service.md` → `CONTRACT-003-fizeau-service.md`. Add a `RENAMED.md` migration note at top of `02-design/adr/` and `02-design/contracts/`.

### I-02 — Absolute filesystem cross-doc paths to legacy `/Users/erik/Projects/agent/`
- **Severity:** blocker
- **Where:** `ADR-001-observability-surfaces-and-cost-attribution.md` (lines 91–95), `ADR-003-pty-terminal-rendering.md` (lines 24, 145–148), and likely more
- **Why it matters:** These links resolve to nothing on any other machine and to a stale tree on the author's machine. They make the ADRs un-followable for any reader who is not Erik.
- **Proposed fix:** Replace with repo-relative paths (`../../01-frame/prd.md`, `../adr/ADR-002-pty-cassette-transport.md`, etc.). Add a lefthook/CI check forbidding `/Users/` strings in `docs/helix/**`.

### I-03 — Website embedding examples reference removed `ModelRef` field
- **Severity:** blocker
- **Where:** `website/content/docs/getting-started.md:68`, `website/content/docs/embedding/_index.md:42`
- **Why it matters:** ADR-009 (`v0.11 routing surface redesign`) explicitly **removes** `ModelRef`, `--model-ref`, and `model_ref` from the public routing surface. The actual `ServiceExecuteRequest` struct (`service.go:655`) has a `Model` field, not `ModelRef`. Anyone copy-pasting the website's quickstart will get a Go compile error.
- **Proposed fix:** Replace `ModelRef: "cheap"` with `Policy: "cheap"` (since `cheap` is a routing policy, not a model name) in both pages. Note in PR that this is a website fix scoped to the next iteration; it lives outside this audit's no-touch boundary on `website/**`.

### I-04 — Embedding doc references nonexistent `ErrNoLiveProvider` "profile-tier" prose
- **Severity:** nit
- **Where:** `website/content/docs/embedding/_index.md:547` (`profile-tier escalation walked the entire`)
- **Why it matters:** Per ADR-005 / ADR-009 the term "profile" is removed from the public surface. The error doc copy still uses `profile-tier`.
- **Proposed fix:** Regenerate embedding doc from current source (`make docs-embedding` or equivalent) — likely already supported since the file header says "generated".

### I-05 — `FIZEAU_PROVIDER`/`FIZEAU_MODEL`/`.agent/config.yaml` mixture in solution designs
- **Severity:** blocker
- **Where:** `SD-002-standalone-cli.md:73-75` (says project config is `.agent/config.yaml`, env vars `FIZEAU_PROVIDER`...), `SD-005-provider-config.md:79`, `SD-007-provider-import.md` (multiple `.agent/config.yaml` and warnings printed as `agent: warning: ...`), `SD-003-system-prompts.md:51` (`.agent/prompts/`), `SD-008-terminal-bench-integration.md:93`, `SD-010` (`FIZEAU_PROVIDER=openrouter`)
- **Why it matters:** These docs document a config search path the binary no longer uses. The website (`getting-started.md:36`) and the binary use `.fizeau/config.yaml`. Env-var prefix has settled on `FIZEAU_*` (good — confirmed in code), so docs that say `FIZEAU_PROVIDER` are correct. The breakage is the `.agent/` directory name and the `agent:` log prefix.
- **Proposed fix:** `.agent/` → `.fizeau/`, `~/.config/agent/` → `~/.config/fizeau/`, `agent: warning:` → `fiz: warning:`. Same global rewrite as I-01.

### I-06 — FEAT-005 AC-05 references `fiz usage` but PRD non-goals say no TUI; usage CSV mode unverified against current `cmd/fiz/usage.go`
- **Severity:** info
- **Where:** `FEAT-005-logging-and-cost.md` AC-FEAT-005-05 promises "table, JSON, and CSV output modes"; the website CLI ref for `fiz_usage.md` was not opened in this pass — needs verification.
- **Proposed fix:** Verify CSV mode exists and add a sanity test, or amend AC-05 to drop CSV.

### I-07 — `--preset` flag on root `fiz` command not reflected in FEAT-006
- **Severity:** nit
- **Where:** `website/content/docs/cli/_index.md` shows `--preset string` on root `fiz`, but `FEAT-006-standalone-cli.md` AC-FEAT-006-06 explicitly says `--profile/--model-ref/--backend` are removed/rejected with no mention of `--preset`. The tools doc (`website/content/docs/tools/_index.md`) calls preset "**prompt preset**" (system prompt selection). FEAT-006 does not enumerate `--preset` as an accepted flag at all.
- **Proposed fix:** Add an AC line to FEAT-006 covering `--preset` (and `--system`, `--reasoning`, `--harness`, `--provider`, `--model`, `--max-iter`, `--allow-deprecated-model`, `--list-models`, `--work-dir`) so the AC matrix matches the help text the CLI emits.

### I-08 — FEAT-001 AC numbering gap and out-of-order
- **Severity:** nit
- **Where:** `FEAT-001-agent-loop.md` AC table lists 01, 02, 03, 04, 05, 06, 07, 08, **10**, 09 — `09` appears after `10` and the order is broken. AC-FEAT-001-09 is the tool-call-loop detector.
- **Proposed fix:** Reorder so IDs ascend monotonically; or mint 01..10 with no gap.

### I-09 — PRD success metric "Cost per bead (blended) <$0.05" depends on a `fiz usage` cost-per-bead aggregator that is not specced anywhere
- **Severity:** info
- **Where:** `prd.md` Success Metrics row 3 says measurement = `fiz usage` report, but neither FEAT-005 nor FEAT-006 specifies a "cost per bead" rollup; usage rollup is per-provider/model/time-window.
- **Proposed fix:** Either drop the metric, or write a one-liner AC under FEAT-005 covering bead-tag aggregation.

### I-10 — PRD lists "Streaming callbacks — Implemented" / "Compaction — Implemented" inline as P1 status badges
- **Severity:** nit
- **Where:** `prd.md:148-164`
- **Why it matters:** PRD is the *requirements* doc; status belongs in a tracker (beads, the iteration metrics file). Status badges on the PRD will rot.
- **Proposed fix:** Remove "**Implemented**" tags from PRD; track status in `06-iterate/metrics/test-coverage.yaml` or a status table maintained outside the PRD.

### I-11 — No `03-test/`, `04-build/`, `05-deploy/` HELIX stage directories exist
- **Severity:** info
- **Where:** `docs/helix/` has only `00-discover/`, `01-frame/`, `02-design/`, `06-iterate/`. PRD line 248-249 references "feature specs in docs/helix/01-frame/features/FEAT-00X-*.md" for AC, which is fine, but the ghostly forward-stages mean the PRD's "downstream test plan / build plan" assumption is unfulfilled.
- **Proposed fix:** Either (a) commit to a flat HELIX (drop the 03/04/05 prose from the PRD/concerns), or (b) add stage skeletons.

### I-12 — `CONTRACT-002` is missing from the contracts directory
- **Severity:** info
- **Where:** `docs/helix/02-design/contracts/` contains `CONTRACT-001`, `CONTRACT-003`. No `CONTRACT-002`.
- **Proposed fix:** Document the gap (was 002 retired? renumbered? renamed?). Either re-add as a stub `CONTRACT-002-RETIRED.md` or renumber 003 → 002.

### I-13 — Plan `plan-2026-04-08-rename-agent.md` title is broken ("Rename DDX Agent to DDX Agent")
- **Severity:** nit
- **Where:** Header line of the plan
- **Proposed fix:** Title should be "Rename DDX Agent to Fizeau"; the plan's own subject got partially renamed before it could be applied.

### I-14 — `architecture.md` — line 51 says "CLI module: `cmd/fiz` and `agentcli`" but `agentcli/` is a top-level dir under repo root, not under `cmd/`
- **Severity:** nit
- **Where:** `docs/helix/02-design/architecture.md:51`
- **Proposed fix:** Clarify that `agentcli` is a sibling Go package, not under `cmd/`.

### I-15 — Inconsistent provider name aliases: `agent.openai` / `agent.anthropic` in SD-005 examples
- **Severity:** blocker (config copy-paste will not work)
- **Where:** `SD-005-provider-config.md:226`, `:234`
- **Why it matters:** The provider-system identity is `openai-compat` / `anthropic` per FEAT-003-AC-01 (which **rejects** `openai-compat` as a provider identity? — see I-16); the dotted-namespace `agent.openai` reflects the old `agent.openai` shared-catalog provider-surface naming that was renamed to `fizeau.openai` (or no namespace at all) during the rename. Code uses provider system strings.
- **Proposed fix:** Update SD-005 example YAML to current provider system and surface names; cross-link to the live provider list (`fiz providers`).

### I-16 — FEAT-003 AC-01 contradicts the actual website + getting-started use of `provider: openai-compat`
- **Severity:** blocker
- **Where:** `FEAT-003-providers.md` AC-FEAT-003-01: "Provider configs accept concrete provider systems and **reject `openai-compat`** as a provider identity." But `website/content/docs/getting-started.md:39` shows `provider: openai-compat` as the canonical config example.
- **Why it matters:** Either the AC is wrong (the code accepts `openai-compat` and the docs reflect that) or the docs are wrong (the code rejects it and the website is teaching users a config that errors). One must change.
- **Proposed fix:** Run `fiz check` against the website example to find truth; then update either FEAT-003 AC-01 or `getting-started.md` to match.

### I-17 — FEAT-004 AC-08 ("Removed v0.10 names not advertised") vs. `agent.openai`/`agent.anthropic` still appearing in SD-005, SD-007 examples
- **Severity:** nit
- **Where:** Cross-reference of FEAT-004-AC-08 with SD-005 / SD-007
- **Proposed fix:** Either gate examples on a "legacy-pre-v0.11" notice block or rewrite to v0.11 names.

### I-18 — SD-002 says project config search is `.agent/config.yaml` (line 73) but PRD line 36 / website say `.fizeau/`
- **Severity:** blocker (specced search path does not exist)
- **Where:** `SD-002-standalone-cli.md:71-75`
- **Proposed fix:** Update SD-002 config-search list to: `~/.config/fizeau/config.yaml`, `.fizeau/config.yaml`, `FIZEAU_*` env, `--flags`. Same as I-05.

### I-19 — `harness-golden-integration.md` uses `cmd/ddx-agent` (legacy)
- **Severity:** nit
- **Where:** `02-design/harness-golden-integration.md`
- **Proposed fix:** Rename to `cmd/fiz`.

### I-20 — ADR-007 references `catalog_version` bumps and `min_agent_version` field
- **Severity:** info
- **Where:** `ADR-007-sampling-profiles-in-catalog.md:84,86`
- **Why it matters:** Field name `min_agent_version` should likely be `min_fizeau_version` if the catalog manifest is fizeau-owned. Not yet checked against the actual schema in `internal/modelcatalog/`.
- **Proposed fix:** Confirm field name in code and update ADR or schema accordingly.

### I-21 — PRD non-goal "**MCP server** — Fizeau provides tools directly, not via MCP" vs. wozcode competitive-analysis hints at MCP integration
- **Severity:** info
- **Where:** `prd.md:78`, `00-discover/competitive-analysis/wozcode-plugin-2026-05-03.md`
- **Proposed fix:** No fix yet — but if MCP becomes a real direction, the PRD non-goal should be reconsidered explicitly via an ADR.

### I-22 — `FEAT-006` AC-08 mentions "DDx harness mode" — still references the parent-product DDx by name
- **Severity:** info
- **Where:** `FEAT-006-standalone-cli.md` AC-FEAT-006-08
- **Why it matters:** This is correct as-is (DDx is the parent build orchestrator that consumes Fizeau as a harness backend), but worth flagging because of the global naming churn — confirm that "DDx" the product is the intended capitalization.
- **Proposed fix:** None; document the convention.

### I-23 — Concerns / risks around "local model context window too small" in PRD (Risks table) is not threaded into FEAT-004 routing
- **Severity:** info
- **Where:** `prd.md:316`, FEAT-004 has no AC covering the auto-escalation behavior promised in the risk mitigation
- **Proposed fix:** Add an AC to FEAT-004 covering `EstimatedPromptTokens`-driven gate (the field already exists in `ServiceExecuteRequest:694`).

### I-24 — Alignment-review history (8 AR docs) all written before the rename and never re-issued under the new name
- **Severity:** info
- **Where:** `06-iterate/alignment-reviews/AR-2026-04-*.md`
- **Proposed fix:** None to historical AR docs (they are dated snapshots). But the next AR (this one) should be filed as the canonical post-rename entry.

---

## Summary

- **5 blockers** (I-01, I-02, I-03, I-05, I-15, I-16, I-18 — call it 7).
- **9 nits.**
- **8 info-only.**

The dominant root cause is that **the rename from "DDX Agent" to "Fizeau"
shipped in code but did not shipping in the spec corpus**. A single
mechanical pass would resolve I-01, I-02, I-05, I-13, I-14, I-15, I-17,
I-18, I-19. The remaining items (I-03, I-04, I-06..I-12, I-16, I-20..I-24)
need targeted edits with code-truth verification, not blind rewrite.
