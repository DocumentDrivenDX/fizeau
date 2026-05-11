#!/usr/bin/env bash
# One-shot TerminalBench 2.1 sweep runner.
#
# Default:
#   scripts/benchmark/run_terminalbench_2_1_sweep.sh
#
# Common overrides:
#   scripts/benchmark/run_terminalbench_2_1_sweep.sh --phase canary
#   scripts/benchmark/run_terminalbench_2_1_sweep.sh --phase all
#   scripts/benchmark/run_terminalbench_2_1_sweep.sh --dry-run
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
cd "${REPO_ROOT}"

PHASE="all"
LANES=""
FOUR_FULL_LANES="fiz-openai-gpt-5-5,fiz-openrouter-qwen3-6-27b,fiz-sindri-club-3090-qwen3-6-27b,fiz-vidar-omlx-qwen3-6-27b"
OUT=""
TASKS_DIR="${REPO_ROOT}/benchmark-results/external/terminal-bench-2-1"
SWEEP_PLAN="${REPO_ROOT}/scripts/benchmark/terminalbench-2-1-sweep.yaml"
DRY_RUN=0
PREPARE_ONLY=0
FORCE_RERUN=0
MATRIX_JOBS_MANAGED="${BENCHMARK_MATRIX_JOBS_MANAGED:-auto}"
PER_RUN_BUDGET_USD=""
BUDGET_USD=""
CONFIRM_DELAY="${BENCHMARK_CONFIRM_DELAY:-8}"

usage() {
  cat <<'EOF'
Usage: ./benchmark [flags]

Flags:
  --phase canary|openai-cheap|preferred|full|qwen36-gpt55-full|local-qwen|sonnet-comparison|gpt-comparison|tb21-all|all
  --lanes <id,id,...>
  --out <dir>
  --tasks-dir <dir>
  --sweep-plan <file>
  --dry-run
  --prepare-only
  --force-rerun
  --budget-usd <n>
  --per-run-budget-usd <n>
  --matrix-jobs-managed <n|auto>

The script builds local benchmark artifacts, downloads/validates TB-2.1 tasks,
prints the exact target plan, waits briefly for Ctrl-C, then runs the sweep.
`all` means the staged sweep phases; use `full`/`tb21-all` for the 89-task
catalog.

Short lane aliases:
  openai-gpt55, openrouter-qwen36, sindri, vidar
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --phase)
      PHASE="$2"; shift 2 ;;
    --phase=*)
      PHASE="${1#*=}"; shift ;;
    --lanes)
      LANES="$2"; shift 2 ;;
    --lanes=*)
      LANES="${1#*=}"; shift ;;
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
  full) PHASE="tb21-all" ;;
  preferred) PHASE="local-qwen" ;;
  qwen36-gpt55-full)
    PHASE="tb21-all"
    if [[ -z "${LANES}" ]]; then
      LANES="${FOUR_FULL_LANES}"
    fi
    if [[ "${MATRIX_JOBS_MANAGED}" = "1" ]]; then
      MATRIX_JOBS_MANAGED=16
    fi
    ;;
esac

resolve_matrix_jobs_managed() {
  case "${MATRIX_JOBS_MANAGED}" in
    auto|"")
      case "${PHASE}" in
        tb21-all|openai-cheap)
          # Request enough parallelism for managed cloud lanes. The Go sweep
          # runner clamps each lane to its resource-group cap, so local lanes
          # still run one cell at a time.
          MATRIX_JOBS_MANAGED=16
          ;;
        sonnet-comparison|gpt-comparison|all)
          # Harness comparison phases are cloud-backed but heavier and share
          # subscription/provider state; keep their default below raw provider
          # full-sweep concurrency.
          MATRIX_JOBS_MANAGED=5
          ;;
        *)
          MATRIX_JOBS_MANAGED=1
          ;;
      esac
      ;;
    ''|*[!0-9]*)
      echo "--matrix-jobs-managed must be a positive integer or auto, got ${MATRIX_JOBS_MANAGED}" >&2
      exit 2
      ;;
  esac
  if (( MATRIX_JOBS_MANAGED < 1 )); then
    echo "--matrix-jobs-managed must be >= 1" >&2
    exit 2
  fi
}

