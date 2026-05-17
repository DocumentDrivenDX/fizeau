#!/usr/bin/env bash
# SUMMARY: runtime-probe. Reads profile JSON on stdin; cases on profile.metadata.runtime
# (lucebox | llamacpp | vllm | omlx | ds4 | rapid-mlx); curls the runtime's HTTP
# endpoint; emits a model_server JSON record {name, version, commit, endpoint}
# for embedding into the cell report.
#
# Exit codes:
#   0  endpoint reachable, JSON emitted (commit/version may be empty)
#   3  endpoint unreachable; JSON emitted with status="unreachable"
#   2  malformed input or missing endpoint
set -euo pipefail

usage() {
  cat >&2 <<'EOF'
Usage:
  runtime-probe.sh  (reads profile JSON on stdin)

Required fields in profile:
  provider.base_url       string  HTTP endpoint root (e.g. http://vidar:1234/v1)
  metadata.runtime        string  one of: lucebox, llamacpp, vllm, omlx, ds4, rapid-mlx
                                  (lucebox-* and llama-server are accepted aliases)
EOF
  return 2
}

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
  usage
fi

profile="$(cat)"
if [[ -z "$profile" ]]; then
  echo "runtime-probe: empty profile on stdin" >&2
  exit 2
fi

runtime="$(jq -r '.metadata.runtime // ""' <<<"$profile")"
endpoint="$(jq -r '.provider.base_url // ""' <<<"$profile")"

if [[ -z "$endpoint" ]]; then
  echo "runtime-probe: profile.provider.base_url is empty" >&2
  exit 2
fi

# Strip trailing /v1 (or any trailing slash) for runtime-specific endpoints.
root="${endpoint%/v1}"
root="${root%/}"
v1="$root/v1"

curl_json() {
  curl --silent --show-error --fail --max-time 5 "$1" 2>/dev/null || return 1
}

name=""
version=""
commit=""
status="reachable"

case "$runtime" in
  lucebox|lucebox-*)
    name="lucebox"
    if body="$(curl_json "$root/version")"; then
      version="$(jq -r '.version // .lucebox_version // ""' <<<"$body" 2>/dev/null || true)"
      commit="$(jq -r '.commit // .git_commit // .sha // ""' <<<"$body" 2>/dev/null || true)"
    elif body="$(curl_json "$v1/models")"; then
      version="$(jq -r '.data[0].lucebox_version // ""' <<<"$body" 2>/dev/null || true)"
    else
      status="unreachable"
    fi
    ;;
  llamacpp|llama-server|llama-cpp)
    name="llama-server"
    if body="$(curl_json "$root/props")"; then
      version="$(jq -r '.build_info.build_version // .version // ""' <<<"$body" 2>/dev/null || true)"
      commit="$(jq -r '.build_info.commit // .commit // ""' <<<"$body" 2>/dev/null || true)"
    elif body="$(curl_json "$v1/models")"; then
      version="$(jq -r '.data[0].id // ""' <<<"$body" 2>/dev/null || true)"
    else
      status="unreachable"
    fi
    ;;
  vllm)
    name="vllm"
    if body="$(curl_json "$root/version")"; then
      version="$(jq -r '.version // ""' <<<"$body" 2>/dev/null || true)"
    fi
    if [[ -z "$version" ]]; then
      if body="$(curl_json "$v1/models")"; then
        version="$(jq -r '.data[0].vllm_version // ""' <<<"$body" 2>/dev/null || true)"
      else
        status="unreachable"
      fi
    fi
    ;;
  omlx)
    name="omlx"
    if body="$(curl_json "$v1/models")"; then
      version="$(jq -r '.omlx_version // .data[0].omlx_version // ""' <<<"$body" 2>/dev/null || true)"
      commit="$(jq -r '.commit // .data[0].commit // ""' <<<"$body" 2>/dev/null || true)"
    else
      status="unreachable"
    fi
    ;;
  ds4)
    name="ds4"
    if body="$(curl_json "$root/internal/version")"; then
      version="$(jq -r '.version // ""' <<<"$body" 2>/dev/null || true)"
      commit="$(jq -r '.commit // .git_sha // ""' <<<"$body" 2>/dev/null || true)"
    elif body="$(curl_json "$v1/models")"; then
      version="$(jq -r '.data[0].ds4_version // .data[0].id // ""' <<<"$body" 2>/dev/null || true)"
    else
      status="unreachable"
    fi
    ;;
  rapid-mlx|rapidmlx)
    name="rapid-mlx"
    if body="$(curl_json "$v1/models")"; then
      version="$(jq -r '.rapid_mlx_version // .data[0].rapid_mlx_version // .data[0].id // ""' <<<"$body" 2>/dev/null || true)"
      commit="$(jq -r '.commit // .data[0].commit // ""' <<<"$body" 2>/dev/null || true)"
    else
      status="unreachable"
    fi
    ;;
  "" )
    echo "runtime-probe: profile.metadata.runtime is empty" >&2
    exit 2
    ;;
  *)
    echo "runtime-probe: unsupported runtime: $runtime" >&2
    exit 2
    ;;
esac

jq -cn \
  --arg name "$name" \
  --arg version "$version" \
  --arg commit "$commit" \
  --arg endpoint "$endpoint" \
  --arg status "$status" \
  '{name:$name, version:$version, commit:$commit, endpoint:$endpoint, status:$status}'

if [[ "$status" == "unreachable" ]]; then
  exit 3
fi
exit 0
