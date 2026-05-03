# FZ-029 User-Facing Runtime Message Audit

Scope: active runtime-facing errors, warnings, help/usage text, and logs after
`internal/productinfo` changed to Fizeau/fiz/fizeau.

## Changed

| File | Old runtime text | New runtime text | Reason |
|---|---|---|---|
| `agentcli/catalog_commands.go` | `usage: ddx-agent catalog ...` | `usage: fiz catalog ...` | CLI usage is user-facing. |
| `agentcli/catalog_commands.go` | `run 'ddx-agent catalog update'` | `run 'fiz catalog update'` | Stale-catalog nudge is user-facing. |
| `agentcli/corpus_commands.go` | `usage: ddx-agent corpus ...` | `usage: fiz corpus ...` | CLI usage is user-facing. |
| `agentcli/corpus_commands.go` | `usage: ddx-agent corpus promote ...` | `usage: fiz corpus promote ...` | CLI usage is user-facing. |

The replacement strings are derived from `internal/productinfo.BinaryName`.

## Allowlisted / Out Of Scope

| Surface | Decision | Rationale |
|---|---|---|
| `install.sh` user-facing installer messages and default binary name | Resolved by FZ-033 (`agent-fee0000d`, closed) | Installer messages, artifact names, and binary paths now use `fiz`; runtime audit grep below shows `install.sh` clean. |
| `cmd/bench` benchmark command/help surfaces | Resolved by FZ-035a (`agent-9a551983`, closed) | `cmd/bench` command name/help/errors/tests now use `fiz-bench`; runtime audit grep below shows `cmd/bench` clean. |
| Harness adapter label `ddx-agent` in benchmark harness code/config | Domain label retained | It identifies the TerminalBench/Harbor adapter arm, not the primary user CLI binary. |
| `DDX_AGENT_*` harness env vars | Existing allowlist | `internal/renamecheck` already allowlists harness env vars until the dedicated harness-rename bead updates them. |
| `ddx-virtual-agent` / `ddx-script-agent` registry sentinels in `internal/harnesses/registry.go` | Internal sentinels, never exec'd | Marked in source as never-exec sentinel binaries for non-CLI harness arms; not user-facing strings. |
| Internal XDG state path `ddx/claude-quota.json` and tmux session names `ddx-claude-quota-*` / `ddx-codex-quota-*` in `internal/harnesses/{claude,codex}/quota_*.go` | Internal namespacing | State directory and tmux scope identifiers, not user-facing error/log strings. Tracked separately under harness-rename work. |
| `ddx.usage_source` event type in `internal/harnesses/{claude,codex}/stream.go` | Telemetry/event tag | Out of scope per bead description ("separate from telemetry and docs"). |
| Comments, docs, telemetry, historical execution artifacts, and rename-check rules | Out of audit scope | This bead targets runtime-facing errors, warnings, and logs, separate from docs and telemetry. |

## Verification

- `go test ./agentcli/...` — passing.
- `go test ./cmd/bench/...` — passing.
- Runtime string audit:
  `rg -n 'ddx-agent|DDX Agent|DDx Agent|DDX_AGENT_[A-Z0-9_]+' agentcli cmd internal service*.go routing_errors.go install.sh -g '!**/*_test.go'`
  Only matches are the rename-noise checker's own pattern definitions in `internal/renamecheck/renamecheck.go` (the tool that flags these names elsewhere), which are out of scope.

## Re-attempt confirmation (2026-05-03)

Re-ran the audit grep after FZ-033 and FZ-035a closed: all user-facing runtime surfaces in the bead's scope (`agentcli`, `cmd`, `internal`, root `service*.go`, `routing_errors.go`, `install.sh`) are free of legacy product names except for the allowlisted items above. Targeted CLI tests (`agentcli`, `cmd/bench`) pass.
