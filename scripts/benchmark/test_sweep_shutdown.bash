#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

TMP_ROOT=""

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

cleanup() {
  if [[ -n "${TMP_ROOT}" ]]; then
    pgrep -af "fiz-bench|harbor run" | grep -F "${TMP_ROOT}" | awk '{print $1}' | xargs -r kill -TERM >/dev/null 2>&1 || true
    sleep 1
    pgrep -af "fiz-bench|harbor run" | grep -F "${TMP_ROOT}" | awk '{print $1}' | xargs -r kill -KILL >/dev/null 2>&1 || true
    rm -rf "${TMP_ROOT}"
  fi
}
trap cleanup EXIT

write_fake_tools() {
  local bin_dir="$1"
  mkdir -p "${bin_dir}"

  cat > "${bin_dir}/docker" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
case "${1:-}" in
  info)
    if [[ "${2:-}" == "--format" ]]; then
      echo x86_64
    fi
    exit 0
    ;;
  create)
    echo fake-container
    exit 0
    ;;
  cp)
    dest="${@: -1}"
    mkdir -p "${dest}"
    echo fake > "${dest}/tool"
    exit 0
    ;;
  rm|build|ps)
    exit 0
    ;;
  *)
    exit 0
    ;;
esac
SH
  chmod +x "${bin_dir}/docker"

  cat > "${bin_dir}/curl" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
out=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --output)
      out="$2"
      shift 2
      ;;
    --write-out)
      shift 2
      ;;
    *)
      shift
      ;;
  esac
done
if [[ -n "${out}" ]]; then
  printf '{"choices":[{"message":{"content":"ok"}}]}\n' > "${out}"
fi
printf '200'
SH
  chmod +x "${bin_dir}/curl"

  cat > "${bin_dir}/harbor" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
cmd="${1:-}"
if [[ $# -gt 0 ]]; then
  shift
fi
case "${cmd}" in
  run)
    log="${SWEEP_SHUTDOWN_LOG:?}"
    printf 'fake-harbor started pid=%s args=run %s\n' "$$" "$*" >> "${log}"
    trap 'printf "fake-harbor term pid=%s\n" "$$" >> "${log}"; sleep 0.2; printf "fake-harbor teardown pid=%s\n" "$$" >> "${log}"; exit 143' TERM
    while true; do
      sleep 1
    done
    ;;
  download)
    exit 0
    ;;
  *)
    exit 0
    ;;
esac
SH
  chmod +x "${bin_dir}/harbor"

  cat > "${bin_dir}/go" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
out=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    -o)
      out="$2"
      shift 2
      ;;
    *)
      shift
      ;;
  esac
done
[[ -n "${out}" ]] || exit 0
mkdir -p "$(dirname "${out}")"
if [[ "$(basename "${out}")" == "fiz-bench" ]]; then
  cat > "${out}" <<'BENCH'
#!/usr/bin/env bash
set -euo pipefail
cmd="${1:-}"
if [[ $# -gt 0 ]]; then
  shift
fi
case "${cmd}" in
  sweep)
    out=""
    phase="canary"
    dry_run=0
    while [[ $# -gt 0 ]]; do
      case "$1" in
        --out)
          out="$2"
          shift 2
          ;;
        --phase)
          phase="$2"
          shift 2
          ;;
        --dry-run)
          dry_run=1
          shift
          ;;
        *)
          shift
          ;;
      esac
    done
    if [[ "${dry_run}" = "1" ]]; then
      echo "=== Phase: ${phase} ==="
      echo "  Lane: fiz-vidar-ds4"
      exit 0
    fi
    [[ -n "${out}" ]] || exit 2
    mkdir -p "${out}/${phase}/fiz-vidar-ds4" "${out}/cells/fake"
    printf '{"runs":[],"cells":[]}\n' > "${out}/${phase}/fiz-vidar-ds4/matrix.json"
    printf '{"final_status":"completed"}\n' > "${out}/cells/fake/report.json"
    harbor run --delete --jobs-dir "${out}/cells" --job-name fake &
    child=$!
    trap 'kill -TERM "${child}" >/dev/null 2>&1 || true; wait "${child}" >/dev/null 2>&1 || true; exit 143' TERM INT
    wait "${child}"
    ;;
  matrix-index)
    exit 0
    ;;
  *)
    exit 0
    ;;
esac
BENCH
else
  cat > "${out}" <<'FIZ'
#!/usr/bin/env bash
exit 0
FIZ
fi
chmod +x "${out}"
SH
  chmod +x "${bin_dir}/go"
}

