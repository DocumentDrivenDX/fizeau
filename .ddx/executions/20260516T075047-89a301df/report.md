# PR 5 — Docs and Shell Cleanup

Bead: fizeau-67e67103
Spec: ADR-016 (Cells Are Self-Describing Evidence) /
docs/helix/02-design/plan-2026-05-15-benchmark-runner-simplification.md §PR 5

## Changes

### Operator-facing scripts and docs

- `scripts/benchmark/README.md` — rewritten around the `./benchmark`
  profile + bench-set driver. First code block is now
  `./benchmark --profile P --bench-set B`. Authoring guidance, prereqs,
  output layout, and historical notes section all reference ADR-016 and
  the new authoring model. The `fiz-bench lanes clone` workflow and the
  `fiz-bench matrix-index` consolidation block were dropped from
  current usage; the README's only remaining mention of the retired
  vocab is in the "Historical Notes" section.
- `scripts/benchmark/run_terminalbench_2_1_sweep.sh` — replaced the
  ~1300-line sweep runner with a thin shell wrapper that execs
  `scripts/benchmark/benchmark`. `bash -n` passes.
- `scripts/benchmark/run_medium_model_terminalbench_comparison.sh` —
  same treatment.
- `scripts/benchmark/cost-guards/README.md` — added a 2026-05-16
  historical-note banner; the original `fiz-bench matrix` /
  `fiz-bench matrix-aggregate` instructions are kept as the historical
  reference for the original observation run.

### Public benchmark docs

- `docs/benchmarks/sections/02-terminal-bench.md`,
  `03-profiles-intro.md`, `05-detailed-metrics-intro.md` — reworded
  lane / bench-set vocabulary in terms of profile and bench-set.
- `docs/benchmarks/sections/01-fizeau.md`, `04-pass-rate-narrative.md`,
  `06-model-power-observations.md`,
  `07-context-length-observations.md`, `08-conclusions.md`,
  `harnesses-side-by-side.md`, `providers-details.md`,
  `models-coverage-cost.md` — reworded analytical "lane" → "profile" in
  narrative prose so the page-level vocabulary matches ADR-016.
- `docs/benchmarks/runtime-props.md` — reworded narrative to
  "profile"; left the actual Go type `LaneInfo` reference intact with
  an inline note that renaming is out of scope.

### HELIX governing docs

- `docs/helix/01-frame/prd.md` — updated metrics row and acceptance
  criteria phrasing from "lane" to "profile".
- `docs/helix/01-frame/features/FEAT-008-benchmark-workbench.md` —
  added an explicit ADR-016 reference for the embedded-profile cell
  schema; updated identity column list and filter-by-enum list to use
  profile vocabulary.
- `docs/helix/02-design/solution-designs/SD-009-benchmark-mode.md` —
  added a 2026-05-16 ADR-016 banner at the top.
- `docs/helix/02-design/solution-designs/SD-014-benchmark-site-information-architecture.md`
  — added a 2026-05-16 ADR-016 banner. The bead noted that SD-010,
  SD-012, SD-015 already carry 2026-05-16 banners; SD-010 actually did
  not, so a banner was added there too. SD-012 and SD-015 already had
  matching 2026-05-16 banners and were left untouched.
- `docs/helix/02-design/benchmark-baseline-tb2-qwen-2026-05-01.md` —
  renamed the "Rerun recipe" header to "Rerun command", replaced the
  example invocation with the current `./benchmark` driver, and
  pointed at ADR-016 for the authoring model.

## Acceptance evidence

- **AC1** `rg -i 'lane|recipe|comparison.group|resource.group|lane.alias|profile.inventory' docs/benchmarks/ scripts/benchmark/README.md`:
  remaining hits are all intentional. In Markdown narrative under
  `docs/benchmarks/`, only `runtime-props.md:73` matches and the line
  is an annotated reference to the Go type `LaneInfo`. In
  `scripts/benchmark/README.md`, every match is inside the
  "Historical Notes" or the explicit "no recipe to enrol /
  no resource-group entry" callouts that describe what was removed.
  The remaining `docs/benchmarks/data/*.json`,
  `docs/benchmarks/schema/*.json`, and
  `docs/benchmarks/terminal-bench-2.1-report.html` hits are generated
  artifacts (build-pipeline output, schema field names already
  consumed by the workbench) and out of scope per the bead's "Deleting
  source code (PR 4)" exclusion.
- **AC2** `bash -n scripts/benchmark/run_terminalbench_2_1_sweep.sh`
  exits 0. (The script is now a one-line exec around `./benchmark`.)
- **AC3** The README's first code block opens with
  `./benchmark --profile P --bench-set B`. Verified by reading the
  rendered first fence.
- **AC4** `rg "fiz-bench (sweep|matrix|lanes)" docs/ scripts/benchmark/`:
  every remaining hit lives in historical-note prose — ADR-016 itself
  explaining retirement, the migration plan, the now-banner-stamped
  SD-009 / SD-010, the historical-note banner in
  `scripts/benchmark/cost-guards/README.md`, the historical-notes
  section of `scripts/benchmark/README.md`, the
  `scripts/benchmark/benchmark` script header explaining what it
  replaces, and `docs/research/archive/`.

## Out of scope (per bead)

- Deleting Go scaffolding (`cmd/bench/lanes*.go`,
  `validateSweepLaneEnvMatch`, `runtimeprops.LaneInfo`, etc.) — owned
  by PR 4.
- Editing `scripts/benchmark/profiles/*.yaml` `_header_comment`
  strings (those propagate into `docs/benchmarks/data/profiles.json`
  via the build pipeline) — source-YAML edits belong to a separate
  bead.
- Rewriting the generated `docs/benchmarks/terminal-bench-2.1-report.html`
  artifact and the JSON schema fields (`internal_lane_id`,
  `lane_label`) — schema/build-pipeline work, not docs cleanup.
