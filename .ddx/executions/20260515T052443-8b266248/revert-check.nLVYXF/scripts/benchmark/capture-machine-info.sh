#!/usr/bin/env bash
# capture-machine-info.sh — emit a YAML block describing this inference host.
#
# Auto-detects the running serving engine (vLLM, oMLX, RapidMLX, LM Studio,
# Ollama, …) by scanning processes and probing common OpenAI-compatible
# endpoints, then dumps hardware + OS + network details so the result can be
# pasted under the host's key in scripts/benchmark/machines.yaml.
#
# Output is a single YAML block on stdout, keyed by hostname. Errors and
# diagnostics go to stderr. Exit 0 even when engine detection fails — partial
# data is still useful.
#
# Prereqs (best-effort; missing tools are silently skipped):
#   - bash 4+, awk, sed, tr, hostname, uname
#   - curl, jq          (for endpoint probing)
#   - nvidia-smi        (Linux + NVIDIA)
#   - lscpu, /proc      (Linux)
#   - system_profiler   (macOS)
#   - sysctl            (macOS / *BSD)
#   - tailscale         (optional, for tailnet IP)
#
# Usage:
#   ./capture-machine-info.sh                       # YAML to stdout
#   ./capture-machine-info.sh > /tmp/me.yaml        # save and copy/paste
#   ./capture-machine-info.sh --ports 8020,1234     # extra endpoints to probe
#   ./capture-machine-info.sh --help

set -u

# ---------------------------------------------------------------------------
# args
# ---------------------------------------------------------------------------
EXTRA_PORTS=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    -h|--help)
      sed -n '2,27p' "$0" | sed 's/^# \{0,1\}//'
      exit 0
      ;;
    --ports)
      EXTRA_PORTS="$2"
      shift 2
      ;;
    *)
      echo "unknown arg: $1" >&2
      exit 2
      ;;
  esac
done

# ---------------------------------------------------------------------------
# helpers
# ---------------------------------------------------------------------------
log() { printf '[capture] %s\n' "$*" >&2; }
have() { command -v "$1" >/dev/null 2>&1; }

# Coerce a value to a non-negative integer for YAML scalar fields. Empty,
# non-numeric, or negative inputs become 0 so the emitted YAML always parses.
intval() {
  local v="${1-}"
  if [[ "$v" =~ ^[0-9]+$ ]]; then
    printf '%s' "$v"
  else
    printf '0'
  fi
}

# YAML-safe scalar: wraps anything non-trivial in double quotes and escapes.
yq() {
  local v="${1-}"
  if [[ -z "$v" ]]; then
    printf '""'
    return
  fi
  # escape backslashes and double quotes
  v="${v//\\/\\\\}"
  v="${v//\"/\\\"}"
  # collapse newlines/tabs into spaces
  v="$(printf '%s' "$v" | tr '\n\t' '  ')"
  printf '"%s"' "$v"
}

# Indent every line by N spaces.
indent() {
  local n="$1"
  local pad
  pad="$(printf '%*s' "$n" '')"
  sed "s/^/${pad}/"
}

# Try a curl with short timeouts. Echo body on success, empty on failure.
probe() {
  local url="$1"
  curl -fsS --max-time 2 --connect-timeout 1 "$url" 2>/dev/null || true
}

# ---------------------------------------------------------------------------
# constants
# ---------------------------------------------------------------------------
HOST="$(hostname 2>/dev/null | awk -F. '{print $1}')"
[[ -z "$HOST" ]] && HOST="unknown"
HOST="$(printf '%s' "$HOST" | tr '[:upper:]' '[:lower:]')"

OS_KIND="$(uname -s 2>/dev/null || echo unknown)"
SNAP_TS="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

DEFAULT_PROBE_PORTS=(8000 8020 11434 1234 1235 5000 8080)
if [[ -n "$EXTRA_PORTS" ]]; then
  IFS=',' read -r -a EXTRA_ARR <<<"$EXTRA_PORTS"
  DEFAULT_PROBE_PORTS=("${EXTRA_ARR[@]}" "${DEFAULT_PROBE_PORTS[@]}")
fi

# ---------------------------------------------------------------------------
# OS / CPU / memory
# ---------------------------------------------------------------------------
OS_PRETTY=""
OS_RELEASE=""
KERNEL=""
CPU_MODEL=""
CPU_CORES=""
CPU_SOCKETS=""
MEM_GB=""

