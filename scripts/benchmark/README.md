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

## Evidence Workflow

The benchmark evidence pipeline is:

1. Run a benchmark job and keep the raw artifacts in `benchmark-results/`.
2. Import the resulting run, report, or curated snapshot into JSONL evidence.
3. Validate the imported evidence and append it to an append-only ledger.
4. Generate benchmark-specific or cross-benchmark claims from the ledger.

The canonical operator loop for a TerminalBench matrix is:

```bash
# 1) produce raw matrix artifacts
OPENROUTER_API_KEY=sk-or-... \
  scripts/benchmark/run_medium_model_terminalbench_comparison.sh canary

# 2) project raw matrix artifacts into normalized evidence
go run ./cmd/bench evidence import-terminalbench --matrix <matrix-dir> --out benchmark-results/evidence/terminalbench.jsonl

# 3) validate and append to the ledger
go run ./cmd/bench evidence validate benchmark-results/evidence/terminalbench.jsonl
go run ./cmd/bench evidence append --in benchmark-results/evidence/terminalbench.jsonl --ledger benchmark-results/evidence/ledger.jsonl

# 4) emit benchmark claims from the ledger
go run ./cmd/bench fhi delta --ledger benchmark-results/evidence/ledger.jsonl --left <record_id> --right <record_id>
go run ./cmd/bench fhi rank --ledger benchmark-results/evidence/ledger.jsonl
```

This workflow is anchored by [SD-012](../../docs/helix/02-design/solution-designs/SD-012-benchmark-evidence-ledger.md)
and the benchmark resource notes for
[Rapid-MLX MHI](../../docs/resources/rapid-mlx-mhi-2026-05-06.md),
[SkillsBench](../../docs/resources/skillsbench-2026-05-06.md),
[SWE-bench](../../docs/resources/swebench-2026-05-06.md), and
[HumanEval](../../docs/resources/humaneval-2026-05-06.md).

Beadbench enters the same ledger path through `evidence import-beadbench` from
its `report.json` output, and curated external snapshots enter through
`evidence import-external`. For a concrete beadbench source-run example, see
[the OMLX Qwen reasoning sweep note](../../docs/research/beadbench-omlx-qwen-reasoning-sweep-2026-04-24.md).

Use the command family below for the full evidence loop:

- `go run ./cmd/bench evidence import-terminalbench`
- `go run ./cmd/bench evidence import-beadbench`
- `go run ./cmd/bench evidence import-external`
- `go run ./cmd/bench evidence validate`
- `go run ./cmd/bench evidence append`
- `go run ./cmd/bench fhi delta`
- `go run ./cmd/bench fhi rank`

### 1. Run benchmark jobs

Use the TerminalBench runners to produce the raw matrix or report artifacts:

```bash
# Harness comparison matrix
BASELINE=local TIER=wide REPS=1 FORCE_RERUN=1 JOBS=1 \
  scripts/benchmark/run_vidar_qwen36_terminalbench_baseline.sh

# Compact native-vs-fiz comparison wrapper
OPENROUTER_API_KEY=sk-or-... \
  scripts/benchmark/run_medium_model_terminalbench_comparison.sh canary

# Single-run benchmark report
ANTHROPIC_API_KEY=sk-... ./scripts/benchmark/run_benchmark.sh
```

Those scripts write their raw outputs under `benchmark-results/`, which stays
gitignored. Large logs, model outputs, tarballs, and upstream artifacts belong
there, not in git.

### 2. Import benchmark evidence

Project the source artifacts into normalized JSONL evidence with the bench CLI:

```bash
go run ./cmd/bench evidence import-terminalbench \
  --matrix cmd/bench/testdata/terminalbench-matrix \
  --out benchmark-results/evidence/terminalbench.jsonl

go run ./cmd/bench evidence import-beadbench \
  --report cmd/bench/testdata/beadbench-report/report.json \
  --out benchmark-results/evidence/beadbench.jsonl

go run ./cmd/bench evidence import-external \
  --source cmd/bench/testdata/external-benchmarks/rapid-mlx-mhi.md \
  --out benchmark-results/evidence/external.jsonl
```

