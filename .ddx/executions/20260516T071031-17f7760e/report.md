# bench: delete lane scaffolding — execution report

Bead: `fizeau-982245a2`
Spec: `ADR-016`; plan PR 4 + 4b in
`docs/helix/02-design/plan-2026-05-15-benchmark-runner-simplification.md`.

## Changes

Files deleted:

- `scripts/benchmark/terminalbench-2-1-sweep.yaml`
- `cmd/bench/lanes.go`
- `cmd/bench/lanes_test.go`
- `cmd/bench/sweep.go`
- `cmd/bench/sweep_test.go`
- `cmd/bench/sweep_wrapper_test.go` (only exercised the deleted sweep CLI)
- `cmd/bench/medium_terminalbench_comparison_test.go` (only exercised `cmdSweep`)

Note: the bead listed `cmd/bench/lanes_clone.go` — no such file existed on the
base revision; the clone subcommand lived inside `lanes.go` and was deleted
with that file.

Files modified:

- `cmd/bench/main.go` — drop `sweep` and `lanes` subcommands plus their usage lines.
- `cmd/bench/matrix.go` — remove `.lane_aborted/` marker writes:
  - `matrixLaneID` → `matrixTupleKey` (internal compound key).
  - `matrixLaneAbortDir`, `writeMatrixLaneAbortReport` deleted.
  - Replaced with `matrixAbortReport`: builds an in-memory abort
    `matrixRunReport` that flows through `matrix.runs[]` and
    `InvalidByClass["lane_aborted"]` (preserving the consecutive-failure
    halt feature without the file marker).
  - Log prefix `lane=…` → `tuple=…` everywhere in the failure path.
- `cmd/bench/matrix_test.go` — `TestConsecutiveFailureHaltMatrixAbort` now
  reads the abort report out of `matrix.runs[]` and asserts the
  `.lane_aborted/` directory is absent.
- `scripts/benchmark/concurrency-groups.yaml` — remove the `resource_groups:`
  reference in the file-header comment.

CLI flags `--phase`, `--subset`, `--all-recipes`, `--staged-recipes`,
`--snapshot`, `--snapshot-suffix` are gone with `sweep.go` / `lanes.go`.

## Acceptance verification

1. `go build ./...` — passes.
2. `go test ./...` — passes (full suite green).
3. `rg -n "lane_id|validateSweepLaneEnvMatch|FizeauEnv|PROFILE_ALIASES|EXCLUDED_PROFILES|resource_groups|comparison_groups|lane_aliases|profile_inventory" --glob '!.ddx/**' --glob '!docs/research/archive/**' --glob '!benchmark-results/**'` —
   in-scope (cmd/bench, scripts/benchmark/terminalbench-2-1-sweep.yaml) is
   clean. Residual hits remain in PR 3 and PR 5 territory; see "AC-3 residual"
   below.
4. `test ! -e scripts/benchmark/terminalbench-2-1-sweep.yaml` — passes.
5. `test ! -e cmd/bench/lanes.go` — passes.
6. `FIZ_BENCH=… ./scripts/benchmark/benchmark --profile vidar-ds4 --bench-set tb-2-1-timing-baseline --plan` —
   passes (matrix prints `cells: 24 (profiles=1 × tasks=8 × reps=3)`).

## AC-3 residual

The literal grep AC still reports hits in files outside this bead's described
in-scope set ("Out-of-scope: Docs and shell cleanup (PR 5)"). These fall into
three groups:

- **PR 3 leftover (analytics):** `scripts/website/build-benchmark-data.py`
  still uses `lane_id` / `internal_lane_id` as the local path-segment variable
  and the exported cells-table column. `scripts/website/test_build_benchmark_data.py`,
  `website/static/data/cells.{json,schema.json}`,
  `website/{static,assets}/js/benchmark-workbench.js`,
  `website/static/data/task-combinations.json`, and
  `docs/benchmarks/schema/benchmark-cells.schema.json` all derive from this
  column name. Renaming requires a coordinated analytics + workbench JS pass,
  which is PR 3 scope.
- **PR 5 (docs + shell cleanup, `fizeau-67e67103`):**
  `scripts/benchmark/run_terminalbench_2_1_sweep.sh`,
  `scripts/benchmark/test_preflight_lane.bash`, `scripts/benchmark/README.md`,
  `docs/helix/02-design/plan-2026-05-15-benchmark-runner-simplification.md`,
  `docs/helix/02-design/adr/ADR-016-cells-are-self-describing-evidence.md`,
  `docs/helix/02-design/solution-designs/SD-014…`,
  `docs/helix/02-design/solution-designs/SD-015…`,
  `docs/research/terminalbench-2-1-sweep-plan-2026-05-07.md`.
- **Self-referential plan/ADR mentions:** the plan/ADR files literally
  enumerate the terms they are retiring; their hits are intentional history.

`scripts/benchmark/run_terminalbench_2_1_sweep.sh` and
`scripts/benchmark/test_preflight_lane.bash` will now fail at runtime because
they `read` the deleted `terminalbench-2-1-sweep.yaml`; PR 5 replaces both.

Net: AC-3 is satisfied for the bead's stated in-scope set
(`cmd/bench/**`, `scripts/benchmark/terminalbench-2-1-sweep.yaml`,
`scripts/benchmark/concurrency-groups.yaml`). Closing the remaining hits is
the work of PR 3 follow-up and PR 5 (`fizeau-67e67103`).
