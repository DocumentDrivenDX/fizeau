#!/usr/bin/env bash
set -euo pipefail

# Compare harnesses over TerminalBench.
#
# Defaults run the evidence-grade Vidar/oMLX Qwen3.6 local-model baseline:
#   scripts/benchmark/run_vidar_qwen36_terminalbench_baseline.sh
#
# Faster tiers:
#   TIER=canary scripts/benchmark/run_vidar_qwen36_terminalbench_baseline.sh
#   TIER=wide   scripts/benchmark/run_vidar_qwen36_terminalbench_baseline.sh
#
# Frontier reference cells, one profile per native harness:
#   BASELINE=frontier TIER=canary scripts/benchmark/run_vidar_qwen36_terminalbench_baseline.sh
#
# Useful overrides:
#   REPS=1 FORCE_RERUN=1 OUT=benchmark-results/matrix-my-run ...

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "${ROOT}"

HOST_MACHINE="$(uname -m)"
case "${HOST_MACHINE}" in
  x86_64|amd64)
    HOST_GOARCH="amd64"
    HOST_NODE_ARCH="x64"
    ;;
  aarch64|arm64)
    HOST_GOARCH="arm64"
    HOST_NODE_ARCH="arm64"
    ;;
  *)
    HOST_GOARCH="${HOST_MACHINE}"
    HOST_NODE_ARCH="${HOST_MACHINE}"
    ;;
esac

TIER="${TIER:-full}"
REPS="${REPS:-3}"
JOBS="${JOBS:-1}"
BASELINE="${BASELINE:-local}"
PROFILE="${PROFILE:-vidar-qwen3-6-27b-openai-compat}"
HARNESSES="${HARNESSES:-pi,opencode,fiz}"
OUT="${OUT:-benchmark-results/matrix-${BASELINE}-${TIER}-$(date -u +%Y%m%dT%H%M%SZ)}"
BENCH="${BENCH:-go run ./cmd/bench}"
TASKS_DIR="${TASKS_DIR:-scripts/benchmark/external/terminal-bench-2}"
FIZ_ARTIFACT="${FIZ_ARTIFACT:-benchmark-results/bin/fiz-linux-${HOST_GOARCH}}"

case "${TIER}" in
  canary)
    SUBSET="${SUBSET:-scripts/beadbench/external/termbench-subset-canary.json}"
    ;;
  core)
    SUBSET="${SUBSET:-scripts/beadbench/external/termbench-subset.json}"
    ;;
  wide)
    SUBSET="${SUBSET:-scripts/beadbench/external/termbench-subset-local-wide.json}"
    ;;
  full)
    SUBSET="${SUBSET:-scripts/beadbench/external/termbench-full.json}"
    ;;
  *)
    echo "unknown TIER=${TIER}; use canary, core, wide, or full" >&2
    exit 2
    ;;
esac

need() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "missing required command: $1" >&2
    exit 1
  }
}

need go
need harbor

if [[ ! -d "${TASKS_DIR}" ]]; then
  echo "missing TerminalBench tasks dir: ${TASKS_DIR}" >&2
  echo "run: git submodule update --init ${TASKS_DIR}" >&2
  exit 1
fi

if [[ ! -f "${SUBSET}" ]]; then
  echo "missing subset manifest: ${SUBSET}" >&2
  exit 1
fi

if [[ "${JOBS}" != "1" ]]; then
  echo "warning: JOBS=${JOBS}; JOBS=1 is recommended for a fair single Vidar oMLX baseline" >&2
fi

contains_harness() {
  local needle="$1"
  case ",${HARNESSES}," in
    *",${needle},"*) return 0 ;;
    *) return 1 ;;
  esac
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

host_arch() {
  echo "${HOST_NODE_ARCH}"
}

container_node_arch() {
  # Harbor Docker runs on the host architecture by default. Override only when
  # forcing a non-native container platform.
  echo "${HARBOR_NODE_ARCH:-${HOST_NODE_ARCH}}"
}

ensure_node_tarball() {
  if [[ -n "${HARBOR_NODE_TARBALL:-}" ]]; then
    require_file "${HARBOR_NODE_TARBALL}" "set HARBOR_NODE_TARBALL to an existing Node.js linux tarball"
    return
  fi

  local arch version path url
  arch="$(container_node_arch)"
  version="${HARBOR_NODE_VERSION:-20.19.2}"
  path="benchmark-results/bin/node-v${version}-linux-${arch}.tar.gz"
  if [[ ! -f "${path}" ]]; then
    mkdir -p "$(dirname "${path}")"
    url="https://nodejs.org/dist/v${version}/node-v${version}-linux-${arch}.tar.gz"
    echo "Preparing Node.js for Harbor native CLI install: ${url}"
    curl -fsSL "${url}" -o "${path}"
  fi
  export HARBOR_NODE_TARBALL="${ROOT}/${path}"
}

