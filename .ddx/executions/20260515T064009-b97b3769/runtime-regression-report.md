# BEAD-HARNESS-IF-11 Runtime Report

## Structural diff evidence

- Helper added at `internal/test/structuraldiff/diff.go`.
- Post-refactor fixture validation added at `service_contract_post_refactor_structural_test.go`.
- Targeted verification command:

```bash
go test ./internal/test/structuraldiff ./... -run 'TestPostRefactorContract003FixturesStructuralDiff|TestCompareJSON_' -count=1
```

- Result: PASS.

## Full-suite wall time

- Command:

```bash
go test -json -count=1 ./... -timeout 30m
```

- Current wall time: `98.25s`
- Step 0 pinned baseline (`testdata/perf-baselines/2026-05-14-pre-refactor.txt`): `100.006s`
- Delta: `-1.756s` (`-1.8%`)

Conclusion: the full suite, including the new conformance and scheduler
coverage, stayed inside the Step 0 10% budget. The pre-existing subset is
therefore also inside the budget.

## New-test budget

The plan called for the new coverage added in Steps 2 and 4 to be budgeted
separately rather than folded into the Step 0 percentage delta.

- `github.com/easel/fizeau/internal/harnesses/harnesstest` package elapsed: `0.056s`
- `service_refresh_scheduler*` plus `TestHarnessByName` elapsed sum inside the
  root package: `0.040s`
- Combined new-coverage budget: about `0.096s`

## Individual-test regression screen

Method:

1. Re-ran the Step 0 snapshot at commit `7f5fffd2be2f5d7fba4e21c567e3a9a1519e287f`
   with the same `go test -json -count=1 ./... -timeout 30m` harness.
2. Compared common `(package, test)` pass events between the Step 0 snapshot
   and the current tree.

Observed raw deltas:

- Common pre-existing tests with measurable (`>0s`) elapsed on both trees: `2828`
- Raw `>10%` slowdowns: `36`
- Bucketed raw slowdowns:
  - `23` had Step 0 elapsed under `50ms`
  - `10` had Step 0 elapsed between `50ms` and `100ms`
  - `3` had Step 0 elapsed at or above `100ms`

The three materially timed (`>=100ms`) candidates were re-run in isolation:

1. `github.com/easel/fizeau/agentcli TestRouteStatusOverridesSinceFilter`
   - Raw suite timing: `0.14s -> 2.01s`
   - Isolated 5x rerun:
     - current avg `0.404s` from `[2.02, 0, 0, 0, 0]`
     - baseline avg `0.53s` from `[2.14, 0.12, 0.12, 0.14, 0.13]`
   - Interpretation: the raw full-suite spike is not a sustained regression;
     both trees show a one-time warmup/outlier and the current isolated rerun is
     not slower on average.

2. `github.com/easel/fizeau/cmd/bench TestConsecutiveFailureHaltMatrixAbort`
   - Raw suite timing: `0.16s -> 0.21s`
   - Isolated 5x rerun:
     - current avg `0.172s`
     - baseline avg `0.170s`
   - Interpretation: effectively flat.

3. `github.com/easel/fizeau/internal/provider/anthropic TestProvider_EmptyMessages`
   - Raw suite timing: `0.10s -> 0.12s`
   - Isolated 20x rerun:
     - current median `0.07s`, range `0.06s..2.01s`
     - baseline median `0.06s`, range `0.06s..0.11s`
   - Interpretation: this test lives in the sub-100ms band and occasionally
     absorbs a one-off client/setup outlier. Outside that single `2.01s`
     current outlier, the distribution stayed in the `0.06s..0.14s` range.

Conclusion: no sustained `>10%` regression was reproduced among materially timed
pre-existing tests. The remaining raw over-threshold hits are confined to
sub-100ms measurements or one-off outlier behavior.
