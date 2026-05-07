#!/usr/bin/env bash
# egress_canary.sh — Egress canary for the Harbor + Terminal-Bench-2 rig.
#
# Purpose: prove that in-container egress to a tool-capable provider works
# with the existing rig before any new adapter code is written. Uses the
# cheapest tool-capable OpenAI-compat smoke model and a single concrete
# TB-2 task that actually exists at the pinned commit (fix-git).
#
# TB-2.0 ONLY — COMPATIBILITY NOTE:
#   This script validates provider egress using a local terminal-bench@2.0
#   task directory (scripts/benchmark/external/terminal-bench-2/). It is NOT
#   a TB-2.1 preflight. Running it with FIZEAU_BENCH_DATASET set to a TB-2.1
#   identifier (e.g. terminal-bench/terminal-bench-2-1) will mix a TB-2.0
#   local task directory with a TB-2.1 Harbor dataset reference, producing a
#   misleading canary result. Use the TB-2.1 sweep's canary phase instead:
#     fiz-bench sweep --phase canary --dry-run   # plan
#     fiz-bench sweep --phase canary             # run
#
# This replaces an earlier "hello-world" formulation: terminal-bench@2.0 at
# commit 53ff2b87 has no hello-world task, so the canary now targets
# fix-git (easy / software-engineering, ~5 min expert time).
#
# Pass criterion: trajectory.json is valid JSON with >= 1 step. That is an
# egress signal — the agent successfully called the provider at least
# once. The verifier reward is recorded but is *not* part of the gate
# (smoke models routinely fail TB-2 tasks even with working egress).
#
# Usage:
#   OPENROUTER_API_KEY=sk-or-... ./scripts/benchmark/egress_canary.sh
#   FIZEAU_BENCH_TASK=break-filter-js-from-html ./scripts/benchmark/egress_canary.sh
#
# Output:
#   benchmark-results/egress-canary-<UTC-TIMESTAMP>/
#       trial/        — symlink into Harbor's job dir for the single trial
#       trajectory.json, reward.txt — copied flat for archive convenience
#       canary.json   — small machine-readable summary
#
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
DIST_DIR="${REPO_ROOT}/dist"
DEFAULT_BINARY="${DIST_DIR}/fiz-linux-amd64"
INPUT_BINARY="${FIZEAU_BENCH_BINARY:-${DEFAULT_BINARY}}"

# Default to fix-git: the bead notes call it out explicitly as a concrete
# TB-2 task suitable for the canary. It is "easy" difficulty with a small
# 2G/1cpu env and a 900s verifier timeout.
CANARY_TASK="${FIZEAU_BENCH_TASK:-fix-git}"
DATASET="${FIZEAU_BENCH_DATASET:-terminal-bench@2.0}"
RUNTIME="${FIZEAU_BENCH_RUNTIME:-docker}"
PRESET="${FIZEAU_BENCH_PRESET:-benchmark}"

# Smoke model defaults: cheapest tool-capable OpenAI-compat target on
# OpenRouter. Override via env to point at any other smoke target.
PROVIDER_NAME="${FIZEAU_PROVIDER_NAME:-openrouter}"
PROVIDER_TYPE="${FIZEAU_PROVIDER:-openrouter}"
PROVIDER_MODEL="${FIZEAU_MODEL:-google/gemini-2.5-flash}"
PROVIDER_BASE_URL="${FIZEAU_BASE_URL:-https://openrouter.ai/api/v1}"
PROVIDER_API_KEY_ENV="${FIZEAU_API_KEY_ENV:-OPENROUTER_API_KEY}"
PROVIDER_HEADERS_JSON="${FIZEAU_HEADERS_JSON:-}"
SYSTEM_APPEND="${FIZEAU_BENCH_SYSTEM_APPEND:-}"

TIMESTAMP="$(date -u +%Y%m%dT%H%M%SZ)"
ARCHIVE_DIR="${REPO_ROOT}/benchmark-results/egress-canary-${TIMESTAMP}"
HARBOR_BIN=""