npm_pack_once() {
  local package_spec="$1"
  local out_dir="$2"
  local match_prefix="$3"
  mkdir -p "${out_dir}"
  local existing
  existing="$(find "${out_dir}" -maxdepth 1 -type f -name "${match_prefix}-*.tgz" | sort | tail -n 1 || true)"
  if [[ -n "${existing}" ]]; then
    printf '%s\n' "${ROOT}/${existing}"
    return
  fi
  echo "Packing ${package_spec} for Harbor install" >&2
  npm pack "${package_spec}" --pack-destination "${out_dir}" >/dev/null
  existing="$(find "${out_dir}" -maxdepth 1 -type f -name "${match_prefix}-*.tgz" | sort | tail -n 1 || true)"
  if [[ -z "${existing}" ]]; then
    echo "npm pack did not create ${match_prefix}-*.tgz in ${out_dir}" >&2
    exit 1
  fi
  printf '%s\n' "${ROOT}/${existing}"
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
  if [[ "${HARBOR_SKIP_NATIVE_HOME:-0}" == "1" ]]; then
    return
  fi
  if [[ ! -d "${HOME}/${home_name}" ]]; then
    return
  fi

  local out_dir out_path
  out_dir="benchmark-results/bin/native-homes"
  out_path="${out_dir}/${out_name}"
  mkdir -p "${out_dir}"
  local tmp
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
    *)
      cp -a "${HOME}/${home_name}/." "${tmp}/${home_name}/"
      ;;
  esac
  tar -czf "${out_path}" -C "${tmp}" "${home_name}"
  rm -rf "${tmp}"
  export "${env_name}=${ROOT}/${out_path}"
}

prepare_claude_artifact() {
  local default_artifact="benchmark-results/bin/claude-linux-${HOST_GOARCH}/claude"
  if [[ -n "${HARBOR_CLAUDE_ARTIFACT:-}" ]]; then
    require_file "${HARBOR_CLAUDE_ARTIFACT}" "set HARBOR_CLAUDE_ARTIFACT to an existing Claude Code binary"
    prepare_home_tarball "HARBOR_CLAUDE_HOME_TARBALL" ".claude" "claude-home.tgz"
    return
  fi
  if [[ -f "${default_artifact}" ]]; then
    export HARBOR_CLAUDE_ARTIFACT="${ROOT}/${default_artifact}"
    prepare_home_tarball "HARBOR_CLAUDE_HOME_TARBALL" ".claude" "claude-home.tgz"
    return
  fi
  if [[ "${HARBOR_USE_INSTALLED_CLAUDE_BINARY:-0}" == "1" ]] && command -v claude >/dev/null 2>&1 && [[ "$(host_arch)" == "$(container_node_arch)" ]]; then
    export HARBOR_CLAUDE_ARTIFACT="$(readlink -f "$(command -v claude)")"
    prepare_home_tarball "HARBOR_CLAUDE_HOME_TARBALL" ".claude" "claude-home.tgz"
    return
  fi

  need npm
  ensure_node_tarball
  local version spec
  version="${HARBOR_CLAUDE_VERSION:-$(installed_claude_version)}"
  spec="@anthropic-ai/claude-code${version:+@${version}}"
  export HARBOR_CLAUDE_PACKAGE_TARBALL="${HARBOR_CLAUDE_PACKAGE_TARBALL:-$(npm_pack_once "${spec}" "benchmark-results/bin/npm-packages" "anthropic-ai-claude-code")}"
  prepare_home_tarball "HARBOR_CLAUDE_HOME_TARBALL" ".claude" "claude-home.tgz"
}

prepare_codex_artifact() {
  local default_artifact="benchmark-results/bin/codex-linux-${HOST_GOARCH}/codex"
  if [[ -n "${HARBOR_CODEX_ARTIFACT:-}" ]]; then
    require_file "${HARBOR_CODEX_ARTIFACT}" "set HARBOR_CODEX_ARTIFACT to an existing Codex binary"
    prepare_home_tarball "HARBOR_CODEX_HOME_TARBALL" ".codex" "codex-home.tgz"
    return
  fi
  if [[ -f "${default_artifact}" ]]; then
    export HARBOR_CODEX_ARTIFACT="${ROOT}/${default_artifact}"
    prepare_home_tarball "HARBOR_CODEX_HOME_TARBALL" ".codex" "codex-home.tgz"
    return
  fi
  if command -v codex >/dev/null 2>&1 && [[ "$(host_arch)" == "$(container_node_arch)" ]]; then
    export HARBOR_CODEX_ARTIFACT="$(readlink -f "$(command -v codex)")"
    prepare_home_tarball "HARBOR_CODEX_HOME_TARBALL" ".codex" "codex-home.tgz"
    return
  fi

  need npm
  ensure_node_tarball
  local version spec
  version="${HARBOR_CODEX_VERSION:-$(installed_codex_version)}"
  spec="@openai/codex${version:+@${version}}"
  export HARBOR_CODEX_PACKAGE_TARBALL="${HARBOR_CODEX_PACKAGE_TARBALL:-$(npm_pack_once "${spec}" "benchmark-results/bin/npm-packages" "openai-codex")}"
  prepare_home_tarball "HARBOR_CODEX_HOME_TARBALL" ".codex" "codex-home.tgz"
}

