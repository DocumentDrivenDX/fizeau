#!/usr/bin/env bash
# run_benchmark.sh — Run the full fiz benchmark subset and emit a report.
#
# Usage:
#   ANTHROPIC_API_KEY=sk-... ./scripts/benchmark/run_benchmark.sh
#   OPENROUTER_API_KEY=sk-or-... ./scripts/benchmark/run_benchmark.sh
#   FIZEAU_BENCH_BINARY=/path/to/fiz-linux-amd64 ./scripts/benchmark/run_benchmark.sh
#
# Output:
#   benchmark-results/report-<TIMESTAMP>.json
#
# The report includes:
#   - actual agent SHA/version and binary identity
#   - provider/model/preset/runtime config snapshot
#   - per-task pass/fail/timeout outcomes and artifact paths
#   - aggregate resolved-task rate summary
#
# Prerequisites:
#   pip install harbor-framework
#   harbor dataset pull terminal-bench/terminal-bench-2
#   Docker running locally
#
# See scripts/benchmark/README.md and SD-009 §3–§7 for full documentation.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
DIST_DIR="${REPO_ROOT}/dist"
DEFAULT_BINARY="${DIST_DIR}/fiz-linux-amd64"
INPUT_BINARY="${FIZEAU_BENCH_BINARY:-${DEFAULT_BINARY}}"
SUBSET_FILE="${FIZEAU_BENCH_SUBSET_FILE:-${SCRIPT_DIR}/task-subset-v2.yaml}"
RESULTS_DIR="${FIZEAU_BENCH_RESULTS_DIR:-${REPO_ROOT}/benchmark-results}"
TIMESTAMP="$(date -u +%Y%m%dT%H%M%SZ)"
REPORT_FILE="${RESULTS_DIR}/report-${TIMESTAMP}.json"
DATASET="${FIZEAU_BENCH_DATASET:-terminal-bench@2.0}"
RUNTIME="${FIZEAU_BENCH_RUNTIME:-docker}"
PRESET="${FIZEAU_BENCH_PRESET:-default}"
PROVIDER_NAME="${FIZEAU_PROVIDER_NAME:-benchmark}"
PROVIDER_TYPE="${FIZEAU_PROVIDER:-anthropic}"
PROVIDER_MODEL="${FIZEAU_MODEL:-qwen/qwen3.6-plus}"
PROVIDER_BASE_URL="${FIZEAU_BASE_URL:-}"
PROVIDER_API_KEY_ENV="${FIZEAU_API_KEY_ENV:-ANTHROPIC_API_KEY}"
PROVIDER_HEADERS_JSON="${FIZEAU_HEADERS_JSON:-}"
PROVIDER_REASONING="${FIZEAU_REASONING:-}"
SYSTEM_APPEND="${FIZEAU_BENCH_SYSTEM_APPEND:-}"
TIMEOUT_MULTIPLIER="${FIZEAU_BENCH_TIMEOUT_MULTIPLIER:-1.0}"
DRY_RUN="${FIZEAU_BENCH_DRY_RUN:-0}"
SHA_OVERRIDE="${FIZEAU_BENCH_SHA:-}"
HARBOR_BIN=""

BUNDLE_DIR="$(mktemp -d /tmp/ddx-bench-agent-XXXXXX)"
STAGED_BINARY="${BUNDLE_DIR}/fiz-linux-amd64"
STAGED_AGENT_CONFIG="${BUNDLE_DIR}/harbor_agent.py"
TASK_RESULTS_FILE="$(mktemp /tmp/ddx-bench-tasks-XXXXXX.jsonl)"
SCORES_FILE="$(mktemp /tmp/ddx-bench-scores-XXXXXX.json)"
cleanup() {
    rm -f "${TASK_RESULTS_FILE}"
    rm -f "${SCORES_FILE}"
    rm -rf "${BUNDLE_DIR}"
}
trap cleanup EXIT

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

    echo "ERROR: Harbor install completed but no executable was found on PATH or at ${HOME}/.local/bin/harbor."
    exit 1
}

