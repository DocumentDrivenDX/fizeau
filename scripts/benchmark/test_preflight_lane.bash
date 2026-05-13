#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
REAL_CURL="$(command -v curl)"

TMP_ROOT=""
SERVER_PIDS=()

cleanup() {
  local pid
  for pid in "${SERVER_PIDS[@]:-}"; do
    kill "${pid}" >/dev/null 2>&1 || true
    wait "${pid}" >/dev/null 2>&1 || true
  done
  if [[ -n "${TMP_ROOT}" ]]; then
    rm -rf "${TMP_ROOT}"
  fi
}
trap cleanup EXIT

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

assert_contains() {
  local haystack="$1"
  local needle="$2"
  [[ "${haystack}" == *"${needle}"* ]] || fail "missing ${needle}"$'\n'"${haystack}"
}

assert_not_contains() {
  local haystack="$1"
  local needle="$2"
  [[ "${haystack}" != *"${needle}"* ]] || fail "unexpected ${needle}"$'\n'"${haystack}"
}

write_fake_tools() {
  local bin_dir="$1"
  mkdir -p "${bin_dir}"

  cat > "${bin_dir}/curl" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >> "${PREFLIGHT_TEST_CURL_LOG:?}"
for arg in "$@"; do
  if [[ "${arg}" == *"openrouter.ai"* ]]; then
    echo "unexpected OpenRouter preflight call" >&2
    exit 97
  fi
done
exec "${REAL_CURL:?}" "$@"
SH
  chmod +x "${bin_dir}/curl"

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
  rm|build)
    exit 0
    ;;
  *)
    exit 0
    ;;
esac
SH
  chmod +x "${bin_dir}/docker"

  cat > "${bin_dir}/harbor" <<'SH'
#!/usr/bin/env bash
exit 0
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
    phase="all"
    lanes=""
    dry_run=0
    while [[ $# -gt 0 ]]; do
      case "$1" in
        --phase) phase="$2"; shift 2 ;;
        --phase=*) phase="${1#*=}"; shift ;;
        --lanes) lanes="$2"; shift 2 ;;
        --lanes=*) lanes="${1#*=}"; shift ;;
        --dry-run) dry_run=1; shift ;;
        *) shift ;;
      esac
    done
    if [[ "${dry_run}" = "1" ]]; then
      echo "=== Phase: ${phase} ==="
      if [[ -n "${lanes}" ]]; then
        IFS=',' read -ra lane_items <<< "${lanes}"
        for lane in "${lane_items[@]}"; do
          echo "  Lane: ${lane}"
        done
      else
        echo "  Lane: all"
      fi
    else
      echo "[sweep] phase=${phase} lane=fake rg=fake starting"
    fi
    ;;
  matrix-index)
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

start_mock_server() {
  local mode="$1"
  local port_file="$2"
  local request_log="$3"
  python3 - "${mode}" "${port_file}" "${request_log}" <<'PY' &
import http.server
import json
import pathlib
import sys

mode = sys.argv[1]
port_file = pathlib.Path(sys.argv[2])
request_log = pathlib.Path(sys.argv[3])

class Handler(http.server.BaseHTTPRequestHandler):
    def do_POST(self):
        length = int(self.headers.get("content-length") or "0")
        raw = self.rfile.read(length)
        request_log.write_text(request_log.read_text() + raw.decode("utf-8", errors="replace") + "\n" if request_log.exists() else raw.decode("utf-8", errors="replace") + "\n")
        try:
            body = json.loads(raw)
        except Exception:
            body = {}
        if mode == "reasoning-low-fails" and body.get("reasoning") == "low":
            self.send_response(400)
            self.send_header("content-type", "application/json")
            self.end_headers()
            self.wfile.write(json.dumps({
                "error": {
                    "message": 'reasoning="low" is not supported by provider type "llama-server"'
                }
            }).encode())
            return
        self.send_response(200)
        self.send_header("content-type", "application/json")
        self.end_headers()
        self.wfile.write(b'{"choices":[{"message":{"content":""}}]}')

    def log_message(self, fmt, *args):
        return

server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), Handler)
port_file.write_text(str(server.server_address[1]))
server.serve_forever()
PY
  local pid=$!
  SERVER_PIDS+=("${pid}")
  for _ in {1..100}; do
    if [[ -s "${port_file}" ]]; then
      return 0
    fi
    sleep 0.05
  done
  fail "mock server did not start"
}

