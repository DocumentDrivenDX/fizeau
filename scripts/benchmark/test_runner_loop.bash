#!/usr/bin/env bash
# Integration test for the ./benchmark run path (bead fizeau-a2fd070b).
#
# Exercises all five ACs of the bead description:
#
#  1. ./benchmark --profile sindri-lucebox --bench-set tb-2-1-canary produces
#     cells with report.json embedding profile, command, env_redacted +
#     fiz.txt, fiz.err, session/ artifacts.
#  2. <out>/sweep.json captures task_executor_version +
#     harbor_runner_image_digest; each cell's report.json references them.
#  3. Re-running without --force-rerun skips cells with terminal report.json.
#  4. --retry-invalid reruns invalid cells; new cell links via attempt_of and
#     prior cell receives superseded_by back-link.
#  5. Transient 5xx triggers bounded exponential backoff + eventual success.
#
# Uses HARBOR_TASK_EXECUTOR_DRY_RUN=1 (the harbor task-executor's documented
# dry-run mode) for ACs 1–4 so docker is not required. AC5 swaps in a stub
# executor (BENCH_TASK_EXECUTOR_OVERRIDE) that emits 5xx for the first few
# attempts and then succeeds, to drive the retry loop deterministically.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BENCHMARK="${SCRIPT_DIR}/benchmark"

TMP_ROOT=""
cleanup() {
  if [[ -n "${TMP_ROOT}" && -d "${TMP_ROOT}" ]]; then
    rm -rf "${TMP_ROOT}"
  fi
}
trap cleanup EXIT

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

assert_eq() {
  local got="$1" want="$2" msg="${3:-values differ}"
  [[ "${got}" == "${want}" ]] || fail "${msg}: got=${got} want=${want}"
}

assert_nonempty() {
  local v="$1" msg="${2:-value is empty}"
  [[ -n "${v}" ]] || fail "${msg}"
}

require() {
  command -v "$1" >/dev/null 2>&1 || fail "required tool not found: $1"
}

require jq
require yq
require sha256sum

TMP_ROOT="$(mktemp -d)"
TASKS_DIR="${TMP_ROOT}/tasks"
OUT_DIR="${TMP_ROOT}/out"
STUB_DIR="${TMP_ROOT}/stubs"
mkdir -p "${TASKS_DIR}" "${OUT_DIR}" "${STUB_DIR}"

PROFILE="sindri-lucebox"
BENCH_SET="tb-2-1-canary"
DATASET="terminal-bench-2-1"
TASKS=(cancel-async-tasks log-summary-date-ranges configure-git-webserver)

echo "==> AC1+AC2: initial sweep produces cells + sweep.json"

env HARBOR_TASK_EXECUTOR_DRY_RUN=1 \
    BENCH_TASKS_DIR="${TASKS_DIR}" \
    BENCH_HARBOR_DIGEST_OVERRIDE="sha256:test-digest-aaa" \
    "${BENCHMARK}" \
      --profile "${PROFILE}" \
      --bench-set "${BENCH_SET}" \
      --reps 1 \
      --out "${OUT_DIR}" \
    >/dev/null

# sweep.json present with both fields
sweep_json="${OUT_DIR}/sweep.json"
[[ -f "${sweep_json}" ]] || fail "sweep.json missing at ${sweep_json}"
te_ver="$(jq -r '.task_executor_version // ""' "${sweep_json}")"
hd="$(jq -r '.harbor_runner_image_digest // ""' "${sweep_json}")"
assert_nonempty "${te_ver}" "sweep.json: task_executor_version empty"
assert_nonempty "${hd}"     "sweep.json: harbor_runner_image_digest empty"
assert_eq "${hd}" "sha256:test-digest-aaa" "sweep.json: digest mismatch"

