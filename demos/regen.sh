#!/usr/bin/env bash
# Regenerate all demo asciicast files from canonical session JSONLs in
# demos/sessions/. Deterministic — no live LLM calls, no `asciinema rec`.
#
# Usage:  ./demos/regen.sh
#         make demos-regen
set -euo pipefail

cd "$(dirname "$0")/.."

REGEN="demos/regen.py"
SESS_DIR="demos/sessions"
OUT_DIR="website/static/demos"
WIDTH="${FIZEAU_DEMO_WIDTH:-80}"
HEIGHT="${FIZEAU_DEMO_HEIGHT:-24}"

mkdir -p "$OUT_DIR"

regen_one() {
  local name="$1"
  local prompt="$2"
  python3 "$REGEN" \
    --in  "$SESS_DIR/${name}.jsonl" \
    --out "$OUT_DIR/${name}.cast" \
    --title "Fizeau: ${name}" \
    --prompt "$prompt" \
    --width "$WIDTH" --height "$HEIGHT"
}

regen_one file-read    "Read main.go and explain what this program does"
regen_one file-edit    "Read config.yaml, change the server port from 8080 to 9090, then verify"
regen_one bash-explore "List all Go files in this project and summarize the package structure"

echo "Regenerated demo casts under $OUT_DIR/ (${WIDTH}x${HEIGHT})."
