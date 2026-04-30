---
ddx:
  id: fizeau-public-symbol-rename-table-2026-04-30
  bead: agent-81cd8519
  parent: agent-2b694e0e
  created: 2026-04-30
---

# FZ-012a public Go symbol rename table

This is a decision artifact only. It audits exported Go symbols containing
`Agent`/`agent` and decides whether each is product branding that must become
Fizeau, or domain terminology that should remain.

## Audit Commands

```bash
rg -n '\b(DdxAgent|AgentInfo|AgentName|[A-Z][A-Za-z0-9]*Agent[A-Za-z0-9]*|Agent[A-Z][A-Za-z0-9]*)\b' *.go --glob '*.go'
rg -n '\b(DdxAgent|AgentInfo|AgentName|[A-Z][A-Za-z0-9]*Agent[A-Za-z0-9]*|Agent[A-Z][A-Za-z0-9]*)\b' --glob '*.go' internal agentcli cmd
rg -n '^type |^func |^var |^const ' *.go
```

## Decision Table

| symbol / surface | location | decision | reason |
|---|---|---|---|
| `DdxAgent` | root package, `service.go` | rename | Product-specific public service interface. It names this module rather than the generic LLM-agent domain. Rename to `Fizeau` or `FizeauService` in the dedicated public API rename bead. |
| `New(...) (DdxAgent, error)` | root package, `service.go` | update signature with interface rename | Constructor name can remain `New`, but its return type should follow the `DdxAgent` rename. |
| `ServiceOptions` comments mentioning `DdxAgent` | root package, `service.go` | update prose | Comments describe the public product service and should use the renamed interface. |
| `UsageReportOptions` comments mentioning `DdxAgent.UsageReport` | root package, `service_session_projection.go` | update prose | Comment should follow the public interface rename. |
| `AgentInfo` | `internal/benchmark/external/termbench/trajectory.go` | keep | Domain/schema term from Terminal-Bench/Harbor trajectory output: identifies the executor recorded in a benchmark trajectory, not product branding. |
| `AgentName` | not currently defined | no code action | No exported Go symbol named `AgentName` exists in the current tree. If introduced later as product branding, it should use `FizeauName`; if it identifies a benchmark executor, it may remain domain terminology. |
| `SurfaceAgentOpenAI` / `SurfaceAgentAnthropic` | `internal/modelcatalog` | keep | Internal model-catalog surface names distinguish agent-compatible provider APIs from other possible catalog surfaces. This is domain terminology, not product branding. |
| `DefaultAgentTimeoutSec` | `internal/benchmark/external/termbench` | keep | Mirrors benchmark task semantics: an agent has a wall-clock budget. Not product branding. |
| `MaxAgentTimeoutSec` | `internal/benchmark/external/termbench` | keep | Mirrors benchmark task semantics and YAML field naming. Not product branding. |
| `HarborAgent` | `cmd/bench/matrix.go` | keep | Refers to the Harbor adapter's agent class/name in benchmark metadata. Product-facing label values may change under benchmark label beads, but the field name is domain-specific. |
| `NonZeroAgentExitCodeError` | agentcli tests / process behavior | keep | Describes a generic child-agent process failure category, not Fizeau branding. |

## Follow-Up Rules

- Rename public root product symbols in the API rename bead, not as part of this
  decision bead.
- Keep generic LLM-agent terminology where the symbol describes the domain,
  a benchmark schema, or a third-party task format.
- When a kept domain symbol carries a product-facing string value such as
  `ddx-agent`, change the value in the relevant benchmark/product rename bead
  without renaming the Go symbol itself.

## Acceptance Traceback

Bead `agent-81cd8519` requires a decision table for `DdxAgent`, `AgentInfo`,
`AgentName`, and any other exported agent-named symbols.

- `DdxAgent`: listed with rename decision.
- `AgentInfo`: listed with keep decision.
- `AgentName`: listed as absent with future rule.
- Other exported agent-named symbols surfaced by the audit are listed with
  keep/rename decisions and reasons.
