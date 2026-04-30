---
ddx:
  id: cleanup-go-inventory-2026-04-30
  bead: agent-616d51ed
  parent: agent-996dca04
  base-rev: 10446491543d694cecf0ade4a542980f305e5ed9
  created: 2026-04-30
  format: cleanup-criteria-2026-04-30
---

# CL-002 — Go package and file inventory (pre-Fizeau rename)

This is an inventory artifact only. No code is deleted in this bead. The
`delete` and `follow-up` rows below are the input to CL-005 (the Go
deletion bead). Every row cites concrete `go list`, `go test`, and `rg`
evidence per the table format defined in
`docs/research/cleanup-criteria-2026-04-30.md`.

The goal is to shrink the Fizeau rename surface, not to scrub the
codebase. Anything uncertain is `keep` or `follow-up`, never `delete`.

## Method and baseline evidence

Baseline commands run against `HEAD` =
`10446491543d694cecf0ade4a542980f305e5ed9`:

- `go list ./...` → 59 packages (full list captured at end of doc).
- `go test ./...` → all packages pass (PASS green; tail output shows
  `ok` or `[no test files]` for every package).
- `go vet ./...` → clean (exit 0).
- `golangci-lint run ./...` → 0 issues (`.golangci.yml` enables only
  `misspell`).

Reverse-importer table built by parsing
`go list -f '{{.ImportPath}}: {{.Imports}} {{.TestImports}} {{.XTestImports}}' ./...`
and grouping by repo-internal import path. Every direct importer
(production or test) of every package is captured.

Packages **not transitively reachable from any `cmd/*` binary**
(`go list -deps ./cmd/agent ./cmd/bench ./cmd/benchscore ./cmd/catalogdist`
diff against `go list ./...`):

```
github.com/DocumentDrivenDX/agent/configinit
github.com/DocumentDrivenDX/agent/eval/navigation
github.com/DocumentDrivenDX/agent/internal/execution
github.com/DocumentDrivenDX/agent/internal/provider/conformance
github.com/DocumentDrivenDX/agent/internal/pty
github.com/DocumentDrivenDX/agent/internal/ptytest
github.com/DocumentDrivenDX/agent/scripts
```

Each of these is examined in the table below. Packages reachable from a
binary are not enumerated as candidates — they are live by definition
and the rename will pick them up.

## Inventory table