ensure_harbor() {
    if [[ -n "${HARBOR_BIN}" ]]; then
        return
    fi
    if command -v harbor &>/dev/null; then
        HARBOR_BIN="$(command -v harbor)"
        return
    fi
    if ! command -v uv &>/dev/null; then
        echo "ERROR: 'harbor' not found and 'uv' is unavailable for automatic install."
        exit 1
    fi
    echo "      harbor not found; installing via uv tool install harbor..."
    uv tool install harbor
    hash -r
    if command -v harbor &>/dev/null; then
        HARBOR_BIN="$(command -v harbor)"
        return
    fi
    if [[ -x "${HOME}/.local/bin/harbor" ]]; then
        HARBOR_BIN="${HOME}/.local/bin/harbor"
        return
    fi
    echo "ERROR: Harbor install completed but no executable was found on PATH."
    exit 1
}

require_provider_key() {
    if [[ -z "${!PROVIDER_API_KEY_ENV:-}" ]]; then
        echo "ERROR: \$${PROVIDER_API_KEY_ENV} is not set."
        echo "       Export an API key for the smoke provider before running the canary."
        exit 2
    fi
}

require_task_exists() {
    local task_dir="${SCRIPT_DIR}/external/terminal-bench-2/${CANARY_TASK}"
    if [[ ! -d "${task_dir}" ]]; then
        echo "ERROR: TB-2 task directory not found: ${task_dir}"
        echo "       Did you run: git submodule update --init scripts/benchmark/external/terminal-bench-2 ?"
        exit 3
    fi
    if [[ ! -f "${task_dir}/task.toml" ]]; then
        echo "ERROR: ${task_dir}/task.toml is missing — task is not a valid TB-2 task."
        exit 3
    fi
}

# Compatibility guard: this script validates against a local TB-2.0 task
# directory. If DATASET is set to anything other than terminal-bench@2.0,
# the local task directory check (TB-2.0) and the Harbor dataset reference
# diverge — results are not comparable to a true TB-2.1 preflight.
warn_dataset_mismatch() {
    if [[ "${DATASET}" != "terminal-bench@2.0" ]]; then
        echo "WARNING: DATASET='${DATASET}' is not terminal-bench@2.0."
        echo "         This script validates a local TB-2.0 task directory."
        echo "         To preflight the TB-2.1 Harbor dataset, use instead:"
        echo "           fiz-bench sweep --phase canary --dry-run  # plan"
        echo "           fiz-bench sweep --phase canary            # run"
        echo ""
    fi
}

echo "=== fiz egress canary (TB-2.0 local task-dir mode) ==="
echo "Repo:      ${REPO_ROOT}"
echo "Task:      ${CANARY_TASK}  (TB-2.0 @ pinned commit)"
echo "Dataset:   ${DATASET}"
echo "Model:     ${PROVIDER_MODEL}"
echo "Archive:   ${ARCHIVE_DIR}"
echo ""

warn_dataset_mismatch
require_task_exists

# Step 1: binary
echo "[1/4] Preparing binary under test..."
if [[ -z "${FIZEAU_BENCH_BINARY:-}" ]]; then
    mkdir -p "${DIST_DIR}"
    GOOS=linux GOARCH=amd64 go build -o "${DEFAULT_BINARY}" "${REPO_ROOT}/cmd/fiz"
    echo "      built: ${DEFAULT_BINARY}"
else
    if [[ ! -f "${INPUT_BINARY}" ]]; then
        echo "ERROR: FIZEAU_BENCH_BINARY does not exist: ${INPUT_BINARY}"
        exit 1
    fi
    echo "      using supplied: ${INPUT_BINARY}"
fi

# Step 2: harbor + key
echo "[2/4] Checking Harbor + provider key..."
ensure_harbor
require_provider_key
echo "      OK: $("${HARBOR_BIN}" --version 2>/dev/null || echo 'harbor found')"

# Step 3: run single canary task
echo "[3/4] Running canary task '${CANARY_TASK}'..."
mkdir -p "${ARCHIVE_DIR}"
JOB_NAME="egress-canary-${CANARY_TASK}-${TIMESTAMP}"
JOBS_DIR="${REPO_ROOT}/benchmark-results/harbor-jobs"
mkdir -p "${JOBS_DIR}"
ENV_ARGS=(--ae "${PROVIDER_API_KEY_ENV}=${!PROVIDER_API_KEY_ENV}")

