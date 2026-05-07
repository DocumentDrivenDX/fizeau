#!/usr/bin/env bash
set -euo pipefail

# Run the medium-cost fiz-wrapper TerminalBench comparison.
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
OFFICIAL_PROFILES=(
  fiz-harness-claude-sonnet-4-6
  fiz-harness-codex-gpt-5-4-mini
  fiz-harness-pi-gpt-5-4-mini
  fiz-harness-opencode-gpt-5-4-mini
  fiz-openrouter-claude-sonnet-4-6
  fiz-openrouter-gpt-5-4-mini
)
OFFICIAL_PROFILES_CSV="$(IFS=,; echo "${OFFICIAL_PROFILES[*]}")"

common_env=(
  "TIER=${TIER}"
  "REPS=1"
  "JOBS=1"
  "FORCE_RERUN=1"
  "HARBOR_FORCE_BUILD=${HARBOR_FORCE_BUILD:-1}"
  "OUT=${OUT}"
)

echo "Running medium-cost TerminalBench comparison"
echo "  tier: ${TIER}"
echo "  out:  ${OUT}"
echo

echo "1/1 fiz wrapper lanes via Harbor"
env \
  "${common_env[@]}" \
  BASELINE=local \
  HARNESSES=fiz \
  PROFILE="${OFFICIAL_PROFILES_CSV}" \
  "${RUNNER}"

echo
echo "Comparison complete:"
echo "  ${OUT}/matrix.json"
echo "  ${OUT}/matrix.md"
echo "  ${OUT}/costs.json"