| id | path | type | classification | reason | evidence | superseder / replacement | follow-up |
|---|---|---|---|---|---|---|---|
| `CL-002.01` | `internal/execution/` (package + `deadline.go`, `deadline_test.go`) | go-package | delete | Orphan: package is not imported by any production code or test outside its own package. The functionality (`WrapProviderWithDeadlinesTimeouts`, `ErrProviderRequestTimeout`) is mirrored in-package by `service_execute.go` (`wrapProviderRequestTimeout`, `errProviderRequestTimeout`) to avoid an import cycle. The CLI does not import it either, so the comment claiming it is "reachable via the CLI command layer" is stale. | (a) `go list ./...` includes it (line 11 of full list). (b) `rg -n 'DocumentDrivenDX/agent/internal/execution' --glob '*.go'` returns no matches anywhere in the repo (the only `.go` references are non-import comments in `service_execute.go:697,1361,1369` mentioning the package by name in a doc comment). (c) `rg -n '\bWrapProviderWithDeadlines\b' --glob '*.go'` outside `internal/execution/`: only matches in `service_execute.go` are doc comments, not calls. (d) Trial removal: `rm -rf internal/execution` then `go list ./...` exit 0; `go vet ./...` exit 0; `go build ./...` exit 0; `go test -count=1 ./...` exit 0 (every package still PASS); `golangci-lint run ./...` reports `0 issues`. Restored after each trial; no trial commit. | None — service_execute.go already contains the in-package equivalent. The deletion bead may also drop the three doc comments in service_execute.go that reference `internal/execution.*` since they describe a non-existent path. | When CL-005 lands the delete: also remove the stale doc-comment references at `service_execute.go:697-700`, `service_execute.go:1361-1362`, `service_execute.go:1369` (or rewrite to drop the `internal/execution` mention). Re-run the trial commands above on `HEAD` before staging. |
| `CL-002.02` | `internal/ptytest/` (package: `assertions.go`, `scenario.go`, `scenario_test.go`, `docker_conformance_integration_test.go`, `testdata/docker-conformance/`) | go-package | keep | Test-helper package with no Go importers but explicitly named in `ADR-002` and `ADR-004` as the canonical PTY cassette assertion layer. Its own integration tests run under `-tags=integration` and validate the cassette transport conformance fixtures (`top.sh/yaml`, `less.sh/yaml`, `vim.sh/yaml`). Removing it would delete documented HELIX-phase test infrastructure and break the ADR-002 conformance suite. | (a) `go list ./...` includes it. (b) `rg -n 'DocumentDrivenDX/agent/internal/ptytest' --glob '*.go'` returns matches only inside `internal/ptytest/` itself. (c) `rg -n 'internal/ptytest' docs/` returns four hits in `docs/helix/02-design/harness-golden-integration.md:174,180,190` and ADR-002/ADR-004 (`docs/helix/02-design/adr/ADR-002-pty-cassette-transport.md:104`, `docs/helix/02-design/adr/ADR-004-terminal-harness-build-vs-buy.md:60,62`). (d) `go test ./internal/ptytest/...` PASS. (e) `go test -tags=integration ./internal/ptytest/...` PASS. | n/a | None for CL-005. Future consumer wiring is tracked under PTY/harness work, not under cleanup. |
| `CL-002.03` | `internal/pty/` (`doc.go` only) | go-package | keep | Pure documentation marker package introduced for bead `agent-949a5ba4` per ADR-002/ADR-004/SPIKE-001/SPIKE-002. Contains a single `package pty` doc comment defining the architectural boundary; no Go symbols. Removing it loses load-bearing architectural context. | (a) `go list ./...` includes it. (b) `internal/pty/doc.go` is the only `.go` file at that level (subdirs `cassette`, `session`, `terminal` are separate packages with importers). (c) `rg -n 'internal/pty\b' docs/` lists `harness-golden-integration.md`, `ADR-002`, `ADR-004` referencing the boundary. | n/a | None. |
| `CL-002.04` | `configinit/` (`configinit.go`) | go-package | keep | Public marker package whose blank-import side-effect registers the `internal/config` ConfigPath loader with the root `agent` package. Documented in `README.md:94` as the canonical embedding pattern for external consumers (`import _ "github.com/DocumentDrivenDX/agent/configinit"`). Also referenced in `website/content/docs/getting-started.md`. | (a) `go list ./...` includes it. (b) `rg -n 'configinit' README.md website/content/docs/getting-started.md` confirms documented public usage. (c) `rg -n 'DocumentDrivenDX/agent/configinit' --glob '*.go'` outside `configinit/` returns no in-repo `.go` importers — but the package is by design a marker for *external* consumers, so absence of internal importers is expected. | n/a | None. The Fizeau rename will retitle the import path; that is FZ work, not CL. |
| `CL-002.05` | `eval/navigation/` (`eval_test.go`, `fixtures.yaml`) | go-package | keep | Test-only micro-eval package documented in `SD-009 §6` (referenced from `eval_test.go:7`). Runs under `go test ./eval/navigation` and validates that the agent loop wires up the `find/grep/ls/read` navigation tool set without a live LLM. | (a) `go list ./...` includes it. (b) `go test ./eval/navigation` PASS. (c) `rg -n 'eval/navigation' docs/` returns SD-009 references. | n/a | None. |
| `CL-002.06` | `scripts/` (`coverage-ratchet.go`) | go-package | keep | `package main` build target invoked by `Makefile` (`coverage-ratchet`, `coverage-bump`, `coverage-trend` all run `go run scripts/coverage-ratchet.go ...`). Active CI tooling. | (a) `go list ./...` includes it. (b) `rg -n 'scripts/coverage-ratchet.go' Makefile` matches three Make targets. | n/a | None. |
| `CL-002.07` | `internal/provider/conformance/` (`doc.go`, `run.go`) | go-package | keep | Test-only shared provider conformance suite. Imported by `internal/provider/anthropic.test` and `internal/provider/openai.test` (`go list -test -f '{{.ImportPath}}: {{.Imports}}'` shows both `.test` packages reference it). Not reachable from any binary because it is only imported by `_test.go` files. | (a) `go list ./...` includes it. (b) `go list -test -f '{{.ImportPath}}: {{.Imports}}' ./internal/provider/anthropic ./internal/provider/openai` shows `internal/provider/conformance` in the test-import lists. (c) `rg -n 'provider/conformance' --glob '*_test.go'` matches `internal/provider/anthropic/*` and `internal/provider/openai/*` test files. | n/a | None. |