unused_port() {
  python3 - <<'PY'
import socket

sock = socket.socket()
sock.bind(("127.0.0.1", 0))
print(sock.getsockname()[1])
sock.close()
PY
}

write_plan() {
  local out="$1"
  local sindri_url="$2"
  local vidar_url="$3"
  local reasoning_mode="$4"
  python3 - "${REPO_ROOT}/scripts/benchmark/terminalbench-2-1-sweep.yaml" "${out}" "${sindri_url}" "${vidar_url}" "${reasoning_mode}" <<'PY'
import pathlib
import sys

import yaml

src, out, sindri_url, vidar_url, reasoning_mode = sys.argv[1:]
plan = yaml.safe_load(pathlib.Path(src).read_text())

for rg in plan["resource_groups"]:
    if rg["id"] == "rg-sindri-club-3090-llamacpp":
        rg["base_url"] = sindri_url
    if rg["id"] == "rg-vidar-omlx":
        rg["base_url"] = vidar_url

for lane in plan["lanes"]:
    if lane["id"] == "fiz-sindri-llamacpp-qwen3-6-27b":
        lane["fizeau_env"]["FIZEAU_BASE_URL"] = sindri_url
        if reasoning_mode == "low":
            lane.setdefault("sampling", {})["reasoning"] = "low"
        elif reasoning_mode == "none":
            lane.setdefault("sampling", {}).pop("reasoning", None)
    if lane["id"] in {
        "fiz-vidar-omlx-qwen3-6-27b",
        "fiz-harness-pi-vidar-qwen3-6-27b",
        "fiz-harness-opencode-vidar-qwen3-6-27b",
    }:
        lane["fizeau_env"]["FIZEAU_BASE_URL"] = vidar_url

pathlib.Path(out).write_text(yaml.safe_dump(plan, sort_keys=False))
PY
}

run_sweep_script() {
  local plan="$1"
  local tasks_dir="$2"
  shift 2
  (
    cd "${REPO_ROOT}"
    PATH="${TMP_ROOT}/bin:${PATH}" \
      REAL_CURL="${REAL_CURL}" \
      PREFLIGHT_TEST_CURL_LOG="${TMP_ROOT}/curl.log" \
      BENCHMARK_BIN_DIR="${TMP_ROOT}/.local/bin" \
      BENCHMARK_RUNTIME_DIR="${TMP_ROOT}/.local/share/fizeau/benchmark-runtime" \
      BENCHMARK_CONFIRM_DELAY=0 \
      OMLX_API_KEY=local \
      VLLM_API_KEY=local \
      LLAMACPP_API_KEY=local \
      scripts/benchmark/run_terminalbench_2_1_sweep.sh \
        --sweep-plan "${plan}" \
        --tasks-dir "${tasks_dir}" \
        --out "${TMP_ROOT}/out" \
        "$@"
  )
}

TestPreflightLaneMisconfiguredReasoning() {
  local port_file="${TMP_ROOT}/reasoning.port"
  local request_log="${TMP_ROOT}/reasoning.requests"
  start_mock_server reasoning-low-fails "${port_file}" "${request_log}"
  local url="http://127.0.0.1:$(<"${port_file}")/v1"
  local plan="${TMP_ROOT}/reasoning-low.yaml"
  write_plan "${plan}" "${url}" "${url}" low

  local output status
  status=0
  output="$(run_sweep_script "${plan}" "${TMP_ROOT}/tasks" --phase full --lanes sindri-llamacpp,vidar --dry-run 2>&1)" || status=$?
  [[ "${status}" -ne 0 ]] || fail "misconfigured reasoning unexpectedly passed"
  assert_contains "${output}" "[preflight] fiz-sindri-llamacpp-qwen3-6-27b FAILED"
  assert_contains "${output}" 'reasoning="low" is not supported by provider type "llama-server"'
  assert_not_contains "${output}" "starting"
}

