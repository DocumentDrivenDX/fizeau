# Fizeau Benchmark Scripts

This directory contains scripts and config for running fiz under
Terminal-Bench/Harbor and capturing reproducible benchmark baselines.

---

## Prerequisites

```bash
# Terminal-Bench dataset
harbor dataset pull terminal-bench/terminal-bench-2

# Docker (local runtime)
docker info  # must succeed
```

If `harbor` is not already installed, `smoke_run.sh` and `run_benchmark.sh`
now install it automatically with `uv tool install harbor`.

---

## Egress Canary (rig validation, before any new adapter work)

`egress_canary.sh` proves in-container egress to a tool-capable smoke
provider before spending budget on full benchmarks or building new
adapters. It targets a single concrete TB-2 task (`fix-git` by default —
the bead-pinned canary; terminal-bench@2.0 has no `hello-world` task at
the pinned commit) and accepts an **egress signal** (trajectory.json with
≥ 1 step) as success. The verifier reward is recorded but is *not* part
of the gate, since smoke models routinely fail TB-2 tasks.

```bash
OPENROUTER_API_KEY=sk-or-... ./scripts/benchmark/egress_canary.sh
```

Output: `benchmark-results/egress-canary-<UTC-TIMESTAMP>/` containing the
trajectory, reward, a `trial/` symlink, and a `canary.json` summary.

---

## Smoke Run (adapter validation)

Validates the Harbor adapter works end-to-end on a single task (~2 min).

```bash
# With Anthropic API key
ANTHROPIC_API_KEY=sk-... ./scripts/benchmark/smoke_run.sh

# With OpenRouter
OPENROUTER_API_KEY=sk-or-... ./scripts/benchmark/smoke_run.sh
```

**Passing criterion**:
- fiz exits 0
- `trajectory.json` is valid JSON with ≥ 1 step
- `reward.txt` exists (0 or 1 — both valid for smoke)

---

## Full Benchmark Run (baseline capture)

Runs the committed 15-task subset and emits a machine-readable report.

```bash
ANTHROPIC_API_KEY=sk-... ./scripts/benchmark/run_benchmark.sh
```

The report is written to `benchmark-results/report-<TIMESTAMP>.json` and
contains git SHA, agent version, model, provider, per-task outcomes, and
the aggregate resolved-task rate.

### Commit-independent runs

You can point the same harness at a prebuilt fiz binary instead of
rebuilding the current checkout:

```bash
FIZEAU_BENCH_BINARY=/path/to/fiz-linux-amd64 \
FIZEAU_BENCH_SHA=<commit-under-test> \
./scripts/benchmark/run_benchmark.sh
```

This is the required path for evidence-grade before/after comparison across
multiple agent SHAs.

### Dry-run harness validation

Use dry-run mode to validate the staged binary/config and recorded metadata
without invoking Harbor:

```bash
FIZEAU_BENCH_DRY_RUN=1 \
FIZEAU_BENCH_BINARY=/path/to/fiz-linux-amd64 \
FIZEAU_MODEL=qwen/qwen3.6-plus \
./scripts/benchmark/run_benchmark.sh
```

### Comparing two baselines

```bash
# Capture baseline before a change
ANTHROPIC_API_KEY=sk-... ./scripts/benchmark/run_benchmark.sh
# ... make changes ...
# Capture new run
ANTHROPIC_API_KEY=sk-... ./scripts/benchmark/run_benchmark.sh

# Compare (jq example)
jq '{resolved_rate: .summary.resolved_task_rate, tasks: [.tasks[] | {id, outcome}]}' \
  benchmark-results/report-*.json
```

---

## Task Subset

The evidence-grade 15-task benchmark subset is defined in `task-subset-v2.yaml`.
`task-subset-v1.yaml` remains in the repo as the historical placeholder manifest
from the original benchmark design and should not be used for before/after claims.
**Do not modify task IDs without updating the version field and filing a bead.**

Subset version policy: see SD-009 §3.

## Evidence-Grade Comparison Config

The pinned comparison controls live in `evidence-grade-comparison.env`. That file
freezes the intended `before` and `after` SHAs for the original benchmark-change
window plus the shared dataset, subset, runtime, preset, and provider/model route.
The only per-run inputs that should change are:

- `FIZEAU_BENCH_BINARY`
- `FIZEAU_BENCH_SHA` (set to either `FIZEAU_BENCH_BEFORE_SHA` or `FIZEAU_BENCH_AFTER_SHA`)

---

## Vidar Qwen3.6 Harness Matrix

`run_vidar_qwen36_terminalbench_baseline.sh` runs the shared Vidar oMLX
OpenAI-compatible profile against `fiz`, `pi`, and `opencode` through Harbor.
Use this path when comparing harness capability on the same local model/provider.

