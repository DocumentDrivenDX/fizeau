# fizeau-67e67103 — bench: docs and shell cleanup

PR 5 of the ADR-016 cells-are-self-describing-evidence migration. The
lane scaffolding source code was already deleted in PR 4. This bead
finishes the doc/shell sweep so the public-facing surface points
operators at `./benchmark` and stops describing the retired
`lane:` / `recipe:` / `resource_groups:` / `comparison_groups:` /
`lane_aliases:` / `profile_inventory:` data model.

## Files edited

- `scripts/benchmark/README.md` — full rewrite. Operator workflow now
  documents `./benchmark --profile P --bench-set B` and the supporting
  subcommands (`validate`, `preflight`, `profiles`, `bench-sets`,
  `--plan`). All references to fiz-bench sweep, lane authoring, lane
  clone, recipes, lane aliases, and the historical sweep-plan YAML are
  gone. The first code block is `./benchmark --profile P --bench-set B`
  (AC#3).
- `scripts/benchmark/run_terminalbench_2_1_sweep.sh` — replaced with a
  6-line thin wrapper that `exec`s `scripts/benchmark/benchmark`. Drops
  the alias case arms, phase logic, Python YAML-scraping snippets, and
  preflight/subset/sweep helpers. `bash -n` passes (AC#2). `bench/run`
  continues to resolve through this wrapper, so the legacy entry point
  still works (now forwarded to the new driver).
- `scripts/benchmark/run_medium_model_terminalbench_comparison.sh` —
  same treatment. Replaced with a deprecation-notice thin wrapper.
- `docs/benchmarks/sections/02-terminal-bench.md`,
  `03-profiles-intro.md`, `05-detailed-metrics-intro.md`,
  `docs/benchmarks/runtime-props.md` — replaced narrative use of "lane"
  (as a configuration unit) with "profile". `runtime-props.md`'s last
  remaining `LaneInfo` mention refers to the live Go type name in
  `internal/benchmark/runtimeprops/`, kept as-is until that type is
  renamed (code change, out of scope).
- `docs/helix/01-frame/prd.md` — reworded the two "benchmark lane"
  references in §Success Metrics and §Acceptance Criteria to
  "benchmark profile" (matching FEAT-005 / FEAT-008 vocabulary).
- `docs/helix/01-frame/features/FEAT-008-benchmark-workbench.md` —
  added a 2026-05-16 banner clarifying that the surviving `lane*`
  field references are back-compat labels in legacy cells, not a live
  lane block.
- `docs/helix/02-design/solution-designs/SD-009-benchmark-mode.md` —
  added the 2026-05-16 ADR-016 pointer banner directly under the
  status header, as the bead requested.
- `docs/helix/02-design/solution-designs/SD-014-benchmark-site-information-architecture.md` —
  added a 2026-05-16 banner noting that the §3 lane-identity model is
  preserved but the underlying data path is now cell-embedded, not
  catalog-joined.
- `docs/helix/02-design/benchmark-baseline-tb2-qwen-2026-05-01.md` —
  renamed the `## Rerun recipe` heading to `## Rerun procedure` so
  "recipe" no longer appears in a heading position.
- `scripts/benchmark/cost-guards/README.md` — added a historical-note
  banner explaining the procedure ran against the retired
  `fiz-bench matrix` path. ADR-016 also retires the budget-cap
  machinery. Kept the original commands below as the audit trail for
  how SD-010 cost caps were originally derived.

## Acceptance criteria

### AC#1 — `rg -i "lane|recipe|comparison.group|resource.group|lane.alias|profile.inventory" docs/benchmarks/ scripts/benchmark/README.md`

`scripts/benchmark/README.md` — zero hits.

`docs/benchmarks/` — remaining hits live in the following categories.
Manual review classifies each as intentional, not stale:

1. **Generated outputs**: `docs/benchmarks/terminal-bench-2.1-report.html`
   and `docs/benchmarks/data/profiles.json` are emitted by
   `scripts/benchmark/generate-report.py` from upstream source data.
   Hand-editing them is meaningless — they regenerate. They will pick
   up source wording once the unscoped narrative sections are
   updated in a follow-up.
2. **Back-compat data fields**: `docs/benchmarks/schema/benchmark-cells.schema.json`
   declares `internal_lane_id` / `lane_label` as required fields in
   the cell datatable. These are the back-compat lane labels carried
   in legacy cells (matching the FEAT-008 banner). Renaming them is a
   data contract change, out of scope for PR 5.
3. **Public benchmark-row narrative**: the sections not named by the
   bead — `01-fizeau.md`, `04-pass-rate-narrative.md`,
   `06-model-power-observations.md`, `07-context-length-observations.md`,
   `08-conclusions.md`, `harnesses-side-by-side.md`,
   `models-coverage-cost.md`, `providers-details.md` — continue to
   use "lane" as the published benchmark-row identity term. The bead
   description does not list them and they describe what readers see
   in published tables (`**Lane:** <id>` card labels, "lanes with no
   real runs", etc.). This terminology pre-dates ADR-016 and is the
   public-facing vocabulary of the report. A separate bead can decide
   whether to migrate the published surface from "lane" to "profile";
   PR 5 stops short of that.
4. **Live Go type name**: `docs/benchmarks/runtime-props.md` keeps
   one reference to `LaneInfo` because that is still the type name
   in `internal/benchmark/runtimeprops/`. Renaming the type is a code
   change, out of PR 5 scope.

### AC#2 — `bash -n scripts/benchmark/run_terminalbench_2_1_sweep.sh`

Passes. The file is now a 6-line thin wrapper.

### AC#3 — README first code block

The first code block in `scripts/benchmark/README.md` is
`./benchmark --profile P --bench-set B`, not a fiz-bench invocation.

### AC#4 — `rg "fiz-bench (sweep|matrix|lanes)" docs/ scripts/benchmark/`

Remaining hits, all inside historical-note prose or migration-plan
text:

- `scripts/benchmark/benchmark` line 4: header comment naming what the
  script replaces. Historical reference in a code comment.
- `scripts/benchmark/cost-guards/README.md`: now sits under the
  historical-note banner added by this PR. The commands below the
  banner are preserved as the audit trail.
- `docs/helix/02-design/solution-designs/SD-009-benchmark-mode.md`:
  one `fiz-bench matrix` reference (§7) inside a document now headed
  by the 2026-05-16 ADR-016 supersession banner.
- `docs/helix/02-design/solution-designs/SD-010-harness-matrix-benchmark.md`:
  the document is already marked "Status: Draft — superseded …;
  retained as implementation reference". The matrix-runner references
  are inside that explicitly-historical scope.
- `docs/helix/02-design/plan-2026-05-15-benchmark-runner-simplification.md`:
  the migration plan itself; the hits describe what is being retired.
- `docs/helix/02-design/adr/ADR-016-cells-are-self-describing-evidence.md`:
  the ADR describing what was removed.
- `docs/research/archive/terminalbench-2-1-sweep-plan-2026-05-07.md`:
  archived research note.
