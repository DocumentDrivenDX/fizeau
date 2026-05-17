# bead fizeau-a2fd070b execution report

## Changes

- `scripts/benchmark/benchmark` — added per-cell execution loop, sweep
  metadata capture, harbor task-executor invocation glue, report.json
  composition, env redaction, resume-default skip, `--force-rerun`,
  `--retry-invalid` (with `attempt_of` / `superseded_by` linking),
  `--reps N`, and bounded exponential-backoff retry for transient
  task-executor errors (connection refused/reset, HTTP 5xx, EOF parse).
- `scripts/benchmark/test_runner_loop.bash` — integration test that
  exercises all five ACs end-to-end.

## AC ↔ evidence

| AC | Evidence |
|----|----------|
| 1  | `test_runner_loop.bash` "AC1+AC2: initial sweep produces cells + sweep.json" — asserts every cell carries embedded `profile`, `command`, `env_redacted`, plus sibling `fiz.txt`, `fiz.err`, `session/`. |
| 2  | Same block asserts `<out>/sweep.json` contains `task_executor_version` + `harbor_runner_image_digest`, and each cell's `report.json` mirrors both values. |
| 3  | "AC3: rerun (no flag) skips terminal cells" — re-invokes the runner without `--force-rerun` and asserts the cell directory count is unchanged. |
| 4  | "AC4: --retry-invalid reruns invalid cells with attempt_of + superseded_by" — injects `invalid_class` into one cell + an orphan `cell-state.json` into another, reruns with `--retry-invalid`, and asserts both back-links. |
| 5  | "AC5: transient 5xx triggers exponential backoff + eventual success" — stub executor emits `HTTP 503` for two attempts then succeeds; the retry log shows three attempts and the cell finalises `completed`. A second pass with an always-failing stub asserts the retry cap closes the cell with `final_status=transient_exhausted`. |

## Verification

```bash
shellcheck -x scripts/benchmark/benchmark scripts/benchmark/test_runner_loop.bash
bash scripts/benchmark/test_runner_loop.bash
```

Both pass locally.
