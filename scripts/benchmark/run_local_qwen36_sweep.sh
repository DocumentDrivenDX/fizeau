#!/usr/bin/env bash
# One-shot local Qwen3.6 sweep wrapper.
#
# Defaults to a real full TB-2.1 run for Sindri and Vidar:
#   scripts/benchmark/run_local_qwen36_sweep.sh
#
# Preferred 15-task subset:
#   scripts/benchmark/run_local_qwen36_sweep.sh --preferred
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
cd "${REPO_ROOT}"

PHASE="full"
LANES="sindri,vidar"
OUT=""
MATRIX_JOBS_MANAGED="${LOCAL_QWEN36_JOBS:-2}"
EXTRA_ARGS=()

usage() {
  cat <<'EOF'
Usage: scripts/benchmark/run_local_qwen36_sweep.sh [flags]

Flags:
  --phase canary|preferred|full
  --canary                 Alias for --phase canary
  --preferred              Alias for --phase preferred
  --full                   Alias for --phase full
  --lanes <names>          Comma-separated: sindri,vidar,bragi,pi,opencode or full lane IDs
                           Default: sindri,vidar
  --out <dir>              Output directory
  --jobs <n>               Matrix jobs managed value (default: 2)
  --dry-run
  --prepare-only
  --force-rerun

Examples:
  scripts/benchmark/run_local_qwen36_sweep.sh
  scripts/benchmark/run_local_qwen36_sweep.sh --preferred
  scripts/benchmark/run_local_qwen36_sweep.sh --phase full --lanes sindri,vidar
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --phase)
      PHASE="$2"; shift 2 ;;
    --phase=*)
      PHASE="${1#*=}"; shift ;;
    --canary)
      PHASE="canary"; shift ;;
    --preferred)
      PHASE="preferred"; shift ;;
    --full)
      PHASE="full"; shift ;;
    --lanes)
      LANES="$2"; shift 2 ;;
    --lanes=*)
      LANES="${1#*=}"; shift ;;
    --out)
      OUT="$2"; shift 2 ;;
    --out=*)
      OUT="${1#*=}"; shift ;;
    --jobs)
      MATRIX_JOBS_MANAGED="$2"; shift 2 ;;
    --jobs=*)
      MATRIX_JOBS_MANAGED="${1#*=}"; shift ;;
    --dry-run|--prepare-only|--force-rerun)
      EXTRA_ARGS+=("$1"); shift ;;
    -h|--help)
      usage; exit 0 ;;
    *)
      echo "unknown flag: $1" >&2
      usage >&2
      exit 2 ;;
  esac
done

map_lane() {
  case "$1" in
    sindri) echo "fiz-sindri-club-3090-qwen3-6-27b" ;;
    vidar) echo "fiz-vidar-omlx-qwen3-6-27b" ;;
    bragi) echo "fiz-bragi-club-3090-qwen3-6-27b" ;;
    pi) echo "fiz-harness-pi-vidar-qwen3-6-27b" ;;
    opencode) echo "fiz-harness-opencode-vidar-qwen3-6-27b" ;;
    *) echo "$1" ;;
  esac
}

IFS=',' read -r -a lane_parts <<< "${LANES}"
resolved_lanes=()
for lane in "${lane_parts[@]}"; do
  lane="${lane//[[:space:]]/}"
  [[ -n "${lane}" ]] || continue
  resolved_lanes+=("$(map_lane "${lane}")")
done
if [[ "${#resolved_lanes[@]}" = "0" ]]; then
  echo "--lanes did not resolve to any lane IDs" >&2
  exit 2
fi
LANES="$(IFS=,; echo "${resolved_lanes[*]}")"

if [[ -z "${OUT}" ]]; then
  OUT="${REPO_ROOT}/benchmark-results/local-qwen36-${PHASE}-$(date -u +%Y%m%dT%H%M%SZ)"
elif [[ "${OUT}" != /* ]]; then
  OUT="${REPO_ROOT}/${OUT}"
fi

exec "${REPO_ROOT}/benchmark" \
  --phase "${PHASE}" \
  --lanes "${LANES}" \
  --out "${OUT}" \
  --matrix-jobs-managed "${MATRIX_JOBS_MANAGED}" \
  "${EXTRA_ARGS[@]}"
