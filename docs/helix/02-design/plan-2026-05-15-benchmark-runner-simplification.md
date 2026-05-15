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
  lanes by invoking the regular `fiz` binary for each cell.

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
- `cmd/bench` has become a second execution product parallel to `fiz`.
  Benchmark execution should not require understanding a separate Go binary
  with its own routing/configuration model.
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
8. Benchmark execution uses `fiz`, not `fiz-bench`. Go benchmark helpers may
   validate, summarize, import evidence, or convert reports, but they do not
   own the cell execution loop.

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

- `./benchmark` resolves the selected recipes and lanes into a concrete cell
  list, then invokes `fiz` with explicit CLI flags and environment for each
  cell.
- Building `fiz` as a local prerequisite is acceptable. Docker/task
  preparation is not implicit.
- YAML parsing and validation can be implemented in shell plus `yq`/Python, or
  by small support commands, but support commands must not own execution.
- The shell script owns matrix expansion, output path creation, resume checks,
  process supervision, and the `fiz` invocation.

For simple prompt/file benchmarks, a cell is:

```bash
env "${lane_env[@]}" \
  fiz --json --work-dir "$REPO_ROOT" "${lane_fiz_args[@]}" \
  -p "@$task_prompt" \
  > "$cell_dir/fiz.jsonl"
```

For TerminalBench, a cell is still a Harbor task run, because Harbor owns the
task container and grader. The installed Harbor agent should invoke `fiz`; the
benchmark script should pass lane env and task id explicitly:

```bash
env "${lane_env[@]}" \
  harbor run "$task_id" \
  --agent fizeau \
  --output-dir "$cell_dir" \
  --env FIZEAU_PROVIDER=... \
  --env FIZEAU_MODEL=...
```

That is still the same principle: the benchmark runner expands the matrix;
`fiz` performs agent execution; Harbor only supplies the benchmark harness.

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

## Regression Guardrails

This cleanup must not silently drop behavior that current benchmark runs depend
on. Before deleting any `cmd/bench sweep` or `cmd/bench matrix` execution path,
capture the current behavior as parity fixtures:

- **Plan parity:** for representative invocations, old and new planners produce
  the same selected task ids, lane ids, reps, resource groups, and output cell
  keys.
- **Command parity:** for one local lane, one managed provider lane, and one
  harness lane, the new runner emits the same effective `fiz`/Harbor command
  inputs as the old runner.
- **Resume parity:** existing completed `report.json` cells are skipped; invalid
  cells rerun only with the retry-invalid option; force-rerun ignores terminal
  reports.
- **Concurrency parity:** resource-group caps still serialize local endpoints
  and allow independent resource groups to run concurrently.
- **Failure parity:** lane abort after repeated identical failures is either
  preserved in the shell runner or deliberately moved to a documented
  post-processing/preflight check before old code is removed.
- **Telemetry parity:** generated cell directories still contain the artifacts
  required by existing report generation and evidence import.
- **Signal parity:** interrupting the script terminates active child processes
  and leaves resumable cell state.

Add a checked-in `scripts/benchmark/testdata/plans/` directory with golden
expected plans for at least:

- `recipes=timing-baseline lanes=vidar-ds4`
- `recipes=or-passing lanes=sindri-llamacpp,vidar-ds4`
- `recipes=tb21-all lanes=openai-gpt55`
- `preset=<current staged equivalent>` if presets survive

The new script's `--plan --json` output should be compared to those fixtures in
tests. If a fixture changes, the diff must show an intentional benchmark
contract change, not incidental refactor churn.

## Current Functionality Inventory

These features must be preserved, explicitly replaced, or explicitly retired
with a commit that names the removal:

- TerminalBench task selection from recipe task-list files.
- Repetition count per recipe.
- Resume, force-rerun, and retry-invalid behavior.
- Per-resource-group concurrency limits.
- Local/managed lane distinction for default concurrency and key handling.
- Harbor task execution and grading.
- FizeauAgent forwarding of lane env into `fiz`.
- Per-cell `report.json`, `fiz.txt`/session logs, trajectory artifacts, and
  runtime props where available.
- Invalid setup/provider/quota/auth classification.
- Consecutive identical failure lane abort.
- Budget and per-run budget caps, or a deliberate replacement.
- Preflight endpoint probe as an explicit command.
- Report aggregation and evidence import compatibility.
- Stale Harbor container cleanup, if still needed after the shell process model
  is simplified.

Do not remove the old Go execution path until this inventory has an owner and a
parity or retirement decision for each line.

## Migration Steps