KERNEL="$(uname -sr 2>/dev/null || true)"

if [[ "$OS_KIND" == "Linux" ]]; then
  if [[ -r /etc/os-release ]]; then
    # shellcheck disable=SC1091
    OS_PRETTY="$(. /etc/os-release; printf '%s' "${PRETTY_NAME:-}")"
    OS_RELEASE="$(. /etc/os-release; printf '%s' "${VERSION_ID:-}")"
  fi
  if have lscpu; then
    CPU_MODEL="$(lscpu 2>/dev/null | awk -F: '/^Model name/{sub(/^ +/,"",$2); print $2; exit}')"
    CPU_CORES="$(lscpu 2>/dev/null | awk -F: '/^CPU\(s\)/{gsub(/ /,"",$2); print $2; exit}')"
    CPU_SOCKETS="$(lscpu 2>/dev/null | awk -F: '/^Socket\(s\)/{gsub(/ /,"",$2); print $2; exit}')"
  fi
  if [[ -z "$CPU_MODEL" && -r /proc/cpuinfo ]]; then
    CPU_MODEL="$(awk -F: '/model name/{sub(/^ +/,"",$2); print $2; exit}' /proc/cpuinfo)"
    CPU_CORES="$(grep -c ^processor /proc/cpuinfo 2>/dev/null || echo "")"
  fi
  if [[ -r /proc/meminfo ]]; then
    local_kb="$(awk '/^MemTotal:/{print $2; exit}' /proc/meminfo)"
    if [[ -n "$local_kb" ]]; then
      MEM_GB="$(awk -v k="$local_kb" 'BEGIN{printf "%.0f", k/1024/1024}')"
    fi
  fi
elif [[ "$OS_KIND" == "Darwin" ]]; then
  OS_PRETTY="macOS $(sw_vers -productVersion 2>/dev/null || echo)"
  OS_RELEASE="$(sw_vers -buildVersion 2>/dev/null || echo)"
  if have sysctl; then
    CPU_MODEL="$(sysctl -n machdep.cpu.brand_string 2>/dev/null || true)"
    CPU_CORES="$(sysctl -n hw.ncpu 2>/dev/null || true)"
    CPU_SOCKETS="$(sysctl -n hw.packages 2>/dev/null || echo 1)"
    mem_bytes="$(sysctl -n hw.memsize 2>/dev/null || true)"
    if [[ -n "$mem_bytes" ]]; then
      MEM_GB="$(awk -v b="$mem_bytes" 'BEGIN{printf "%.0f", b/1024/1024/1024}')"
    fi
  fi
fi

# ---------------------------------------------------------------------------
# GPU (NVIDIA + Apple)
# ---------------------------------------------------------------------------
GPU_VENDOR=""
GPU_MODEL=""
GPU_VRAM_MB=""
GPU_DRIVER=""
GPU_RUNTIME=""        # CUDA / ROCm / MLX
GPU_RUNTIME_VERSION=""
GPU_UTIL_PCT=""
GPU_MEM_USED_MB=""

if have nvidia-smi; then
  GPU_VENDOR="NVIDIA"
  if line="$(nvidia-smi --query-gpu=name,memory.total,driver_version,utilization.gpu,memory.used --format=csv,noheader,nounits 2>/dev/null | head -n1)"; then
    if [[ -n "$line" ]]; then
      IFS=',' read -r g_name g_mem g_drv g_util g_used <<<"$line"
      GPU_MODEL="$(printf '%s' "$g_name" | sed 's/^ *//;s/ *$//')"
      GPU_VRAM_MB="$(printf '%s' "$g_mem" | tr -d ' ')"
      GPU_DRIVER="$(printf '%s' "$g_drv" | tr -d ' ')"
      GPU_UTIL_PCT="$(printf '%s' "$g_util" | tr -d ' ')"
      GPU_MEM_USED_MB="$(printf '%s' "$g_used" | tr -d ' ')"
    fi
  fi
  if have nvcc; then
    GPU_RUNTIME="CUDA"
    GPU_RUNTIME_VERSION="$(nvcc --version 2>/dev/null | awk '/release/{for(i=1;i<=NF;i++)if($i=="release"){print $(i+1); exit}}' | tr -d ',')"
  else
    nv_cuda="$(nvidia-smi 2>/dev/null | awk '/CUDA Version/{for(i=1;i<=NF;i++)if($i=="CUDA"){print $(i+2); exit}}')"
    if [[ -n "$nv_cuda" ]]; then
      GPU_RUNTIME="CUDA"
      GPU_RUNTIME_VERSION="$nv_cuda"
    fi
  fi
