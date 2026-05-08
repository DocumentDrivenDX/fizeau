#!/usr/bin/env bash
# Dedicated TB-2.1 sweep for Fiz native/provider + GPT-5.5.
#
# Defaults to direct OpenAI with high parallelism:
#   scripts/benchmark/run_gpt55_sweep.sh
#
# OpenRouter variant:
#   scripts/benchmark/run_gpt55_sweep.sh --provider openrouter
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
cd "${REPO_ROOT}"

PROVIDER="${GPT55_PROVIDER:-openai}"
PHASE="full"
OUT=""
JOBS="${GPT55_MATRIX_JOBS:-16}"
BUDGET_USD="${GPT55_BUDGET_USD:-250}"
PER_RUN_BUDGET_USD="${GPT55_PER_RUN_BUDGET_USD:-8}"
FORCE_RERUN=0
DRY_RUN=0
PREPARE_ONLY=0
CONFIRM_DELAY="${BENCHMARK_CONFIRM_DELAY:-8}"

usage() {
  cat <<'EOF'
Usage: scripts/benchmark/run_gpt55_sweep.sh [flags]

Flags:
  --provider openai|openrouter
  --phase canary|preferred|full
                            Run canary, 15-task preferred subset, or all 89 tasks (default: full)
  --preferred               Alias for --phase preferred
  --full                    Alias for --phase full
  --out <dir>               Output directory
  --jobs <n>                Concurrent TerminalBench cells for this lane (default: 16)
  --budget-usd <n>          Total matrix budget cap (default: 250)
  --per-run-budget-usd <n>  Per-cell budget cap (default: 8)
  --force-rerun             Rerun cells even if reports already exist
  --dry-run                 Build/prepare and print plan only
  --prepare-only            Same as --dry-run after runtime/task preparation

Environment:
  OPENAI_API_KEY            Required for --provider openai
  OPENROUTER_API_KEY        Required for --provider openrouter
  GPT55_PROVIDER            Default --provider override
  GPT55_MATRIX_JOBS         Default --jobs override
  GPT55_BUDGET_USD          Default --budget-usd override
  GPT55_PER_RUN_BUDGET_USD  Default --per-run-budget-usd override
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --provider)
      PROVIDER="$2"; shift 2 ;;
    --provider=*)
      PROVIDER="${1#*=}"; shift ;;
    --phase)
      PHASE="$2"; shift 2 ;;
    --phase=*)
      PHASE="${1#*=}"; shift ;;
    --preferred)
      PHASE="preferred"; shift ;;
    --full)
      PHASE="full"; shift ;;
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

case "${PROVIDER}" in
  openai)
    PROFILE_ID="fiz-openai-gpt-5-5"
    PROVIDER_TYPE="openai"
    MODEL_ID="gpt-5.5"
    BASE_URL="https://api.openai.com/v1"
    API_KEY_ENV="OPENAI_API_KEY"
    RESOURCE_GROUP="rg-openai-gpt55"
    PROVIDER_SURFACE="openai"
    REASONING_ENV=""
    REASONING_SAMPLING=""
    ;;
  openrouter)
    PROFILE_ID="fiz-openrouter-gpt-5-5"
    PROVIDER_TYPE="openrouter"
    MODEL_ID="openai/gpt-5.5"
    BASE_URL="https://openrouter.ai/api/v1"
    API_KEY_ENV="OPENROUTER_API_KEY"
    RESOURCE_GROUP="rg-openrouter-gpt55"
    PROVIDER_SURFACE="openrouter"
    REASONING_ENV="      FIZEAU_REASONING: medium"
    REASONING_SAMPLING="      reasoning: medium"
    ;;
  *)
    echo "unknown --provider ${PROVIDER}; use openai or openrouter" >&2
    exit 2
    ;;
esac

case "${PHASE}" in
  canary)
    BENCHMARK_PHASE="canary"
    ESTIMATE_CELLS=9
    ESTIMATE_LOW="2"
    ESTIMATE_HIGH="8"
    ;;
  full)
    BENCHMARK_PHASE="tb21-all"
    ESTIMATE_CELLS=267
    ESTIMATE_LOW="60"
    ESTIMATE_HIGH="240"
    ;;
  preferred)
    BENCHMARK_PHASE="gpt-comparison"
    ESTIMATE_CELLS=45
    ESTIMATE_LOW="10"
    ESTIMATE_HIGH="40"
    ;;
  *)
    echo "unknown --phase ${PHASE}; use canary, preferred, or full" >&2
    exit 2
    ;;