## Notes on candidates that did not make the cut

- **`internal/sessionlog`**, **`internal/safefs`**, **`internal/compactionctx`**,
  **`internal/productinfo`**, **`internal/provider/limits`**,
  **`internal/provider/lucebox`**, **`internal/provider/vllm`**: each
  has `[no test files]` in `go test ./...`, but each has multiple
  production importers (verified in the reverse-importer dump). All
  classify trivially as `keep` and are not enumerated in the main
  table.
- **Root-package `service_*.go` files**: all live in `package agent`
  and are wired into `service.go`, `service_execute.go`, etc. The
  rename will retitle them as part of FZ-001; CL-002 does not touch
  them.
- **`agentcli/*.go`**: every file is part of the CLI binary built by
  `cmd/agent`. The rename touches them; CL-002 does not.
- **`internal/execution/deadline_test.go`** is implicitly covered by
  row `CL-002.01` — the test file dies with the package.

## Trial-removal evidence (CL-002.01 — `internal/execution`)

Steps performed locally, not committed (worktree restored to
`HEAD = 10446491543d694cecf0ade4a542980f305e5ed9` after each):

```
$ cp -r internal/execution /tmp/exec_backup
$ rm -rf internal/execution
$ go list ./...
# exit 0; produces a 58-package list (no internal/execution).
$ go vet ./...
# exit 0
$ go build ./...
# exit 0
$ go test -count=1 ./...
# exit 0 — every package PASS or [no test files]; tail confirms
#   ok    github.com/DocumentDrivenDX/agent/...   (all green)
$ golangci-lint run ./...
# 0 issues
$ mv /tmp/exec_backup internal/execution
```

The deletion bead (CL-005) must re-run this exact sequence on `HEAD`
before staging, and additionally drop the stale doc-comment references
in `service_execute.go` listed in the row's `follow-up` column.

## Per-deletion checklist for CL-005 (`CL-002.01` row)

Reproduced from `cleanup-criteria-2026-04-30.md` so the deletion bead
can tick each item:

- [ ] Inventory row `CL-002.01` exists (this doc) and is cited in the
      CL-005 commit message subject.
- [ ] Re-run the row's evidence commands on `HEAD`; results still
      match (`rg` returns no Go importers; trial-removal sequence is
      green).
- [ ] No new references have appeared since this inventory landed
      (`rg -n 'DocumentDrivenDX/agent/internal/execution' --glob '*.go'`
      empty; `rg -n '\bWrapProviderWithDeadlines\b' --glob '*.go'`
      outside `internal/execution/` empty).
- [ ] `go test ./...`, `go vet ./...`, `golangci-lint run ./...` all
      pass after the deletion is staged.
- [ ] Stale doc-comment references in `service_execute.go:697-700,
      1361-1362, 1369` are dropped or rewritten in the same commit.
- [ ] Commit stages only `internal/execution/deadline.go`,
      `internal/execution/deadline_test.go`, the empty
      `internal/execution/` directory, and the `service_execute.go`
      doc-comment edits — no drive-by changes.
- [ ] Commit message subject ends with `[CL-002.01]`.

## Out of scope (deferred)

- Symbol-level dead-code analysis inside live packages
  (`agentcli/*.go`, root `service_*.go`, etc.). The active linter
  config (`misspell` only) does not flag unused exports, and a
  full audit is out of scope for the rename-surface goal.
- `cmd/bench` / `cmd/benchscore` / `cmd/catalogdist` cleanup. Each
  binary is reachable; rename surface is bounded.
- HELIX retrospective scripts/fixtures under `bench/`, `demos/`,
  `testdata/`. Those belong to CL-004 (scripts/fixtures inventory),
  not this Go-only inventory.

## Full `go list ./...` capture (HEAD = `10446491`)

