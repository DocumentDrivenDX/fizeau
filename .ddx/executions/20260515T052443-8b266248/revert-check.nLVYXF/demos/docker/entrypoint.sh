#!/usr/bin/env bash
# entrypoint.sh — start llama-server in the background, wait for it to be
# ready, then exec the user's command (defaults to bash).
#
# Tunables (env):
#   LLAMA_PORT       (default 8080)
#   FIZ_DEMO_MODEL   (default /models/model.gguf)
#   FIZ_DEMO_CTX     (default 4096)
#   LLAMA_THREADS    (default = nproc)
#   SKIP_LLAMA       (set to 1 to skip the auto-start; useful for one-off
#                    `docker run ... fiz models list` style invocations)
set -euo pipefail

LLAMA_PORT="${LLAMA_PORT:-8080}"
MODEL="${FIZ_DEMO_MODEL:-/models/model.gguf}"
CTX="${FIZ_DEMO_CTX:-4096}"
THREADS="${LLAMA_THREADS:-$(nproc)}"

NGL_ARGS=()
if [[ -n "${LLAMA_NGL:-}" ]]; then
  NGL_ARGS=(--n-gpu-layers "$LLAMA_NGL")
fi

if [[ "${SKIP_LLAMA:-0}" != "1" ]]; then
  echo "[entrypoint] starting llama-server (model=$MODEL ctx=$CTX threads=$THREADS port=$LLAMA_PORT ngl=${LLAMA_NGL:-0})" >&2
  /usr/local/bin/llama-server \
      --model "$MODEL" \
      --ctx-size "$CTX" \
      --threads "$THREADS" \
      --port "$LLAMA_PORT" \
      --host 127.0.0.1 \
      --jinja \
      --log-disable \
      "${NGL_ARGS[@]}" \
      >/tmp/llama-server.log 2>&1 &
  LLAMA_PID=$!
  echo "$LLAMA_PID" > /tmp/llama-server.pid

  # Health-wait: the /health endpoint flips to {"status":"ok"} once weights
  # are loaded. Give it 90s — first boot dominates because we're paging the
  # ~390 MB model file off disk.
  for i in $(seq 1 90); do
    if curl -fsS "http://127.0.0.1:${LLAMA_PORT}/health" >/dev/null 2>&1; then
      echo "[entrypoint] llama-server ready after ${i}s" >&2
      break
    fi
    if ! kill -0 "$LLAMA_PID" 2>/dev/null; then
      echo "[entrypoint] llama-server died — log tail:" >&2
      tail -40 /tmp/llama-server.log >&2 || true
      exit 1
    fi
    sleep 1
  done
fi

exec "$@"