`import-terminalbench` is the path for Harbor matrix outputs, `import-beadbench`
is the path for beadbench `report.json`, and `import-external` is the path for
curated external benchmark snapshots, including Rapid-MLX MHI, SkillsBench,
SWE-bench, and HumanEval fixtures. Beadbench and other local runs enter the
ledger through their importer paths, not by hand-editing the schema.

Raw benchmark artifacts remain in the gitignored `benchmark-results/` tree.
Only curated snapshots or source hashes that have been normalized into ledger
records should be committed. This repo currently keeps importer fixtures in
`cmd/bench/testdata/external-benchmarks/` and `cmd/bench/testdata/fhi/`.
Approved checked-in curated snapshots, if added later, should live under
`scripts/benchmark/evidence/<snapshot-id>.jsonl`.

### 3. Validate and append to the ledger

```bash
go run ./cmd/bench evidence validate benchmark-results/evidence/terminalbench.jsonl
go run ./cmd/bench evidence append \
  --in benchmark-results/evidence/terminalbench.jsonl \
  --ledger benchmark-results/evidence/ledger.jsonl
```

Append-only ledgers keep the normalized records and their source hashes or
source URLs together. Checked-in curated snapshots, when approved, live under
`scripts/benchmark/evidence/<snapshot-id>.jsonl` and must contain normalized
records only. Raw artifacts stay in `benchmark-results/`; curated snapshots or
source hashes are the committed evidence.

### 4. Generate FHI claims

Use `go run ./cmd/bench fhi delta` for pairwise benchmark claims and
`go run ./cmd/bench fhi rank` for cross-benchmark FHI rankings:

```bash
go run ./cmd/bench fhi delta \
  --ledger benchmark-results/evidence/ledger.jsonl \
  --left terminalbench-delta-left-opus-claude-code \
  --right terminalbench-delta-right-opus-claude-code

go run ./cmd/bench fhi rank \
  --ledger benchmark-results/evidence/ledger.jsonl
```

Pinned axes:

- `fhi delta` compares only two records on the same benchmark axis. Keep the
  benchmark, subset, scorer, formula version, and denominator policy fixed;
  vary only the subject axis you are measuring.
- `fhi rank` compares subjects across the same evidence window and formula.
  Keep the included benchmark set fixed so local-stack rows and frontier rows
  are scored against the same ledger window.

Harness-vs-harness example:

```text
TerminalBench tb2-wide@903487e82ad1998f0c20b721a7df66ec815ea673
model=opus-4.7, provider=anthropic, subset=tb2-wide, scorer=verifier

fiz-native       81.0 ± 2.1
claude-code      81.7 ± 1.8
delta            -0.7
```

Required pins for this claim:

- same model and model snapshot
- same provider surface
- same benchmark version, dataset commit, and subset/version
- same scorer and denominator rule
- only `subject.harness` changes

Local-vs-frontier example:

```text
FHI formula=fhi/v1, evidence window=2026-Q2
benchmarks=terminalbench-wide, beadbench-v1, skillsbench-import

local stack:
  fiz=0.10
  harness=fiz-native
  provider=omlx
  provider_version=0.8.10
  model=Qwen 3.6 27B MLX 8-bit
  quantization=8-bit
  hardware=Mac Studio M3 Ultra 512GB

frontier baseline:
  harness=fiz-native
  provider=anthropic
  model=Opus 4.7

Qwen 3.6 27B 8-bit via oMLX    FHI 50
Opus 4.7 via Anthropic          FHI 56
delta                           -6
```

Required pins for this claim:

- same FHI formula version and evidence window
- same included benchmark set and denominator rule
- same claim generator version
- local rows must include runtime, quantization, hardware, and endpoint facts
- frontier rows must include provider surface and capture timestamp when no
  explicit provider version exists

The command surface is covered by the `cmd/bench` tests:

- `cmd/bench/evidence_test.go`
- `cmd/bench/terminalbench_import_test.go`
- `cmd/bench/beadbench_import_test.go`
- `cmd/bench/external_import_test.go`
- `cmd/bench/fhi_test.go`
- `cmd/bench/main_test.go`

`go run ./cmd/bench <subcommand> --help` exposes the documented flags for the
command families above, so the README commands have matching CLI help even when
there is not a dedicated fixture-driven test for a particular flag combination.

