# Fizeau Benchmark Scripts

This directory contains the TerminalBench 2.1 benchmark driver, profiles,
bench-sets, adapters, and task subsets for evaluating `fiz`.

The execution path is `./benchmark` (shell driver), governed by
[ADR-016](../../docs/helix/02-design/adr/ADR-016-cells-are-self-describing-evidence.md).
`cmd/bench` survives only as analytics tooling (`aggregate`,
`import-evidence`, `validate`, `plan --json`); it no longer owns cell
execution.

## Active Entry Point

```bash
./benchmark --profile P --bench-set B
```

Both flags are required (no implicit defaults for paid or local-heavy
runs). Comma-separated profile lists are supported:

```bash
./benchmark --profile vidar-ds4 --bench-set tb-2-1-timing-baseline
./benchmark --profile sindri-llamacpp,vidar-ds4 --bench-set tb-2-1-or-passing
./benchmark --profile openai-gpt-5-5 --bench-set tb-2-1-all
```

Supporting subcommands:

```bash
./benchmark --profile P --bench-set B --plan   # validate + print matrix; pure
./benchmark validate                           # schema + cross-reference, offline
./benchmark preflight --profile P              # probe endpoints (the only command that does)
./benchmark profiles                           # list available profile IDs
./benchmark bench-sets                         # list available bench-set IDs
```

Rules (per ADR-016):

1. `--plan` is pure: no file writes, no Docker pulls, no preflight, no
   `benchmark-results/` directory creation.
2. The cell directory is created only when the cell is about to start.
3. `validate` is fast and offline (schema + cross-reference only).
4. `preflight` is the only command that probes endpoints.
5. Manifest reading happens in one place: `fiz-bench plan --json` emits
   JSON, the shell consumes JSON, never YAML.

## Authoring a Profile

A profile is the single authoring unit. To add or vary one, copy an
existing file under `profiles/` and edit:

```bash
cp scripts/benchmark/profiles/<existing>.yaml scripts/benchmark/profiles/<new>.yaml
$EDITOR scripts/benchmark/profiles/<new>.yaml
./benchmark validate
```

Each profile carries the full configuration needed to invoke `fiz` once:
provider (type/model/base_url/api_key_env), sampling, limits, pricing,
`agent_timeout_multiplier`, `harness` (anthropic / codex / pi / opencode /
none), `surface` (`fiz_provider_native` / `fiz_harness_anthropic` / …),
`concurrency_group` (rate-limit shard key), metadata
(model_family, model_id, quant_label, runtime, server, backend, …), and
versioning (resolved_at, snapshot, snapshot_notes).

For DS4 specifically, MTP is a server/model property, not a Fizeau env
knob: the live `GET /props` capture reports `model.mtp=true`, and the
benchmark runtime-props extractor carries that into each cell's evidence.

## Bench-sets and Concurrency Groups

- `bench-sets/<id>.yaml` declares *what* to run: framework, dataset, task
  list, default reps. Bench-sets are admin metadata only — they do not
  appear in cell records or output paths.
- `concurrency-groups.yaml` declares rate-limit shard caps. Profiles join
  via `concurrency_group:`.

## Prerequisites

```bash
docker info
```

The runner installs Harbor with `uv tool install harbor` when Harbor is not
already available. The selected TerminalBench 2.1 tasks are downloaded
under `bench/results/external/terminal-bench-2-1` by default.

Provider keys are required only for selected profiles:

- `OPENAI_API_KEY` for OpenAI profiles
- `OPENROUTER_API_KEY` for OpenRouter profiles

Local profiles default to non-empty placeholder keys for local endpoints.

## Benchmark Workbench Browser Smoke

Use the checked-in browser regression command when changing the benchmark
workbench page, its data artifacts, or the static Hugo wiring:

```bash
make benchmark-workbench-smoke
```

The command builds the website, opens `/benchmarks/explorer/` in headless
Chromium, waits for DuckDB/Perspective to initialize, and asserts the row
count, filters, pairwise comparison table, task links, and default hidden
`terminalbench_task_url` column. `hugo` must be on `PATH`; Playwright
Chromium is installed automatically on first run.

## Output And Resume

Benchmark cells are self-describing evidence (ADR-016). Each cell embeds
its full resolved profile snapshot at write time; analytics never needs to
join back to the source YAML.

```text
benchmark-results/<canonical>/cells/
  <framework>-<dataset>/
    <task>/
      <cell-id>/                    # 20260516T103045Z-a4c1
        report.json                 # embeds full resolved profile
        fiz.txt                     # raw fiz log
        session/                    # trajectory artifacts
```

Cell IDs are ISO-timestamp + short random suffix. They are monotonic,
unique, and race-free — chronological sort gives history.

Re-running the same `(profile, bench-set)` resumes from existing cells
whose `final_status` is terminal (`pass` / `fail` / `timeout`).
`--retry-invalid` re-runs cells classified `invalid` (typical use case:
infrastructure failure has been fixed). `--force-rerun` ignores terminal
state.

## Consolidating Existing Results

Use `fiz-bench aggregate` and `fiz-bench import-evidence` to scan historical
`benchmark-results/**/report.json` files and emit indexes / aggregate
summaries. These commands read embedded `cell.profile.*` metadata directly
— there is no separate profile-YAML join step.

## Active Config Files

- `profiles/*.yaml`
- `bench-sets/*.yaml`
- `concurrency-groups.yaml`
- `task-subset-tb21-canary.yaml`
- `task-subset-tb21-full.yaml`
- `task-subset-tb21-all.yaml`
- `task-subset-tb21-openai-cheap.yaml`
- `Dockerfile.agent-runtime`
- `harbor_agent.py`
- `harness_adapters/`
- `harbor_adapters/`