expand_lane_alias() {
  case "$1" in
    openai-gpt55|gpt55|openai) echo "fiz-openai-gpt-5-5" ;;
    openrouter-qwen36|or-qwen36|qwen36-or) echo "fiz-openrouter-qwen3-6-27b" ;;
    sindri|sindri-vllm) echo "fiz-sindri-club-3090-qwen3-6-27b" ;;
    sindri-llamacpp|sindri-llcpp) echo "fiz-sindri-club-3090-llamacpp-qwen3-6-27b" ;;
    vidar|vidar-omlx) echo "fiz-vidar-omlx-qwen3-6-27b" ;;
    vidar-ds4|ds4) echo "fiz-vidar-ds4" ;;
    *) echo "$1" ;;
  esac
}

normalize_lanes() {
  local raw="$1"
  local out=""
  local item expanded
  IFS=',' read -ra items <<< "${raw}"
  for item in "${items[@]}"; do
    item="${item#"${item%%[![:space:]]*}"}"
    item="${item%"${item##*[![:space:]]}"}"
    if [[ -z "${item}" ]]; then
      continue
    fi
    expanded="$(expand_lane_alias "${item}")"
    if [[ -n "${out}" ]]; then
      out+=","
    fi
    out+="${expanded}"
  done
  echo "${out}"
}

if [[ -n "${LANES}" ]]; then
  LANES="$(normalize_lanes "${LANES}")"
fi

case "${PHASE}" in
  canary|openai-cheap|local-qwen|sonnet-comparison|gpt-comparison|tb21-all|or-passing|timing-baseline|all) ;;
  *)
    echo "unknown --phase ${PHASE}" >&2
    exit 2 ;;
esac

resolve_matrix_jobs_managed

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
  # Canonical root is keyed on fiz_tools_version (agent-behavior identity),
  # not fiz semver. The constant lives in internal/fiztools/version.go; the
  # shell wrapper extracts it so it agrees with cmd/bench's
  # resolveCanonicalFizRoot. Bumping FizToolsVersion in Go routes new sweeps
  # into a fresh canonical root automatically.
  FIZ_TOOLS_VERSION="$(grep -oE 'const Version = [0-9]+' "${REPO_ROOT}/internal/fiztools/version.go" 2>/dev/null | grep -oE '[0-9]+$' || echo 1)"
  OUT="${REPO_ROOT}/benchmark-results/fiz-tools-v${FIZ_TOOLS_VERSION}"
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

  # Operator escape hatch: skip docker rebuild and reuse the existing bundle.
  # Useful when the upstream agent image fails to build (e.g., transitive npm
  # deps with bun installs) but the existing bundle is sufficient. Set
  # SKIP_AGENT_RUNTIME_REBUILD=1 to enable.
  if [[ "${SKIP_AGENT_RUNTIME_REBUILD:-0}" = "1" && -f "${HARBOR_AGENT_RUNTIME_BUNDLE}" ]]; then
    echo "Reusing existing agent runtime bundle: ${HARBOR_AGENT_RUNTIME_BUNDLE} (SKIP_AGENT_RUNTIME_REBUILD=1)"
    export HARBOR_AGENT_RUNTIME_BUNDLE
    prepare_home_tarball "HARBOR_CLAUDE_HOME_TARBALL" ".claude" "claude-home.tgz"
    prepare_home_tarball "HARBOR_CODEX_HOME_TARBALL" ".codex" "codex-home.tgz"
    return
  fi

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
    openai-cheap) echo "${REPO_ROOT}/scripts/benchmark/task-subset-tb21-openai-cheap.yaml" ;;
    local-qwen|sonnet-comparison|gpt-comparison) echo "${REPO_ROOT}/scripts/benchmark/task-subset-tb21-full.yaml" ;;
    tb21-all) echo "${REPO_ROOT}/scripts/benchmark/task-subset-tb21-all.yaml" ;;
    or-passing) echo "${REPO_ROOT}/scripts/benchmark/task-subset-tb21-or-passing.yaml" ;;
    timing-baseline) echo "${REPO_ROOT}/scripts/benchmark/task-subset-tb21-timing-baseline.yaml" ;;
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
if count == 0:
    # Idempotent: task.toml is already mutated (cached preflight reuse).
    if 'docker_image intentionally omitted' in text:
        sys.exit(0)
    raise SystemExit(f"{task}: no docker_image assignment found and not pre-mutated")
