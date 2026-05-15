# bench

This directory is the canonical home for repository benchmark assets.

## Layout

```text
bench/
  corpus/          versioned benchmark tasks
  scorer/          scorer package used by cmd/benchscore
  results/         benchmark outputs; mostly gitignored
  run              repo-root benchmark runner wrapper
```

`bench/run` delegates to `scripts/benchmark/run_terminalbench_2_1_sweep.sh`.
Benchmark outputs land under `bench/results/`; only the version marker and
indexes are tracked.

Each file in `bench/corpus/` is a YAML (or JSON) task:

```yaml
id: unique-task-id
description: "Human-readable description"
prompt: |
  The prompt sent to the agent.
expected_tools:
  - read
  - find
permissions: safe
reasoning: low
tags:
  - tool-use
```
