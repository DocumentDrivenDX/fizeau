#!/usr/bin/env bash
# smoke_run.sh — Run a single Terminal-Bench task to validate the ddx-agent adapter.
# See docs/helix/02-design/solution-designs/SD-009-benchmark-mode.md §4 for the full
# smoke-run workflow and passing criteria.
#
# Usage:
#   ANTHROPIC_API_KEY=sk-... ./scripts/benchmark/smoke_run.sh
#   OPENROUTER_API_KEY=sk-or-... ./scripts/benchmark/smoke_run.sh
#
# Prerequisites:
#   pip install harbor-framework
#   harbor dataset pull terminal-bench/terminal-bench-2
#   Docker running locally
#
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
DIST_DIR="${REPO_ROOT}/dist"
BINARY="${DIST_DIR}/ddx-agent-linux-amd64"
SMOKE_TASK="tb2-read-and-describe"

echo "=== ddx-agent benchmark smoke run ==="
echo "Repo: ${REPO_ROOT}"
echo "Task: ${SMOKE_TASK}"
echo ""

# Step 1: Build linux/amd64 binary
echo "[1/4] Building linux/amd64 binary..."
mkdir -p "${DIST_DIR}"
GOOS=linux GOARCH=amd64 go build -o "${BINARY}" "${REPO_ROOT}/cmd/ddx-agent"
echo "      Built: ${BINARY}"

# Step 2: Validate Harbor is installed
echo "[2/4] Checking Harbor installation..."
if ! command -v harbor &>/dev/null; then
    echo "ERROR: 'harbor' command not found. Install with: pip install harbor-framework"
    exit 1
fi
echo "      OK: $(harbor --version 2>/dev/null || echo 'harbor found')"

# Step 3: Run single smoke task
echo "[3/4] Running smoke task '${SMOKE_TASK}'..."
JOB_OUTPUT=$(harbor run \
    --dataset terminal-bench/terminal-bench-2 \
    --agent ddx-agent \
    --task-id "${SMOKE_TASK}" \
    --runtime docker \
    --agent-config "${SCRIPT_DIR}/harbor_agent.py" \
    2>&1)
echo "${JOB_OUTPUT}"

# Extract job ID from output
JOB_ID=$(echo "${JOB_OUTPUT}" | grep -oP '(?<=job-id: )[a-z0-9-]+' || echo "")
if [[ -z "${JOB_ID}" ]]; then
    echo "WARNING: Could not extract job ID from output; checking latest job directory"
    JOB_DIR=$(ls -td ~/.harbor/jobs/*/ 2>/dev/null | head -1)
else
    JOB_DIR="${HOME}/.harbor/jobs/${JOB_ID}"
fi

# Step 4: Validate results
echo "[4/4] Validating results..."
TRIAL_DIR=$(ls -d "${JOB_DIR}/trials/"*/ 2>/dev/null | head -1)
if [[ -z "${TRIAL_DIR}" ]]; then
    echo "ERROR: No trial directory found under ${JOB_DIR}"
    exit 1
fi

REWARD_FILE="${TRIAL_DIR}/verifier/reward.txt"
TRAJECTORY_FILE="${TRIAL_DIR}/agent/trajectory.json"

# Check reward file exists
if [[ ! -f "${REWARD_FILE}" ]]; then
    echo "ERROR: reward.txt not found at ${REWARD_FILE}"
    exit 1
fi
REWARD=$(cat "${REWARD_FILE}")
echo "      reward.txt = ${REWARD}"

# Check trajectory is valid JSON with at least 1 step
if [[ ! -f "${TRAJECTORY_FILE}" ]]; then
    echo "ERROR: trajectory.json not found at ${TRAJECTORY_FILE}"
    exit 1
fi
STEP_COUNT=$(python3 -c "import json; d=json.load(open('${TRAJECTORY_FILE}')); print(len(d.get('steps', [])))" 2>/dev/null || echo "0")
if [[ "${STEP_COUNT}" -lt 1 ]]; then
    echo "ERROR: trajectory.json has ${STEP_COUNT} steps (expected >= 1)"
    exit 1
fi
echo "      trajectory.json: ${STEP_COUNT} steps"

echo ""
echo "=== Smoke run PASSED ==="
echo "  Harness: ddx-agent exited cleanly, trajectory produced, reward captured"
echo "  Task result: reward=${REWARD} (1=pass, 0=fail; both valid for smoke)"
echo "  Trial dir: ${TRIAL_DIR}"
