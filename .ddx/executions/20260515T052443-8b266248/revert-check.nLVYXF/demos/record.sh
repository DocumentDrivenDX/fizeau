#!/usr/bin/env bash
# Record all demo reels. Requires fiz binary and asciinema.
# Usage: ./demos/record.sh [--lmstudio URL]
#
# By default uses FIZEAU_BASE_URL from env or http://localhost:1234/v1.
# Session logs are saved to demos/sessions/ for CI replay.
set -euo pipefail

cd "$(dirname "$0")/.."

# Ensure fiz is built
make build

LMSTUDIO_URL="${FIZEAU_BASE_URL:-http://localhost:1234/v1}"
MODEL="${FIZEAU_MODEL:-qwen/qwen3-coder-next}"

export FIZEAU_BASE_URL="$LMSTUDIO_URL"
export FIZEAU_MODEL="$MODEL"

echo "Recording demos against $LMSTUDIO_URL with model $MODEL"

for script in demos/scripts/demo-*.sh; do
  name=$(basename "$script" .sh | sed 's/demo-//')
  cast="website/static/demos/${name}.cast"
  echo "Recording: $name -> $cast"
  asciinema rec "$cast" \
    --cols 100 --rows 30 \
    --title "Fizeau: $name" \
    --command "bash $script" \
    --overwrite
done

# Copy session logs for CI replay
echo "Copying session logs..."
mkdir -p demos/sessions
latest_sessions=$(ls -t .fizeau/sessions/*.jsonl 2>/dev/null | head -3)
i=0
for session in $latest_sessions; do
  cp "$session" "demos/sessions/"
  echo "  $session -> demos/sessions/"
done

echo "Done. Cast files in website/static/demos/, sessions in demos/sessions/"