### Claim axes

Harness-vs-harness claims must pin the benchmark axes and vary only the
harness. For a TerminalBench comparison, keep the model, provider, benchmark
version, subset id/version, scorer, evidence window, denominator policy, and
run environment fixed while comparing two harness rows such as `fiz-native`
and `claude-code`.

Example harness-vs-harness claim:

```text
FHI formula=fhi/v1, evidence window=2026-Q2
benchmarks=terminal-bench tb2-wide@<dataset_commit>
model=Opus 4.7
provider=anthropic

fiz-native     81.0 ± 2.1
claude-code    81.7 ± 1.8
delta         -0.7
```

Pinned axes:

- formula version
- evidence window
- benchmark name, version, dataset commit, subset id, subset version, and scorer
- model raw name and canonical model id when known
- provider name and provider surface
- denominator rule and run environment
- only the harness row changes

Local-vs-frontier claims must pin the same formula version, evidence window,
benchmark set, and denominator rules on both sides. The local row also needs the
deployment-class facts that make the environment auditable:

- runtime/server name and version
- model artifact id or checksum when available
- quantization and precision
- hardware class, memory, accelerator backend, and OS/architecture
- endpoint type and provider surface
- context limit and the reasoning/sampling controls actually applied

The frontier row should carry the same benchmark and formula pins, plus the
provider and model snapshot/version captured by the source.

Example local-vs-frontier claim:

```text
FHI formula=fhi/v1, evidence window=2026-Q2
benchmarks=terminal-bench, beadbench-v1, skillsbench-import
dataset_commit=903487e82ad1998f0c20b721a7df66ec815ea673
subset_id=tb2-wide
subset_version=v2
scorer=verifier
denominator_rule=count_valid_tasks

local stack:
  fiz=0.10
  harness=fiz-native
  provider=omlx
  provider_version=0.8.10
  model=Qwen3.6-27B-MLX-8bit
  quantization=8-bit
  hardware=Mac Studio
  runtime=omlx-0.8.10

frontier stack:
  harness=claude-code
  provider=anthropic
  provider_surface=messages
  model=Opus 4.7
  snapshot=anthropic/claude-4.7-20260501
```

---

## Vidar Qwen3.6 Harness Matrix

`run_vidar_qwen36_terminalbench_baseline.sh` runs the shared Vidar oMLX
OpenAI-compatible profile against `fiz`, `pi`, and `opencode` through Harbor.
Use this path when comparing harness capability on the same local model/provider.

Before running, make sure the local model endpoint is reachable from Harbor task
containers as `http://vidar:1235/v1`. The script sets `OMLX_API_KEY=local` by
default because the local OpenAI-compatible endpoint only needs a non-empty key.

The one-shot `./benchmark` entrypoint now builds a cached runtime image for the
task containers. That Docker build installs Node.js plus Claude Code, Codex,
Pi, and OpenCode inside the image, then exports `/installed-agent` as the Harbor
runtime bundle. Host state is limited to the Fiz binary plus data/config
tarballs such as `.claude` and `.codex`; provider binaries are not copied from
the host.

The same entrypoint also rebuilds the selected TerminalBench 2.1 task images
locally from each task's `environment/Dockerfile`. It writes a per-run overlay
task tree under `benchmark-results/.../task-images/`, removes `docker_image`
from that overlay, and lets Harbor build from the task Dockerfiles with Docker
layer caching. This avoids depending on shared local image tags, which Harbor
removes during trial cleanup. It keeps the sweep on the Docker host architecture
by default and fails during preflight if a selected task cannot be built for
that architecture. The downloaded TB-2.1 task tree also defaults to
`benchmark-results/external/terminal-bench-2-1`, not the scripts directory.

Provider sampling compatibility is part of the benchmark contract, not an
operator tweak. Native OpenAI GPT-5-family lanes (`provider.type: openai`, for
example `fiz-openai-gpt-5-5`) use OpenAI's default sampling controls on the
wire; the runner must not forward `FIZEAU_TEMPERATURE` or `FIZEAU_TOP_P`, and
the provider strips those fields if they arrive from config. OpenRouter and
other OpenAI-compatible lanes keep the profile sampling controls, including
`temperature`, `top_p`, `top_k`, `min_p`, and `repetition_penalty` where the
compat provider accepts them. This distinction is covered by `cmd/bench` and
`internal/provider/openai` tests.