resolve_provider_api_key() {
    if [[ -n "${!PROVIDER_API_KEY_ENV:-}" ]]; then
        return
    fi

    local config_path=""
    local config_key=""
    local fallback_provider=""
    local -a config_candidates=(
        "${REPO_ROOT}/.fizeau/config.yaml"
        "${HOME}/.config/fizeau/config.yaml"
    )

    if [[ "${PROVIDER_NAME}" == "openrouter" || "${PROVIDER_BASE_URL}" == *"openrouter.ai"* ]]; then
        fallback_provider="openrouter"
    fi

    for config_path in "${config_candidates[@]}"; do
        [[ -f "${config_path}" ]] || continue
        for candidate_provider in "${PROVIDER_NAME}" "${fallback_provider}"; do
            [[ -n "${candidate_provider}" ]] || continue
            config_key="$(awk -v provider="${candidate_provider}" '
                BEGIN { in_providers = 0; current = "" }
                /^[[:space:]]*providers:[[:space:]]*$/ { in_providers = 1; next }
                in_providers && /^[^[:space:]]/ { in_providers = 0; current = "" }
                !in_providers { next }
                /^  [^:#]+:[[:space:]]*$/ {
                    current = $0
                    sub(/^  /, "", current)
                    sub(/:[[:space:]]*$/, "", current)
                    next
                }
                current == provider && /^    api_key:[[:space:]]*/ {
                    value = $0
                    sub(/^    api_key:[[:space:]]*/, "", value)
                    gsub(/^[[:space:]]+|[[:space:]]+$/, "", value)
                    if (value ~ /^".*"$/ || value ~ /^'\''.*'\''$/) {
                        value = substr(value, 2, length(value) - 2)
                    }
                    print value
                    exit
                }
            ' "${config_path}")"
            if [[ "${config_key}" =~ ^\$\{([A-Za-z_][A-Za-z0-9_]*)\}$ ]]; then
                config_key="${!BASH_REMATCH[1]:-}"
            fi
            if [[ -n "${config_key}" ]]; then
                export "${PROVIDER_API_KEY_ENV}=${config_key}"
                echo "      sourced ${PROVIDER_API_KEY_ENV} from ${config_path}"
                return
            fi
        done
    done
}

echo "=== fiz benchmark run ==="
echo "Repo:    ${REPO_ROOT}"
echo "Binary:  ${INPUT_BINARY}"
echo "Subset:  ${SUBSET_FILE}"
echo "Report:  ${REPORT_FILE}"
echo ""

# ---------------------------------------------------------------------------- #
# Step 1: Prepare the binary-under-test bundle
# ---------------------------------------------------------------------------- #
echo "[1/5] Preparing benchmark harness bundle..."
if [[ -z "${FIZEAU_BENCH_BINARY:-}" ]]; then
    mkdir -p "${DIST_DIR}"
    GOOS=linux GOARCH=amd64 go build \
        -ldflags "-X main.GitCommit=$(git -C "${REPO_ROOT}" rev-parse --short HEAD 2>/dev/null || echo dev)" \
        -o "${DEFAULT_BINARY}" "${REPO_ROOT}/cmd/fiz"
    echo "      Built current checkout binary: ${DEFAULT_BINARY}"
else
    if [[ ! -f "${INPUT_BINARY}" ]]; then
        echo "ERROR: FIZEAU_BENCH_BINARY does not exist: ${INPUT_BINARY}"
        exit 1
    fi
    echo "      Using supplied binary: ${INPUT_BINARY}"
fi

cp "${SCRIPT_DIR}/harbor_agent.py" "${STAGED_AGENT_CONFIG}"
cp "${INPUT_BINARY}" "${STAGED_BINARY}"
chmod 755 "${STAGED_BINARY}"
echo "      Staged bundle: ${BUNDLE_DIR}"

# ---------------------------------------------------------------------------- #
# Step 2: Validate prerequisites
# ---------------------------------------------------------------------------- #
echo "[2/5] Checking prerequisites..."
if ! command -v python3 &>/dev/null; then
    echo "ERROR: 'python3' not found."
    exit 1
fi
if [[ "${DRY_RUN}" != "1" ]]; then
    ensure_harbor
    resolve_provider_api_key
    echo "      harbor: $("${HARBOR_BIN}" --version 2>/dev/null || echo 'found')"