elif [[ "$OS_KIND" == "Darwin" ]]; then
  GPU_VENDOR="Apple"
  if have system_profiler; then
    GPU_MODEL="$(system_profiler SPDisplaysDataType 2>/dev/null | awk -F: '/Chipset Model/{sub(/^ +/,"",$2); print $2; exit}')"
  fi
  if [[ -z "$GPU_MODEL" && -n "$CPU_MODEL" ]]; then
    GPU_MODEL="$CPU_MODEL (integrated)"
  fi
  GPU_RUNTIME="MLX/Metal"
  if have xcrun; then
    GPU_RUNTIME_VERSION="$(xcrun metal --version 2>/dev/null | head -n1 || true)"
  fi
elif have rocminfo; then
  GPU_VENDOR="AMD"
  GPU_MODEL="$(rocminfo 2>/dev/null | awk -F: '/Marketing Name/{sub(/^ +/,"",$2); print $2; exit}')"
  GPU_RUNTIME="ROCm"
  if have hipconfig; then
    GPU_RUNTIME_VERSION="$(hipconfig --version 2>/dev/null | head -n1)"
  fi
fi

# ---------------------------------------------------------------------------
# Network
# ---------------------------------------------------------------------------
TAILSCALE_IP=""
if have tailscale; then
  TAILSCALE_IP="$(tailscale ip -4 2>/dev/null | head -n1 || true)"
fi
PUBLIC_IP=""
# don't fetch public IP by default — opt in with FETCH_PUBLIC_IP=1
if [[ "${FETCH_PUBLIC_IP:-0}" == "1" ]]; then
  PUBLIC_IP="$(probe https://api.ipify.org)"
fi
PRIMARY_IP=""
if [[ "$OS_KIND" == "Linux" ]] && have ip; then
  PRIMARY_IP="$(ip -o -4 route get 1.1.1.1 2>/dev/null | awk '{for(i=1;i<=NF;i++)if($i=="src"){print $(i+1); exit}}')"
elif [[ "$OS_KIND" == "Darwin" ]] && have ipconfig; then
  PRIMARY_IP="$(ipconfig getifaddr en0 2>/dev/null || true)"
fi

# ---------------------------------------------------------------------------
# Engine detection
# ---------------------------------------------------------------------------
ENGINE_NAME="unknown"
ENGINE_VERSION=""
ENGINE_PORT=""
ENGINE_ENDPOINT=""
ENGINE_PID=""

MODEL_ID=""
MODEL_QUANT=""
MODEL_WEIGHTS_PATH=""
MODEL_CONTEXT=""
SAMPLING_TEMP=""
SAMPLING_TOP_P=""
SAMPLING_TOP_K=""
SAMPLING_REP_PEN=""
SAMPLING_MAX_TOKENS=""
SERVER_KV_CACHE=""
SERVER_MAX_BATCH=""
SERVER_MAX_SEQ=""

# 1) process scan — match argv[0] / executable, not env var values.
# Use `ps -eo pid,comm,args` and grep argv[0] to avoid false positives where
# strings like "vllm" appear inside HARBOR_INSTRUCTION or FIZEAU_PROVIDER.
detect_engine_proc() {
  local name="$1" pat="$2"
  ps -eo pid=,comm=,args= 2>/dev/null | awk -v pat="$pat" '
    {
      pid=$1; comm=$2;
      # argv[0] is the third whitespace-separated field of `args=`
      argv0=$3;
      if (comm ~ pat || argv0 ~ pat) { print pid; exit }
    }'
}

