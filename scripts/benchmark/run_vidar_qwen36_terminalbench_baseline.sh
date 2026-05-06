#!/usr/bin/env bash
set -euo pipefail

# Compare pi, opencode, and fiz on Vidar oMLX Qwen3.6-27B over TerminalBench.
#
# Defaults run the evidence-grade 89-task TB-2 subset:
#   scripts/benchmark/run_vidar_qwen36_terminalbench_baseline.sh
#
# Faster tiers:
#   TIER=canary scripts/benchmark/run_vidar_qwen36_terminalbench_baseline.sh
#   TIER=core   scripts/benchmark/run_vidar_qwen36_terminalbench_baseline.sh
#
# Useful overrides:
#   REPS=1 FORCE_RERUN=1 OUT=benchmark-results/matrix-my-run ...

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "${ROOT}"

TIER="${TIER:-full}"
REPS="${REPS:-3}"
JOBS="${JOBS:-1}"
PROFILE="${PROFILE:-vidar-qwen3-6-27b-openai-compat}"
HARNESSES="${HARNESSES:-pi,opencode,fiz}"
OUT="${OUT:-benchmark-results/matrix-vidar-qwen36-${TIER}-$(date -u +%Y%m%dT%H%M%SZ)}"
BENCH="${BENCH:-go run ./cmd/bench}"
TASKS_DIR="${TASKS_DIR:-scripts/benchmark/external/terminal-bench-2}"
FIZ_ARTIFACT="${FIZ_ARTIFACT:-benchmark-results/bin/fiz-linux-amd64}"

case "${TIER}" in
  canary)
    SUBSET="${SUBSET:-scripts/beadbench/external/termbench-subset-canary.json}"
    ;;
  core)
    SUBSET="${SUBSET:-scripts/beadbench/external/termbench-subset.json}"
    ;;
  full)
    SUBSET="${SUBSET:-scripts/beadbench/external/termbench-full.json}"
    ;;
  *)
    echo "unknown TIER=${TIER}; use canary, core, or full" >&2
    exit 2
    ;;
esac

need() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "missing required command: $1" >&2
    exit 1
  }
}

need go
need harbor

if [[ ! -d "${TASKS_DIR}" ]]; then
  echo "missing TerminalBench tasks dir: ${TASKS_DIR}" >&2
  echo "run: git submodule update --init ${TASKS_DIR}" >&2
  exit 1
fi

if [[ ! -f "${SUBSET}" ]]; then
  echo "missing subset manifest: ${SUBSET}" >&2
  exit 1
fi

if [[ "${JOBS}" != "1" ]]; then
  echo "warning: JOBS=${JOBS}; JOBS=1 is recommended for a fair single Vidar oMLX baseline" >&2
fi

contains_harness() {
  local needle="$1"
  case ",${HARNESSES}," in
    *",${needle},"*) return 0 ;;
    *) return 1 ;;
  esac
}

require_file() {
  local path="$1"
  local help="$2"
  if [[ ! -f "${path}" ]]; then
    echo "missing required file: ${path}" >&2
    echo "${help}" >&2
    exit 1
  fi
}

if contains_harness "opencode" && [[ -z "${HARBOR_OPENCODE_ARTIFACT:-}" ]]; then
  require_file \
    "benchmark-results/bin/opencode-1.3.17-linux-x64/opencode" \
    "set HARBOR_OPENCODE_ARTIFACT or prepare the pinned opencode artifact; see scripts/benchmark/README.md"
fi

if contains_harness "pi"; then
  if [[ -z "${HARBOR_NODE_TARBALL:-}" ]]; then
    require_file \
      "benchmark-results/bin/node-v20.19.2-linux-x64.tar.gz" \
      "set HARBOR_NODE_TARBALL or prepare the pinned Node artifact; see scripts/benchmark/README.md"
  fi
  if [[ -z "${HARBOR_PI_PACKAGE_TARBALL:-}" ]]; then
    require_file \
      "benchmark-results/bin/pi-coding-agent-0.67.1/package.tgz" \
      "set HARBOR_PI_PACKAGE_TARBALL or prepare the pinned pi package artifact; see scripts/benchmark/README.md"
  fi
fi

# Some OpenAI-compatible clients require a non-empty key even for local oMLX.
export OMLX_API_KEY="${OMLX_API_KEY:-local}"

mkdir -p "$(dirname "${FIZ_ARTIFACT}")"
GOOS="${GOOS:-linux}" GOARCH="${GOARCH:-amd64}" go build -o "${FIZ_ARTIFACT}" ./cmd/fiz
export HARBOR_AGENT_ARTIFACT="${HARBOR_AGENT_ARTIFACT:-${ROOT}/${FIZ_ARTIFACT}}"

matrix_args=(
  matrix
  --profiles="${PROFILE}"
  --harnesses="${HARNESSES}"
  --reps="${REPS}"
  --subset="${SUBSET}"
  --tasks-dir="${TASKS_DIR}"
  --out="${OUT}"
  --jobs="${JOBS}"
)

if [[ "${FORCE_RERUN:-0}" == "1" ]]; then
  matrix_args+=(--force-rerun)
else
  matrix_args+=(--resume)
fi

if [[ -n "${PER_RUN_BUDGET_USD:-}" ]]; then
  matrix_args+=(--per-run-budget-usd="${PER_RUN_BUDGET_USD}")
fi

if [[ -n "${BUDGET_USD:-}" ]]; then
  matrix_args+=(--budget-usd="${BUDGET_USD}")
fi

echo "Running Vidar Qwen3.6 TerminalBench matrix"
echo "  tier:      ${TIER}"
echo "  subset:    ${SUBSET}"
echo "  profile:   ${PROFILE}"
echo "  harnesses: ${HARNESSES}"
echo "  reps:      ${REPS}"
echo "  jobs:      ${JOBS}"
echo "  out:       ${OUT}"
echo

${BENCH} "${matrix_args[@]}"
${BENCH} matrix-aggregate "${OUT}"

echo
echo "Wrote:"
echo "  ${OUT}/matrix.json"
echo "  ${OUT}/matrix.md"
echo "  ${OUT}/costs.json"
