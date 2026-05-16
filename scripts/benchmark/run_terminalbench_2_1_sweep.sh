#!/usr/bin/env bash
# Deprecated thin wrapper kept so legacy invocations keep resolving.
# Use scripts/benchmark/benchmark directly — see ADR-016 and
# docs/helix/02-design/plan-2026-05-15-benchmark-runner-simplification.md.
set -euo pipefail
exec "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/benchmark" "$@"
