#!/usr/bin/env bash
# Regenerate all demo asciicast files from canonical session JSONLs in
# demos/sessions/. Deterministic — no live LLM calls, no `asciinema rec`.
#
# Usage:  ./demos/regen.sh
#         make demos-regen
#
# Tunables (env):
#   FIZEAU_DEMO_WIDTH       (default 80)
#   FIZEAU_DEMO_HEIGHT      (default 24)
#   FIZEAU_LATENCY_THRESH   ms threshold above which an LLM turn gets
#                           fast-forwarded with a banner (default 8000)
#   FIZEAU_FF_COMPRESSED_S  virtual playback length for each ff segment
#                           (default 2.0)
set -euo pipefail

cd "$(dirname "$0")/.."

REGEN="demos/regen.py"
SESS_DIR="demos/sessions"
OUT_DIR="website/static/demos"
WIDTH="${FIZEAU_DEMO_WIDTH:-80}"
HEIGHT="${FIZEAU_DEMO_HEIGHT:-24}"
LAT_MS="${FIZEAU_LATENCY_THRESH:-8000}"
FF_S="${FIZEAU_FF_COMPRESSED_S:-2.0}"

mkdir -p "$OUT_DIR"

regen_one() {
  local name="$1"
  local prompt="$2"
  if [[ ! -f "$SESS_DIR/${name}.jsonl" ]]; then
    echo "[regen] skip $name (no $SESS_DIR/${name}.jsonl yet — capture first)" >&2
    return 0
  fi
  local extra=()
  if [[ -f "$SESS_DIR/${name}.preface.json" ]]; then
    extra+=(--preface "$SESS_DIR/${name}.preface.json")
  fi
  python3 "$REGEN" \
    --in  "$SESS_DIR/${name}.jsonl" \
    --out "$OUT_DIR/${name}.cast" \
    --title "Fizeau: ${name}" \
    --prompt "$prompt" \
    --width "$WIDTH" --height "$HEIGHT" \
    --latency-threshold-ms "$LAT_MS" \
    --compressed-s "$FF_S" \
    "${extra[@]}"
}

# quickstart is a pure-Docker reel: install + download model + first query.
# It carries the model-load slow op so it exercises the time-compression
# branch in regen.py.
regen_one quickstart   "list go files in ."

regen_one cost-cap-halt "fiz --cost-cap-usd 0.005 -p 'add a doc comment to each .go file in this directory (one at a time)'"

regen_one file-read    "Read main.go and explain what this program does"
regen_one file-edit    "Read config.yaml, change the server port from 8080 to 9090, then verify"
regen_one bash-explore "List all Go files in this project and summarize the package structure"

echo "Regenerated demo casts under $OUT_DIR/ (${WIDTH}x${HEIGHT}, ff>${LAT_MS}ms)."