Before running, make sure the local model endpoint is reachable from Harbor task
containers as `http://vidar:1235/v1`. The script sets `OMLX_API_KEY=local` by
default because the local OpenAI-compatible endpoint only needs a non-empty key.

The `benchmark-results/` tree is ignored by git, so the pinned in-container
artifacts must be staged locally before a run. Defaults are:

- `benchmark-results/bin/opencode-1.3.17-linux-x64/opencode`
- `benchmark-results/bin/node-v20.19.2-linux-x64.tar.gz`
- `benchmark-results/bin/pi-coding-agent-0.67.1/package.tgz`

You can override those with `HARBOR_OPENCODE_ARTIFACT`, `HARBOR_NODE_TARBALL`,
and `HARBOR_PI_PACKAGE_TARBALL`.

One reproducible way to prepare the artifacts:

```bash
mkdir -p benchmark-results/bin

curl -fL \
  https://nodejs.org/dist/v20.19.2/node-v20.19.2-linux-x64.tar.gz \
  -o benchmark-results/bin/node-v20.19.2-linux-x64.tar.gz

docker run --rm --platform linux/amd64 \
  -v "$PWD/benchmark-results/bin:/out" \
  debian:bookworm-slim \
  sh -lc 'apt-get update >/dev/null &&
    apt-get install -y curl ca-certificates unzip >/dev/null &&
    curl -fsSL https://opencode.ai/install | VERSION=1.3.17 bash &&
    mkdir -p /out/opencode-1.3.17-linux-x64 &&
    cp /root/.opencode/bin/opencode /out/opencode-1.3.17-linux-x64/opencode &&
    chmod 755 /out/opencode-1.3.17-linux-x64/opencode'

docker run --rm --platform linux/amd64 \
  -v "$PWD/benchmark-results/bin:/out" \
  node:20-bookworm-slim \
  sh -lc 'npm install -g @mariozechner/pi-coding-agent@0.67.1 >/dev/null &&
    mkdir -p /out/pi-coding-agent-0.67.1 &&
    tar -C /usr/local/lib/node_modules/@mariozechner \
      -czf /out/pi-coding-agent-0.67.1/package.tgz pi-coding-agent'
```

For fair timings against a single local provider, run one harness at a time and
wait for it to finish before starting the next. Do not run separate matrix
processes concurrently against the same Vidar/oMLX server.

```bash
# Isolated opencode canary. This is the path used to validate opencode parity
# after the adapter fixes on 2026-05-06.
HARBOR_AGENT_TIMEOUT_MULTIPLIER=4 \
TIER=canary \
HARNESSES=opencode \
REPS=1 \
FORCE_RERUN=1 \
JOBS=1 \
OUT=benchmark-results/matrix-vidar-qwen36-canary-opencode-$(date -u +%Y%m%dT%H%M%SZ) \
scripts/benchmark/run_vidar_qwen36_terminalbench_baseline.sh

# Then run pi and fiz in separate invocations for uncontaminated timing.
TIER=canary HARNESSES=pi REPS=1 FORCE_RERUN=1 JOBS=1 \
  scripts/benchmark/run_vidar_qwen36_terminalbench_baseline.sh
TIER=canary HARNESSES=fiz REPS=1 FORCE_RERUN=1 JOBS=1 \
  scripts/benchmark/run_vidar_qwen36_terminalbench_baseline.sh
```

The opencode Harbor adapter intentionally runs with `--print-logs
--log-level DEBUG`, a fixed `--title terminal-bench`, and an explicit task
directory. Each run archives:

- `/logs/agent/opencode.txt` — JSON events plus internal opencode logs
- `/logs/agent/opencode.config.json` — generated provider/model config
- `/logs/agent/opencode-data.tgz` — opencode session/data directory

Known-good isolated canary result from 2026-05-06:

| Harness | Task | Status | Reward | Wall seconds |
|---------|------|--------|--------|--------------|
| opencode | `git-leak-recovery` | `graded_pass` | 1 | 527 |
| opencode | `fix-git` | `graded_pass` | 1 | 860 |
| opencode | `log-summary-date-ranges` | `graded_pass` | 1 | 499 |

Aggregate: `3/3`, mean reward `1.0`, standard deviation `0.0`.

### Wider local baseline

Use `TIER=wide` when the canary passes and you want broader task coverage
without jumping to the full 89-task TB-2 sweep:

```bash
TIER=wide HARNESSES=fiz REPS=1 FORCE_RERUN=1 JOBS=1 \
  scripts/benchmark/run_vidar_qwen36_terminalbench_baseline.sh
```