Version overrides are build args exposed as environment variables:

- `BENCHMARK_CONTAINER_GOARCH` or `HARBOR_CONTAINER_GOARCH` (defaults to the
  Docker host architecture)
- `HARBOR_NODE_VERSION`
- `HARBOR_CLAUDE_VERSION`
- `HARBOR_CODEX_VERSION`
- `HARBOR_PI_VERSION`
- `HARBOR_OPENCODE_VERSION`
- `BENCHMARK_FORCE_TASK_IMAGE_BUILD=1` to rebuild task images without cache

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

### Medium-model fiz-wrapper comparison

Use the dedicated entrypoint for the medium-cost fiz-wrapper comparison:

```bash
OPENROUTER_API_KEY=sk-or-... \
  scripts/benchmark/run_medium_model_terminalbench_comparison.sh
```

That single command is the official comparison entrypoint. The wrapper sets
`TIER=wide`, `REPS=1`, `JOBS=1`, `FORCE_RERUN=1`, `HARBOR_FORCE_BUILD=1`, and
`HARNESSES=fiz` internally, then runs one Harbor-installed `FizeauAgent`
against the official profile CSV. To do only the 3-task preflight:

```bash
OPENROUTER_API_KEY=sk-or-... \
  scripts/benchmark/run_medium_model_terminalbench_comparison.sh canary
```

Cells included in the official comparison:

- `fiz-harness-claude-sonnet-4-6`
- `fiz-harness-codex-gpt-5-4-mini`
- `fiz-openrouter-claude-sonnet-4-6`
- `fiz-openrouter-gpt-5-4-mini`

To run a cloud-hosted Qwen3.6 comparison lane through the same Fiz native
provider path, use the dedicated OpenRouter Qwen wrapper:

```bash
OPENROUTER_API_KEY=sk-or-... \
  scripts/benchmark/run_openrouter_qwen36_sweep.sh --phase canary

OPENROUTER_API_KEY=sk-or-... \
  scripts/benchmark/run_openrouter_qwen36_sweep.sh
```

The default model is `qwen/qwen3.6-27b` (`fiz-openrouter-qwen3-6-27b`), with
OpenRouter pricing captured in
`scripts/benchmark/profiles/fiz-openrouter-qwen3-6-27b.yaml`. The full run uses
the same 15-task TB-2.1 subset and 3 reps as the other full sweep lanes.

The raw Harbor Claude/Codex/pi/opencode adapters remain diagnostics only. The
official claims use `fiz-*` lanes routed through `FizeauAgent`, not native
Harbor adapter rows. Pi/OpenCode wrapped local-model coverage now lives in the
TerminalBench 2.1 sweep plan.

Artifacts are written under
`benchmark-results/matrix-medium-model-<tier>-<UTC>/`:

- `matrix.json`
- `matrix.md`
- `costs.json`
- per-cell logs under `*/logs/agent/`

Invalid cells are classified as `invalid_quota`, `invalid_auth`,
`invalid_setup`, or `invalid_provider`. They stay visible in `matrix.md` with
their cause and log path, but they are excluded from mean reward and
denominator handling when the matrix is aggregated.

### Bootstrap Subset Selection

`TIER=wide` should not rely on a hand-maintained sense of task difficulty. Use
the selector below to bootstrap a 15-task subset from public
Terminal-Bench-2 leaderboard trial rewards:

```bash
scripts/benchmark/select_terminalbench_subset.py
```

The selector reads reward files from
`harborframework/terminal-bench-2-leaderboard` on Hugging Face, groups
submissions into broad `frontier`, `medium_frontier`, and `non_frontier`
families by Fizeau catalog power, and emits:

- tasks almost everyone passes
- tasks almost nobody passes
- tasks mostly passed by frontier submissions
- tasks mostly passed by medium-frontier submissions
- tasks consistently passed by non-frontier submissions
- probes where the monotonic difficulty assumption appears false

