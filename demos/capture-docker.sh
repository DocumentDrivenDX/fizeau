#!/usr/bin/env bash
# Capture fresh fiz session JSONLs by running fiz inside the demos Docker
# image (CPU variant by default). The image bundles llama-server +
# Qwen2.5-Coder-0.5B and serves an OpenAI-compatible API on 127.0.0.1:8080
# inside the container, so capture is fully offline — no OpenRouter, no GPU.
#
# Usage:
#   ./demos/capture-docker.sh                    # capture all demos
#   ./demos/capture-docker.sh quickstart         # one demo
#   IMAGE=fiz-demos-cpu:dev ./demos/capture-docker.sh
#   VARIANT=gpu MODEL=/models/qwen3-7b.gguf ./demos/capture-docker.sh
#
# Requires: docker (with buildx). The image is built automatically the
# first time. Bake with `make demos-docker-build` to pre-build.
set -euo pipefail

cd "$(dirname "$0")/.."
REPO_ROOT="$(pwd)"

VARIANT="${VARIANT:-cpu}"
IMAGE="${IMAGE:-fiz-demos-${VARIANT}:local}"
DOCKERFILE="demos/docker/Dockerfile.${VARIANT}"
SESS_DIR="$REPO_ROOT/demos/sessions"
PRESET="${PRESET:-cheap}"
PROVIDER="${PROVIDER:-local}"
KEEP_SCRATCH="${KEEP_SCRATCH:-0}"
ONLY="${1:-}"

if [[ ! -f "$DOCKERFILE" ]]; then
  echo "[capture-docker] no such Dockerfile: $DOCKERFILE" >&2
  exit 2
fi

# Build image if missing.
if ! docker image inspect "$IMAGE" >/dev/null 2>&1; then
  echo "[capture-docker] building $IMAGE from $DOCKERFILE (first run; ~5 min on cold cache, mostly model download)" >&2
  docker build -f "$DOCKERFILE" -t "$IMAGE" "$REPO_ROOT"
fi

mkdir -p "$SESS_DIR"

# Demo definitions: name|prompt
# These prompts are sized for the tiny 0.5B-Coder model — they are file-tool
# tasks the model can answer in 1-2 turns. Larger reels (cost-cap, replay,
# embed) are captured against bigger backends; see demos/CANDIDATES.md.
DEMOS=(
  "quickstart|List the Go files in this project. Reply with just the filenames."
  "file-read|Read main.go and explain in one sentence what it does."
  "file-edit|Edit config.yaml to change port 8080 to 9090. Reply 'done' when finished."
  "bash-explore|Use bash to run: find . -name '*.go'. Then list the files you found."
)

stage_scratch() {
  local scratch="$1"
  mkdir -p "$scratch/cmd/server" "$scratch/internal/api" "$scratch/internal/db"
  cat > "$scratch/main.go" <<'EOF'
package main

import (
	"fmt"
	"log"
	"net/http"
)

func main() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Hello from Fizeau!")
	})
	log.Println("listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
EOF
  cat > "$scratch/config.yaml" <<'EOF'
server:
  port: 8080
  host: 0.0.0.0
log_level: info
EOF
  cat > "$scratch/cmd/server/main.go" <<'EOF'
package main

import "log"

func main() { log.Println("server entry point") }
EOF
  cat > "$scratch/internal/api/handler.go" <<'EOF'
package api

import "net/http"

func Handler(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }
EOF
  cat > "$scratch/internal/api/middleware.go" <<'EOF'
package api

import "net/http"

func Logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
	})
}
EOF
  cat > "$scratch/internal/db/postgres.go" <<'EOF'
package db

type Conn struct{ DSN string }

func Open(dsn string) *Conn { return &Conn{DSN: dsn} }
EOF
}

run_demo() {
  local name="$1" prompt="$2"
  if [[ -n "$ONLY" && "$name" != "$ONLY" ]]; then
    return 0
  fi

  local scratch
  scratch="$(mktemp -d -t fiz-demos-docker-XXXXXX)"
  trap 'if [[ "$KEEP_SCRATCH" != "1" ]]; then rm -rf "$scratch"; else echo "[capture-docker] kept $scratch"; fi' RETURN
  stage_scratch "$scratch"

  echo
  echo "[capture-docker] === $name ==="
  echo "[capture-docker] prompt: $prompt"

  # Per-run docker invocation. Each invocation cold-starts llama-server inside
  # the container so the model-load latency lands in the captured session
  # (regen.py compresses it on playback). Faster path for batches: launch a
  # long-lived container and exec into it — see README.
  local started_at
  started_at="$(date +%s)"
  local out_jsonl="$SESS_DIR/${name}.jsonl"
  local log
  log="$(mktemp -t fiz-docker-${name}-XXXXXX.log)"

  set +e
  docker run --rm \
      --network none \
      -v "$scratch:/work:rw" \
      -v "$SESS_DIR:/out:rw" \
      -e PROVIDER="$PROVIDER" \
      -e PRESET="$PRESET" \
      -e PROMPT="$prompt" \
      -e DEMO_NAME="$name" \
      -w /work \
      "$IMAGE" \
      bash -c '
        set -e
        fiz --json --preset "$PRESET" --provider "$PROVIDER" \
            --work-dir /work -p "$PROMPT" >/tmp/fiz.log 2>&1 || \
              { tail -40 /tmp/fiz.log >&2; exit 7; }
        sess=$(ls -1t /work/.fizeau/sessions/*.jsonl 2>/dev/null | head -1)
        if [ -z "$sess" ]; then
          echo "no session JSONL produced" >&2; exit 8
        fi
        cp "$sess" "/out/${DEMO_NAME}.jsonl"
      ' \
      >"$log" 2>&1
  local rc=$?
  set -e

  # --network none is set so the container demonstrates "no internet",
  # which forces everything (model, binaries) to be baked into the image.

  if [[ $rc -ne 0 ]]; then
    echo "[capture-docker] FAILED rc=$rc — log:" >&2
    sed 's/^/    /' "$log" >&2
    rm -f "$log"
    return $rc
  fi
  rm -f "$log"

  if [[ ! -f "$out_jsonl" ]]; then
    echo "[capture-docker] no output at $out_jsonl" >&2
    return 9
  fi

  local tin tout
  tin="$(jq -r 'select(.type=="session.end") | .data.tokens.input // 0' "$out_jsonl" | head -1)"
  tout="$(jq -r 'select(.type=="session.end") | .data.tokens.output // 0' "$out_jsonl" | head -1)"
  echo "[capture-docker] -> $out_jsonl  (tokens: $tin in / $tout out)"
}

# Note about --network none + PROMPT env passing: docker run takes -e flags
# *before* the image name. The invocation above is correct; documenting here.

for entry in "${DEMOS[@]}"; do
  name="${entry%%|*}"
  prompt="${entry#*|}"
  run_demo "$name" "$prompt"
done

echo
echo "[capture-docker] done."
echo "[capture-docker] next: make demos-regen"