TestPreflightLaneOK() {
  local port_file="${TMP_ROOT}/ok.port"
  local request_log="${TMP_ROOT}/ok.requests"
  start_mock_server ok "${port_file}" "${request_log}"
  local url="http://127.0.0.1:$(<"${port_file}")/v1"
  local plan="${TMP_ROOT}/ok.yaml"
  write_plan "${plan}" "${url}" "${url}" none

  local output
  output="$(run_sweep_script "${plan}" "${TMP_ROOT}/tasks" --phase full --lanes sindri-llamacpp,vidar --dry-run 2>&1)"
  assert_contains "${output}" "[preflight] fiz-sindri-llamacpp-qwen3-6-27b OK (1 token in "
  assert_contains "${output}" "bench runner:       ${TMP_ROOT}/.local/bin/fiz-bench"
  assert_contains "${output}" "runtime artifacts:  ${TMP_ROOT}/.local/share/fizeau/benchmark-runtime"
  assert_contains "${output}" "Dry-run plan:"
  assert_contains "${output}" "Lane: fiz-sindri-llamacpp-qwen3-6-27b"
  [[ -x "${TMP_ROOT}/.local/bin/fiz-bench" ]] || fail "missing temp fiz-bench"
  [[ -x "${TMP_ROOT}/.local/share/fizeau/benchmark-runtime/fiz-linux-amd64" ]] || fail "missing temp Harbor fiz artifact"
}

TestPreflightLaneConnectionRefused() {
  local port
  port="$(unused_port)"
  local plan="${TMP_ROOT}/refused.yaml"
  write_plan "${plan}" "http://127.0.0.1:${port}/v1" "http://127.0.0.1:${port}/v1" none

  local output status
  status=0
  output="$(run_sweep_script "${plan}" "${TMP_ROOT}/tasks" --phase full --lanes sindri-llamacpp 2>&1)" || status=$?
  [[ "${status}" -ne 0 ]] || fail "connection-refused preflight unexpectedly passed"
  assert_contains "${output}" "[preflight] fiz-sindri-llamacpp-qwen3-6-27b FAILED: connection refused"
  assert_not_contains "${output}" "starting"
}

TestPreflightManagedCloudSkipped() {
  local plan="${TMP_ROOT}/cloud.yaml"
  cp "${REPO_ROOT}/scripts/benchmark/terminalbench-2-1-sweep.yaml" "${plan}"
  : > "${TMP_ROOT}/curl.log"

  local output
  output="$(run_sweep_script "${plan}" "${TMP_ROOT}/tasks" --phase full --lanes openrouter-qwen36 --dry-run 2>&1)"
  assert_contains "${output}" "[preflight] fiz-openrouter-qwen3-6-27b SKIPPED (managed cloud lane)"
  if [[ -s "${TMP_ROOT}/curl.log" ]]; then
    fail "managed cloud preflight called curl: $(<"${TMP_ROOT}/curl.log")"
  fi
}

main() {
  TMP_ROOT="$(mktemp -d)"
  write_fake_tools "${TMP_ROOT}/bin"
  make_tasks_dir "${TMP_ROOT}/tasks"
  : > "${TMP_ROOT}/curl.log"

  local test_name
  for test_name in \
    TestPreflightLaneMisconfiguredReasoning \
    TestPreflightLaneOK \
    TestPreflightLaneConnectionRefused \
    TestPreflightManagedCloudSkipped
  do
    "${test_name}"
    echo "ok - ${test_name}"
  done
}

main "$@"