1. **Add pure planner and validator.**
   - Add `./benchmark --plan`, `./benchmark validate`, `./benchmark lanes`, and
     `./benchmark recipes`.
   - The planner may call a small helper for YAML parsing, but its output is a
     shell-consumable JSON cell list and it does not execute cells.
   - Move lane alias resolution into YAML.
   - Add profile/lane/resource/task validation.
   - Acceptance: `./benchmark --plan --recipes timing-baseline --lanes ds4-mtp`
     prints a plan or fails with a precise config error and creates no files.

2. **Add parity fixtures before changing execution.**
   - Capture golden plan JSON for representative current invocations.
   - Capture effective command inputs for one local provider lane, one managed
     provider lane, and one harness lane.
   - Acceptance: old runner and new planner agree on task ids, lane ids, reps,
     resource groups, and cell output keys for the fixture set.

3. **Make the shell wrapper the runner.**
   - Replace phase logic, Python YAML snippets, alias case arms, and implicit
     preflight with a small matrix loop over planner JSON.
   - Invoke `fiz` directly for simple cells and Harbor's FizeauAgent for
     TerminalBench cells.
   - Keep binary bootstrap, default paths, output path creation, resume checks,
     and signal handling in shell.
   - Acceptance: `bash -n scripts/benchmark/run_terminalbench_2_1_sweep.sh`
     passes and a plan-only run has no side effects.

4. **Split preparation from execution.**
   - Move task download, task overlay mutation, task Docker image builds, and
     runtime image builds behind `./benchmark prepare`.
   - `run` fails with a clear "prepare artifact missing" message instead of
     doing preparation silently.
   - Acceptance: starting from empty `benchmark-results/external`, `--plan`
     and `validate` are still useful; `run` fails before output directory
     creation with the exact missing preparation command.

5. **Make recipes/lane selection orthogonal.**
   - Remove `recipes[].lanes`.
   - Add optional `presets[]` for named lane+recipe bundles.
   - Update the shell runner so `--recipes` and `--lanes` are both required
     unless `--preset` is provided.
   - Acceptance: any lane can run with any recipe unless validation marks the
     combination incompatible for a concrete reason.

6. **Port or retire reporting behavior.**
   - Keep report aggregation and evidence import as support tooling if useful.
   - Move execution-only responsibilities out of `cmd/bench`.
   - Acceptance: existing report-generation/import tests pass against output
     from the shell runner, or the plan records the replacement command and
     test.

7. **Clean up legacy vocabulary.**
   - Remove `--phase`, `--subset`, `--all-recipes`, `--staged-recipes`,
     `preferred`, `full`, and `qwen36-gpt55-full` from the public script.
   - Keep compatibility only in internal helpers if needed for one release,
     with deprecation tests and a dated removal bead.
   - Acceptance: `./benchmark --help` shows only recipes, lanes, presets,
     prepare, validate, plan, and run concepts.

8. **Wire DS4 MTP as a normal lane.**
   - Add `vidar-ds4-mtp` profile and lane alias data.
   - Confirm whether MTP is a request parameter, provider option, or server
     startup mode; remove placeholder env if it is not real.
   - Acceptance: `./benchmark --plan --recipes timing-baseline --lanes ds4-mtp`
     works from a clean checkout, and `validate` proves profile/lane consistency.

9. **Delete or demote the old Go runner.**
   - Once parity fixtures pass and the functionality inventory has decisions,
     remove `cmd/bench sweep/matrix` execution commands or mark them as
     deprecated support tools that cannot launch benchmark cells.
   - Acceptance: no documentation tells operators to start a benchmark through
     `fiz-bench`; `rg -n "fiz-bench (sweep|matrix)|cmd/bench.*execution"`
     finds only historical notes or tests for removed behavior.

## Implementation Order

Recommended bead sequence:

1. Validator and pure planner.
2. Parity fixture capture.
3. Shell matrix runner that calls `fiz`.
4. Explicit preparation command.
5. Orthogonal recipes and lanes schema change.
6. Reporting/evidence compatibility pass.
7. Legacy CLI cleanup.
8. DS4 MTP lane repair on the simplified schema.
9. Old Go runner deletion/demotion.

The first three steps can land before the schema break. They immediately fix
the "empty directories and nothing runs" experience. The schema break should
come after plan/validate coverage is strong enough to catch regressions.

## Non-Goals

- Do not change matrix report format.
- Do not change evidence import.
- Do not run full TerminalBench as part of this cleanup.
- Do not add another lane-authoring layer before the runner contract is made
  simple.
- Do not leave two blessed benchmark execution paths. During migration there
  may be a dual-run period, but it must have parity tests and a deletion bead.

## Open Decisions

- Whether `prepare` should be per recipe+lane selection or global for all task
  images. Default should be selected-only to avoid expensive surprise work.
- Whether endpoint preflight is automatic on `run` or explicit via
  `--preflight`. Default should be explicit until the command is fast and
  reliably scoped.
- Whether `presets[]` are needed at all after operators get comfortable with
  `--recipes` and `--lanes`.
