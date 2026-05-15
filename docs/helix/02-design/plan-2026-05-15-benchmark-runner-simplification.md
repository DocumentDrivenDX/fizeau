# Plan: Benchmark Runner Simplification

Date: 2026-05-15

## Problem

The TerminalBench runner has too many overlapping control surfaces. Starting a
run can create empty output directories, build binaries, build Docker images,
download or mutate task trees, normalize shell aliases, run endpoint preflight,
and only then reach the actual sweep planner. That makes a failed or skipped
run look like "nothing ran" even when the system spent time preparing hidden
state.

The desired model is simpler:

- A benchmark script is the only operator entrypoint.
- A recipe is only a task set plus run defaults.
- A lane is only a model/provider/harness target plus resource constraints.
- The runner executes the Cartesian product of selected recipes and selected
  lanes.

Everything else is preparation, validation, or reporting. Those steps should be
explicit commands, not surprising side effects of starting a run.

## Current Friction

Observed sources of complexity:

- `scripts/benchmark/run_terminalbench_2_1_sweep.sh` mixes setup, aliasing,
  task download, Docker image prebuilds, endpoint preflight, plan display,
  confirmation, and execution.
- `cmd/bench sweep` already has the better conceptual model, but it still
  exposes legacy `--phase`, `--subset`, `--all-recipes`, and
  `--staged-recipes` paths.
- Recipes contain lane enrollment, so a lane can exist but still not run for a
  selected recipe.
- Lane definitions and profile YAML are separate sources of truth; missing
  profiles are only discovered late.
- Short aliases live in shell code instead of the lane catalog.
- `--dry-run` at the shell layer can still perform expensive preparation before
  printing the runnable plan.
- Output directories can be created before any real cell starts, leaving
  confusing empty structures after failed setup.

## Target Contract

The public operator contract should be:

```bash
./benchmark --lanes <lane[,lane...]> --recipes <recipe[,recipe...]>
./benchmark --lanes <lane[,lane...]> --recipes <recipe[,recipe...]> --plan
./benchmark prepare --recipes <recipe[,recipe...]> --lanes <lane[,lane...]>
./benchmark validate
```

Rules:

1. `--plan` is pure. It validates configuration and prints the exact matrix. It
   does not build, download, preflight endpoints, or create output directories.
2. `prepare` is explicit. It may build binaries, fetch TerminalBench tasks, and
   build task images. Its output is named and reusable.
3. `run` creates output directories only immediately before launching a lane.
4. Recipes and lanes are orthogonal. A recipe does not enroll lanes. Recipe
   selection picks tasks/defaults; lane selection picks execution targets.
5. If an operator omits `--lanes`, the script errors and prints available lane
   ids. No hidden "all lanes" default for paid or local-heavy runs.
6. If an operator omits `--recipes`, the script errors and prints available
   recipe ids. No legacy `phase=all` default.
7. Aliases are data, not shell code. Every alias resolves through the lane
   catalog and is visible in `./benchmark lanes`.

## Data Model

Reshape the sweep plan around three top-level collections:

```yaml
recipes:
  - id: timing-baseline
    tasks: scripts/benchmark/task-subset-tb21-timing-baseline.yaml
    reps: 3
    defaults:
      matrix_jobs_managed: 16

lanes:
  - id: vidar-ds4
    aliases: [ds4, vidar-ds4]
    profile: vidar-ds4
    resource_group: vidar-ds4
    env:
      FIZEAU_PROVIDER: ds4
      FIZEAU_MODEL: deepseek-v4-flash
      FIZEAU_BASE_URL: http://192.168.2.106:1236/v1
      FIZEAU_API_KEY_ENV: DS4_API_KEY

resource_groups:
  - id: vidar-ds4
    max_concurrency: 1
```

Remove recipe lane lists. If a named operational bundle truly needs a curated
lane set, model it as a saved invocation preset, not as the recipe itself:

```yaml
presets:
  - id: timing-ds4
    recipes: [timing-baseline]
    lanes: [vidar-ds4, vidar-ds4-mtp]
```

Presets are optional operator conveniences. The executable model remains
recipes × lanes.

## Command Shape

Keep one script, but make it thin:

```bash
./benchmark --recipes timing-baseline --lanes ds4
./benchmark --recipes timing-baseline,or-passing --lanes ds4,ds4-mtp
./benchmark --preset timing-ds4
./benchmark --plan --recipes timing-baseline --lanes ds4-mtp
./benchmark prepare --recipes timing-baseline --lanes ds4-mtp
./benchmark lanes
./benchmark recipes
./benchmark validate
```

Implementation detail:

- `./benchmark` should call `.local/bin/fiz-bench` after ensuring the binary
  exists.
- Building `.local/bin/fiz-bench` is acceptable as a fast local prerequisite,
  but Docker/task preparation is not implicit.