task.write_text(new)
PY
}

safe_task_image_name() {
  printf '%s' "$1" | tr -c 'A-Za-z0-9_.-' '-'
}

prepare_local_task_images() {
  local overlay unique_file id src rel dest dockerfile original_image digest digest_short safe_id tag build_args
  # Single canonical preflight overlay shared across sweeps. The mutation we
  # apply (commenting out docker_image so Harbor builds from the bundled
  # Dockerfile) is deterministic on task content, so it's safe to reuse
  # across runs of the same fiz_tools_version. Set
  # BENCHMARK_FORCE_TASK_IMAGE_BUILD=1 to force a rebuild from scratch.
  overlay="${REPO_ROOT}/benchmark-results/external/terminal-bench-2-1-${CONTAINER_GOARCH}-preflight"
  unique_file="$(mktemp)"
  awk -F '\t' '!seen[$2]++ {print $2 "\t" $3}' "${TARGET_TASKS_FILE}" > "${unique_file}"

  if [[ "${BENCHMARK_FORCE_TASK_IMAGE_BUILD:-0}" = "1" ]]; then
    rm -rf "${overlay}"
  fi
  mkdir -p "${overlay}"
  echo "Preflight-building selected TerminalBench task images locally for linux/${CONTAINER_GOARCH}"
  echo "  overlay: ${overlay}"

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
    if [[ -d "${dest}" && -f "${dest}/task.toml" && "${BENCHMARK_FORCE_TASK_IMAGE_BUILD:-0}" != "1" ]]; then
      # Preflight already built for this task content. Skip the cp+mutate;
      # docker build below is a no-op cache hit if image layers exist.
      :
    else
      mkdir -p "$(dirname "${dest}")"
      rm -rf "${dest}"
      cp -a "${src}" "${dest}"
    fi

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
  python3 - "${SWEEP_PLAN}" "${PHASE}" "${key}" "${LANES}" <<'PY'
import sys
from pathlib import Path

import yaml

plan = yaml.safe_load(Path(sys.argv[1]).read_text())
phase_id = sys.argv[2]
key = sys.argv[3]
lane_filter = {item.strip() for item in sys.argv[4].split(",") if item.strip()}

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
if lane_filter:
    selected_lane_ids &= lane_filter
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
  if [[ -n "${LANES}" ]]; then
    out_args+=(--lanes "${LANES}")
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

run_plan_phases() {
  if [[ "${PHASE}" = "all" ]]; then
    printf '%s\n' canary local-qwen sonnet-comparison gpt-comparison
  else
    printf '%s\n' "${PHASE}"
  fi
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

write_version_marker() {
  mkdir -p "${OUT}"
  if [[ -x "${REPO_ROOT}/fiz" ]]; then
    "${REPO_ROOT}/fiz" version --json > "${OUT}/.fiz-benchmark-version.json"
  else
    python3 - "${OUT}/.fiz-benchmark-version.json" <<'PY'
import json
import pathlib
import subprocess
import sys

path = pathlib.Path(sys.argv[1])
version = subprocess.run(["git", "describe", "--tags", "--abbrev=0"], text=True, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL).stdout.strip() or "unknown"
commit = subprocess.run(["git", "rev-parse", "--short", "HEAD"], text=True, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL).stdout.strip()
dirty = subprocess.run(["git", "diff", "--quiet"]).returncode != 0
path.write_text(json.dumps({"version": version, "commit": commit, "dirty": dirty, "built": ""}) + "\n")
PY
  fi
}

fiz_version_label() {
  python3 - "${OUT}/.fiz-benchmark-version.json" <<'PY'
import json
import sys

with open(sys.argv[1]) as f:
    print(json.load(f).get("version") or "unknown")
PY
}

refresh_indexes() {
  # bench matrix-index defaults --fiz-tools-version to internal/fiztools.Version,
  # which is the canonical agent-behavior identity. The fiz semver/commit
  # info from .fiz-benchmark-version.json stays in OUT for run-level
  # provenance but isn't the path key.
  "${BENCH_BIN}" matrix-index \
    --work-dir "${REPO_ROOT}" \
    --root "${OUT}" \
    --out "${OUT}/indexes" \
    --dataset terminal-bench-2-1
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
  if [[ -n "${LANES}" ]]; then
    echo "  lanes:              ${LANES}"
  fi
  echo "  output:             ${OUT}"
  echo "  tasks source:       ${SOURCE_TASKS_DIR}"
  echo "  tasks runtime:      ${TASKS_DIR}"
  echo "  sweep plan:         ${SWEEP_PLAN}"
  echo "  bench runner:       ${BENCH_BIN}"
  echo "  Harbor artifact:    ${HARBOR_AGENT_ARTIFACT}"
  echo "  runtime bundle:     ${HARBOR_AGENT_RUNTIME_BUNDLE}"
  echo "  Docker arch:        ${CONTAINER_GOARCH}"
  echo "  managed jobs:       ${MATRIX_JOBS_MANAGED}"
  if [[ -n "${LANES}" ]]; then
    echo "  resume command:     scripts/benchmark/run_terminalbench_2_1_sweep.sh --phase ${PHASE} --lanes ${LANES} --out ${OUT} --matrix-jobs-managed ${MATRIX_JOBS_MANAGED}"
  else
    echo "  resume command:     scripts/benchmark/run_terminalbench_2_1_sweep.sh --phase ${PHASE} --out ${OUT} --matrix-jobs-managed ${MATRIX_JOBS_MANAGED}"
  fi
  echo
  echo "Resolved tasks:"
  awk '{print "  " $1 ": " $2}' "${TARGET_TASKS_FILE}" | sort -u
  echo
  echo "Dry-run plan:"
  local dry_phase
  while IFS= read -r dry_phase; do
    local dry_args=(
      sweep
      --work-dir "${REPO_ROOT}"
      --sweep-plan "${SWEEP_PLAN}"
      --phase "${dry_phase}"
      --tasks-dir "${TASKS_DIR}"
      --out "${OUT}"
      --matrix-jobs-managed "${MATRIX_JOBS_MANAGED}"
    )
    if [[ -n "${LANES}" ]]; then
      dry_args+=(--lanes "${LANES}")
    fi
    dry_args+=(--dry-run)
    "${BENCH_BIN}" "${dry_args[@]}"
  done < <(run_plan_phases)
}

need docker
need python3
docker info >/dev/null

TARGET_TASKS_FILE="$(mktemp)"
trap 'rm -f "${TARGET_TASKS_FILE}"' EXIT

build_artifacts
write_version_marker
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

while IFS= read -r phase_to_run; do
  run_sweep_phase "${phase_to_run}"
  if [[ "${phase_to_run}" = "canary" && "${PHASE}" = "all" ]]; then
    blocking_canary_failures
  fi
done < <(run_plan_phases)

refresh_indexes

echo
echo "Sweep output: ${OUT}"
