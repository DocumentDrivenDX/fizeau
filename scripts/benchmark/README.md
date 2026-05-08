# Fizeau Benchmark Scripts

This directory contains the active TerminalBench 2.1 benchmark runner,
profiles, adapters, and task subsets for evaluating `fiz`.

## Active Entry Point

Use the repo-root `./benchmark` wrapper. It delegates to
`scripts/benchmark/run_terminalbench_2_1_sweep.sh` and is the only supported
benchmark script entry point.

```bash
./benchmark --phase canary
./benchmark --phase openai-cheap
./benchmark --phase full --lanes openai-gpt55
./benchmark --phase full --lanes openrouter-qwen36
./benchmark --phase full --lanes sindri,vidar
./benchmark --phase full --lanes openrouter-qwen36,sindri,vidar
./benchmark --phase qwen36-gpt55-full
```

Short lane aliases:

- `openai-gpt55` -> `fiz-openai-gpt-5-5`
- `openrouter-qwen36` -> `fiz-openrouter-qwen3-6-27b`
- `sindri` -> `fiz-sindri-club-3090-qwen3-6-27b`
- `vidar` -> `fiz-vidar-omlx-qwen3-6-27b`

## Prerequisites

```bash
docker info
```

The runner installs Harbor with `uv tool install harbor` when Harbor is not
already available. The selected TerminalBench 2.1 tasks are downloaded under
`benchmark-results/external/terminal-bench-2-1` by default.

Provider keys are required only for selected lanes:

- `OPENAI_API_KEY` for `openai-gpt55`
- `OPENROUTER_API_KEY` for `openrouter-qwen36` and other OpenRouter lanes

Local provider lanes default to non-empty placeholder keys for local endpoints.

## Main Phases

- `canary`: 3 small tasks to prove selected lanes start and write artifacts.
- `openai-cheap`: 23 lower-cost GPT-5.5 tasks at `k=5`, selected from observed
  GPT-5.5 costs plus Qwen3.6 token-count proxies.
- `full` / `tb21-all`: all 89 TerminalBench 2.1 tasks.
- `qwen36-gpt55-full`: full run over OpenAI GPT-5.5, OpenRouter Qwen3.6 27B,
  Sindri Qwen3.6 27B, and Vidar Qwen3.6 27B.

The sweep plan is `scripts/benchmark/terminalbench-2-1-sweep.yaml`.

## Output And Resume

Until the version-rooted layout lands, pass an explicit `--out` when you want
to resume into an existing run:

```bash
./benchmark --phase openai-cheap --out benchmark-results/openai-cheap-20260508T040410Z
```

The runner resumes existing `report.json` files under the selected phase/lane
directory. Existing phase/lane directories are operational artifacts, not the
analysis model.

The planned canonical model is:

```text
benchmark-results/fiz-<version-or-commit>/
  .fiz-benchmark-version.json
  cells/terminal-bench-2-1/<task>/<provider>/<model>/<harness>/rep-001/report.json
  indexes/runs.jsonl
  indexes/summary.csv
  indexes/summary.md
```

Phase outputs should become views over canonical cells.

## Consolidating Existing Results

Use `fiz-bench matrix-index` to scan historical `benchmark-results/**/report.json`
files and emit a phase-independent index.

```bash
go run ./cmd/bench matrix-index \
  --root benchmark-results \
  --canonical-out benchmark-results/fiz-dev \
  --copy
```

The index preserves each row's original `source_path` and marks missing old
fiz version metadata as `unknown`.

## Active Config Files

- `terminalbench-2-1-sweep.yaml`
- `task-subset-tb21-canary.yaml`
- `task-subset-tb21-full.yaml`
- `task-subset-tb21-all.yaml`
- `task-subset-tb21-openai-cheap.yaml`
- `profiles/*.yaml`
- `Dockerfile.agent-runtime`
- `harbor_agent.py`
- `harness_adapters/`
- `harbor_adapters/`

Older TB-2.0 wrappers and one-off lane scripts were removed. Git history keeps
their implementation details if an old experiment needs to be reconstructed.