else
    echo "      dry-run enabled; skipping harbor availability check"
fi

# ---------------------------------------------------------------------------- #
# Step 3: Extract task IDs from subset YAML
# ---------------------------------------------------------------------------- #
echo "[3/5] Parsing task subset..."
TASK_IDS="$(awk '/^[[:space:]]*- id:[[:space:]]*/ { print $3 }' "${SUBSET_FILE}")"
TASK_COUNT=$(echo "${TASK_IDS}" | wc -l | tr -d ' ')
SUBSET_VERSION="$(awk -F': *' '$1 == "version" { gsub(/"/, "", $2); print $2; exit }' "${SUBSET_FILE}")"
if [[ -z "${SUBSET_VERSION}" ]]; then
    SUBSET_VERSION="unknown"
fi
echo "      Subset v${SUBSET_VERSION}: ${TASK_COUNT} tasks"

mkdir -p "${RESULTS_DIR}"

if command -v shasum &>/dev/null; then
    BINARY_SHA256="$(shasum -a 256 "${INPUT_BINARY}" | awk '{print $1}')"
elif command -v sha256sum &>/dev/null; then
    BINARY_SHA256="$(sha256sum "${INPUT_BINARY}" | awk '{print $1}')"
else
    BINARY_SHA256="unknown"
fi

if [[ -n "${SHA_OVERRIDE}" ]]; then
    GIT_SHA="${SHA_OVERRIDE}"
elif [[ -z "${FIZEAU_BENCH_BINARY:-}" ]]; then
    GIT_SHA="$(git -C "${REPO_ROOT}" rev-parse HEAD 2>/dev/null || echo unknown)"
else
    GIT_SHA="unknown"
fi
GIT_SHA_SHORT="$(printf '%s' "${GIT_SHA}" | cut -c1-12)"
BINARY_VERSION="$("${INPUT_BINARY}" --version 2>/dev/null | head -1 || echo unknown)"
HARNESS_REPO_SHA="$(git -C "${REPO_ROOT}" rev-parse HEAD 2>/dev/null || echo unknown)"

# ---------------------------------------------------------------------------- #
# Step 4: Run each task via Harbor
# ---------------------------------------------------------------------------- #
echo "[4/5] Running tasks..."

if [[ "${DRY_RUN}" == "1" ]]; then
    cat <<EOF
DRY RUN
  staged_agent_config=${STAGED_AGENT_CONFIG}
  staged_binary=${STAGED_BINARY}
  harness_repo_sha=${HARNESS_REPO_SHA}
  agent_git_sha=${GIT_SHA}
  agent_version=${BINARY_VERSION}
  dataset=${DATASET}
  runtime=${RUNTIME}
  preset=${PRESET}
  provider_name=${PROVIDER_NAME}
  provider_type=${PROVIDER_TYPE}
  provider_model=${PROVIDER_MODEL}
  provider_api_key_env=${PROVIDER_API_KEY_ENV}
  provider_base_url=${PROVIDER_BASE_URL}
  provider_headers_json=${PROVIDER_HEADERS_JSON}
  provider_reasoning=${PROVIDER_REASONING}
  agent_timeout_multiplier=${TIMEOUT_MULTIPLIER}
  subset_version=${SUBSET_VERSION}
  subset_file=${SUBSET_FILE}
  binary_sha256=${BINARY_SHA256}
EOF
    exit 0
fi

PASS=0
FAIL=0
TIMEOUT=0
ERROR=0
HARBOR_JOBS_DIR="${RESULTS_DIR}/harbor-jobs"
mkdir -p "${HARBOR_JOBS_DIR}"

