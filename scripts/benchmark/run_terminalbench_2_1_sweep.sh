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
TASKS_DIR="${REPO_ROOT}/benchmark-results/external/terminal-bench-2-1"
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
  --phase canary|local-qwen|sonnet-comparison|gpt-comparison|tb21-all|all
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
  canary|local-qwen|sonnet-comparison|gpt-comparison|tb21-all|all) ;;
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
SOURCE_TASKS_DIR="${TASKS_DIR}"
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
  if [[ -n "${BENCHMARK_CONTAINER_GOARCH:-}" ]]; then
    goarch_from_machine "${BENCHMARK_CONTAINER_GOARCH}"
    return
  fi
  if [[ -n "${HARBOR_CONTAINER_GOARCH:-}" ]]; then
    goarch_from_machine "${HARBOR_CONTAINER_GOARCH}"
    return
  fi

  local arch
  arch="$(docker info --format '{{.Architecture}}' 2>/dev/null || true)"
  if [[ -z "${arch}" ]]; then
    arch="$(uname -m)"
  fi
  goarch_from_machine "${arch}"
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

prepare_agent_runtime_bundle() {
  local context_dir image tag container_id tmp_bundle_dir node_version claude_version codex_version pi_version opencode_version
  context_dir="${REPO_ROOT}/benchmark-results/bin/agent-runtime-context-${CONTAINER_GOARCH}"
  HARBOR_AGENT_RUNTIME_BUNDLE="${REPO_ROOT}/benchmark-results/bin/agent-runtime-linux-${CONTAINER_GOARCH}.tgz"
  image="fizeau/terminalbench-agent-runtime"
  tag="${image}:$(git rev-parse --short HEAD 2>/dev/null || echo local)-${CONTAINER_GOARCH}"
  node_version="${HARBOR_NODE_VERSION:-20.19.2}"
  claude_version="${HARBOR_CLAUDE_VERSION:-$(installed_claude_version)}"
  codex_version="${HARBOR_CODEX_VERSION:-$(installed_codex_version)}"
  pi_version="${HARBOR_PI_VERSION:-0.67.1}"
  opencode_version="${HARBOR_OPENCODE_VERSION:-1.3.17}"

  rm -rf "${context_dir}"
  mkdir -p "${context_dir}"
  cp "${FIZ_ARTIFACT}" "${context_dir}/fiz"

  echo "Building cached agent runtime image: ${tag}"
  docker build \
    --platform "linux/${CONTAINER_GOARCH}" \
    --build-arg "TARGETARCH=${CONTAINER_GOARCH}" \
    --build-arg "NODE_VERSION=${node_version}" \
    --build-arg "CLAUDE_CODE_VERSION=${claude_version}" \
    --build-arg "CODEX_VERSION=${codex_version}" \
    --build-arg "PI_VERSION=${pi_version}" \
    --build-arg "OPENCODE_VERSION=${opencode_version}" \
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
    tb21-all) echo "${REPO_ROOT}/scripts/benchmark/task-subset-tb21-all.yaml" ;;
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

task_docker_image() {
  python3 - "$1" <<'PY'
import pathlib
import sys
import tomllib

task = pathlib.Path(sys.argv[1])
data = tomllib.loads(task.read_text())
environment = data.get("environment") if isinstance(data.get("environment"), dict) else {}
image = data.get("docker_image") or environment.get("docker_image")
if not isinstance(image, str) or not image:
    raise SystemExit(f"{task}: missing docker_image")
print(image)
PY
}

remove_task_docker_image() {
  python3 - "$1" <<'PY'
import pathlib
import re
import sys

task = pathlib.Path(sys.argv[1])
text = task.read_text()
new, count = re.subn(
    r'(?m)^docker_image\s*=\s*"[^"]*"\s*$',
    '# docker_image intentionally omitted; Harbor builds environment/Dockerfile locally.',
    text,
    count=1,
)
if count != 1:
    raise SystemExit(f"{task}: expected exactly one docker_image assignment")
task.write_text(new)
PY
}

safe_task_image_name() {
  printf '%s' "$1" | tr -c 'A-Za-z0-9_.-' '-'
}