if contains_harness "opencode" && [[ -z "${HARBOR_OPENCODE_ARTIFACT:-}" ]]; then
  require_file \
    "benchmark-results/bin/opencode-1.3.17-linux-x64/opencode" \
    "set HARBOR_OPENCODE_ARTIFACT or prepare the pinned opencode artifact; see scripts/benchmark/README.md"
fi

if contains_harness "pi"; then
  if [[ -z "${HARBOR_NODE_TARBALL:-}" ]]; then
    require_file \
      "benchmark-results/bin/node-v20.19.2-linux-x64.tar.gz" \
      "set HARBOR_NODE_TARBALL or prepare the pinned Node artifact; see scripts/benchmark/README.md"
  fi
  if [[ -z "${HARBOR_PI_PACKAGE_TARBALL:-}" ]]; then
    require_file \
      "benchmark-results/bin/pi-coding-agent-0.67.1/package.tgz" \
      "set HARBOR_PI_PACKAGE_TARBALL or prepare the pinned pi package artifact; see scripts/benchmark/README.md"
  fi
fi

if contains_harness "claude"; then
  prepare_claude_artifact
fi

if contains_harness "codex"; then
  prepare_codex_artifact
fi

# Some OpenAI-compatible clients require a non-empty key even for local oMLX.
export OMLX_API_KEY="${OMLX_API_KEY:-local}"

mkdir -p "$(dirname "${FIZ_ARTIFACT}")"
rm -f "${FIZ_ARTIFACT}"
GOOS="${GOOS:-linux}" GOARCH="${GOARCH:-${HOST_GOARCH}}" go build -buildvcs=false -o "${FIZ_ARTIFACT}" ./cmd/fiz
export HARBOR_AGENT_ARTIFACT="${HARBOR_AGENT_ARTIFACT:-${ROOT}/${FIZ_ARTIFACT}}"

run_matrix() {
  local harnesses="$1"
  local profiles="$2"
  local out_dir="$3"
  local matrix_args=(
    matrix
    --profiles="${profiles}"
    --harnesses="${harnesses}"
    --reps="${REPS}"
    --subset="${SUBSET}"
    --tasks-dir="${TASKS_DIR}"
    --out="${out_dir}"
    --jobs="${JOBS}"
  )

  if [[ "${FORCE_RERUN:-0}" == "1" ]]; then
    matrix_args+=(--force-rerun)
  else
    matrix_args+=(--resume)
  fi

  if [[ -n "${PER_RUN_BUDGET_USD:-}" ]]; then
    matrix_args+=(--per-run-budget-usd="${PER_RUN_BUDGET_USD}")
  fi

  if [[ -n "${BUDGET_USD:-}" ]]; then
    matrix_args+=(--budget-usd="${BUDGET_USD}")
  fi

  echo "Running TerminalBench matrix"
  echo "  baseline:  ${BASELINE}"
  echo "  tier:      ${TIER}"
  echo "  subset:    ${SUBSET}"
  echo "  profile:   ${profiles}"
  echo "  harnesses: ${harnesses}"
  echo "  reps:      ${REPS}"
  echo "  jobs:      ${JOBS}"
  echo "  out:       ${out_dir}"
  echo

  ${BENCH} "${matrix_args[@]}"
}

case "${BASELINE}" in
  local)
    run_matrix "${HARNESSES}" "${PROFILE}" "${OUT}"
    ;;
  frontier)
    # Native Claude/Codex reference cells deliberately do not share a model.
    # Run them as paired one-cell invocations so the matrix does not produce
    # meaningless claude×codex-profile cross-products.
    run_matrix "claude" "${CLAUDE_PROFILE:-claude-native-sonnet-4-6}" "${OUT}"
    run_matrix "codex" "${CODEX_PROFILE:-codex-native-gpt-5-4-mini}" "${OUT}"
    rm -f "${OUT}/matrix.json"
    ;;
  *)
    echo "unknown BASELINE=${BASELINE}; use local or frontier" >&2
    exit 2
    ;;
esac
${BENCH} matrix-aggregate "${OUT}"

echo
echo "Wrote:"
echo "  ${OUT}/matrix.json"
echo "  ${OUT}/matrix.md"
echo "  ${OUT}/costs.json"
