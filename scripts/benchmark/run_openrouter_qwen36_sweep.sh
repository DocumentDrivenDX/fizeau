#!/usr/bin/env bash
# Dedicated TB-2.1 sweep for Fiz native/provider + Qwen3.6 27B via OpenRouter.
#
# Canary:
#   scripts/benchmark/run_openrouter_qwen36_sweep.sh --phase canary
#
# Full 15-task subset, 3 reps:
#   scripts/benchmark/run_openrouter_qwen36_sweep.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
cd "${REPO_ROOT}"

PHASE="full"
OUT=""
JOBS="${QWEN36_OPENROUTER_JOBS:-10}"
BUDGET_USD="${QWEN36_OPENROUTER_BUDGET_USD:-10}"
PER_RUN_BUDGET_USD="${QWEN36_OPENROUTER_PER_RUN_BUDGET_USD:-1}"
FORCE_RERUN=0
DRY_RUN=0
PREPARE_ONLY=0
CONFIRM_DELAY="${BENCHMARK_CONFIRM_DELAY:-8}"

PROFILE_ID="fiz-openrouter-qwen3-6-27b"
PROVIDER_TYPE="openrouter"
MODEL_ID="qwen/qwen3.6-27b"
BASE_URL="https://openrouter.ai/api/v1"
API_KEY_ENV="OPENROUTER_API_KEY"
RESOURCE_GROUP="rg-openrouter-qwen36-27b"

usage() {
  cat <<'EOF'
Usage: scripts/benchmark/run_openrouter_qwen36_sweep.sh [flags]

Flags:
  --phase canary|full       Run the 3-task canary or 15-task full subset (default: full)
  --out <dir>               Output directory
  --jobs <n>                Concurrent TerminalBench cells for this lane (default: 10)
  --budget-usd <n>          Total matrix budget cap (default: 10)
  --per-run-budget-usd <n>  Per-cell budget cap (default: 1)
  --force-rerun             Rerun cells even if reports already exist
  --dry-run                 Build/prepare and print plan only
  --prepare-only            Same as --dry-run after runtime/task preparation

Environment:
  OPENROUTER_API_KEY                       Required
  QWEN36_OPENROUTER_JOBS                   Default --jobs override
  QWEN36_OPENROUTER_BUDGET_USD             Default --budget-usd override
  QWEN36_OPENROUTER_PER_RUN_BUDGET_USD     Default --per-run-budget-usd override
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --phase)
      PHASE="$2"; shift 2 ;;
    --phase=*)
      PHASE="${1#*=}"; shift ;;
    --out)
      OUT="$2"; shift 2 ;;
    --out=*)
      OUT="${1#*=}"; shift ;;
    --jobs)
      JOBS="$2"; shift 2 ;;
    --jobs=*)
      JOBS="${1#*=}"; shift ;;
    --budget-usd)
      BUDGET_USD="$2"; shift 2 ;;
    --budget-usd=*)
      BUDGET_USD="${1#*=}"; shift ;;
    --per-run-budget-usd)
      PER_RUN_BUDGET_USD="$2"; shift 2 ;;
    --per-run-budget-usd=*)
      PER_RUN_BUDGET_USD="${1#*=}"; shift ;;
    --force-rerun)
      FORCE_RERUN=1; shift ;;
    --dry-run)
      DRY_RUN=1; shift ;;
    --prepare-only)
      PREPARE_ONLY=1; shift ;;
    -h|--help)
      usage; exit 0 ;;
    *)
      echo "unknown flag: $1" >&2
      usage >&2
      exit 2 ;;
  esac
done

case "${PHASE}" in
  canary)
    BENCHMARK_PHASE="canary"
    ESTIMATE_CELLS=9
    ESTIMATE_LOW="0.20"
    ESTIMATE_HIGH="1.00"
    ;;
  full)
    BENCHMARK_PHASE="gpt-comparison"
    ESTIMATE_CELLS=45
    ESTIMATE_LOW="1.00"
    ESTIMATE_HIGH="5.00"
    ;;
  *)
    echo "unknown --phase ${PHASE}; use canary or full" >&2
    exit 2
    ;;
esac

if [[ -z "${OPENROUTER_API_KEY:-}" ]]; then
  echo "OPENROUTER_API_KEY is required" >&2
  exit 1