The wide tier uses
`scripts/beadbench/external/termbench-subset-local-wide.json`: 15 valid TB-2
tasks at the pinned dataset commit. It is intentionally separate from the
historical `termbench-subset.json`, which still contains five task IDs that are
not present in the pinned TB-2 tree.

### Claude and Codex reference baselines

Set `BASELINE=frontier` to run native Claude Code and Codex reference cells:

```bash
BASELINE=frontier TIER=canary REPS=1 FORCE_RERUN=1 JOBS=1 \
  scripts/benchmark/run_vidar_qwen36_terminalbench_baseline.sh
```

These cells are **not model-controlled comparisons** against Vidar/Qwen. They
answer a different question: how the native subscription/reference harnesses
perform on the same TerminalBench tasks. Keep them in a separate table from the
local-model rows unless the analysis explicitly calls out the model confound.

The runner uses paired one-cell invocations so Claude Code only runs with
`claude-native-sonnet-4-6` and Codex only runs with `codex-native-gpt-5-4`.
This avoids meaningless cross-products like Claude Code with the Codex profile.

Prepare linux artifacts before running inside Harbor containers:

```bash
mkdir -p benchmark-results/bin/claude-linux-amd64 benchmark-results/bin/codex-linux-amd64

# Either copy pinned linux/amd64 binaries into these locations, or set:
#   HARBOR_CLAUDE_ARTIFACT=/path/to/claude
#   HARBOR_CODEX_ARTIFACT=/path/to/codex
```

Authentication can use API keys passed by the profile env vars
(`ANTHROPIC_API_KEY`, `OPENAI_API_KEY`) or a tarred harness home directory:

```bash
tar -C "$HOME" -czf benchmark-results/bin/claude-home.tgz .claude
tar -C "$HOME" -czf benchmark-results/bin/codex-home.tgz .codex

HARBOR_CLAUDE_HOME_TARBALL=benchmark-results/bin/claude-home.tgz \
HARBOR_CODEX_HOME_TARBALL=benchmark-results/bin/codex-home.tgz \
BASELINE=frontier TIER=canary REPS=1 FORCE_RERUN=1 JOBS=1 \
  scripts/benchmark/run_vidar_qwen36_terminalbench_baseline.sh
```

Do not commit those tarballs or benchmark-result artifacts.

---

## Harbor Adapter

`harbor_agent.py` is the `BaseInstalledAgent` Python adapter that Harbor uses
to install and run fiz inside each task container. It handles:

1. **`install()`** — copies the `linux/amd64` binary and writes a provider config
2. **`run()`** — invokes `fiz --json --preset benchmark -p "<task>"`
3. **`populate_context_post_run()`** — converts session JSONL to ATIF v1.4 trajectory

The adapter's provider and prompt behavior are controlled by benchmark env vars
supplied by the runner, including:

- `FIZEAU_PROVIDER_NAME`
- `FIZEAU_PROVIDER`
- `FIZEAU_MODEL`
- `FIZEAU_BASE_URL`
- `FIZEAU_API_KEY_ENV`
- `FIZEAU_HEADERS_JSON`
- `FIZEAU_BENCH_PRESET`
- `FIZEAU_BENCH_SYSTEM_APPEND`

**Build the linux/amd64 binary before running**:

```bash
GOOS=linux GOARCH=amd64 go build -o scripts/benchmark/fiz-linux-amd64 ./cmd/fiz
```

### Credential injection

The adapter passes `ANTHROPIC_API_KEY` and `OPENROUTER_API_KEY` from the host
into the container via `get_env()`. A minimal config file is written during
`install()` that references `${ANTHROPIC_API_KEY}` — fiz's config loader
expands env vars at load time.

If the expected key env var is unset, the benchmark scripts now fall back to the
matching provider entry in the host fiz config (`.fizeau/config.yaml` or
`~/.config/fizeau/config.yaml`) and export the discovered `api_key` before
invoking Harbor.

---

## Thresholds (from SD-009 §5)

| Metric | Regression floor | Aspirational |
|--------|-----------------|-------------|
| Resolved-task rate | ≥ 55% | ≥ 70% |
| Clarification-question rate | < 10% | < 5% |
| Shell anti-pattern rate | < 30% of bash calls | < 10% |
| Structured-edit success rate | ≥ 70% | ≥ 90% |

---

## References

- `SD-008-terminal-bench-integration.md` — Harbor/Terminal-Bench integration audit
- `SD-009-benchmark-mode.md` — Benchmark mode, metrics, thresholds
- `benchmark-baseline-2026-04-08.md` — Initial baseline characterization