if have ps; then
  pid="$(detect_engine_proc vllm '^([^ ]*\/)?(vllm|python[0-9.]*)$')"
  # python alone is too broad — require a vllm argv anywhere in cmdline of *that* pid
  if [[ -n "$pid" ]]; then
    if ps -p "$pid" -o args= 2>/dev/null | grep -qE '(^|/)vllm( |$)|vllm\.entrypoints'; then
      ENGINE_NAME="vllm"; ENGINE_PID="$pid"
      if have vllm; then
        ENGINE_VERSION="$(vllm --version 2>/dev/null | awk '{print $NF; exit}')"
      fi
    fi
  fi
  if [[ "$ENGINE_NAME" == "unknown" ]]; then
    pid="$(detect_engine_proc mlx '(mlx[-_]server|mlx_lm\.server|omlx|rapidmlx)$')"
    if [[ -n "$pid" ]]; then
      ENGINE_NAME="mlx"; ENGINE_PID="$pid"
    fi
  fi
  if [[ "$ENGINE_NAME" == "unknown" ]]; then
    pid="$(detect_engine_proc ollama '(^|/)ollama$')"
    if [[ -n "$pid" ]]; then
      ENGINE_NAME="ollama"; ENGINE_PID="$pid"
      if have ollama; then
        ENGINE_VERSION="$(ollama --version 2>/dev/null | awk '{print $NF; exit}')"
      fi
    fi
  fi
  if [[ "$ENGINE_NAME" == "unknown" ]]; then
    pid="$(detect_engine_proc lmstudio '(lm[-_]studio|lmstudio|LM Studio)')"
    if [[ -n "$pid" ]]; then
      ENGINE_NAME="lmstudio"; ENGINE_PID="$pid"
    fi
  fi
fi

# 2) HTTP probe — pick the first port with /v1/models reachable.
for port in "${DEFAULT_PROBE_PORTS[@]}"; do
  body="$(probe "http://127.0.0.1:${port}/v1/models")"
  if [[ -n "$body" ]]; then
    ENGINE_PORT="$port"
    ENGINE_ENDPOINT="http://127.0.0.1:${port}"
    if have jq; then
      MODEL_ID="$(printf '%s' "$body" | jq -r '.data[0].id // empty' 2>/dev/null)"
    fi
    # try to refine engine label via /v1/server-info or /api/version
    info="$(probe "http://127.0.0.1:${port}/v1/server-info")"
    if [[ -n "$info" && "$ENGINE_NAME" == "unknown" ]]; then
      if printf '%s' "$info" | grep -qi vllm; then ENGINE_NAME="vllm"; fi
      if printf '%s' "$info" | grep -qi mlx;  then ENGINE_NAME="mlx";  fi
    fi
    ver="$(probe "http://127.0.0.1:${port}/api/version")"
    if [[ -n "$ver" && -z "$ENGINE_VERSION" ]]; then
      ENGINE_VERSION="$(printf '%s' "$ver" | jq -r '.version // empty' 2>/dev/null)"
      [[ -z "$ENGINE_VERSION" ]] && ENGINE_VERSION="$(printf '%s' "$ver" | head -c 80)"
    fi
    # Ollama-style /api/tags also signals an engine
    if [[ "$ENGINE_NAME" == "unknown" ]]; then
      tags="$(probe "http://127.0.0.1:${port}/api/tags")"
      if [[ -n "$tags" ]]; then ENGINE_NAME="ollama"; fi
    fi
    break
  fi
done

# 3) Pull richer details from /v1/models entry if jq available
if [[ -n "$MODEL_ID" && -n "$ENGINE_ENDPOINT" ]] && have jq; then
  body="$(probe "${ENGINE_ENDPOINT}/v1/models")"
  if [[ -n "$body" ]]; then
    MODEL_CONTEXT="$(printf '%s' "$body" | jq -r '.data[0].max_model_len // .data[0].context_length // empty' 2>/dev/null)"
    MODEL_QUANT="$(printf '%s' "$body" | jq -r '.data[0].quantization // .data[0].quant // empty' 2>/dev/null)"
    MODEL_WEIGHTS_PATH="$(printf '%s' "$body" | jq -r '.data[0].path // .data[0].root // empty' 2>/dev/null)"
  fi
fi

# 4) infer quant label from model id (common suffix patterns)
if [[ -z "$MODEL_QUANT" && -n "$MODEL_ID" ]]; then
  case "$MODEL_ID" in
    *AWQ*|*-awq*)       MODEL_QUANT="AWQ" ;;
    *GPTQ*|*-gptq*)     MODEL_QUANT="GPTQ" ;;
    *FP8*|*-fp8*)       MODEL_QUANT="FP8" ;;
    *INT4*|*-int4*|*Q4*) MODEL_QUANT="INT4" ;;
    *INT8*|*-int8*|*Q8*) MODEL_QUANT="INT8" ;;
    *bf16*|*BF16*)      MODEL_QUANT="BF16" ;;
    *) ;;
  esac