while IFS= read -r TASK_ID; do
    [[ -z "${TASK_ID}" ]] && continue
    echo "  → ${TASK_ID}"

    TASK_START="$(date -u +%s%3N)"
    TASK_JOB_NAME="${TASK_ID}-${TIMESTAMP}"
    TASK_JOB_DIR="${HARBOR_JOBS_DIR}/${TASK_JOB_NAME}"
    JOB_ID=""
    ENV_ARGS=()
    if [[ -n "${!PROVIDER_API_KEY_ENV:-}" ]]; then
        ENV_ARGS+=(--ae "${PROVIDER_API_KEY_ENV}=${!PROVIDER_API_KEY_ENV}")
    fi
    HARBOR_OUT="$( \
        PYTHONPATH="${BUNDLE_DIR}${PYTHONPATH:+:${PYTHONPATH}}" \
        HARBOR_AGENT_ARTIFACT="${STAGED_BINARY}" \
        FIZEAU_BENCH_PRESET="${PRESET}" \
        FIZEAU_PROVIDER_NAME="${PROVIDER_NAME}" \
        FIZEAU_PROVIDER="${PROVIDER_TYPE}" \
        FIZEAU_MODEL="${PROVIDER_MODEL}" \
        FIZEAU_BASE_URL="${PROVIDER_BASE_URL}" \
        FIZEAU_API_KEY_ENV="${PROVIDER_API_KEY_ENV}" \
        FIZEAU_HEADERS_JSON="${PROVIDER_HEADERS_JSON}" \
        FIZEAU_REASONING="${PROVIDER_REASONING}" \
        FIZEAU_BENCH_SYSTEM_APPEND="${SYSTEM_APPEND}" \
        "${HARBOR_BIN}" run \
        --yes \
        --dataset "${DATASET}" \
        --include-task-name "${TASK_ID}" \
        --n-tasks 1 \
        --agent-import-path "harbor_agent:DDXAgent" \
        --model "${PROVIDER_MODEL}" \
        --env "${RUNTIME}" \
        --jobs-dir "${HARBOR_JOBS_DIR}" \
        --job-name "${TASK_JOB_NAME}" \
        --agent-timeout-multiplier "${TIMEOUT_MULTIPLIER}" \
        "${ENV_ARGS[@]}" \
        2>&1)" || true
    TASK_END="$(date -u +%s%3N)"
    TASK_DURATION_MS=$(( TASK_END - TASK_START ))

    REWARD=""
    STATUS="error"
    JOB_DIR="${TASK_JOB_DIR}"
    TRIAL_DIR=""
    REWARD_FILE=""
    TRAJECTORY_FILE=""
    if [[ -d "${JOB_DIR}" ]]; then
        TRIAL_DIR="$(find "${JOB_DIR}" -mindepth 1 -maxdepth 1 -type d | head -1 || echo "")"
        if [[ -n "${TRIAL_DIR}" ]]; then
            REWARD_FILE="${TRIAL_DIR}/verifier/reward.txt"
            TRAJECTORY_FILE="${TRIAL_DIR}/agent/trajectory.json"
            if [[ -f "${REWARD_FILE}" ]]; then
                REWARD="$(tr -d '[:space:]' < "${REWARD_FILE}")"
                if [[ "${REWARD}" == "1" ]]; then
                    STATUS="pass"
                    PASS=$(( PASS + 1 ))
                elif [[ "${REWARD}" == "0" ]]; then
                    STATUS="fail"
                    FAIL=$(( FAIL + 1 ))
                else
                    STATUS="unknown"
                    ERROR=$(( ERROR + 1 ))
                fi
            else
                if echo "${HARBOR_OUT}" | grep -qi "timeout"; then
                    STATUS="timeout"
                    TIMEOUT=$(( TIMEOUT + 1 ))
                else
                    STATUS="error"
                    ERROR=$(( ERROR + 1 ))
                fi
            fi
        else
            STATUS="error"
            ERROR=$(( ERROR + 1 ))
        fi
    else
        STATUS="error"
        ERROR=$(( ERROR + 1 ))
    fi

    echo "    outcome=${STATUS} reward=${REWARD} duration=${TASK_DURATION_MS}ms"

    python3 -c "
import json, sys
print(json.dumps({
    'task_id': sys.argv[1],
    'outcome': sys.argv[2],
    'reward': sys.argv[3] if sys.argv[3] else None,
    'duration_ms': int(sys.argv[4]),
    'job_id': sys.argv[5] if sys.argv[5] else None,
    'job_dir': sys.argv[6] if sys.argv[6] else None,
    'trial_dir': sys.argv[7] if sys.argv[7] else None,
    'reward_file': sys.argv[8] if sys.argv[8] else None,
    'trajectory_file': sys.argv[9] if sys.argv[9] else None,
}))" \
        "${TASK_ID}" "${STATUS}" "${REWARD}" "${TASK_DURATION_MS}" "${JOB_ID}" "${JOB_DIR}" "${TRIAL_DIR}" "${REWARD_FILE}" "${TRAJECTORY_FILE}" \
        >> "${TASK_RESULTS_FILE}"
