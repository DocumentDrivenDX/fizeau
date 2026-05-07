#!/usr/bin/env bash
# One-shot TerminalBench 2.1 sweep runner.
#
# Default:
#   scripts/benchmark/run_terminalbench_2_1_sweep.sh
#
# Common overrides:
#   scripts/benchmark/run_terminalbench_2_1_sweep.sh --phase canary
#   scripts/benchmark/run_terminalbench_2_1_sweep.sh --phase all --out benchmark-results/sweep-my-run
#   scripts/benchmark/run_terminalbench_2_1_sweep.sh --dry-run
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
cd "${REPO_ROOT}"

PHASE="all"
OUT=""
TASKS_DIR="${REPO_ROOT}/scripts/benchmark/external/terminal-bench-2-1"
SWEEP_PLAN="${REPO_ROOT}/scripts/benchmark/terminalbench-2-1-sweep.yaml"
DRY_RUN=0
PREPARE_ONLY=0
FORCE_RERUN=0
MATRIX_JOBS_MANAGED=1
PER_RUN_BUDGET_USD=""
BUDGET_USD=""
CONFIRM_DELAY="${BENCHMARK_CONFIRM_DELAY:-8}"

usage() {
  cat <<'EOF'
Usage: ./benchmark [flags]

Flags:
  --phase canary|local-qwen|sonnet-comparison|gpt-comparison|all
  --out <dir>
  --tasks-dir <dir>
  --sweep-plan <file>
  --dry-run
  --prepare-only
  --force-rerun
  --budget-usd <n>
  --per-run-budget-usd <n>
  --matrix-jobs-managed <n>

The script builds local benchmark artifacts, downloads/validates TB-2.1 tasks,
prints the exact target plan, waits briefly for Ctrl-C, then runs the sweep.
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --phase)
      PHASE="$2"; shift 2 ;;
    --phase=*)
      PHASE="${1#*=}"; shift ;;
    --out)
      OUT="$2"; shift 2 ;;
    --out=*)
      OUT="${1#*=}"; shift ;;
    --tasks-dir)
      TASKS_DIR="$2"; shift 2 ;;
    --tasks-dir=*)
      TASKS_DIR="${1#*=}"; shift ;;
    --sweep-plan)
      SWEEP_PLAN="$2"; shift 2 ;;
    --sweep-plan=*)
      SWEEP_PLAN="${1#*=}"; shift ;;
    --dry-run)
      DRY_RUN=1; shift ;;
    --prepare-only)
      PREPARE_ONLY=1; shift ;;
    --force-rerun)
      FORCE_RERUN=1; shift ;;
    --budget-usd)
      BUDGET_USD="$2"; shift 2 ;;
    --budget-usd=*)
      BUDGET_USD="${1#*=}"; shift ;;
    --per-run-budget-usd)
      PER_RUN_BUDGET_USD="$2"; shift 2 ;;
    --per-run-budget-usd=*)
      PER_RUN_BUDGET_USD="${1#*=}"; shift ;;
    --matrix-jobs-managed)
      MATRIX_JOBS_MANAGED="$2"; shift 2 ;;
    --matrix-jobs-managed=*)
      MATRIX_JOBS_MANAGED="${1#*=}"; shift ;;
    -h|--help)
      usage; exit 0 ;;
    *)
      echo "unknown flag: $1" >&2
      usage >&2
      exit 2 ;;
  esac
done

case "${PHASE}" in
  canary|local-qwen|sonnet-comparison|gpt-comparison|all) ;;
  *)
    echo "unknown --phase ${PHASE}" >&2
    exit 2 ;;
esac