prepare_local_task_images() {
  local overlay unique_file id src rel dest dockerfile original_image digest digest_short safe_id tag build_args
  overlay="${OUT}/task-images/terminal-bench-2-1-${CONTAINER_GOARCH}"
  unique_file="$(mktemp)"
  awk -F '\t' '!seen[$2]++ {print $2 "\t" $3}' "${TARGET_TASKS_FILE}" > "${unique_file}"

  rm -rf "${overlay}"
  mkdir -p "${overlay}"
  echo "Preflight-building selected TerminalBench task images locally for linux/${CONTAINER_GOARCH}"

  while IFS=$'\t' read -r id src; do
    [[ -n "${id}" ]] || continue
    dockerfile="${src}/environment/Dockerfile"
    if [[ ! -f "${dockerfile}" ]]; then
      echo "TB-2.1 task ${id} has no environment/Dockerfile; refusing to fall back to registry image" >&2
      echo "source task: ${src}" >&2
      rm -f "${unique_file}"
      exit 1
    fi

    rel="${src#${SOURCE_TASKS_DIR}/}"
    if [[ "${rel}" = "${src}" ]]; then
      echo "internal error: task ${id} did not resolve under ${SOURCE_TASKS_DIR}: ${src}" >&2
      rm -f "${unique_file}"
      exit 1
    fi
    dest="${overlay}/${rel}"
    mkdir -p "$(dirname "${dest}")"
    rm -rf "${dest}"
    cp -a "${src}" "${dest}"

    original_image="$(task_docker_image "${src}/task.toml")"
    digest="$(basename "${src}")"
    digest_short="${digest:0:12}"
    safe_id="$(safe_task_image_name "${id}")"
    tag="fizeau/tb21-preflight-${safe_id}:${digest_short}-${CONTAINER_GOARCH}"
    build_args=(
      build
      --platform "linux/${CONTAINER_GOARCH}"
      -t "${tag}"
      -f "${dockerfile}"
    )
    if [[ "${BENCHMARK_FORCE_TASK_IMAGE_BUILD:-0}" = "1" ]]; then
      build_args+=(--no-cache)
    fi
    build_args+=("${src}/environment")

    echo "  ${id}: ${original_image} -> local Dockerfile build cache (${tag})"
    docker "${build_args[@]}"
    remove_task_docker_image "${dest}/task.toml"
  done < "${unique_file}"

  rm -f "${unique_file}"
  TASKS_DIR="${overlay}"
  export DOCKER_DEFAULT_PLATFORM="linux/${CONTAINER_GOARCH}"
}

prepare_env_keys() {
  export OMLX_API_KEY="${OMLX_API_KEY:-local}"
  export VLLM_API_KEY="${VLLM_API_KEY:-local}"
  export RAPID_MLX_API_KEY="${RAPID_MLX_API_KEY:-local}"
  if [[ -z "${OPENROUTER_API_KEY:-}" ]] && selected_plan_requires_key "OPENROUTER_API_KEY"; then
    echo "OPENROUTER_API_KEY is required for selected OpenRouter lanes" >&2
    exit 1
  fi
  if [[ -z "${OPENAI_API_KEY:-}" ]] && selected_plan_requires_key "OPENAI_API_KEY"; then
    echo "OPENAI_API_KEY is required for selected OpenAI lanes" >&2
    exit 1
  fi
}

selected_plan_requires_key() {
  local key="$1"
  python3 - "${SWEEP_PLAN}" "${PHASE}" "${key}" <<'PY'
import sys
from pathlib import Path

import yaml

plan = yaml.safe_load(Path(sys.argv[1]).read_text())
phase_id = sys.argv[2]
key = sys.argv[3]

phases = plan.get("phases") or []
if phase_id == "all":
    selected_phases = phases
else:
    selected_phases = [p for p in phases if p.get("id") == phase_id]

selected_lane_ids = {
    lane_id
    for phase in selected_phases
    for lane_id in (phase.get("lanes") or [])
}
lanes = plan.get("lanes") or []
for lane in lanes:
    if lane.get("id") not in selected_lane_ids:
        continue
    env = lane.get("fizeau_env") or {}
    if env.get("FIZEAU_API_KEY_ENV") == key:
        raise SystemExit(0)
raise SystemExit(1)
PY
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
  echo "  tasks source:       ${SOURCE_TASKS_DIR}"
  echo "  tasks runtime:      ${TASKS_DIR}"
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
prepare_local_task_images
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