The generated manifest is written to
`scripts/beadbench/external/termbench-subset-external-bootstrap.json`. Review
the monotonicity report before replacing the committed `wide` subset.

Submission directories are interpreted as `Harness__Model` when that delimiter
is present. The `Model` field is matched against
`internal/modelcatalog/catalog/models.yaml` first, using the same broad display
variant behavior as catalog model eligibility. Leaderboard-only models that are
not yet in the catalog are assigned bootstrap powers in
`scripts/benchmark/terminalbench_model_power.json`; exact submission-name
overrides use a `submission:` prefix for rows that do not expose a clean
`Harness__Model` split or represent a multi-model stack.

### Benchmark Evidence

Benchmark runner outputs are benchmark-specific, but long-term model power
should be derived from normalized raw evidence keyed by model, harness,
provider, and benchmark. New importers should project source reports into the
schema at `scripts/benchmark/benchmark-evidence.schema.json`; see
`docs/helix/02-design/solution-designs/SD-012-benchmark-evidence-ledger.md`.
The relevant command surface is `go run ./cmd/bench evidence import-terminalbench`,
`go run ./cmd/bench evidence import-beadbench`, `go run ./cmd/bench evidence
import-external`, `go run ./cmd/bench evidence append`, and `go run ./cmd/bench
fhi delta|rank`.

Curated external snapshots can be imported with:

```bash
go run ./cmd/bench evidence import-external \
  --source cmd/bench/testdata/external-benchmarks/rapid-mlx-mhi.md \
  --out /tmp/rapid-mlx.jsonl
```

The same command accepts SkillsBench CSV, SWE-bench CSV, and HumanEval JSONL
fixtures. Imported rows keep `unknown` harness/provider values explicit instead
of dropping them, and HumanEval rows are flagged as low-cost model-power
evidence rather than primary FHI coverage.

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
`claude-native-sonnet-4-6` (`sonnet-4.6`) and Codex only runs with
`codex-native-gpt-5-4-mini` (`gpt-5.4-mini`). This avoids meaningless
cross-products like Claude Code with the Codex profile.

Prepare linux artifacts before running inside Harbor containers:

```bash
scripts/benchmark/run_medium_model_terminalbench_comparison.sh canary
```

The runner prepares native Claude/Codex harness artifacts automatically. Harbor
Docker jobs are assumed to run on the host architecture by default, so the
runner builds `fiz` for the host `GOARCH`, looks for native CLI artifacts under
`benchmark-results/bin/claude-linux-<goarch>/claude` and
`benchmark-results/bin/codex-linux-<goarch>/codex`, and otherwise uses the
locally installed `claude` / `codex` binaries when they match the container
architecture. If a non-native container platform is forced, set
`HARBOR_NODE_ARCH=x64` or `HARBOR_NODE_ARCH=arm64` and the runner will instead
`npm pack` the matching CLI packages, upload the corresponding Node.js tarball,
and install the CLIs inside the Harbor container. If API key environment
variables are not present, it also packages local `~/.claude` and `~/.codex`
auth/config state into gitignored tarballs under
`benchmark-results/bin/native-homes/` so the container can use the same logged-in
CLI accounts. Set `HARBOR_SKIP_NATIVE_HOME=1` to disable that behavior.

Manual overrides remain available:

```bash
HARBOR_CLAUDE_ARTIFACT=/path/to/claude \
HARBOR_CODEX_ARTIFACT=/path/to/codex \
  scripts/benchmark/run_medium_model_terminalbench_comparison.sh canary
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

To compare `fiz` against the same medium-cost model families through
OpenRouter, run separate `fiz` cells with the official OpenRouter profiles:

```bash
TIER=canary HARNESSES=fiz PROFILE=fiz-openrouter-gpt-5-4-mini REPS=1 FORCE_RERUN=1 JOBS=1 \
  scripts/benchmark/run_vidar_qwen36_terminalbench_baseline.sh

TIER=canary HARNESSES=fiz PROFILE=fiz-openrouter-claude-sonnet-4-6 REPS=1 FORCE_RERUN=1 JOBS=1 \
  scripts/benchmark/run_vidar_qwen36_terminalbench_baseline.sh
```

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
