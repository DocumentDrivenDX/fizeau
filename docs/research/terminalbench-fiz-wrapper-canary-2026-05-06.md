# TerminalBench fiz-wrapper canary memo

Date: 2026-05-06

## Command

Final run:

```bash
scripts/benchmark/run_medium_model_terminalbench_comparison.sh canary
```

Exit status: `0`

The wrapper wrote artifacts to:

- `benchmark-results/matrix-medium-model-canary-20260507T034749Z/matrix.json`
- `benchmark-results/matrix-medium-model-canary-20260507T034749Z/matrix.md`
- `benchmark-results/matrix-medium-model-canary-20260507T034749Z/costs.json`

## Official Lanes

The final matrix contains all six official `fiz-*` lanes:

- `fiz-harness-claude-sonnet-4-6`
- `fiz-harness-codex-gpt-5-4-mini`
- `fiz-harness-pi-gpt-5-4-mini`
- `fiz-harness-opencode-gpt-5-4-mini`
- `fiz-openrouter-claude-sonnet-4-6`
- `fiz-openrouter-gpt-5-4-mini`

Evidence:

- `matrix.json` lists the six profiles under `.profiles`.
- `matrix.md` renders the same six lanes in the reward table header.

## Result Summary

The canary did not reach graded task execution in this worktree because the
TerminalBench task submodule content is missing under
`scripts/benchmark/external/terminal-bench-2/<task-id>`.

Counts from `matrix.json`:

- Pass: `0`
- Fail: `0`
- Invalid: `18`
- Invalid classes:
  - `invalid_setup`: `18`

Per-cell summary:

- Each of the six official lanes has `n_runs=3`, `n_valid=0`, `n_invalid=3`.
- Every invalid run is classified as `install_fail_permanent` in the raw
  matrix row, but the aggregate classifier maps that setup failure to
  `invalid_setup`.

Evidence:

- `matrix.json` reports `"invalid_runs": 18` and `"invalid_by_class": {"invalid_setup": 18}`.
- `matrix.md` includes an `## Invalid runs` section with `invalid_setup` for
  every task row.
- `matrix.md` contains no graded pass/fail rows for this canary.

## Tool-Call / Session-Log Notes

No per-agent logs were produced in the final artifact directory:

- `find benchmark-results/matrix-medium-model-canary-20260507T034749Z -path '*/logs/agent/*' -type f | wc -l` returned `0`

Interpretation:

- Because all six lanes failed at setup before agent start, there were no
  tool-call or session-log traces to compare across harnesses.
- Any tool-call/session-log differences are therefore unavailable from this run
  and should not be inferred.

## Validation

Required test slice:

```bash
go test ./cmd/bench ./agentcli ./internal/harnesses/...
```

Result: passed after the small benchmark-runner fixes and the invalid-class
classifier update.
