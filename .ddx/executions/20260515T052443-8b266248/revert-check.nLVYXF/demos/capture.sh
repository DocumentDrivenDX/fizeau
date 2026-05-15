#!/usr/bin/env bash
# Capture fresh fiz session JSONLs against a real OpenRouter model and copy
# them into demos/sessions/ for use by demos/regen.{sh,py}.
#
# Usage:   ./demos/capture.sh
#          KEEP_SCRATCH=1 ./demos/capture.sh   # leave scratch dir for inspection
#          MODEL=qwen/qwen3.6-27b ./demos/capture.sh
#
# Requires: $OPENROUTER_API_KEY in env, ./fiz binary at repo root.
set -euo pipefail

cd "$(dirname "$0")/.."
REPO_ROOT="$(pwd)"

MODEL="${MODEL:-qwen/qwen3.6-27b}"
PROVIDER="${PROVIDER:-openrouter}"
PRESET="${PRESET:-cheap}"
KEEP_SCRATCH="${KEEP_SCRATCH:-0}"

SESS_DIR="$REPO_ROOT/demos/sessions"
FIZ_BIN="$REPO_ROOT/fiz"

if [[ ! -x "$FIZ_BIN" ]]; then
  echo "[capture] fiz binary missing at $FIZ_BIN -- building"
  (cd "$REPO_ROOT" && go build -o fiz ./cmd/fiz)
fi

if [[ -z "${OPENROUTER_API_KEY:-}" ]]; then
  echo "[capture] ERROR: OPENROUTER_API_KEY not set" >&2
  exit 2
fi

SCRATCH="$(mktemp -d -t fiz-demos-XXXXXX)"
echo "[capture] scratch dir: $SCRATCH"

cleanup() {
  if [[ "$KEEP_SCRATCH" = "1" ]]; then
    echo "[capture] KEEP_SCRATCH=1 -- leaving $SCRATCH"
  else
    rm -rf "$SCRATCH"
  fi
}
trap cleanup EXIT

# ---------------------------------------------------------------------------
# Stage scratch project: small Go HTTP server + config + a couple of stub pkgs
# so the explore/edit prompts have meaningful surface area.
# ---------------------------------------------------------------------------
mkdir -p "$SCRATCH/cmd/server" "$SCRATCH/internal/api" "$SCRATCH/internal/db"

cat > "$SCRATCH/main.go" <<'EOF'
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

cat > "$SCRATCH/config.yaml" <<'EOF'
server:
  port: 8080
  host: 0.0.0.0
log_level: info
EOF

cat > "$SCRATCH/cmd/server/main.go" <<'EOF'
package main

import "log"

func main() {
	log.Println("server entry point")
}
EOF

cat > "$SCRATCH/internal/api/handler.go" <<'EOF'
package api

import "net/http"

// Handler is the root HTTP handler.
func Handler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}
EOF

cat > "$SCRATCH/internal/api/middleware.go" <<'EOF'
package api

import "net/http"

// Logging is a request-logging middleware.
func Logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
	})
}
EOF

cat > "$SCRATCH/internal/db/postgres.go" <<'EOF'
package db

// Conn is a placeholder Postgres connection.
type Conn struct{ DSN string }

// Open returns a new Conn.
func Open(dsn string) *Conn { return &Conn{DSN: dsn} }
EOF

echo "[capture] staged scratch project ($(find "$SCRATCH" -type f | wc -l | tr -d ' ') files)"

# ---------------------------------------------------------------------------
# Demo definitions: name|prompt
# Prompts are kept short and end with a "be concise" instruction so the rendered
# 80x24 cast stays under ~30 seconds.
# ---------------------------------------------------------------------------
DEMOS=(
  "file-read|Read main.go and explain what this program does. Be concise -- 2-3 sentences max."
  "file-edit|Read config.yaml, change the server port from 8080 to 9090, then verify by re-reading the file. One short sentence at the end confirming the change."
  "bash-explore|List all Go files in this project (use the bash tool with: find . -name '*.go') and summarize the package structure as a brief tree. Use 'project/' as the root label (do NOT include the absolute or temp path). Keep it under 10 lines."
)

run_demo() {
  local name="$1" prompt="$2"
  local started_at
  started_at="$(date +%s)"
  echo
  echo "[capture] === $name ==="
  echo "[capture] prompt: $prompt"

  # Run fiz; capture stdout/stderr but do not let JSON parse errors abort us
  # before we can locate the session file.
  local log="$SCRATCH/${name}.fiz.log"
  set +e
  "$FIZ_BIN" \
    --json \
    --preset "$PRESET" \
    --provider "$PROVIDER" \
    --model "$MODEL" \
    --work-dir "$SCRATCH" \
    -p "$prompt" \
    >"$log" 2>&1
  local rc=$?
  set -e

  if [[ $rc -ne 0 ]]; then
    echo "[capture] fiz exited rc=$rc -- last 20 lines of log:" >&2
    tail -20 "$log" >&2
    exit $rc
  fi

  # Find the newest session JSONL produced after we started.
  local sess
  sess="$(find "$SCRATCH/.fizeau/sessions" -maxdepth 1 -name '*.jsonl' -newermt "@$started_at" -print 2>/dev/null \
            | xargs -I{} stat -c '%Y {}' {} 2>/dev/null \
            | sort -n | tail -1 | awk '{print $2}')"
  if [[ -z "$sess" ]]; then
    # Fallback: just take the newest file overall.
    sess="$(ls -1t "$SCRATCH/.fizeau/sessions"/*.jsonl 2>/dev/null | head -1)"
  fi
  if [[ -z "$sess" || ! -f "$sess" ]]; then
    echo "[capture] ERROR: no session JSONL produced for $name" >&2
    tail -20 "$log" >&2
    exit 3
  fi

  # Sanity: model field must reflect what we requested.
  local got_model
  got_model="$(head -1 "$sess" | jq -r '.data.model // .data.requested_model // ""')"
  if [[ "$got_model" != "$MODEL" ]]; then
    echo "[capture] WARN: session model='$got_model' != requested '$MODEL'" >&2
  fi

  cp "$sess" "$SESS_DIR/${name}.jsonl"
  local cost tokens_in tokens_out
  cost="$(jq -r 'select(.type=="session.end") | .data.cost_usd // 0' "$sess" | head -1)"
  tokens_in="$(jq -r 'select(.type=="session.end") | .data.tokens.input // 0' "$sess" | head -1)"
  tokens_out="$(jq -r 'select(.type=="session.end") | .data.tokens.output // 0' "$sess" | head -1)"
  echo "[capture] -> $SESS_DIR/${name}.jsonl  (model=$got_model, tokens=${tokens_in} in / ${tokens_out} out, cost=\$${cost})"
}

mkdir -p "$SESS_DIR"
for entry in "${DEMOS[@]}"; do
  name="${entry%%|*}"
  prompt="${entry#*|}"
  run_demo "$name" "$prompt"
done

echo
echo "[capture] done -- captured $(printf '%s\n' "${DEMOS[@]}" | wc -l | tr -d ' ') sessions in $SESS_DIR/"
echo "[capture] next step: make demos-regen"
