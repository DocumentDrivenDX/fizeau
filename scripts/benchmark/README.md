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
./benchmark --phase full --lanes sindri-llamacpp,vidar
./benchmark --phase full --lanes openrouter-qwen36,sindri-llamacpp,vidar
./benchmark --phase qwen36-gpt55-full
```

Short lane aliases:

- `openai-gpt55` -> `fiz-openai-gpt-5-5`
- `openrouter-qwen36` -> `fiz-openrouter-qwen3-6-27b`
- `sindri-llamacpp` -> `fiz-sindri-llamacpp-qwen3-6-27b`
- `sindri-vllm` -> `fiz-sindri-vllm-qwen3-6-27b`
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

Benchmark cells live in the version-rooted canonical tree, keyed on the
dimensions that vary per run:

```text
benchmark-results/fiz-<version>/
  cells/<dataset>/<task>/<profile_id>/rep-NNN/
    report.json         # carries fiz_tools_version + profile_id (small set of stamped fields)
    fiz-<task>-rep<N>/  # Harbor job artifacts, work/, etc.
  profiles/<profile_id>.yaml   # snapshotted profile catalog (server, model_family, quant, runtime)
  indexes/runs.jsonl           # one row per cell, joinable to profiles/ on profile_id
  indexes/summary.{csv,md}
  <phase>/<lane>/matrix.json   # per-invocation summary; cells live in the shared cells/ tree
```

Reasoning:

- `profile_id` is the natural primary key — it uniquely encodes
  (server, runtime, model, quant, sampling) by construction. Two profiles
  with the same provider+model but different physical servers (sindri vs
  bragi) get separate cells, which the previous `<provider>/<model>` layout
  collapsed.
- Profile metadata (server / model_family / quant_label / provider_surface /
  runtime / hardware_label / endpoint) lives in the per-version snapshot at
  `profiles/`, joined on `profile_id` at index time. Editing
  `scripts/benchmark/profiles/*.yaml` does NOT retroactively rewrite
  historical cells.
- `fiz_tools_version` (constant in `internal/fiztools`) tracks agent
  behavior separately from fiz semver. Bump only when prompts / tool
  schemas / agent loop logic change, not for routing/test/CI work.

Re-running the same phase/lane resumes from existing graded `report.json`
files automatically. Cells classified as `invalid_*` are skipped on
resume by default; pass `--retry-invalid` to opt in to re-running them
(typical use case: infrastructure failure has been fixed and cells
should be re-attempted).

Only `profiles/` and `indexes/` are intended to be checked in. The raw
`cells/` tree stays gitignored — large and contains Harbor job artifacts.

## Consolidating Existing Results

Use `fiz-bench matrix-index` to scan historical `benchmark-results/**/report.json`
files, snapshot the profile catalog, and emit a phase-independent index.

```bash
go run ./cmd/bench matrix-index \
  --copy \
  --root benchmark-results \
  --canonical-out benchmark-results/fiz-tools-v1 \
  --fiz-tools-version 1
```

`--copy` migrates each source cell directory into the canonical layout
(at `<canonical>/cells/<dataset>/<task>/<profile_id>/rep-NNN/`) and stamps
`fiz_tools_version=1` on historical reports. The index row preserves each
row's original `source_path` for traceability. Cells filed under unknown
profile_ids land in an "unknown" projection bucket — fix by adding a
`metadata:` block to the corresponding profile YAML.

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