make_tasks_dir() {
  local tasks_dir="$1"
  mkdir -p "${tasks_dir}"
  python3 - "${REPO_ROOT}" "${tasks_dir}" <<'PY'
import pathlib
import re
import sys

repo = pathlib.Path(sys.argv[1])
tasks = pathlib.Path(sys.argv[2])
ids = set()
for path in (repo / "scripts" / "benchmark").glob("task-subset-tb21-*.yaml"):
    for line in path.read_text().splitlines():
        match = re.match(r"\s*-\s*id:\s*([A-Za-z0-9_.-]+)", line)
        if match:
            ids.add(match.group(1))
for task_id in ids:
    task_dir = tasks / task_id
    (task_dir / "environment").mkdir(parents=True, exist_ok=True)
    (task_dir / "task.toml").write_text('docker_image = "example/test:latest"\n')
    (task_dir / "environment" / "Dockerfile").write_text("FROM scratch\n")
PY
}

reset_fixture() {
  cleanup
  TMP_ROOT="$(mktemp -d)"
  write_fake_tools "${TMP_ROOT}/bin"
  make_tasks_dir "${TMP_ROOT}/tasks"
  mkdir -p "${TMP_ROOT}/out"
}

sweep_processes() {
  pgrep -af "fiz-bench|harbor run" | grep -F "${TMP_ROOT}" || true
}

wait_for_log() {
  local needle="$1"
  local deadline=$((SECONDS + 10))
  while (( SECONDS < deadline )); do
    if [[ -f "${TMP_ROOT}/sweep.log" ]] && grep -q "${needle}" "${TMP_ROOT}/sweep.log"; then
      return 0
    fi
    sleep 0.1
  done
  fail "timed out waiting for log ${needle}"
}

assert_temp_artifacts() {
  [[ -x "${TMP_ROOT}/.local/bin/fiz-bench" ]] || fail "missing temp fiz-bench"
  [[ -x "${TMP_ROOT}/.local/share/fizeau/benchmark-runtime/fiz-linux-amd64" ]] || fail "missing temp Harbor fiz artifact"
}

wait_for_no_sweep_processes() {
  local timeout="$1"
  local deadline=$((SECONDS + timeout))
  local survivors
  while (( SECONDS < deadline )); do
    survivors="$(sweep_processes)"
    if [[ -z "${survivors}" ]]; then
      return 0
    fi
    sleep 1
  done
  fail "surviving sweep processes after ${timeout}s:"$'\n'"$(sweep_processes)"
}

start_sweep() {
  PATH="${TMP_ROOT}/bin:${PATH}" \
  SWEEP_SHUTDOWN_LOG="${TMP_ROOT}/sweep.log" \
  BENCHMARK_BIN_DIR="${TMP_ROOT}/.local/bin" \
  BENCHMARK_RUNTIME_DIR="${TMP_ROOT}/.local/share/fizeau/benchmark-runtime" \
  BENCHMARK_CONFIRM_DELAY=0 \
  BENCHMARK_SWEEP_TERM_GRACE_SECONDS=5 \
  HARBOR_SKIP_NATIVE_HOME=1 \
    bash "${REPO_ROOT}/scripts/benchmark/run_terminalbench_2_1_sweep.sh" \
      --phase canary \
      --lanes ds4 \
      --tasks-dir "${TMP_ROOT}/tasks" \
      --out "${TMP_ROOT}/out" \
      > "${TMP_ROOT}/wrapper.out" 2>&1 &
  echo "$!"
}

TestSweepShutdownKillReapsProcessTree() {
  reset_fixture
  local sweep_pid
  sweep_pid="$(start_sweep)"
  wait_for_log "fake-harbor started"
  assert_temp_artifacts
  kill "${sweep_pid}"
  wait "${sweep_pid}" >/dev/null 2>&1 || true
  wait_for_no_sweep_processes 30
}

TestSweepShutdownNoOrphans() {
  reset_fixture
  local sweep_pid
  sweep_pid="$(start_sweep)"
  wait_for_log "fake-harbor started"
  kill -TERM "${sweep_pid}"
  wait "${sweep_pid}" >/dev/null 2>&1 || true
  wait_for_no_sweep_processes 60
}

TestSweepShutdownReportState() {
  reset_fixture
  local sweep_pid
  sweep_pid="$(start_sweep)"
  wait_for_log "fake-harbor started"
  kill -TERM "${sweep_pid}"
  wait "${sweep_pid}" >/dev/null 2>&1 || true
  wait_for_no_sweep_processes 60
  if find "${TMP_ROOT}/out" -name 'report.json.tmp' -print -quit | grep -q .; then
    fail "half-written report.json.tmp found"
  fi
  python3 - "${TMP_ROOT}/out" <<'PY'
import json
import pathlib
import sys

root = pathlib.Path(sys.argv[1])
for report in root.rglob("report.json"):
    data = json.loads(report.read_text())
    if not isinstance(data, dict) or not data.get("final_status"):
        raise SystemExit(f"incomplete report: {report}")
PY
}

main() {
  for test_name in \
    TestSweepShutdownKillReapsProcessTree \
    TestSweepShutdownNoOrphans \
    TestSweepShutdownReportState
  do
    echo "=== RUN ${test_name}"
    "${test_name}"
    echo "--- PASS ${test_name}"
  done
}

main "$@"
