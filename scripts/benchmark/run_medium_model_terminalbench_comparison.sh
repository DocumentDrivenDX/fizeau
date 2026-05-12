#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

usage() {
  cat <<'EOF'
Usage: scripts/benchmark/run_medium_model_terminalbench_comparison.sh [canary|wide] [--dry-run]

Run the official medium-model TerminalBench fiz-wrapper comparison through the
existing benchmark sweep machinery.

Modes:
  canary  Run the smaller medium-model canary phase.
  wide    Run the full medium-model comparison phase. This is the default.

Flags:
  --dry-run  Print the resolved sweep plan without launching a benchmark run.

The comparison lanes are:
  fiz-harness-claude-sonnet-4-6
  fiz-harness-codex-gpt-5-4-mini
  fiz-harness-pi-gpt-5-4-mini
  fiz-harness-opencode-gpt-5-4-mini
  fiz-openrouter-claude-sonnet-4-6
  fiz-openrouter-gpt-5-4-mini
EOF
}

TIER="wide"
DRY_RUN=0
while [[ $# -gt 0 ]]; do
  case "$1" in
    canary|wide)
      TIER="$1"
      shift
      ;;
    --dry-run)
      DRY_RUN=1
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "unknown flag: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

if [[ "${DRY_RUN}" -eq 0 && -z "${OPENROUTER_API_KEY:-}" ]]; then
  echo "missing OPENROUTER_API_KEY; required for the fiz OpenRouter lanes" >&2
  exit 1
fi

case "${TIER}" in
  canary) PHASE="medium-model-canary" ;;
  wide) PHASE="medium-model" ;;
esac

export BENCHMARK_CONFIRM_DELAY="${BENCHMARK_CONFIRM_DELAY:-0}"
args=(--phase "${PHASE}")
if [[ "${DRY_RUN}" -eq 1 ]]; then
  args+=(--dry-run)
fi
exec "${REPO_ROOT}/benchmark" "${args[@]}"