esac

if [[ -z "${!API_KEY_ENV:-}" ]]; then
  echo "${API_KEY_ENV} is required for --provider ${PROVIDER}" >&2
  exit 1
fi
if [[ "${PROVIDER}" = "openai" && "${!API_KEY_ENV}" == sk-or-v1* ]]; then
  echo "OPENAI_API_KEY contains an OpenRouter key (sk-or-v1...). Use a native OpenAI API key for --provider openai." >&2
  exit 1
fi
if [[ "${PROVIDER}" = "openrouter" && "${!API_KEY_ENV}" != sk-or-v1* ]]; then
  echo "warning: OPENROUTER_API_KEY does not look like an OpenRouter key (sk-or-v1...)" >&2
fi

if [[ -z "${OUT}" ]]; then
  OUT="${REPO_ROOT}/benchmark-results/gpt55-${PROVIDER}-${PHASE}-$(date -u +%Y%m%dT%H%M%SZ)"
elif [[ "${OUT}" != /* ]]; then
  OUT="${REPO_ROOT}/${OUT}"
fi
mkdir -p "${OUT}"

PLAN="${OUT}/gpt55-${PROVIDER}-sweep.yaml"
cat > "${PLAN}" <<EOF
spec-id: terminalbench-2.1-gpt55-${PROVIDER}-$(date -u +%Y%m%d)
created: "$(date -u +%Y-%m-%d)"
dataset: terminal-bench/terminal-bench-2-1

defaults:
  reps: 3
  resume: true

phases:
  - id: canary
    description: Fiz native/provider GPT-5.5 ${PROVIDER} canary.
    reps: 3
    subset: terminalbench-2-1-canary
    lanes:
      - ${PROFILE_ID}

  - id: gpt-comparison
    description: Fiz native/provider GPT-5.5 ${PROVIDER} preferred 15-task subset.
    reps: 3
    subset: terminalbench-2-1-full
    lanes:
      - ${PROFILE_ID}

  - id: tb21-all
    description: Fiz native/provider GPT-5.5 ${PROVIDER} full 89-task suite.
    reps: 3
    subset: terminalbench-2-1-all
    lanes:
      - ${PROFILE_ID}

comparison_groups:
  - id: cg-gpt55-${PROVIDER}
    type: frontier_provider
    question: How does Fiz native/provider perform on TB-2.1 with GPT-5.5 via ${PROVIDER}?
    lanes:
      - ${PROFILE_ID}

resource_groups:
  - id: ${RESOURCE_GROUP}
    base_url: "${BASE_URL}"
    provider_type: ${PROVIDER_TYPE}
    max_concurrency: ${JOBS}
    budget:
      per_run_usd_cap: ${PER_RUN_BUDGET_USD}
      per_phase_usd_cap: ${BUDGET_USD}

lanes:
  - id: ${PROFILE_ID}
    profile_id: ${PROFILE_ID}
    lane_type: fiz_provider_native
    phases: [canary, gpt-comparison]
    comparison_groups: [cg-gpt55-${PROVIDER}]
    resource_group: ${RESOURCE_GROUP}
    fizeau_env:
      FIZEAU_PROVIDER: ${PROVIDER_TYPE}
      FIZEAU_MODEL: "${MODEL_ID}"
      FIZEAU_BASE_URL: "${BASE_URL}"
      FIZEAU_API_KEY_ENV: ${API_KEY_ENV}
${REASONING_ENV}
    model_family: gpt-5
    model_id: "${MODEL_ID}"
    quant_label: cloud-hosted
    provider_surface: ${PROVIDER_SURFACE}
    runtime: fiz-native-provider
    sampling:
      temperature: 0.0
${REASONING_SAMPLING}
EOF

echo "GPT-5.5 benchmark"
echo "  provider:           ${PROVIDER}"
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