abs_path() {
  local path="$1"
  if [[ "${path}" = /* ]]; then
    printf '%s\n' "${path}"
  else
    printf '%s\n' "${REPO_ROOT}/${path}"
  fi
}

TASKS_DIR="$(abs_path "${TASKS_DIR}")"
SWEEP_PLAN="$(abs_path "${SWEEP_PLAN}")"
if [[ -z "${OUT}" ]]; then
  OUT="${REPO_ROOT}/benchmark-results/sweep-$(date -u +%Y%m%dT%H%M%SZ)"
else
  OUT="$(abs_path "${OUT}")"
fi

need() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "missing required command: $1" >&2
    exit 1
  }
}

ensure_harbor() {
  if command -v harbor >/dev/null 2>&1; then
    return
  fi
  if command -v uv >/dev/null 2>&1; then
    echo "Installing Harbor with uv..."
    uv tool install harbor
    return
  fi
  echo "missing required command: harbor" >&2
  echo "install Harbor or install uv so this script can run: uv tool install harbor" >&2
  exit 1
}

goarch_from_machine() {
  case "$1" in
    x86_64|amd64) echo "amd64" ;;
    aarch64|arm64) echo "arm64" ;;
    *) echo "$1" ;;
  esac
}

container_goarch() {
  local arch
  arch="$(docker info --format '{{.Architecture}}' 2>/dev/null || true)"
  if [[ -z "${arch}" ]]; then
    arch="$(uname -m)"
  fi
  goarch_from_machine "${arch}"
}

container_node_arch() {
  case "${CONTAINER_GOARCH:-$(container_goarch)}" in
    amd64) echo "x64" ;;
    arm64) echo "arm64" ;;
    *) echo "${CONTAINER_GOARCH}" ;;
  esac
}

build_artifacts() {
  need go
  mkdir -p "${REPO_ROOT}/benchmark-results/bin"

  CONTAINER_GOARCH="$(container_goarch)"
  BENCH_BIN="${REPO_ROOT}/benchmark-results/bin/fiz-bench"
  FIZ_ARTIFACT="${REPO_ROOT}/benchmark-results/bin/fiz-linux-${CONTAINER_GOARCH}"

  echo "Building benchmark runner: ${BENCH_BIN}"
  rm -f "${BENCH_BIN}"
  go build -buildvcs=false -o "${BENCH_BIN}" ./cmd/bench

  echo "Building Harbor agent artifact: ${FIZ_ARTIFACT} (GOOS=linux GOARCH=${CONTAINER_GOARCH})"
  rm -f "${FIZ_ARTIFACT}"
  GOOS=linux GOARCH="${CONTAINER_GOARCH}" go build -buildvcs=false -o "${FIZ_ARTIFACT}" ./cmd/fiz
  chmod 755 "${BENCH_BIN}" "${FIZ_ARTIFACT}"
  export HARBOR_AGENT_ARTIFACT="${FIZ_ARTIFACT}"
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

ensure_node_tarball() {
  if [[ -n "${HARBOR_NODE_TARBALL:-}" ]]; then
    require_file "${HARBOR_NODE_TARBALL}" "set HARBOR_NODE_TARBALL to an existing Node.js Linux tarball"
    return
  fi

  local arch version path url
  arch="$(container_node_arch)"
  version="${HARBOR_NODE_VERSION:-20.19.2}"
  path="${REPO_ROOT}/benchmark-results/bin/node-v${version}-linux-${arch}.tar.gz"
  if [[ ! -f "${path}" ]]; then
    need curl
    mkdir -p "$(dirname "${path}")"
    url="https://nodejs.org/dist/v${version}/node-v${version}-linux-${arch}.tar.gz"
    echo "Preparing Node.js for Harbor harness install: ${url}"
    curl -fsSL "${url}" -o "${path}"
  fi
  export HARBOR_NODE_TARBALL="${path}"
}

npm_pack_once() {
  local package_spec="$1"
  local out_dir="$2"
  local match_prefix="$3"
  local expected_version="${4:-}"
  mkdir -p "${out_dir}"
  local existing
  if [[ -n "${expected_version}" ]]; then
    existing="$(find "${out_dir}" -maxdepth 1 -type f -name "${match_prefix}-${expected_version}.tgz" | sort | tail -n 1 || true)"
  else
    existing="$(find "${out_dir}" -maxdepth 1 -type f -name "${match_prefix}-*.tgz" | sort | tail -n 1 || true)"
  fi
  if [[ -n "${existing}" ]]; then
    printf '%s\n' "${existing}"
    return
  fi
  need npm
  echo "Packing ${package_spec} for Harbor harness install" >&2
  npm pack "${package_spec}" --pack-destination "${out_dir}" >/dev/null
  if [[ -n "${expected_version}" ]]; then
    existing="$(find "${out_dir}" -maxdepth 1 -type f -name "${match_prefix}-${expected_version}.tgz" | sort | tail -n 1 || true)"
  else
    existing="$(find "${out_dir}" -maxdepth 1 -type f -name "${match_prefix}-*.tgz" | sort | tail -n 1 || true)"
  fi
  if [[ -z "${existing}" ]]; then
    echo "npm pack did not create ${match_prefix}-*.tgz in ${out_dir}" >&2
    exit 1
  fi
  printf '%s\n' "${existing}"
}

installed_claude_version() {
  if command -v claude >/dev/null 2>&1; then
    claude --version 2>/dev/null | awk '{print $1; exit}'
  fi
}

installed_codex_version() {
  if command -v codex >/dev/null 2>&1; then
    codex --version 2>/dev/null | awk '{print $2; exit}'
  fi
}

installed_opencode_binary() {
  if command -v opencode >/dev/null 2>&1; then
    readlink -f "$(command -v opencode)"
  fi
}

prepare_home_tarball() {
  local env_name="$1"
  local home_name="$2"
  local out_name="$3"
  local current="${!env_name:-}"
  if [[ -n "${current}" ]]; then
    require_file "${current}" "set ${env_name} to an existing ${home_name} tarball"
    return
  fi
  if [[ "${HARBOR_SKIP_NATIVE_HOME:-0}" = "1" || ! -d "${HOME}/${home_name}" ]]; then
    return
  fi

  local out_dir out_path tmp
  out_dir="${REPO_ROOT}/benchmark-results/bin/native-homes"
  out_path="${out_dir}/${out_name}"
  mkdir -p "${out_dir}"
  tmp="$(mktemp -d)"
  mkdir -p "${tmp}/${home_name}"
  case "${home_name}" in
    .claude)
      for rel in .credentials.json settings.json mcp-needs-auth-cache.json config plugins; do
        if [[ -e "${HOME}/${home_name}/${rel}" ]]; then
          cp -a "${HOME}/${home_name}/${rel}" "${tmp}/${home_name}/"
        fi
      done
      ;;
    .codex)
      for rel in auth.json config.toml version.json rules; do
        if [[ -e "${HOME}/${home_name}/${rel}" ]]; then
          cp -a "${HOME}/${home_name}/${rel}" "${tmp}/${home_name}/"
        fi
      done
      ;;
  esac
  tar -czf "${out_path}" -C "${tmp}" "${home_name}"
  rm -rf "${tmp}"
  export "${env_name}=${out_path}"
}

prepare_claude_package_tarball() {
  ensure_node_tarball
  if [[ -n "${HARBOR_CLAUDE_PACKAGE_TARBALL:-}" ]]; then
    require_file "${HARBOR_CLAUDE_PACKAGE_TARBALL}" "set HARBOR_CLAUDE_PACKAGE_TARBALL to an existing Claude Code package tarball"
    return
  fi
  local version spec
  version="${HARBOR_CLAUDE_VERSION:-$(installed_claude_version)}"
  spec="@anthropic-ai/claude-code${version:+@${version}}"
  export HARBOR_CLAUDE_PACKAGE_TARBALL="$(npm_pack_once "${spec}" "${REPO_ROOT}/benchmark-results/bin/npm-packages" "anthropic-ai-claude-code" "${version}")"
}

prepare_codex_package_tarball() {
  ensure_node_tarball
  if [[ -n "${HARBOR_CODEX_PACKAGE_TARBALL:-}" ]]; then
    require_file "${HARBOR_CODEX_PACKAGE_TARBALL}" "set HARBOR_CODEX_PACKAGE_TARBALL to an existing Codex package tarball"
    return
  fi
  local version spec
  version="${HARBOR_CODEX_VERSION:-$(installed_codex_version)}"
  spec="@openai/codex${version:+@${version}}"
  export HARBOR_CODEX_PACKAGE_TARBALL="$(npm_pack_once "${spec}" "${REPO_ROOT}/benchmark-results/bin/npm-packages" "openai-codex" "${version}")"
}

prepare_pi_harness_artifact() {
  ensure_node_tarball
  if [[ -n "${HARBOR_PI_PACKAGE_TARBALL:-}" ]]; then
    require_file "${HARBOR_PI_PACKAGE_TARBALL}" "set HARBOR_PI_PACKAGE_TARBALL to an existing pi package tarball"
    return
  fi
  local existing version spec
  existing="${REPO_ROOT}/benchmark-results/bin/pi-coding-agent-0.67.1/package.tgz"
  if [[ -f "${existing}" ]]; then
    export HARBOR_PI_PACKAGE_TARBALL="${existing}"
    return
  fi
  version="${HARBOR_PI_VERSION:-0.67.1}"
  spec="@mariozechner/pi-coding-agent@${version}"
  export HARBOR_PI_PACKAGE_TARBALL="$(npm_pack_once "${spec}" "${REPO_ROOT}/benchmark-results/bin/npm-packages" "mariozechner-pi-coding-agent" "${version}")"
}

prepare_opencode_harness_artifact() {
  if [[ -n "${HARBOR_OPENCODE_ARTIFACT:-}" ]]; then
    require_file "${HARBOR_OPENCODE_ARTIFACT}" "set HARBOR_OPENCODE_ARTIFACT to an existing OpenCode Linux binary"
    return
  fi
  local installed_binary fallback
  installed_binary="$(installed_opencode_binary)"
  if [[ -n "${installed_binary}" && -f "${installed_binary}" ]] && file "${installed_binary}" | grep -q "ELF"; then
    export HARBOR_OPENCODE_ARTIFACT="${installed_binary}"
    return
  fi
  fallback="${REPO_ROOT}/benchmark-results/bin/opencode-1.3.17-linux-$(container_node_arch)/opencode"
  if [[ -f "${fallback}" ]]; then
    export HARBOR_OPENCODE_ARTIFACT="${fallback}"
    return
  fi
  echo "OpenCode Linux binary not found. Set HARBOR_OPENCODE_ARTIFACT or install opencode on the host." >&2
  exit 1
}

prepare_agent_runtime_bundle() {
  prepare_claude_package_tarball
  prepare_codex_package_tarball
  prepare_pi_harness_artifact
  prepare_opencode_harness_artifact

  local context_dir image tag container_id tmp_bundle_dir
  context_dir="${REPO_ROOT}/benchmark-results/bin/agent-runtime-context-${CONTAINER_GOARCH}"
  HARBOR_AGENT_RUNTIME_BUNDLE="${REPO_ROOT}/benchmark-results/bin/agent-runtime-linux-${CONTAINER_GOARCH}.tgz"
  image="fizeau/terminalbench-agent-runtime"
  tag="${image}:$(git rev-parse --short HEAD 2>/dev/null || echo local)-${CONTAINER_GOARCH}"

  rm -rf "${context_dir}"
  mkdir -p "${context_dir}"
  cp "${FIZ_ARTIFACT}" "${context_dir}/fiz"
  cp "${HARBOR_OPENCODE_ARTIFACT}" "${context_dir}/opencode"
  cp "${HARBOR_NODE_TARBALL}" "${context_dir}/node.tgz"
  cp "${HARBOR_CLAUDE_PACKAGE_TARBALL}" "${context_dir}/claude-code.tgz"
  cp "${HARBOR_CODEX_PACKAGE_TARBALL}" "${context_dir}/codex.tgz"
  cp "${HARBOR_PI_PACKAGE_TARBALL}" "${context_dir}/pi.tgz"

  echo "Building cached agent runtime image: ${tag}"
  docker build \
    --build-arg "TARGETARCH=${CONTAINER_GOARCH}" \
    -f "${REPO_ROOT}/scripts/benchmark/Dockerfile.agent-runtime" \
    -t "${tag}" \
    "${context_dir}"

  tmp_bundle_dir="$(mktemp -d)"
  container_id="$(docker create "${tag}")"
  docker cp "${container_id}:/installed-agent" "${tmp_bundle_dir}/installed-agent"
  docker rm "${container_id}" >/dev/null
  tar -czf "${HARBOR_AGENT_RUNTIME_BUNDLE}" -C "${tmp_bundle_dir}/installed-agent" .
  rm -rf "${tmp_bundle_dir}"
  export HARBOR_AGENT_RUNTIME_BUNDLE

  prepare_home_tarball "HARBOR_CLAUDE_HOME_TARBALL" ".claude" "claude-home.tgz"
  prepare_home_tarball "HARBOR_CODEX_HOME_TARBALL" ".codex" "codex-home.tgz"
}

ensure_tasks() {
  ensure_harbor
  if find "${TASKS_DIR}" -name task.toml -print -quit 2>/dev/null | grep -q .; then
    return
  fi
  mkdir -p "${TASKS_DIR}"
  echo "Downloading TerminalBench 2.1 tasks into ${TASKS_DIR}"
  harbor download terminal-bench/terminal-bench-2-1 --output-dir "${TASKS_DIR}" --overwrite
}

subset_for_phase() {
  case "$1" in
    canary) echo "${REPO_ROOT}/scripts/benchmark/task-subset-tb21-canary.yaml" ;;
    local-qwen|sonnet-comparison|gpt-comparison) echo "${REPO_ROOT}/scripts/benchmark/task-subset-tb21-full.yaml" ;;
    *) return 1 ;;
  esac
}

phases_to_validate() {
  case "${PHASE}" in
    all)
      printf '%s\n' canary local-qwen sonnet-comparison gpt-comparison ;;
    *)
      printf '%s\n' "${PHASE}" ;;
  esac
}

task_ids_from_subset() {
  awk '/^[[:space:]]*-[[:space:]]*id:/ {print $3}' "$1"
}

resolve_task_dir() {
  local id="$1"
  local direct="${TASKS_DIR}/${id}"
  local named="${TASKS_DIR}/terminal-bench/${id}"
  local matches

  if [[ -f "${direct}/task.toml" ]]; then
    printf '%s\n' "${direct}"
    return
  fi
  if [[ -f "${named}/task.toml" ]]; then
    printf '%s\n' "${named}"
    return
  fi
  matches="$(find "${named}" -mindepth 2 -maxdepth 2 -name task.toml -print 2>/dev/null || true)"
  if [[ "$(printf '%s\n' "${matches}" | sed '/^$/d' | wc -l)" = "1" ]]; then
    dirname "${matches}"
    return
  fi
  return 1
}

validate_tasks() {
  local subset id resolved
  while read -r phase; do
    subset="$(subset_for_phase "${phase}")"
    [[ -f "${subset}" ]] || { echo "missing subset manifest: ${subset}" >&2; exit 1; }
    while read -r id; do
      [[ -n "${id}" ]] || continue
      if ! resolved="$(resolve_task_dir "${id}")"; then
        echo "TB-2.1 task ${id} from ${subset} does not resolve under ${TASKS_DIR}" >&2
        exit 1
      fi
      printf '%s\t%s\t%s\n' "${phase}" "${id}" "${resolved}" >> "${TARGET_TASKS_FILE}"
    done < <(task_ids_from_subset "${subset}")
  done < <(phases_to_validate | sort -u)
}

prepare_env_keys() {
  export OMLX_API_KEY="${OMLX_API_KEY:-local}"
  export VLLM_API_KEY="${VLLM_API_KEY:-local}"
  export RAPID_MLX_API_KEY="${RAPID_MLX_API_KEY:-local}"
  if [[ "${PHASE}" != "local-qwen" && -z "${OPENROUTER_API_KEY:-}" ]]; then
    echo "OPENROUTER_API_KEY is required for selected OpenRouter lanes" >&2
    exit 1
  fi
}

append_optional_sweep_args() {
  local -n out_args="$1"
  out_args+=(--matrix-jobs-managed "${MATRIX_JOBS_MANAGED}")
  if [[ "${FORCE_RERUN}" = "1" ]]; then
    out_args+=(--force-rerun)
  fi
  if [[ -n "${PER_RUN_BUDGET_USD}" ]]; then
    out_args+=(--per-run-budget-usd "${PER_RUN_BUDGET_USD}")
  fi
  if [[ -n "${BUDGET_USD}" ]]; then
    out_args+=(--budget-usd "${BUDGET_USD}")
  fi
}

sweep_args_for_phase() {
  local phase="$1"
  local args=(
    sweep
    --work-dir "${REPO_ROOT}"
    --sweep-plan "${SWEEP_PLAN}"
    --phase "${phase}"
    --tasks-dir "${TASKS_DIR}"
    --out "${OUT}"
  )
  append_optional_sweep_args args
  printf '%q ' "${args[@]}"
}

run_sweep_phase() {
  local phase="$1"
  local args=(
    sweep
    --work-dir "${REPO_ROOT}"
    --sweep-plan "${SWEEP_PLAN}"
    --phase "${phase}"
    --tasks-dir "${TASKS_DIR}"
    --out "${OUT}"
  )
  append_optional_sweep_args args
  "${BENCH_BIN}" "${args[@]}"
}

blocking_canary_failures() {
  python3 - "$OUT" <<'PY'
import json
import pathlib
import sys

root = pathlib.Path(sys.argv[1]) / "canary"
bad = []
for path in root.glob("*/matrix.json"):
    data = json.loads(path.read_text())
    for run in data.get("runs", []):
        invalid = run.get("invalid_class") or ""
        err = run.get("error") or ""
        if invalid in {"invalid_setup", "invalid_auth", "invalid_provider"}:
            bad.append((path.parent.name, run.get("task_id"), run.get("rep"), invalid, err[:180]))
        elif "fiz binary not found" in err:
            bad.append((path.parent.name, run.get("task_id"), run.get("rep"), "invalid_setup", err[:180]))
if bad:
    print("canary blocked: setup/auth/provider failures detected", file=sys.stderr)
    for row in bad[:20]:
        print("\t".join(str(x) for x in row), file=sys.stderr)
    if len(bad) > 20:
        print(f"... {len(bad)-20} more", file=sys.stderr)
    sys.exit(1)
PY
}

print_summary() {
  echo
  echo "TerminalBench 2.1 sweep target"
  echo "  phase:              ${PHASE}"
  echo "  output:             ${OUT}"
  echo "  tasks:              ${TASKS_DIR}"
  echo "  sweep plan:         ${SWEEP_PLAN}"
  echo "  bench runner:       ${BENCH_BIN}"
  echo "  Harbor artifact:    ${HARBOR_AGENT_ARTIFACT}"
  echo "  runtime bundle:     ${HARBOR_AGENT_RUNTIME_BUNDLE}"
  echo "  Docker arch:        ${CONTAINER_GOARCH}"
  echo "  resume command:     scripts/benchmark/run_terminalbench_2_1_sweep.sh --phase ${PHASE} --out ${OUT}"
  echo
  echo "Resolved tasks:"
  awk '{print "  " $1 ": " $2}' "${TARGET_TASKS_FILE}" | sort -u
  echo
  echo "Dry-run plan:"
  "${BENCH_BIN}" sweep \
    --work-dir "${REPO_ROOT}" \
    --sweep-plan "${SWEEP_PLAN}" \
    --phase "${PHASE}" \
    --tasks-dir "${TASKS_DIR}" \
    --out "${OUT}" \
    --matrix-jobs-managed "${MATRIX_JOBS_MANAGED}" \
    --dry-run
}

need docker
need python3
docker info >/dev/null

TARGET_TASKS_FILE="$(mktemp)"
trap 'rm -f "${TARGET_TASKS_FILE}"' EXIT

build_artifacts
prepare_agent_runtime_bundle
ensure_tasks
validate_tasks
prepare_env_keys
print_summary

if [[ "${PREPARE_ONLY}" = "1" || "${DRY_RUN}" = "1" ]]; then
  exit 0
fi

if [[ "${CONFIRM_DELAY}" != "0" ]]; then
  echo
  echo "Starting in ${CONFIRM_DELAY}s. Press Ctrl-C now if these targets are wrong."
  sleep "${CONFIRM_DELAY}"
fi

if [[ "${PHASE}" = "all" ]]; then
  run_sweep_phase canary
  blocking_canary_failures
  run_sweep_phase local-qwen
  run_sweep_phase sonnet-comparison
  run_sweep_phase gpt-comparison
else
  run_sweep_phase "${PHASE}"
fi

echo
echo "Sweep output: ${OUT}"
