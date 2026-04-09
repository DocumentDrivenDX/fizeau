# DDX Agent Benchmark Scripts

This directory contains scripts and config for running ddx-agent under
Terminal-Bench/Harbor and capturing reproducible benchmark baselines.

---

## Prerequisites

```bash
# Python packages
pip install harbor-framework

# Terminal-Bench dataset
harbor dataset pull terminal-bench/terminal-bench-2

# Docker (local runtime)
docker info  # must succeed
```

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
- ddx-agent exits 0
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

The fixed 15-task benchmark subset is defined in `task-subset-v1.yaml`.
**Do not modify task IDs without updating the version field and filing a bead.**

Subset version policy: see SD-009 §3.

---

## Harbor Adapter

`harbor_agent.py` is the `BaseInstalledAgent` Python adapter that Harbor uses
to install and run ddx-agent inside each task container. It handles:

1. **`install()`** — copies the `linux/amd64` binary and writes a provider config
2. **`run()`** — invokes `ddx-agent --json --preset benchmark -p "<task>"`
3. **`populate_context_post_run()`** — converts session JSONL to ATIF v1.4 trajectory

**Build the linux/amd64 binary before running**:

```bash
GOOS=linux GOARCH=amd64 go build -o scripts/benchmark/ddx-agent-linux-amd64 ./cmd/ddx-agent
```

### Credential injection

The adapter passes `ANTHROPIC_API_KEY` and `OPENROUTER_API_KEY` from the host
into the container via `get_env()`. A minimal config file is written during
`install()` that references `${ANTHROPIC_API_KEY}` — ddx-agent's config loader
expands env vars at load time.

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
