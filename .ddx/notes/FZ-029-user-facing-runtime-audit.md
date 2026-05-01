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
| `install.sh` user-facing installer messages and default binary name | Out of scope for this bead | Owned by open bead FZ-033 (`agent-fee0000d`), whose contract updates installer messages, artifact names, binary paths, and `tests/install_sh_acceptance.sh` together. |
| `cmd/bench` benchmark command/help surfaces | Out of scope for this bead | Owned by open bead FZ-035a (`agent-9a551983`), whose contract updates `cmd/bench` command name/help/errors/tests from `ddx-agent-bench` to `fiz-bench`. |
| Harness adapter label `ddx-agent` in benchmark harness code/config | Domain label, retained until benchmark rename beads land | It identifies the TerminalBench/Harbor adapter arm, not the primary user CLI binary. |
| `DDX_AGENT_*` harness env vars | Existing allowlist | `internal/renamecheck` already allowlists harness env vars until the dedicated harness-rename bead updates them. |
| Comments, docs, telemetry, historical execution artifacts, and rename-check rules | Out of audit scope | This bead targets runtime-facing errors, warnings, and logs, separate from docs and telemetry. |

## Verification

- `go test ./agentcli/...`
- `go test ./cmd/bench/...`
- Runtime string audit:
  `rg -n 'ddx-agent|DDX Agent|DDx Agent|\\.agent|AGENT_[A-Z0-9_]+|DDX_AGENT_[A-Z0-9_]+' agentcli cmd internal service*.go routing_errors.go install.sh -g '!**/*_test.go'`
