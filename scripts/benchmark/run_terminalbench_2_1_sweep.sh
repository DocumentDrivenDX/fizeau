#!/usr/bin/env bash
# run_terminalbench_2_1_sweep.sh — convenience wrapper for the TB-2.1 sweep runner.
#
# Usage:
#   ./scripts/benchmark/run_terminalbench_2_1_sweep.sh [flags]
#
# All flags are forwarded to `fiz-bench sweep`. Common flags:
#   --phase canary|local-qwen|sonnet-comparison|gpt-comparison|all
#   --dry-run          Print plan without launching Harbor
#   --out <dir>        Output directory (default: benchmark-results/sweep-<timestamp>)
#   --resume           Skip terminal cells (default: true)
#   --force-rerun      Rerun even terminal cells
#   --tasks-dir <dir>  Path to TB-2.1 tasks directory (enables Harbor grading)
#   --budget-usd <n>   Total sweep budget cap in USD
#   --per-run-budget-usd <n>  Per-run budget cap in USD
#   --matrix-jobs-managed <n> Concurrent cells for managed-provider lanes (default: 1)
#   --sweep-plan <file> Override sweep plan YAML (default: scripts/benchmark/terminalbench-2-1-sweep.yaml)
#
# Prerequisites:
#   - fiz-bench binary on PATH (go install ./cmd/bench or go build -o fiz-bench ./cmd/bench)
#   - OPENROUTER_API_KEY set for managed provider lanes
#   - Local model endpoints reachable for local lanes (vidar:1235, grendel:8000, etc.)
#
# Example — dry-run all phases:
#   ./scripts/benchmark/run_terminalbench_2_1_sweep.sh --dry-run
#
# Example — run canary phase with Harbor grading:
#   ./scripts/benchmark/run_terminalbench_2_1_sweep.sh \
#     --phase canary \
#     --tasks-dir /path/to/terminal-bench-2-1 \
#     --per-run-budget-usd 5.0
#
# Example — resume local-qwen phase from existing output:
#   ./scripts/benchmark/run_terminalbench_2_1_sweep.sh \
#     --phase local-qwen \
#     --out benchmark-results/sweep-20260507T120000Z \
#     --resume
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

if ! command -v fiz-bench >/dev/null 2>&1; then
    echo "error: fiz-bench not found on PATH" >&2
    echo "  Build it with: go build -o fiz-bench ./cmd/bench" >&2
    exit 1
fi

exec fiz-bench sweep \
    --work-dir "${REPO_ROOT}" \
    --sweep-plan "${REPO_ROOT}/scripts/benchmark/terminalbench-2-1-sweep.yaml" \
    "$@"