done <<< "${TASK_IDS}"

# ---------------------------------------------------------------------------- #
# Step 5: Assemble and write the report
# ---------------------------------------------------------------------------- #
echo "[5/5] Writing report..."

go run ./cmd/benchscore -tasks-jsonl "${TASK_RESULTS_FILE}" > "${SCORES_FILE}"

python3 - <<PY
import json
from pathlib import Path

tasks_raw = Path("${TASK_RESULTS_FILE}").read_text().strip()
tasks = [json.loads(line) for line in tasks_raw.splitlines() if line.strip()]
scores = json.loads(Path("${SCORES_FILE}").read_text())

total = len(tasks)
passed = sum(1 for t in tasks if t["outcome"] == "pass")
failed = sum(1 for t in tasks if t["outcome"] == "fail")
timed_out = sum(1 for t in tasks if t["outcome"] == "timeout")
errored = sum(1 for t in tasks if t["outcome"] in ("error", "unknown"))
resolved_rate = round(passed / total, 4) if total > 0 else 0.0

report = {
    "schema_version": "2",
    "captured": "${TIMESTAMP}",
    "harness_repo_sha": "${HARNESS_REPO_SHA}",
    "agent_version": "${BINARY_VERSION}",
    "agent_git_sha": "${GIT_SHA}",
    "agent_git_sha_short": "${GIT_SHA_SHORT}",
    "agent_binary_path": "${INPUT_BINARY}",
    "agent_binary_sha256": "${BINARY_SHA256}",
    "subset_version": "${SUBSET_VERSION}",
    "subset_file": "${SUBSET_FILE}",
    "dataset": "${DATASET}",
    "config": {
        "preset": "${PRESET}",
        "system_append": "${SYSTEM_APPEND}",
        "runtime": "${RUNTIME}",
        "provider_name": "${PROVIDER_NAME}",
        "provider_type": "${PROVIDER_TYPE}",
        "provider_model": "${PROVIDER_MODEL}",
        "provider_base_url": "${PROVIDER_BASE_URL}",
        "provider_api_key_env": "${PROVIDER_API_KEY_ENV}",
        "provider_headers_json": "${PROVIDER_HEADERS_JSON}",
        "provider_reasoning": "${PROVIDER_REASONING}",
        "agent_timeout_multiplier": ${TIMEOUT_MULTIPLIER},
    },
    "summary": {
        "total_tasks": total,
        "passed": passed,
        "failed": failed,
        "timed_out": timed_out,
        "errored": errored,
        "resolved_task_rate": resolved_rate,
        "clarification_question_rate": scores["summary"]["clarification_question_rate"],
        "shell_anti_pattern_rate": scores["summary"]["shell_anti_pattern_rate"],
        "structured_edit_success_rate": scores["summary"]["structured_edit_success_rate"],
    },
    "thresholds": {
        "resolved_task_rate_floor": 0.55,
        "resolved_task_rate_target": 0.70,
    },
    "threshold_check": {
        "resolved_task_rate_passes_floor": resolved_rate >= 0.55,
    },
    "scoring": scores,
    "tasks": tasks,
}

out_path = "${REPORT_FILE}"
Path(out_path).write_text(json.dumps(report, indent=2))
print(f"  Report: {out_path}")
print(f"  Resolved-task rate: {passed}/{total} = {resolved_rate:.1%}")
if resolved_rate >= 0.55:
    print("  [PASS] Meets regression floor (≥ 55%)")
else:
    print("  [FAIL] Below regression floor (< 55%)")
PY

echo ""
echo "=== Benchmark run complete ==="
echo "  Report: ${REPORT_FILE}"
echo ""
echo "To compare with another run:"
echo "  jq '{rate: .summary.resolved_task_rate, tasks: [.tasks[] | {id: .task_id, outcome}]}' \\"
echo "    benchmark-results/report-*.json"
