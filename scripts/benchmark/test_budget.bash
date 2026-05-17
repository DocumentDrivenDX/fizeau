#!/usr/bin/env bash
# Integration test for the ./benchmark --max-cost-usd budget cap (bead
# fizeau-25209a13). Verifies all three ACs of the bead description:
#
#  1. ./benchmark --profile sindri-lucebox --bench-set tb-2-1-canary
#     --max-cost-usd 0.01 produces at least one cell with
#     final_status=budget_halted.
#  2. <out>/budget.json accumulates cost_usd_at_run_time across closed
#     cells.
#  3. Budget-halted cells have process_outcome=setup_failed and a non-empty
#     'note' field referencing the cap.
#
# Uses BENCH_TASK_EXECUTOR_OVERRIDE to stand in for the harbor task-executor.
# The stub writes a result.json whose cost_usd already exceeds 0.01 on the
# very first cell, which is how the sindri-lucebox profile (all-zero pricing,
# harness-native cost reporting) drives the cap when run live too.

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

fail() { echo "FAIL: $*" >&2; exit 1; }

assert_eq() {
  local got="$1" want="$2" msg="${3:-values differ}"
  [[ "${got}" == "${want}" ]] || fail "${msg}: got=${got} want=${want}"
}

assert_nonempty() {
  [[ -n "$1" ]] || fail "${2:-value is empty}"
}

require() { command -v "$1" >/dev/null 2>&1 || fail "required tool not found: $1"; }
require jq
require yq

TMP_ROOT="$(mktemp -d)"
TASKS_DIR="${TMP_ROOT}/tasks"
OUT_DIR="${TMP_ROOT}/out"
STUB_DIR="${TMP_ROOT}/stubs"
mkdir -p "${TASKS_DIR}" "${OUT_DIR}" "${STUB_DIR}"

PROFILE="sindri-lucebox"
BENCH_SET="tb-2-1-canary"
DATASET="terminal-bench-2-1"

# Stub task-executor: report a cost_usd of 0.05 on every cell. The first
# closed cell pushes the accumulator above 0.01 and trips the cap; later
# cells should become budget_halted placeholders and never reach this stub.
cat >"${STUB_DIR}/costly" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
spec="$(cat)"
cell_dir="$(jq -r '.cell_dir' <<<"$spec")"
task_id="$(jq -r '.task_id' <<<"$spec")"
mkdir -p "$cell_dir"
jq -n --arg t "$task_id" \
  '{task_id:$t, final_status:"completed", status:"completed",
    input_tokens:1000, output_tokens:1000, cached_input_tokens:0,
    cost_usd:0.05}' \
  >"$cell_dir/result.json"
EOF
chmod +x "${STUB_DIR}/costly"

echo "==> AC1+AC2+AC3: --max-cost-usd 0.01 halts before the next cell"

env BENCH_TASKS_DIR="${TASKS_DIR}" \
    BENCH_TASK_EXECUTOR_OVERRIDE="${STUB_DIR}/costly" \
    BENCH_HARBOR_DIGEST_OVERRIDE="sha256:test-digest-budget" \
    "${BENCHMARK}" \
      --profile "${PROFILE}" \
      --bench-set "${BENCH_SET}" \
      --reps 1 \
      --max-cost-usd 0.01 \
      --out "${OUT_DIR}" \
    >/dev/null

# AC2: budget.json present with the configured cap and accumulator > 0.
budget_json="${OUT_DIR}/budget.json"
[[ -f "${budget_json}" ]] || fail "budget.json missing at ${budget_json}"
cap="$(jq -r '.max_cost_usd' "${budget_json}")"
assert_eq "${cap}" "0.01" "budget.json max_cost_usd mismatch"
total="$(jq -r '.total_cost_usd' "${budget_json}")"
halted_flag="$(jq -r '.halted' "${budget_json}")"
n_cells="$(jq -r '.cells | length' "${budget_json}")"
[[ "${n_cells}" -ge 1 ]] || fail "budget.json cells: expected >=1, got ${n_cells}"
case "${total}" in
  0|0.0|0.00|"") fail "budget.json total_cost_usd did not accumulate (got ${total})" ;;
esac
assert_eq "${halted_flag}" "true" "budget.json halted should be true after cap hit"

# AC1: at least one report.json has final_status=budget_halted.
mapfile -t halted_reports < <(grep -rl '"final_status": "budget_halted"' \
  "${OUT_DIR}/cells" 2>/dev/null || true)
[[ ${#halted_reports[@]} -ge 1 ]] \
  || fail "expected >=1 budget_halted cell; found ${#halted_reports[@]}"

# AC3: every halted cell has process_outcome=setup_failed and a non-empty
# note referencing the cap.
for report in "${halted_reports[@]}"; do
  po="$(jq -r '.process_outcome // ""' "${report}")"
  assert_eq "${po}" "setup_failed" \
    "halted cell ${report}: process_outcome must be setup_failed"
  note="$(jq -r '.note // ""' "${report}")"
  assert_nonempty "${note}" "halted cell ${report}: note must be non-empty"
  case "${note}" in
    *"--max-cost-usd"*|*"max-cost-usd"*|*"cap"*|*"budget"*) ;;
    *) fail "halted cell ${report}: note does not reference the cap (note=${note})" ;;
  esac
  # final_status must round-trip via jq (not just substring).
  fs="$(jq -r '.final_status // ""' "${report}")"
  assert_eq "${fs}" "budget_halted" "halted cell ${report}: final_status"
  ic="$(jq -r '.invalid_class // ""' "${report}")"
  assert_eq "${ic}" "" "halted cell ${report}: invalid_class should be empty"
done

# Sanity: budget.json's recorded total must equal the sum of cells[].cost_usd.
recomputed="$(jq -r '[.cells[].cost_usd] | add' "${budget_json}")"
assert_eq "${total}" "${recomputed}" "budget.json total != sum(cells.cost_usd)"

# At least one fully-executed cell exists (the one whose cost tripped the cap).
ran_cells="$(grep -rL '"final_status": "budget_halted"' "${OUT_DIR}/cells" 2>/dev/null \
              | grep '/report\.json$' | wc -l | tr -d ' ')"
[[ "${ran_cells}" -ge 1 ]] \
  || fail "expected >=1 executed cell that produced positive cost"

echo "==> bonus: re-running with the same cap leaves the cap intact"

env BENCH_TASKS_DIR="${TASKS_DIR}" \
    BENCH_TASK_EXECUTOR_OVERRIDE="${STUB_DIR}/costly" \
    BENCH_HARBOR_DIGEST_OVERRIDE="sha256:test-digest-budget" \
    "${BENCHMARK}" \
      --profile "${PROFILE}" \
      --bench-set "${BENCH_SET}" \
      --reps 1 \
      --max-cost-usd 0.01 \
      --out "${OUT_DIR}" \
    >/dev/null

cap_after="$(jq -r '.max_cost_usd' "${budget_json}")"
assert_eq "${cap_after}" "0.01" "budget.json cap should be preserved on resume"

echo "PASS: all 3 budget acceptance scenarios verified"