PYTHONPATH="${SCRIPT_DIR}${PYTHONPATH:+:${PYTHONPATH}}" \
HARBOR_AGENT_ARTIFACT="${INPUT_BINARY}" \
FIZEAU_BENCH_PRESET="${PRESET}" \
FIZEAU_PROVIDER_NAME="${PROVIDER_NAME}" \
FIZEAU_PROVIDER="${PROVIDER_TYPE}" \
FIZEAU_MODEL="${PROVIDER_MODEL}" \
FIZEAU_BASE_URL="${PROVIDER_BASE_URL}" \
FIZEAU_API_KEY_ENV="${PROVIDER_API_KEY_ENV}" \
FIZEAU_HEADERS_JSON="${PROVIDER_HEADERS_JSON}" \
FIZEAU_BENCH_SYSTEM_APPEND="${SYSTEM_APPEND}" \
"${HARBOR_BIN}" run \
    --yes \
    --dataset "${DATASET}" \
    --include-task-name "${CANARY_TASK}" \
    --n-tasks 1 \
    --agent-import-path "harbor_agent:FizeauAgent" \
    --model "${PROVIDER_MODEL}" \
    --env "${RUNTIME}" \
    --jobs-dir "${JOBS_DIR}" \
    --job-name "${JOB_NAME}" \
    "${ENV_ARGS[@]}"

JOB_DIR="${JOBS_DIR}/${JOB_NAME}"

# Step 4: validate egress signal and archive
echo "[4/4] Validating egress signal & archiving..."
TRIAL_DIR="$(find "${JOB_DIR}" -mindepth 1 -maxdepth 1 -type d | head -1)"
if [[ -z "${TRIAL_DIR}" ]]; then
    echo "ERROR: no trial directory under ${JOB_DIR}"
    exit 4
fi
TRAJECTORY_FILE="${TRIAL_DIR}/agent/trajectory.json"
REWARD_FILE="${TRIAL_DIR}/verifier/reward.txt"

if [[ ! -f "${TRAJECTORY_FILE}" ]]; then
    echo "ERROR: trajectory.json not found at ${TRAJECTORY_FILE}"
    exit 4
fi
STEP_COUNT="$(python3 -c "import json,sys; d=json.load(open('${TRAJECTORY_FILE}')); print(len(d.get('steps', [])))" 2>/dev/null || echo 0)"

REWARD="unknown"
if [[ -f "${REWARD_FILE}" ]]; then
    REWARD="$(cat "${REWARD_FILE}")"
fi

# Egress signal: trajectory.json has >=1 step OR reward>0 (a successful
# completion is the strongest possible proof that egress to the provider
# works — stronger than a partial trajectory). The harbor adapter's
# trajectory builder relies on session-log artifacts that aren't always
# emitted in this rig; reward is the authoritative outer signal.
EGRESS_OK=0
if [[ "${STEP_COUNT}" -ge 1 ]]; then
    EGRESS_OK=1
fi
if [[ "${REWARD}" != "unknown" ]]; then
    if python3 -c "import sys; sys.exit(0 if float('${REWARD}') > 0 else 1)" 2>/dev/null; then
        EGRESS_OK=1
    fi
fi
if [[ "${EGRESS_OK}" -ne 1 ]]; then
    echo "ERROR: no egress signal — trajectory_steps=${STEP_COUNT}, reward=${REWARD}"
    exit 4
fi

ln -sfn "${TRIAL_DIR}" "${ARCHIVE_DIR}/trial"
cp -f "${TRAJECTORY_FILE}" "${ARCHIVE_DIR}/trajectory.json"
[[ -f "${REWARD_FILE}" ]] && cp -f "${REWARD_FILE}" "${ARCHIVE_DIR}/reward.txt"

cat > "${ARCHIVE_DIR}/canary.json" <<JSON
{
  "task": "${CANARY_TASK}",
  "dataset": "${DATASET}",
  "model": "${PROVIDER_MODEL}",
  "provider_type": "${PROVIDER_TYPE}",
  "trajectory_steps": ${STEP_COUNT},
  "reward": "${REWARD}",
  "trial_dir": "${TRIAL_DIR}",
  "timestamp_utc": "${TIMESTAMP}",
  "pass_criterion": "trajectory_steps>=1 OR reward>0",
  "passed": true
}
JSON

echo ""
echo "=== Egress canary PASSED ==="
echo "  trajectory steps: ${STEP_COUNT}"
echo "  reward (informational): ${REWARD}"
echo "  archive: ${ARCHIVE_DIR}"