fi

# ---------------------------------------------------------------------------
# Emit YAML
# ---------------------------------------------------------------------------
{
  echo "# Generated by scripts/benchmark/capture-machine-info.sh on ${SNAP_TS}"
  echo "# Host: ${HOST}  OS: ${OS_KIND}  Engine: ${ENGINE_NAME}"
  echo "${HOST}:"
  echo "  label: $(yq "${HOST^}")"
  echo "  chassis: $(yq "")"
  echo "  cpu: $(yq "${CPU_MODEL}")"
  echo "  gpu: $(yq "${GPU_MODEL}${GPU_VRAM_MB:+ ${GPU_VRAM_MB} MiB}")"
  echo "  memory: $(yq "${MEM_GB:+${MEM_GB} GB}")"
  echo "  os: $(yq "${OS_PRETTY:-${OS_KIND}}")"
  echo "  network: $(yq "${TAILSCALE_IP:+Tailscale ${TAILSCALE_IP}}")"
  echo "  notes: $(yq "")"
  echo "  snapshot:"
  echo "    captured_at: $(yq "${SNAP_TS}")"
  echo "    hostname: $(yq "${HOST}")"
  echo "    kernel: $(yq "${KERNEL}")"
  echo "    os_release: $(yq "${OS_RELEASE}")"
  echo "  hardware:"
  echo "    cpu_model: $(yq "${CPU_MODEL}")"
  echo "    cpu_cores: $(intval "${CPU_CORES}")"
  echo "    cpu_sockets: $(intval "${CPU_SOCKETS}")"
  echo "    memory_gb: $(intval "${MEM_GB}")"
  echo "    gpu_vendor: $(yq "${GPU_VENDOR}")"
  echo "    gpu_model: $(yq "${GPU_MODEL}")"
  echo "    gpu_vram_mb: $(intval "${GPU_VRAM_MB}")"
  echo "    gpu_driver: $(yq "${GPU_DRIVER}")"
  echo "    gpu_runtime: $(yq "${GPU_RUNTIME}")"
  echo "    gpu_runtime_version: $(yq "${GPU_RUNTIME_VERSION}")"
  echo "    gpu_util_pct: $(intval "${GPU_UTIL_PCT}")"
  echo "    gpu_mem_used_mb: $(intval "${GPU_MEM_USED_MB}")"
  echo "  network_detail:"
  echo "    primary_ip: $(yq "${PRIMARY_IP}")"
  echo "    tailscale_ip: $(yq "${TAILSCALE_IP}")"
  echo "    public_ip: $(yq "${PUBLIC_IP}")"
  echo "  serving:"
  echo "    engine: $(yq "${ENGINE_NAME}")"
  echo "    engine_version: $(yq "${ENGINE_VERSION}")"
  echo "    endpoint: $(yq "${ENGINE_ENDPOINT}")"
  echo "    port: $(intval "${ENGINE_PORT}")"
  echo "    pid: $(intval "${ENGINE_PID}")"
  echo "    model_id: $(yq "${MODEL_ID}")"
  echo "    quant_label: $(yq "${MODEL_QUANT}")"
  echo "    weights_path: $(yq "${MODEL_WEIGHTS_PATH}")"
  echo "    context_window: $(intval "${MODEL_CONTEXT}")"
  echo "    kv_cache: $(yq "${SERVER_KV_CACHE}")"
  echo "    max_batch: $(intval "${SERVER_MAX_BATCH}")"
  echo "    max_seq_len: $(intval "${SERVER_MAX_SEQ}")"
  echo "  sampling_defaults:"
  echo "    temperature: $(yq "${SAMPLING_TEMP}")"
  echo "    top_p: $(yq "${SAMPLING_TOP_P}")"
  echo "    top_k: $(yq "${SAMPLING_TOP_K}")"
  echo "    repetition_penalty: $(yq "${SAMPLING_REP_PEN}")"
  echo "    max_tokens: $(yq "${SAMPLING_MAX_TOKENS}")"
}

log "wrote YAML for host=${HOST} engine=${ENGINE_NAME} endpoint=${ENGINE_ENDPOINT:-none}"
exit 0
