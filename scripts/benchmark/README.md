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
- `openai-cheap`: 35 lower-cost GPT-5.5 tasks at `k=5`, selected from observed
  GPT-5.5 costs plus Qwen3.6 token-count proxies.
- `full` / `tb21-all`: all 89 TerminalBench 2.1 tasks.
- `qwen36-gpt55-full`: full run over OpenAI GPT-5.5, OpenRouter Qwen3.6 27B,
  Sindri Qwen3.6 27B, and Vidar Qwen3.6 27B.

The sweep plan is `scripts/benchmark/terminalbench-2-1-sweep.yaml`.

## Output And Resume

By default, benchmark runs write to the current fiz version root:

```text
benchmark-results/fiz-<version>/
  .fiz-benchmark-version.json
  cells/terminal-bench-2-1/<task>/<provider>/<model>/<harness>/rep-001/...
  <phase>/<lane>/matrix.json
  indexes/runs.jsonl
  indexes/summary.csv
  indexes/summary.md
```

Re-running the same phase/lane combination resumes from existing terminal
`report.json` files in the version-rooted `cells/` tree. Do not create
timestamped output directories for ordinary benchmark continuation.

Only the version marker and `indexes/` are intended to be checked in. The raw
`cells/` tree and phase summary directories stay gitignored: they are large and
may contain provider secrets in Harbor config/result files. The runner refreshes
`indexes/` after each completed invocation.

## Consolidating Existing Results

Use `fiz-bench matrix-index` to scan historical `benchmark-results/**/report.json`
files and emit a phase-independent index.

```bash
go run ./cmd/bench matrix-index \
  --root benchmark-results \
  --canonical-out benchmark-results/fiz-v0.10.16 \
  --fiz-version v0.10.16
```

The index preserves each row's original `source_path` and marks missing old
fiz version metadata as `unknown`. Do not pass `--copy` for committed
benchmark records; that option is only for local forensic work when a temporary
canonical raw-artifact tree is useful.

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