- The Go CLI owns YAML parsing, alias resolution, validation, plan printing,
  and execution.
- The shell wrapper owns only environment defaults, binary bootstrap, and
  signal-safe process supervision if still needed.

## Startup Behavior

The first visible output for any run should be the resolved plan:

```text
Benchmark plan
  recipes: timing-baseline
  lanes:   vidar-ds4-mtp
  cells:   8 tasks × 1 lane × 3 reps = 24
  output:  benchmark-results/fiz-tools-vN
  prepare: required tasks overlay missing; run ./benchmark prepare ...
```

If preparation is missing, fail before creating output directories. Do not
start downloads or Docker builds from `run` unless the operator passed an
explicit `--prepare` or used the `prepare` subcommand.

## Validation

`./benchmark validate` must fail on the class of issues that caused the
`fiz-vidar-ds4-mtp` confusion:

- lane profile is missing
- profile id does not match lane profile
- lane env and profile provider fields disagree
- alias maps to no lane or multiple lanes
- recipe task file is missing
- resource group is missing
- comparison group references unknown lanes
- selected lane requires a preparation artifact that is absent
- selected lane has placeholder env documented as operational

Validation should be fast and offline. Live endpoint checks belong to
`./benchmark preflight --lanes ...`, not to `validate` or `--plan`.

## Migration Steps

1. **Add pure planner and validator.**
   - Add `fiz-bench benchmark plan`, `validate`, `lanes`, and `recipes` or
     equivalent subcommands under `cmd/bench`.
   - Move lane alias resolution into YAML.
   - Add profile/lane/resource/task validation.
   - Acceptance: `./benchmark --plan --recipes timing-baseline --lanes ds4-mtp`
     prints a plan or fails with a precise config error and creates no files.

2. **Make the shell wrapper thin.**
   - Replace phase logic, Python YAML snippets, alias case arms, and implicit
     preflight with calls into the Go planner.
   - Keep only binary bootstrap, default paths, and signal handling.
   - Acceptance: `bash -n scripts/benchmark/run_terminalbench_2_1_sweep.sh`
     passes and the wrapper has no embedded YAML-parsing Python blocks.

3. **Split preparation from execution.**
   - Move task download, task overlay mutation, task Docker image builds, and
     runtime image builds behind `./benchmark prepare`.
   - `run` fails with a clear "prepare artifact missing" message instead of
     doing preparation silently.
   - Acceptance: starting from empty `benchmark-results/external`, `--plan`
     and `validate` are still useful; `run` fails before output directory
     creation with the exact missing preparation command.

4. **Make recipes/lane selection orthogonal.**
   - Remove `recipes[].lanes`.
   - Add optional `presets[]` for named lane+recipe bundles.
   - Update `cmd/bench sweep` selection so `--recipes` and `--lanes` are both
     required unless `--preset` is provided.
   - Acceptance: any lane can run with any recipe unless validation marks the
     combination incompatible for a concrete reason.

5. **Clean up legacy vocabulary.**
   - Remove `--phase`, `--subset`, `--all-recipes`, `--staged-recipes`,
     `preferred`, `full`, and `qwen36-gpt55-full` from the public script.
   - Keep compatibility only in `fiz-bench sweep` if needed for one release,
     with deprecation tests.
   - Acceptance: `./benchmark --help` shows only recipes, lanes, presets,
     prepare, validate, plan, and run concepts.

6. **Wire DS4 MTP as a normal lane.**
   - Add `vidar-ds4-mtp` profile and lane alias data.
   - Confirm whether MTP is a request parameter, provider option, or server
     startup mode; remove placeholder env if it is not real.
   - Acceptance: `./benchmark --plan --recipes timing-baseline --lanes ds4-mtp`
     works from a clean checkout, and `validate` proves profile/lane consistency.

## Implementation Order

Recommended bead sequence:

1. Validator and pure planner.
2. Thin shell wrapper.
3. Explicit preparation command.
4. Orthogonal recipes and lanes schema change.
5. Legacy CLI cleanup.
6. DS4 MTP lane repair on the simplified schema.

The first three steps can land before the schema break. They immediately fix
the "empty directories and nothing runs" experience. The schema break should
come after plan/validate coverage is strong enough to catch regressions.

## Non-Goals

- Do not change matrix report format.
- Do not change evidence import.
- Do not run full TerminalBench as part of this cleanup.
- Do not add another lane-authoring layer before the runner contract is made
  simple.

## Open Decisions

- Whether `prepare` should be per recipe+lane selection or global for all task
  images. Default should be selected-only to avoid expensive surprise work.
- Whether endpoint preflight is automatic on `run` or explicit via
  `--preflight`. Default should be explicit until the command is fast and
  reliably scoped.
- Whether `presets[]` are needed at all after operators get comfortable with
  `--recipes` and `--lanes`.