```
github.com/DocumentDrivenDX/agent
github.com/DocumentDrivenDX/agent/agentcli
github.com/DocumentDrivenDX/agent/benchscore
github.com/DocumentDrivenDX/agent/catalogdist
github.com/DocumentDrivenDX/agent/cmd/agent
github.com/DocumentDrivenDX/agent/cmd/bench
github.com/DocumentDrivenDX/agent/cmd/benchscore
github.com/DocumentDrivenDX/agent/cmd/catalogdist
github.com/DocumentDrivenDX/agent/configinit
github.com/DocumentDrivenDX/agent/eval/navigation
github.com/DocumentDrivenDX/agent/internal/benchmark/external/termbench
github.com/DocumentDrivenDX/agent/internal/benchmark/profile
github.com/DocumentDrivenDX/agent/internal/compaction
github.com/DocumentDrivenDX/agent/internal/compactionctx
github.com/DocumentDrivenDX/agent/internal/comparison
github.com/DocumentDrivenDX/agent/internal/config
github.com/DocumentDrivenDX/agent/internal/core
github.com/DocumentDrivenDX/agent/internal/corpus
github.com/DocumentDrivenDX/agent/internal/execution
github.com/DocumentDrivenDX/agent/internal/harnesses
github.com/DocumentDrivenDX/agent/internal/harnesses/claude
github.com/DocumentDrivenDX/agent/internal/harnesses/codex
github.com/DocumentDrivenDX/agent/internal/harnesses/gemini
github.com/DocumentDrivenDX/agent/internal/harnesses/opencode
github.com/DocumentDrivenDX/agent/internal/harnesses/pi
github.com/DocumentDrivenDX/agent/internal/harnesses/ptyquota
github.com/DocumentDrivenDX/agent/internal/modelcatalog
github.com/DocumentDrivenDX/agent/internal/observations
github.com/DocumentDrivenDX/agent/internal/productinfo
github.com/DocumentDrivenDX/agent/internal/prompt
github.com/DocumentDrivenDX/agent/internal/provider/anthropic
github.com/DocumentDrivenDX/agent/internal/provider/conformance
github.com/DocumentDrivenDX/agent/internal/provider/limits
github.com/DocumentDrivenDX/agent/internal/provider/lmstudio
github.com/DocumentDrivenDX/agent/internal/provider/lucebox
github.com/DocumentDrivenDX/agent/internal/provider/ollama
github.com/DocumentDrivenDX/agent/internal/provider/omlx
github.com/DocumentDrivenDX/agent/internal/provider/openai
github.com/DocumentDrivenDX/agent/internal/provider/openrouter
github.com/DocumentDrivenDX/agent/internal/provider/registry
github.com/DocumentDrivenDX/agent/internal/provider/virtual
github.com/DocumentDrivenDX/agent/internal/provider/vllm
github.com/DocumentDrivenDX/agent/internal/pty
github.com/DocumentDrivenDX/agent/internal/pty/cassette
github.com/DocumentDrivenDX/agent/internal/pty/session
github.com/DocumentDrivenDX/agent/internal/pty/terminal
github.com/DocumentDrivenDX/agent/internal/ptytest
github.com/DocumentDrivenDX/agent/internal/reasoning
github.com/DocumentDrivenDX/agent/internal/routing
github.com/DocumentDrivenDX/agent/internal/safefs
github.com/DocumentDrivenDX/agent/internal/sampling
github.com/DocumentDrivenDX/agent/internal/sdk/openaicompat
github.com/DocumentDrivenDX/agent/internal/session
github.com/DocumentDrivenDX/agent/internal/sessionlog
github.com/DocumentDrivenDX/agent/internal/tool
github.com/DocumentDrivenDX/agent/occompat
github.com/DocumentDrivenDX/agent/picompat
github.com/DocumentDrivenDX/agent/scripts
github.com/DocumentDrivenDX/agent/telemetry
```

## Acceptance traceback

Bead `agent-616d51ed` AC: *"Inventory cites `go list ./...`,
`go test ./...`, and `rg` evidence for each candidate; every
candidate has keep/delete/follow-up classification."*

- `go list ./...` baseline + per-row mention: §"Method and baseline
  evidence" and full capture.
- `go test ./...` baseline + per-row trial: §"Method and baseline
  evidence" and §"Trial-removal evidence (CL-002.01)".
- `rg` evidence: every row's `evidence` cell cites concrete `rg`
  invocations (paths and globs) and their results.
- Classification: every row in the table carries exactly one of
  `delete` / `keep` / `archive` / `follow-up`. Per the bead's notes,
  uncertain candidates default to `keep`/`follow-up`; only `CL-002.01`
  is `delete`, with full trial-removal evidence and a documented
  follow-up step for the deletion bead.
