# Fizeau Benchmark Scripts

This directory contains the active TerminalBench 2.1 benchmark runner,
profiles, adapters, and task subsets for evaluating `fiz`.

## Active Entry Point

Use the repo-root `./bench/run` wrapper. It delegates to
`scripts/benchmark/run_terminalbench_2_1_sweep.sh` and is the only supported
benchmark script entry point.

```bash
./bench/run --phase canary
./bench/run --phase openai-cheap
./bench/run --phase tb21-all --lanes openai-gpt55
./bench/run --phase tb21-all --lanes openrouter-qwen36
./bench/run --phase tb21-all --lanes sindri-llamacpp,vidar
./bench/run --phase tb21-all --lanes openrouter-qwen36,sindri-llamacpp,vidar
./bench/run --phase qwen36-gpt55-full
./bench/run --phase all
```

The wrapper keeps the historical `--phase` surface and maps those phase ids to
the underlying recipe-based sweep plan.

Short lane aliases are data-driven from the historical alias block in the
deleted TB-2.1 sweep plan. Current defaults include:

- `openai-gpt55` -> `fiz-openai-gpt-5-5`
- `openrouter-qwen36` -> `fiz-openrouter-qwen3-6-27b`
- `sindri-llamacpp` -> `fiz-sindri-llamacpp-qwen3-6-27b`
- `sindri-vllm` -> `fiz-sindri-vllm-qwen3-6-27b`
- `vidar` -> `fiz-vidar-omlx-qwen3-6-27b`
- `vidar-ds4` -> `fiz-vidar-ds4`
- `ds4-mtp` -> `fiz-vidar-ds4-mtp`
- `vidar-ds4-mtp` -> `fiz-vidar-ds4-mtp`

## Authoring Lanes

Use `lanes clone` to create a new sweep lane/profile pair from an existing one
without hand-editing YAML. The command updates the sweep plan, writes the new
profile YAML, enrolls recipes, and adds short aliases in one pass.

This reproduces the intended `fiz-vidar-ds4-mtp` lane/profile in a scratch
checkout without manual edits:

```bash
go run ./cmd/bench lanes clone \
  --work-dir /tmp/fizeau-lane-scratch \
  --from-lane fiz-vidar-ds4 \
  --lane-id fiz-vidar-ds4-mtp \
  --profile-id vidar-ds4-mtp \
  --recipes timing-baseline,or-passing,tb21-all \
  --aliases vidar-ds4-mtp,ds4-mtp \
  --quant-label ds4-native-bf16-mtp \
  --metadata mtp=enabled \
  --resolved-at 2026-05-15 \
  --snapshot-suffix " | mtp=enabled" \
  --dry-run
```

Drop `--dry-run` to write the files. If you already built the helper, replace
`go run ./cmd/bench` with `fiz-bench`.

For ds4 specifically, MTP is a server/model property, not a Fizeau env knob:
the live `GET /props` capture reports `model.mtp=true`, and the benchmark
runtime-props extractor carries that into each cell's evidence.

## Prerequisites

```bash
docker info
```

The runner installs Harbor with `uv tool install harbor` when Harbor is not
already available. The selected TerminalBench 2.1 tasks are downloaded under
`bench/results/external/terminal-bench-2-1` by default.

Benchmark helper binaries and runtime payloads are not benchmark results. By
default the wrapper writes the host `fiz-bench` helper to `.local/bin/` and
container/runtime payloads to `.local/share/fizeau/benchmark-runtime/`. Override
those locations with `BENCHMARK_BIN_DIR` and `BENCHMARK_RUNTIME_DIR` when tests
or one-off runs need isolated scratch space.

Provider keys are required only for selected lanes:

- `OPENAI_API_KEY` for `openai-gpt55`
- `OPENROUTER_API_KEY` for `openrouter-qwen36` and other OpenRouter lanes

Local provider lanes default to non-empty placeholder keys for local endpoints.

## Benchmark Workbench Browser Smoke

Use the checked-in browser regression command when changing the benchmark
workbench page, its data artifacts, or the static Hugo wiring:

```bash
make benchmark-workbench-smoke
```

The command builds the website, opens `/benchmarks/explorer/` in headless
Chromium, waits for DuckDB/Perspective to initialize, and asserts the row
count, filters, pairwise comparison table, task links, and default hidden
`terminalbench_task_url` column. `hugo` must be on `PATH`; Playwright Chromium
is installed automatically on first run.

## Subsets, Recipes, and Phases (deprecated)

The sweep plan (`scripts/benchmark/terminalbench-2-1-sweep.yaml`) declares two
orthogonal blocks since the v2 schema (2026-05-14, fizeau-596ff006):

- **`subsets:`** — pure task lists. Each entry has `id`, `path`, `default_reps`.
  Subsets carry no lane info.
- **`recipes:`** — curated CLI bundles. Each pairs one `subset:` with a `lanes:`
  list and optional overrides (`reps`, `max_concurrency_override`,
  `parallel_policy`, `preflight`, `staged`). Recipes are pure sugar at runtime
  — the executable matrix is `(subset, lane)`. Recipes with `staged: true`
  participate in `--staged-recipes` (the historical `--phase all` gate).

Main recipes in YAML order:

- `canary`: 3 small tasks to prove selected lanes start and write artifacts.
- `local-qwen`: full Qwen3.6-27B local providers vs. fiz native lane.
- `timing-baseline`, `or-passing`, `tb21-all`: targeted/full subsets,
  non-staged.
- `openai-cheap`: 35 lower-cost GPT-5.5 tasks at `k=5`.
- `sonnet-comparison`, `gpt-comparison`: harness-vs-provider comparisons.
- `medium-model-canary`, `medium-model`: official medium-model fiz-wrapper
  comparison sweeps.

`--phase X` and `--phase all` remain as deprecated aliases for `--recipe X` and
`--staged-recipes` respectively.

## Output And Resume

Benchmark cells live in the version-rooted canonical tree, keyed on the
dimensions that vary per run:

```text
bench/results/fiz-tools-v<version>/
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

Use `fiz-bench matrix-index` to scan historical `bench/results/**/report.json`
files, snapshot the profile catalog, and emit a phase-independent index.

```bash
go run ./cmd/bench matrix-index \
  --copy \
  --root bench/results \
  --canonical-out bench/results/fiz-tools-v1 \
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