# One cell per task
for task in "${TASKS[@]}"; do
  task_dir="${OUT_DIR}/cells/${DATASET}/${task}"
  [[ -d "${task_dir}" ]] || fail "cells dir missing for task=${task}: ${task_dir}"
  cells=("${task_dir}"/*/)
  [[ ${#cells[@]} -eq 1 ]] || fail "task=${task}: expected 1 cell, got ${#cells[@]}"
  cell="${cells[0]%/}"
  report="${cell}/report.json"
  [[ -f "${report}" ]] || fail "task=${task}: report.json missing at ${report}"

  # report.json embedded fields
  [[ "$(jq -r '.profile.id // ""' "${report}")" == "${PROFILE}" ]] \
    || fail "task=${task}: report.json profile.id != ${PROFILE}"
  jq -e '.command | type == "array"' "${report}" >/dev/null \
    || fail "task=${task}: report.json missing command array"
  jq -e '.env_redacted | type == "object"' "${report}" >/dev/null \
    || fail "task=${task}: report.json missing env_redacted object"
  jq -e '.task_executor_version' "${report}" >/dev/null \
    || fail "task=${task}: report.json missing task_executor_version"
  jq -e '.harbor_runner_image_digest' "${report}" >/dev/null \
    || fail "task=${task}: report.json missing harbor_runner_image_digest"
  assert_eq "$(jq -r '.task_executor_version' "${report}")" "${te_ver}" \
    "task=${task}: report.json task_executor_version mismatch"
  assert_eq "$(jq -r '.harbor_runner_image_digest' "${report}")" "${hd}" \
    "task=${task}: report.json harbor_runner_image_digest mismatch"

  # Secret redaction: FIZEAU_API_KEY is flagged secret in fiz adapter output
  redacted="$(jq -r '.env_redacted.FIZEAU_API_KEY // ""' "${report}")"
  assert_eq "${redacted}" "***REDACTED***" \
    "task=${task}: FIZEAU_API_KEY should be redacted in env_redacted"

  # Sibling artifacts
  [[ -f "${cell}/fiz.txt" ]] || fail "task=${task}: fiz.txt missing"
  [[ -f "${cell}/fiz.err" ]] || fail "task=${task}: fiz.err missing"
  [[ -d "${cell}/session" ]] || fail "task=${task}: session/ missing"

  # cell-state.json was cleaned up on terminal close
  [[ ! -f "${cell}/cell-state.json" ]] \
    || fail "task=${task}: cell-state.json should be deleted on terminal close"

  # final_status is terminal
  status="$(jq -r '.final_status // ""' "${report}")"
  case "${status}" in
    completed|pass|fail|timeout) ;;
    *) fail "task=${task}: unexpected final_status=${status}" ;;
  esac
done

echo "==> AC3: rerun (no flag) skips terminal cells"

before_count="$(find "${OUT_DIR}/cells" -mindepth 3 -maxdepth 3 -type d | wc -l | tr -d ' ')"
env HARBOR_TASK_EXECUTOR_DRY_RUN=1 \
    BENCH_TASKS_DIR="${TASKS_DIR}" \
    BENCH_HARBOR_DIGEST_OVERRIDE="sha256:test-digest-aaa" \
    "${BENCHMARK}" \
      --profile "${PROFILE}" \
      --bench-set "${BENCH_SET}" \
      --reps 1 \
      --out "${OUT_DIR}" \
    >/dev/null
after_count="$(find "${OUT_DIR}/cells" -mindepth 3 -maxdepth 3 -type d | wc -l | tr -d ' ')"
assert_eq "${after_count}" "${before_count}" "resume default added cells (${before_count} -> ${after_count})"

echo "==> AC4: --retry-invalid reruns invalid cells with attempt_of + superseded_by"

# Mark the cancel-async-tasks cell as invalid by injecting invalid_class.
prior_cell="$(find "${OUT_DIR}/cells/${DATASET}/cancel-async-tasks" -mindepth 1 -maxdepth 1 -type d | head -n1)"
[[ -n "${prior_cell}" ]] || fail "no prior cell found for cancel-async-tasks"
prior_report="${prior_cell}/report.json"
tmp_report="${TMP_ROOT}/prior_report.json"
jq '.invalid_class = "setup_failure" | .final_status = "invalid"' "${prior_report}" >"${tmp_report}"
mv "${tmp_report}" "${prior_report}"
prior_cell_id="$(jq -r '.cell_id' "${prior_report}")"

env HARBOR_TASK_EXECUTOR_DRY_RUN=1 \
    BENCH_TASKS_DIR="${TASKS_DIR}" \
    BENCH_HARBOR_DIGEST_OVERRIDE="sha256:test-digest-aaa" \
    "${BENCHMARK}" \
      --profile "${PROFILE}" \
      --bench-set "${BENCH_SET}" \
      --reps 1 \
      --retry-invalid \
      --out "${OUT_DIR}" \
    >/dev/null

# A new cell should now exist whose attempt_of points at the prior cell.
new_cell=""
for c in "${OUT_DIR}/cells/${DATASET}/cancel-async-tasks"/*/; do
  cdir="${c%/}"
  [[ "${cdir}" == "${prior_cell}" ]] && continue
  if [[ -f "${cdir}/report.json" ]]; then
    ao="$(jq -r '.attempt_of // ""' "${cdir}/report.json")"
    if [[ "${ao}" == "${prior_cell}" ]]; then
      new_cell="${cdir}"
      break
    fi
  fi
done
[[ -n "${new_cell}" ]] || fail "retry-invalid: no new cell with attempt_of pointing at ${prior_cell}"

new_cell_id="$(jq -r '.cell_id' "${new_cell}/report.json")"
prior_superseded="$(jq -r '.superseded_by // ""' "${prior_report}")"
assert_eq "${prior_superseded}" "${new_cell_id}" "prior cell missing superseded_by back-link"
echo "    prior=${prior_cell_id} superseded_by=${new_cell_id}"

# Orphan cell-state.json under --retry-invalid: create one, rerun, verify
# new cell links to it via attempt_of.
orphan_dir="${OUT_DIR}/cells/${DATASET}/log-summary-date-ranges/19700101T000000Z-dead"
mkdir -p "${orphan_dir}"
jq -n --arg p "${PROFILE}" --arg t "log-summary-date-ranges" \
  '{cell_id:"19700101T000000Z-dead", task:$t, profile:$p, started_at:"1970-01-01T00:00:00Z", status:"running"}' \
  >"${orphan_dir}/cell-state.json"

env HARBOR_TASK_EXECUTOR_DRY_RUN=1 \
    BENCH_TASKS_DIR="${TASKS_DIR}" \
    BENCH_HARBOR_DIGEST_OVERRIDE="sha256:test-digest-aaa" \
    "${BENCHMARK}" \
      --profile "${PROFILE}" \
      --bench-set "${BENCH_SET}" \
      --reps 1 \
      --retry-invalid \
      --out "${OUT_DIR}" \
    >/dev/null

orphan_report="${orphan_dir}/report.json"
[[ -f "${orphan_report}" ]] || fail "orphan cell did not receive back-written report.json"
orphan_super="$(jq -r '.superseded_by // ""' "${orphan_report}")"
assert_nonempty "${orphan_super}" "orphan cell missing superseded_by"

found_orphan_retry=0
for c in "${OUT_DIR}/cells/${DATASET}/log-summary-date-ranges"/*/; do
  cdir="${c%/}"
  [[ "${cdir}" == "${orphan_dir}" ]] && continue
  if [[ -f "${cdir}/report.json" ]]; then
    ao="$(jq -r '.attempt_of // ""' "${cdir}/report.json")"
    if [[ "${ao}" == "${orphan_dir}" ]]; then
      found_orphan_retry=1
      break
    fi
  fi
done
[[ ${found_orphan_retry} -eq 1 ]] || fail "no retry cell links attempt_of to orphan ${orphan_dir}"

echo "==> AC5: transient 5xx triggers exponential backoff + eventual success"

STATE_FILE="${TMP_ROOT}/transient-state"
echo 0 >"${STATE_FILE}"
cat >"${STUB_DIR}/flaky" <<EOF
#!/usr/bin/env bash
# Stub task-executor. Fails the first two attempts with HTTP 503 (a transient
# signature recognized by run_executor_with_retry), then succeeds.
set -euo pipefail
state_file="${STATE_FILE}"
cell_dir=""
spec="\$(cat)"
cell_dir="\$(jq -r '.cell_dir' <<<"\$spec")"
n=\$(cat "\${state_file}")
n=\$((n + 1))
echo "\${n}" >"\${state_file}"
if (( n < 3 )); then
  echo "fake-harbor: HTTP 503 service unavailable" >&2
  exit 1
fi
mkdir -p "\$cell_dir"
jq -n --arg t "\$(jq -r '.task_id' <<<"\$spec")" \\
  '{task_id:\$t, final_status:"completed", status:"completed"}' \\
  >"\$cell_dir/result.json"
EOF
chmod +x "${STUB_DIR}/flaky"

OUT2="${TMP_ROOT}/out2"
mkdir -p "${OUT2}"
env BENCH_TASKS_DIR="${TASKS_DIR}" \
    BENCH_TASK_EXECUTOR_OVERRIDE="${STUB_DIR}/flaky" \
    BENCH_RETRY_BACKOFF_BASE=0 \
    BENCH_HARBOR_DIGEST_OVERRIDE="sha256:test-digest-bbb" \
    "${BENCHMARK}" \
      --profile "${PROFILE}" \
      --bench-set "${BENCH_SET}" \
      --reps 1 \
      --out "${OUT2}" \
    >/dev/null

# The state file counter is shared across all cells in the sweep, so only the
# first task processed (cancel-async-tasks, per bench-set yaml order) sees the
# 5xx -> 5xx -> 200 retry sequence; later cells succeed immediately. Target
# that first cell explicitly.
first_cell="$(find "${OUT2}/cells/${DATASET}/cancel-async-tasks" -mindepth 1 -maxdepth 1 -type d | head -n1)"
[[ -n "${first_cell}" ]] || fail "transient sweep: no cell created"
retry_log="${first_cell}/retry-log.txt"
[[ -f "${retry_log}" ]] || fail "transient sweep: retry-log.txt missing"

attempts=$(grep -c '^attempt=' "${retry_log}" || true)
[[ "${attempts}" -ge 3 ]] || fail "transient sweep: expected >=3 attempts, got ${attempts}"
grep -q 'status=0' "${retry_log}" || fail "transient sweep: no successful attempt"

final_status="$(jq -r '.final_status // ""' "${first_cell}/report.json")"
case "${final_status}" in
  completed|pass) ;;
  *) fail "transient sweep: expected terminal-success final_status, got ${final_status}" ;;
esac

# Terminal failure after retry cap: stub that always fails.
cat >"${STUB_DIR}/always_5xx" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
cat >/dev/null
echo "fake-harbor: HTTP 503 service unavailable" >&2
exit 1
EOF
chmod +x "${STUB_DIR}/always_5xx"

OUT3="${TMP_ROOT}/out3"
mkdir -p "${OUT3}"
set +e
env BENCH_TASKS_DIR="${TASKS_DIR}" \
    BENCH_TASK_EXECUTOR_OVERRIDE="${STUB_DIR}/always_5xx" \
    BENCH_RETRY_BACKOFF_BASE=0 \
    BENCH_RETRY_MAX_ATTEMPTS=2 \
    BENCH_HARBOR_DIGEST_OVERRIDE="sha256:test-digest-ccc" \
    "${BENCHMARK}" \
      --profile "${PROFILE}" \
      --bench-set "${BENCH_SET}" \
      --reps 1 \
      --out "${OUT3}" \
    >/dev/null 2>&1
rc=$?
set -e
# The runner does not exit non-zero on per-cell failure today (the cell is
# written with final_status=transient_exhausted). Both behaviors are
# acceptable; the AC is satisfied by the cell record.
: "${rc}"

cap_cell="$(find "${OUT3}/cells/${DATASET}" -mindepth 2 -maxdepth 2 -type d | head -n1)"
[[ -n "${cap_cell}" ]] || fail "retry-cap sweep: no cell created"
cap_status="$(jq -r '.final_status // ""' "${cap_cell}/report.json")"
assert_eq "${cap_status}" "transient_exhausted" "retry-cap sweep: expected transient_exhausted"
cap_attempts=$(grep -c '^attempt=' "${cap_cell}/retry-log.txt" || true)
[[ "${cap_attempts}" -ge 3 ]] || fail "retry-cap sweep: expected >=3 attempts (max + 1), got ${cap_attempts}"

echo "PASS: all 5 acceptance scenarios verified"
