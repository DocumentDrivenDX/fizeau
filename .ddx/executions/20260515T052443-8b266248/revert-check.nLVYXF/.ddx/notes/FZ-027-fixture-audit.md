# FZ-027 Fixture Audit — Legacy Config Env Literals

Audit of test fixtures and scripts containing `.agent`, `~/.config/agent`,
`AGENT_*`, or `DDX_AGENT_*` literals after the runtime rename to `fizeau`.

## Updated — active literals replaced with fizeau names

| File | Old literal | New literal | Rationale |
|------|-------------|-------------|-----------|
| `scripts/benchmark/smoke_run.sh` | `.agent/config.yaml` (repo-local path) | `.fizeau/config.yaml` | Config dir renamed |
| `scripts/benchmark/smoke_run.sh` | `$HOME/.config/agent/config.yaml` | `$HOME/.config/fizeau/config.yaml` | Global config dir renamed |
| `scripts/benchmark/smoke_run.sh` | `AGENT_ENV_ARGS` | `ENV_ARGS` | Local script var, no legacy prefix needed |
| `scripts/benchmark/run_benchmark.sh` | `.agent/config.yaml` (repo-local path) | `.fizeau/config.yaml` | Config dir renamed |
| `scripts/benchmark/run_benchmark.sh` | `$HOME/.config/agent/config.yaml` | `$HOME/.config/fizeau/config.yaml` | Global config dir renamed |
| `scripts/benchmark/run_benchmark.sh` | `AGENT_ENV_ARGS` | `ENV_ARGS` | Local script var |
| `scripts/benchmark/run_benchmark.sh` | `AGENT_TIMEOUT_MULTIPLIER` | `TIMEOUT_MULTIPLIER` | Local script var (reads `DDX_BENCH_AGENT_TIMEOUT_MULTIPLIER`) |
| `scripts/benchmark/run_benchmark.sh` | `AGENT_SHA_OVERRIDE` | `SHA_OVERRIDE` | Local script var (reads `FIZEAU_BENCH_SHA`) |
| `scripts/benchmark/run_benchmark.sh` | `AGENT_GIT_SHA` | `GIT_SHA` | Local script var |
| `scripts/benchmark/run_benchmark.sh` | `AGENT_GIT_SHA_SHORT` | `GIT_SHA_SHORT` | Local script var |
| `scripts/benchmark/run_benchmark.sh` | `AGENT_VERSION` | `BINARY_VERSION` | Local script var capturing `--version` output |
| `install.sh` | `AGENT_INSTALL_DIR` | `FIZEAU_INSTALL_DIR` | User-facing installer env var |
| `install.sh` | `AGENT_VERSION` | `FIZEAU_VERSION` | User-facing installer env var |
| `tests/install_sh_acceptance.sh` | `AGENT_INSTALL_DIR` | `FIZEAU_INSTALL_DIR` | Follows `install.sh` rename |
| `scripts/beadbench/run_beadbench.py` | `~/.config/agent/config.yaml` (default path) | `~/.config/fizeau/config.yaml` | Default config path updated |
| `internal/core/integration_test.go` | `AGENT_OK` (prompt sentinel) | `FIZEAU_OK` | Arbitrary sentinel string, matches `AGENT_` regex |
| `internal/ptytest/docker_conformance_integration_test.go` | `AGENT_PTY_INTEGRATION` | `FIZEAU_PTY_INTEGRATION` | Integration test gate env var |

## Allowlisted — DDX_AGENT_* harness env vars pending rename

These files reference `DDX_AGENT_*` env vars defined in `internal/harnesses/`.
The harness source has not been renamed yet. Added to `renamecheck.go`
allowlist via `skippedDirs` / `skippedFiles` with a comment.

| File | Literals | Mechanism |
|------|----------|-----------|
| `internal/harnesses/claude/quota_cache.go` | `DDX_AGENT_CLAUDE_QUOTA_CACHE` | `skippedDirs["internal/harnesses"]` |
| `internal/harnesses/codex/account.go` | `DDX_AGENT_CODEX_AUTH` | `skippedDirs["internal/harnesses"]` |
| `internal/harnesses/codex/quota_cache.go` | `DDX_AGENT_CODEX_QUOTA_CACHE` | `skippedDirs["internal/harnesses"]` |
| `internal/harnesses/codex/session_token_count.go` | `DDX_AGENT_CODEX_SESSIONS_DIR` | `skippedDirs["internal/harnesses"]` |
| `internal/harnesses/gemini/quota_cache.go` | `DDX_AGENT_GEMINI_QUOTA_CACHE` | `skippedDirs["internal/harnesses"]` |
| `internal/harnesses/gemini/quota_cache_test.go` | `DDX_AGENT_GEMINI_QUOTA_CACHE` | `skippedDirs["internal/harnesses"]` |
| `harness_golden_integration_test.go` | `DDX_AGENT_CLAUDE_QUOTA_CACHE`, `DDX_AGENT_CODEX_QUOTA_CACHE` | `skippedFiles` |
| `service_providers_test.go` | `DDX_AGENT_*` (multiple) | `skippedFiles` |
| `service_route_attempts_test.go` | `DDX_AGENT_*` (multiple) | `skippedFiles` |
| `service_status_test.go` | `DDX_AGENT_*` (multiple) | `skippedFiles` |

## Pre-existing historical allowlist (skippedDirs, unchanged)

| Directory | Reason |
|-----------|--------|
| `internal/renamecheck/` | Contains rule definitions and test fixtures for the checker itself |
| `docs/helix/` | Historical design docs including the rename plan |
| `docs/research/` | Historical research notes |
| `.ddx/` | Bead queue with historical descriptions |

## Remaining findings — documentation, out of scope

Seven `.agent` / `~/.config/agent` occurrences remain in documentation files.
These are not test fixtures and are tracked separately:

- `README.md:129`, `README.md:138` — user-facing docs
- `demos/record.sh:36` — demo script
- `scripts/benchmark/README.md:152-153` — benchmark docs
- `website/content/docs/getting-started.md:36`, `:43` — website docs
