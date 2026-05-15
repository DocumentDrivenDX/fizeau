# FZ-028 Embedded Assets and Testdata Fixture Audit

Audit of `//go:embed` assets, testdata, golden files, JSON fixtures, and
generated snapshots containing old product/config/env names
(`ddx-agent`, `.agent`, `~/.config/agent`, `AGENT_*`, `DDX_AGENT_*`).

## Findings and Dispositions

| File | Line(s) | Old surface | Disposition | Rationale |
|------|---------|-------------|-------------|-----------|
| `internal/modelcatalog/catalog/models.yaml` | 4–5 | `ddx-agent catalog check/update` | **renamed** → `fiz catalog check/update` | `//go:embed` asset shipped with the product; comment documents the active CLI command name. |
| `eval/navigation/fixtures.yaml` | 1 | `ddx-agent` in header comment | **renamed** → `fizeau` | Active eval fixture loaded by `eval/navigation/eval_test.go`; header should reflect current product name. |
| `scripts/beadbench/corpus.yaml` | 7–8 | `ddx-agent corpus validate/promote` | **renamed** → `fiz corpus validate/promote` | Active corpus index loaded by `internal/corpus`; comment documents the current CLI commands. |
| `scripts/beadbench/external/termbench-subset-canary.json` | 5 | `ddx-agent` (twice in `_exclusion_rules`) | **renamed** → `fizeau` | Active JSON fixture loaded by `cmd/bench/external_termbench_test.go`; refers to the product adapter. |
| `scripts/benchmark/task-subset-v2.yaml` | 1, 19 | `ddx-agent` in header comments | **renamed** → `fizeau` | Active benchmark subset; comments describe current product workflows. |
| `scripts/benchmark/evidence-grade-comparison.env` | 4–5 | `ddx-agent binary` / `ddx-agent-linux-amd64` | **renamed** → `fizeau binary` / `fiz-linux-amd64` | Active benchmark env; comments document the binary to build and export path. |
| `scripts/benchmark/task-subset-v1.yaml` | 1 | `ddx-agent` in header comment | **allowlisted** in `renamecheck.go` `skippedFiles` | Historical placeholder subset documented to be retained per CL-004.09 and `docs/research/scripts-fixtures-assets-cleanup-inventory-2026-04-30.md`; old name is historical evidence of creation date. |

## Testdata and Golden Files

No `testdata/` directory entries (under `testdata/`, `internal/*/testdata/`,
etc.) contained old-name hits. No golden files (`.golden` suffix) exist in the
tree.

## No-Hit Confirmed

- `testdata/harness-cassettes/` — no old-name hits.
- `internal/harnesses/*/testdata/` — already covered by the pre-existing
  `internal/harnesses` `skippedDirs` allowlist (DDX_AGENT_* harness env vars,
  pending the harness-rename bead).

## Targeted Tests Run

- `go test ./internal/modelcatalog/...` — PASS
- `go test ./eval/navigation/...` — PASS
- `go test ./internal/corpus/...` — PASS
- `go test ./internal/renamecheck/...` — PASS
- `go test ./cmd/bench/... ./internal/benchmark/...` — PASS
- `python3 scripts/beadbench/test_run_beadbench.py` — 21 tests PASS

## Scoped Old-Name Grep Result

Running `renamecheck.Scan` filtered to the seven files above produces zero
findings after the renames and allowlist entry land.
