---
ddx:
  id: FZ-055-helix-docs-active-file-list
  bead: agent-621a017f
  parent: agent-2b694e0e
  created: 2026-05-01
---

# FZ-055 HELIX governance docs — active file list and allowlist

This note records which `docs/helix/**` files received in-place Fizeau
product-identity updates (active list) and which are left unchanged as
historical evidence (allowlisted).

The `internal/renamecheck` scanner already skips all of `docs/helix/` and
`docs/research/` via `skippedDirs`. This note is the human-readable companion
that explains the classification decision for each file.

## Active docs updated (PRD / FEAT / SD / CONTRACT / product vision)

These files describe the *current* product identity and received in-place
substitutions:

- `DDX Agent` / `DDx Agent` → `Fizeau`
- `ddx-agent` (CLI binary) → `fiz`
- `DdxAgent` (Go type) → `FizeauService`
- `ddx_agent.py` (Python adapter) → `fiz.py`
- `ddx-agent-bench` → `fiz-bench`
- `cmd/ddx-agent` → `cmd/fiz`
- `ddx-agent/config.yaml` (XDG path) → `fizeau/config.yaml`
- `.agent/sessions` (project session log dir) → `.fizeau/sessions`

| Path | Type |
|---|---|
| `docs/helix/00-discover/product-vision.md` | product vision |
| `docs/helix/01-frame/prd.md` | PRD |
| `docs/helix/01-frame/concerns.md` | frame concerns |
| `docs/helix/01-frame/features/FEAT-001-agent-loop.md` | FEAT |
| `docs/helix/01-frame/features/FEAT-002-tools.md` | FEAT |
| `docs/helix/01-frame/features/FEAT-003-providers.md` | FEAT |
| `docs/helix/01-frame/features/FEAT-004-model-routing.md` | FEAT |
| `docs/helix/01-frame/features/FEAT-005-logging-and-cost.md` | FEAT |
| `docs/helix/01-frame/features/FEAT-006-standalone-cli.md` | FEAT |
| `docs/helix/01-frame/features/FEAT-007-self-update-and-installer.md` | FEAT |
| `docs/helix/02-design/solution-designs/SD-001-agent-core.md` | SD |
| `docs/helix/02-design/solution-designs/SD-002-standalone-cli.md` | SD |
| `docs/helix/02-design/solution-designs/SD-003-system-prompts.md` | SD |
| `docs/helix/02-design/solution-designs/SD-004-streaming.md` | SD |
| `docs/helix/02-design/solution-designs/SD-005-provider-config.md` | SD |
| `docs/helix/02-design/solution-designs/SD-006-compaction.md` | SD |
| `docs/helix/02-design/solution-designs/SD-007-provider-import.md` | SD |
| `docs/helix/02-design/solution-designs/SD-008-terminal-bench-integration.md` | SD |
| `docs/helix/02-design/solution-designs/SD-009-benchmark-mode.md` | SD |
| `docs/helix/02-design/solution-designs/SD-010-harness-matrix-benchmark.md` | SD |
| `docs/helix/02-design/contracts/CONTRACT-001-otel-telemetry-capture.md` | CONTRACT |
| `docs/helix/02-design/contracts/CONTRACT-003-ddx-agent-service.md` | CONTRACT |

## Historical docs — allowlisted (old names preserved as evidence)

These files record decisions, research, and reviews made when the product was
named DDX Agent. The old product names are historical evidence; do not rewrite
them. They remain covered by the `docs/helix/` entry in `skippedDirs` in
`internal/renamecheck/renamecheck.go`.

### ADRs

| Path | Notes |
|---|---|
| `docs/helix/02-design/adr/ADR-001-observability-surfaces-and-cost-attribution.md` | Decision made under DDX Agent identity |
| `docs/helix/02-design/adr/ADR-002-pty-cassette-transport.md` | Decision made under DDX Agent identity |
| `docs/helix/02-design/adr/ADR-003-pty-terminal-rendering.md` | Decision made under DDX Agent identity |
| `docs/helix/02-design/adr/ADR-004-terminal-harness-build-vs-buy.md` | Decision made under DDX Agent identity |
| `docs/helix/02-design/adr/ADR-005-smart-routing-replaces-model-routes.md` | Decision made under DDX Agent identity |
| `docs/helix/02-design/adr/ADR-006-overrides-as-routing-failure-signals.md` | Decision made under DDX Agent identity |
| `docs/helix/02-design/adr/ADR-007-sampling-profiles-in-catalog.md` | Decision made under DDX Agent identity |

### Plans

| Path | Notes |
|---|---|
| `docs/helix/02-design/plan-2026-04-08-rename-agent.md` | Historical rename plan (predates Fizeau decision) |
| `docs/helix/02-design/plan-2026-04-08-shared-model-catalog.md` | Historical implementation plan |
| `docs/helix/02-design/plan-2026-04-10-catalog-distribution-and-refresh.md` | Historical implementation plan |
| `docs/helix/02-design/plan-2026-04-10-model-first-routing.md` | Historical implementation plan |
| `docs/helix/02-design/plan-2026-04-19-provider-routing-tool-output.md` | Historical implementation plan |

### Benchmarks and baselines

| Path | Notes |
|---|---|
| `docs/helix/02-design/benchmark-baseline-2026-04-08.md` | Historical baseline (old product identity) |
| `docs/helix/02-design/benchmark-comparison-2026-04-10-evidence-grade.md` | Historical comparison evidence |
| `docs/helix/02-design/benchmark-corpus.md` | Historical corpus definition |
| `docs/helix/02-design/external-benchmarks.md` | Historical external benchmark evidence |
| `docs/helix/02-design/primary-harness-capability-baseline.md` | Historical harness baseline |
| `docs/helix/02-design/epic-validation-e8c1f21c.md` | Historical epic validation record |
| `docs/helix/02-design/harness-golden-integration.md` | Historical integration record |

### SPIKEs

| Path | Notes |
|---|---|
| `docs/helix/02-design/spikes/SPIKE-001-direct-pty-top-rendering.md` | Historical spike |
| `docs/helix/02-design/spikes/SPIKE-002-terminal-driver-recorder-alternatives.md` | Historical spike |

### Alignment reviews

| Path | Notes |
|---|---|
| `docs/helix/06-iterate/alignment-reviews/AR-2026-04-07-config-import.md` | Historical review |
| `docs/helix/06-iterate/alignment-reviews/AR-2026-04-07-repo.md` | Historical review |
| `docs/helix/06-iterate/alignment-reviews/AR-2026-04-08-presets-routing.md` | Historical review |
| `docs/helix/06-iterate/alignment-reviews/AR-2026-04-09-repo.md` | Historical review |
| `docs/helix/06-iterate/alignment-reviews/AR-2026-04-10-repo.md` | Historical review |
| `docs/helix/06-iterate/alignment-reviews/AR-2026-04-12-repo.md` | Historical review |
| `docs/helix/06-iterate/alignment-reviews/AR-2026-04-17-repo.md` | Historical review |
| `docs/helix/06-iterate/alignment-reviews/AR-2026-04-25-routing-and-overrides.md` | Historical review |
| `docs/helix/06-iterate/metrics/test-coverage.yaml` | Historical metrics snapshot |

## Renamecheck coverage

`internal/renamecheck/renamecheck.go` covers the active source tree. The
`docs/helix/` directory is listed in `skippedDirs`, so the checker does not
scan any HELIX file. After this bead lands, the active files above contain only
Fizeau product identity. The historical files above retain old names as
allowlisted evidence.