fi
if [[ "${OPENROUTER_API_KEY}" != sk-or-v1* ]]; then
  echo "warning: OPENROUTER_API_KEY does not look like an OpenRouter key (sk-or-v1...)" >&2
fi

if [[ -z "${OUT}" ]]; then
  OUT="${REPO_ROOT}/benchmark-results/qwen36-openrouter-${PHASE}-$(date -u +%Y%m%dT%H%M%SZ)"
elif [[ "${OUT}" != /* ]]; then
  OUT="${REPO_ROOT}/${OUT}"
fi
mkdir -p "${OUT}"

PLAN="${OUT}/qwen36-openrouter-sweep.yaml"
cat > "${PLAN}" <<EOF
spec-id: terminalbench-2.1-qwen36-openrouter-$(date -u +%Y%m%d)
created: "$(date -u +%Y-%m-%d)"
dataset: terminal-bench/terminal-bench-2-1

defaults:
  reps: 3
  resume: true

phases:
  - id: canary
    description: Fiz native/provider Qwen3.6 27B OpenRouter canary.
    reps: 3
    subset: terminalbench-2-1-canary
    lanes:
      - ${PROFILE_ID}

  - id: gpt-comparison
    description: Fiz native/provider Qwen3.6 27B OpenRouter full subset.
    reps: 3
    subset: terminalbench-2-1-full
    lanes:
      - ${PROFILE_ID}

comparison_groups:
  - id: cg-qwen36-openrouter
    type: openrouter_provider
    question: How does Fiz native/provider perform on TB-2.1 with Qwen3.6 27B via OpenRouter?
    lanes:
      - ${PROFILE_ID}

resource_groups:
  - id: ${RESOURCE_GROUP}
    base_url: "${BASE_URL}"
    provider_type: openrouter
    max_concurrency: ${JOBS}
    budget:
      per_run_usd_cap: ${PER_RUN_BUDGET_USD}
      per_phase_usd_cap: ${BUDGET_USD}

lanes:
  - id: ${PROFILE_ID}
    profile_id: ${PROFILE_ID}
    lane_type: fiz_provider_native
    phases: [canary, gpt-comparison]
    comparison_groups: [cg-qwen36-openrouter]
    resource_group: ${RESOURCE_GROUP}
    fizeau_env:
      FIZEAU_PROVIDER: ${PROVIDER_TYPE}
      FIZEAU_MODEL: "${MODEL_ID}"
      FIZEAU_BASE_URL: "${BASE_URL}"
      FIZEAU_API_KEY_ENV: ${API_KEY_ENV}
      FIZEAU_REASONING: low
    model_family: qwen3-6-27b
    model_id: "${MODEL_ID}"
    quant_label: cloud-hosted
    provider_surface: openrouter
    runtime: fiz-native-provider
    sampling:
      temperature: 0.6
      reasoning: low
      top_p: 0.95
      top_k: 20
EOF

echo "OpenRouter Qwen3.6 benchmark"
echo "  model:              ${MODEL_ID}"
echo "  phase:              ${PHASE} (${BENCHMARK_PHASE})"
echo "  output:             ${OUT}"
echo "  generated plan:     ${PLAN}"
echo "  jobs:               ${JOBS}"
printf '  budget cap:         $%s\n' "${BUDGET_USD}"
printf '  per-run cap:        $%s\n' "${PER_RUN_BUDGET_USD}"
echo "  cells:              ${ESTIMATE_CELLS}"
printf '  rough expected cost: $%s-$%s\n' "${ESTIMATE_LOW}" "${ESTIMATE_HIGH}"
echo

args=(
  --phase "${BENCHMARK_PHASE}"
  --out "${OUT}"
  --sweep-plan "${PLAN}"
  --matrix-jobs-managed "${JOBS}"
  --budget-usd "${BUDGET_USD}"
  --per-run-budget-usd "${PER_RUN_BUDGET_USD}"
)
if [[ "${FORCE_RERUN}" = "1" ]]; then
  args+=(--force-rerun)
fi
if [[ "${DRY_RUN}" = "1" ]]; then
  args+=(--dry-run)
fi
if [[ "${PREPARE_ONLY}" = "1" ]]; then
  args+=(--prepare-only)
fi

BENCHMARK_CONFIRM_DELAY="${CONFIRM_DELAY}" exec "${REPO_ROOT}/benchmark" "${args[@]}"
