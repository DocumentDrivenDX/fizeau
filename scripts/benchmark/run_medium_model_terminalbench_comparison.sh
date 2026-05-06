#!/usr/bin/env bash
set -euo pipefail

# Run the medium-cost native-vs-fiz TerminalBench comparison.
#
# Defaults:
#   - tier: wide (15 tasks)
#   - reps: 1
#   - jobs: 1
#   - output: benchmark-results/matrix-medium-model-<tier>-<UTC>
#
# Usage:
#   scripts/benchmark/run_medium_model_terminalbench_comparison.sh
#   scripts/benchmark/run_medium_model_terminalbench_comparison.sh canary
#   scripts/benchmark/run_medium_model_terminalbench_comparison.sh wide

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "${ROOT}"

TIER="${1:-wide}"
case "${TIER}" in
  canary|wide) ;;
  *)
    echo "usage: $0 [canary|wide]" >&2
    exit 2
    ;;
esac

if [[ -z "${OPENROUTER_API_KEY:-}" ]]; then
  echo "missing OPENROUTER_API_KEY; required for fiz OpenRouter comparison cells" >&2
  exit 1
fi

OUT="benchmark-results/matrix-medium-model-${TIER}-$(date -u +%Y%m%dT%H%M%SZ)"
RUNNER="scripts/benchmark/run_vidar_qwen36_terminalbench_baseline.sh"

common_env=(
  "TIER=${TIER}"
  "REPS=1"
  "JOBS=1"
  "FORCE_RERUN=1"
  "OUT=${OUT}"
)

echo "Running medium-cost TerminalBench comparison"
echo "  tier: ${TIER}"
echo "  out:  ${OUT}"
echo

echo "1/3 Native Claude/Codex reference cells"
env \
  "${common_env[@]}" \
  BASELINE=frontier \
  HARNESSES=claude,codex \
  "${RUNNER}"

echo
echo "2/3 fiz on GPT-5.4 Mini via OpenRouter"
env \
  "${common_env[@]}" \
  BASELINE=local \
  HARNESSES=fiz \
  PROFILE=gpt-5-4-mini-openrouter \
  "${RUNNER}"

echo
echo "3/3 fiz on Claude Sonnet 4.6 via OpenRouter"
env \
  "${common_env[@]}" \
  BASELINE=local \
  HARNESSES=fiz \
  PROFILE=claude-sonnet-4-6 \
  "${RUNNER}"

echo
echo "Comparison complete:"
echo "  ${OUT}/matrix.json"
echo "  ${OUT}/matrix.md"
echo "  ${OUT}/costs.json"
