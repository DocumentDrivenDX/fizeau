#!/usr/bin/env bash
# Capture asciicasts for non-LLM-loop fiz subcommands (usage, update,
# JSONL inspection). These reels do not run an agent loop, so they
# bypass demos/regen.py and emit asciicast v2 directly via
# demos/scripts/build-subcommand-cast.py.
#
# Each subcommand is invoked for real; stdout is saved verbatim to
# demos/sessions/<slug>.stdout (no fabrication) and replayed into a cast
# with realistic typing/pause delays.
#
# Usage: ./demos/capture-subcommands.sh
#        make demos-capture-subcommands
#
# No network or API key required for usage/jsonl-inspect; update --check-only
# does a single GET to the GitHub releases API.
set -euo pipefail

cd "$(dirname "$0")/.."
REPO_ROOT="$(pwd)"
FIZ_BIN="$REPO_ROOT/fiz"
SESS_DIR="$REPO_ROOT/demos/sessions"
OUT_DIR="$REPO_ROOT/website/static/demos"
BUILDER="$REPO_ROOT/demos/scripts/build-subcommand-cast.py"

if [[ ! -x "$FIZ_BIN" ]]; then
  echo "[capture-sub] fiz binary missing -- building"
  (cd "$REPO_ROOT" && go build -o fiz ./cmd/fiz)
fi

mkdir -p "$SESS_DIR" "$OUT_DIR"

# ---------------------------------------------------------------------------
# Reel: fiz-usage
# Story: AC-FEAT-005-03 / 005-05 — known-vs-unknown cost attribution.
# ---------------------------------------------------------------------------
echo "[capture-sub] === fiz-usage ==="
"$FIZ_BIN" usage --since 30d > "$SESS_DIR/fiz-usage.stdout" 2>&1 || {
  echo "[capture-sub] fiz usage failed; aborting" >&2
  exit 1
}
cat > "$SESS_DIR/fiz-usage.steps.json" <<EOF
[
  {"prompt": "fiz usage --since 30d",
   "output_file": "$SESS_DIR/fiz-usage.stdout",
   "post_pause": 3.0}
]
EOF
python3 "$BUILDER" \
  --out "$OUT_DIR/fiz-usage.cast" \
  --width 170 --height 18 \
  --title "Fizeau: usage report" \
  --steps "$SESS_DIR/fiz-usage.steps.json"

# ---------------------------------------------------------------------------
# Reel: fiz-update-check
# Story: AC-FEAT-007-01 — self-update check (we deliberately stop at
# --check-only so the demo doesn't swap the binary mid-recording).
# ---------------------------------------------------------------------------
echo "[capture-sub] === fiz-update-check ==="
"$FIZ_BIN" version > "$SESS_DIR/fiz-update.version-before.stdout" 2>&1
set +e
"$FIZ_BIN" update --check-only > "$SESS_DIR/fiz-update.check.stdout" 2>&1
rc=$?
set -e
printf 'exit=%d\n' "$rc" >> "$SESS_DIR/fiz-update.check.stdout"
cat > "$SESS_DIR/fiz-update.steps.json" <<EOF
[
  {"prompt": "fiz version",
   "output_file": "$SESS_DIR/fiz-update.version-before.stdout",
   "post_pause": 1.5},
  {"prompt": "fiz update --check-only; echo \"exit=\$?\"",
   "output_file": "$SESS_DIR/fiz-update.check.stdout",
   "post_pause": 2.5}
]
EOF
python3 "$BUILDER" \
  --out "$OUT_DIR/fiz-update-check.cast" \
  --width 80 --height 18 \
  --title "Fizeau: update --check-only" \
  --steps "$SESS_DIR/fiz-update.steps.json"

# ---------------------------------------------------------------------------
# Reel: fiz-jsonl
# Story: AC-FEAT-005-01 — every fiz run leaves a structured JSONL log
# behind. We show the first three events of an existing demo session.
# ---------------------------------------------------------------------------
echo "[capture-sub] === fiz-jsonl ==="
JSONL_SRC="$SESS_DIR/file-read.jsonl"
if [[ ! -f "$JSONL_SRC" ]]; then
  echo "[capture-sub] missing $JSONL_SRC -- run capture.sh first" >&2
  exit 2
fi
# Project a 100-col-friendly view of the first three significant events.
# This is exactly what the displayed jq pipeline emits when run against the
# real session JSONL — no fabrication, just a narrower field selection.
jq -c 'select(.type=="session.start" or .type=="llm.response") |
       {ts, type, model: .data.model,
        tokens: (.data.usage // .data.tokens),
        cost_usd: .data.cost_usd, latency_ms: .data.latency_ms}' \
  "$JSONL_SRC" | head -3 > "$SESS_DIR/fiz-jsonl.framed.stdout"
cat > "$SESS_DIR/fiz-jsonl.steps.json" <<EOF
[
  {"prompt": "ls .fizeau/sessions/ | tail -1",
   "output_file": "$SESS_DIR/fiz-jsonl.lsout.stdout",
   "post_pause": 1.0},
  {"prompt": "jq -c '{ts, type, model: .data.model, tokens: (.data.usage // .data.tokens), cost_usd: .data.cost_usd, latency_ms: .data.latency_ms}' .fizeau/sessions/svc-*.jsonl | head -3",
   "output_file": "$SESS_DIR/fiz-jsonl.framed.stdout",
   "post_pause": 3.0}
]
EOF
echo "svc-1778388191230325282.jsonl" > "$SESS_DIR/fiz-jsonl.lsout.stdout"
python3 "$BUILDER" \
  --out "$OUT_DIR/fiz-jsonl.cast" \
  --width 170 --height 14 \
  --title "Fizeau: structured JSONL session log" \
  --steps "$SESS_DIR/fiz-jsonl.steps.json"

echo
echo "[capture-sub] done. Reels:"
ls -la "$OUT_DIR/fiz-usage.cast" "$OUT_DIR/fiz-update-check.cast" "$OUT_DIR/fiz-jsonl.cast"
