#!/usr/bin/env bash
# Deprecated thin wrapper kept so legacy invocations keep resolving.
# The medium-model comparison is now expressed by the `tb-2-1-medium-model`
# (or `tb-2-1-medium-model-canary`) bench-set against the relevant profiles.
# Run it via scripts/benchmark/benchmark — see ADR-016 and
# docs/helix/02-design/plan-2026-05-15-benchmark-runner-simplification.md.
set -euo pipefail
exec "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/benchmark" "$@"
