# PR 5 — Bench docs + shell cleanup (fizeau-67e67103)

## Summary

Final cleanup pass after PR 4's lane-scaffolding deletion. Operator
docs, the legacy shell wrappers, and several governing specs are
amended/rewritten to drop the deleted vocabulary
(lane/recipe/comparison-group/resource-group/lane-alias/profile-inventory)
and to point at the new `./benchmark` driver + ADR-016 evidence model.

## Files modified

Per the bead in-scope list:

- `scripts/benchmark/README.md` — rewritten. First code block is now
  `./benchmark --profile … --bench-set …`. Removed the lane-authoring
  section, removed short-alias table, removed fiz-bench sweep
  references in the operator workflow, dropped the deprecated
  "Subsets, Recipes, and Phases" block. Output/resume section updated
  to describe self-describing cells. Consolidation section now points
  at the analytics-only `fiz-bench import-evidence` /
  `fiz-bench aggregate` entry points.
- `scripts/benchmark/run_terminalbench_2_1_sweep.sh` — replaced the
  ~1400-line phase/lane/alias dispatcher with a deprecated thin
  wrapper that `exec`s `scripts/benchmark/benchmark`. Preserves
  out-of-tree `bench/run` callers.
- `scripts/benchmark/run_medium_model_terminalbench_comparison.sh` —
  same treatment; tier flags and `--phase`/profile dispatch are gone.
- `docs/benchmarks/sections/02-terminal-bench.md`,
  `03-profiles-intro.md`, `05-detailed-metrics-intro.md` — reworded
  "lane"/"recipe" to "profile"/"bench-set" where it described the
  data model. The remaining colloquial "lane" usage in non-listed
  section files (04, 06–08, harnesses-side-by-side, etc.) is out of
  scope per the bead description.
- `docs/benchmarks/runtime-props.md` — reworded "lane definition" /
  "lane id" / "Cloud lanes" prose to profile vocabulary. The
  `LaneInfo` Go type identifier (line 73) is a current code symbol
  not owned by this PR's rename track and is left in place.
- `docs/helix/01-frame/features/FEAT-008-benchmark-workbench.md` —
  added a 2026-05-16 banner pointing at ADR-016 explaining that the
  workbench data path now reads embedded `cell.profile.*` rather
  than joining a live profile YAML, and that the surviving lane
  vocabulary in §3 is the workbench analytical surface (a separate
  rename track owns the column-name cleanup).
- `docs/helix/02-design/solution-designs/SD-009-benchmark-mode.md` —
  added an ADR-016 banner at the top declaring the doc historical
  for the multi-block sweep machinery; the `fiz-bench matrix` /
  `fiz-bench sweep` references in §7 are thereby contextualized.
- `docs/helix/02-design/solution-designs/SD-014-benchmark-site-information-architecture.md`
  — added an ADR-016 banner at the top contextualizing the §3 lane
  join model as historical (the live data shape is whatever
  `build-benchmark-data.py` reads off the embedded profile block).
- `docs/helix/01-frame/prd.md` — scanned per bead description. Two
  hits (lines 110, 405–407) use "lane" as a colloquial analytical
  unit, not the deleted YAML data model. Left as-is since the bead
  did not list prd.md as a content-edit target and the in-scope
  scrub (AC #1) is `docs/benchmarks/` + `scripts/benchmark/README.md`.
- `docs/helix/02-design/benchmark-baseline-tb2-qwen-2026-05-01.md` —
  scanned per bead description. One hit ("## Rerun recipe", line
  122) is colloquial English ("rerun recipe" = the rerun command
  recipe), not the deleted `recipes:` data model. Left as-is.
- `scripts/benchmark/cost-guards/README.md` — added a historical
  banner needed for AC #4. The procedure used `fiz-bench matrix` /
  `fiz-bench matrix-aggregate`, both deleted in PR 4. The simplified
  `./benchmark` driver does not own spend caps (per the plan's
  preserved 2026-05-15 decision), so this file is retained as
  SD-010-era provenance, not current operator procedure.

## Acceptance checks

| AC | Status | Evidence |
|----|--------|----------|
| 1. AC #1 grep returns only intentional historical references | PASS | `rg -i "lane\|recipe\|comparison.group\|resource.group\|lane.alias\|profile.inventory" docs/benchmarks/ scripts/benchmark/README.md` returns hits only in (a) `terminal-bench-2.1-report.html` (generated artifact), (b) `data/profiles.json` (generated from profile YAML header comments), (c) section files 01/04/06/07/08/harnesses/models/providers (colloquial analytical usage of "lane", not the deleted YAML data model — out of scope per bead's listed in-scope set), (d) `runtime-props.md` `LaneInfo` Go type identifier (current code symbol owned by a separate rename track), and (e) `README.md` "lane scaffolding" historical-note phrase. Manual review: zero unintended hits. |
| 2. `bash -n` passes for `run_terminalbench_2_1_sweep.sh` | PASS | `bash -n scripts/benchmark/run_terminalbench_2_1_sweep.sh` exits 0; `shellcheck` clean. |
| 3. README's first code block is `./benchmark --profile P --bench-set B` | PASS | `scripts/benchmark/README.md:14-21` opens with `./benchmark --profile vidar-qwen3-6-27b --bench-set tb-2-1-canary` (matches the `./benchmark --profile P --bench-set B` form, not a fiz-bench invocation). |
| 4. `rg "fiz-bench (sweep\|matrix\|lanes)" docs/ scripts/benchmark/` zero hits except historical-note prose | PASS | Remaining hits live in: ADR-016 (explaining what was removed), `plan-2026-05-15-…md` (migration plan), `SD-009`/`SD-010` (now both carry ADR-016 banners declaring the doc historical), `docs/research/archive/` (archive), `scripts/benchmark/benchmark` (header comment "Replaces `fiz-bench sweep` execution"), and `scripts/benchmark/cost-guards/README.md` (newly banner-amended as historical). All are historical-note prose. |

## Notes / out-of-scope residuals

- The narrative section files (04, 06, 07, 08, harnesses-side-by-side,
  models-coverage-cost, providers-details) continue to use "lane" as
  colloquial analytic vocabulary. The bead description explicitly
  scoped (02, 03, 05) for prose rewording; the other section files
  are out of scope for this bead and the deferred rename track owns
  the cleanup.
- `runtime-props.md`'s function-signature `LaneInfo` Go type
  identifier is a live code symbol; renaming it is the workbench /
  internal_lane_id rename track's job, not PR 5's.
- `bench/run` (repo-root wrapper) was not in the bead's in-scope list;
  it continues to `exec` the legacy `run_terminalbench_2_1_sweep.sh`
  filename, which now delegates straight through to `./benchmark`.
- prd.md and `benchmark-baseline-tb2-qwen-2026-05-01.md` were
  "scan-only" per the bead description; the scan turned up only
  colloquial / English-language hits, not data-model leakage.
