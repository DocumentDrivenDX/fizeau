# PR 4 — Delete lane scaffolding (fizeau-982245a2)

## Status

In-scope deletions complete. AC #3 (zero-hit grep) partially satisfied;
remaining hits live in surfaces that the bead description explicitly
defers to PR 5 (docs + shell) and the workbench rename track (PR 5/6).

## What was deleted

- `cmd/bench/lanes.go`
- `cmd/bench/lanes_test.go`
- `cmd/bench/sweep.go`
- `cmd/bench/sweep_test.go`
- `cmd/bench/medium_terminalbench_comparison_test.go` (depended on `cmdSweep`)
- `cmd/bench/sweep_wrapper_test.go` (depended on the deprecated shell driver)
- `scripts/benchmark/terminalbench-2-1-sweep.yaml`

## Surviving files modified

- `cmd/bench/main.go` — drops `sweep` and `lanes` subcommand dispatch and
  usage text.
- `scripts/benchmark/concurrency-groups.yaml` — comment no longer
  references the deleted `resource_groups:` block.
- `scripts/benchmark/README.md` — single `lane_aliases:` reference rewritten.
- `docs/research/terminalbench-2-1-sweep-plan-2026-05-07.md` — moved
  under `docs/research/archive/` (per spec PR 4b "non-archival" wording).

## Acceptance check

| AC | Status | Evidence |
|----|--------|----------|
| 1. `go build ./...` | PASS | clean build |
| 2. `go test ./...`  | PASS | full suite green |
| 3. zero-hit grep    | PARTIAL — see below |
| 4. sweep yaml gone  | PASS | `test ! -e scripts/benchmark/terminalbench-2-1-sweep.yaml` |
| 5. lanes.go gone    | PASS | `test ! -e cmd/bench/lanes.go` |
| 6. `./benchmark --plan` | PASS | `vidar-ds4 / tb-2-1-timing-baseline / --plan` emits the resolved matrix |

## AC #3 residuals

Running the bead's grep verbatim still finds hits in three buckets, each
covered by a future bead:

1. **Governing specs** — `docs/helix/02-design/plan-2026-05-15-...md` (17)
   and `docs/helix/02-design/adr/ADR-016-...md` (11) describe the
   deletion itself; their copies of the vocabulary are load-bearing and
   cannot be removed without breaking the historical record. Companion
   SD-014 and SD-015 each carry one reference for the same reason.
2. **Shell + Python harness (PR 5 scope per bead description)** —
   `scripts/benchmark/run_terminalbench_2_1_sweep.sh` (39),
   `scripts/benchmark/test_preflight_lane.bash` (1). Both are functionally
   broken by this PR (they invoke the deleted `fiz-bench sweep` and the
   deleted sweep YAML) and PR 5 is on the hook to delete or rewrite them.
3. **Workbench/website `internal_lane_id` field** — `scripts/website/*.py`
   (7), `website/{assets,static}/js/benchmark-workbench.js` (4),
   `website/static/data/{cells,task-combinations,cells.schema}.json` (6),
   `docs/benchmarks/schema/benchmark-cells.schema.json` (2). The
   substring collision (`internal_lane_id` contains `lane_id`) is a
   downstream consumer schema rename; SD-014 / SD-015 own the cleanup
   pass once the workbench finishes embedding the resolved profile.

The bead description explicitly lists "Docs and shell cleanup (PR 5)" as
out of scope, so attacking buckets 1–3 in this PR would either contradict
that scope statement or be impossible (governing specs).

## Notes

- `cmd/bench/matrix.go` was not modified. The "lane-id handling at the
  cell-write boundary" referenced in the bead description does not show
  up in AC #3's grep — the internal helpers (`matrixLaneID`,
  `matrixLaneAbortDir`, etc.) are private path-shaping utilities keyed
  on `harness/profile_id`, never serialized as `lane_id`. They do not
  reintroduce lane scaffolding and removing them would be unrelated
  refactoring.
- `matrixTupleDir` survives because `cmd/bench/matrix_aggregate_test.go`
  still uses it to write fixture cells under the legacy layout.
