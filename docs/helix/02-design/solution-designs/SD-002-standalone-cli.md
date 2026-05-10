---
ddx:
  id: SD-002
  depends_on:
    - FEAT-006
    - SD-001
---
# Solution Design: SD-002 — Standalone CLI

**Feature**: FEAT-006 (Standalone CLI)

## Scope

Feature-level design for the `fiz` CLI binary — the thin porcelain that
proves the Fizeau library works end-to-end. The CLI is not the product; the
library is. This design covers the binary, config loading, and session
subcommands.

## Requirements Mapping

### Functional Requirements

| Requirement | Technical Capability | Component | Priority |
|-------------|---------------------|-----------|----------|
| Non-interactive mode (FEAT-006 FR-1..4) | `fiz run "prompt"`, `-p`, stdin | `cmd/fiz` | P0 |
| Exit codes (FEAT-006 FR-4) | 0/1/2 mapping | `cmd/fiz` | P0 |
| Output modes (FEAT-006 FR-5..6) | stdout text, --json, stderr progress | `cmd/fiz` | P0 |
| Config file (FEAT-006 FR-7..10) | YAML config + env + flags | `cmd/fiz` | P0 |
| Session commands (FEAT-006 FR-11..14) | log, replay, usage subcommands | `cmd/fiz` | P1 |
| Harness mode (FEAT-006 FR-15..16) | stdin prompt, JSON output | `cmd/fiz` | P0 |

### NFR Impact

| NFR | Requirement | Design Decision |
|-----|-------------|-----------------|
| Startup time | <50ms to first LLM request | No heavy init; parse config, build one service request, dispatch |
| Binary size | <20MB static binary | Minimal deps, no TUI libraries |
| Zero config | Works with LM Studio defaults | Sensible defaults for localhost:1234 |

## Solution Approach

The CLI is a single `cmd/fiz/main.go` entry point using Go's `flag` stdlib
package (per project concern override — no Cobra). Subcommands are dispatched
by the first positional argument. `run` is the preferred explicit execution
verb; the existing bare `-p` path remains as a compatibility shim.

The CLI is porcelain over the `FizeauService` service contract from
CONTRACT-003. It parses flags, resolves prompt input, constructs a public
`ServiceExecuteRequest`, calls `FizeauService.Execute`, and renders the typed event
stream. It does not construct providers, call `agent.Run()`, own failover, or
write session lifecycle records itself.

### Command Structure

```
fiz run "prompt"             # preferred run path
fiz run @file.md             # prompt from file
echo "prompt" | fiz run      # prompt from stdin
fiz --json run "prompt"      # JSON output
fiz -p "prompt"              # legacy compatibility

fiz log                      # list recent sessions
fiz log <session-id>         # show session detail
fiz replay <session-id>      # human-readable replay
fiz usage                    # cost/token summary
fiz usage --since=7d         # with time window
```

### Config Resolution Order

1. Built-in defaults (localhost:1234, openai-compat, 20 iterations)
2. Global config: `~/.config/fizeau/config.yaml`
3. Project config: `.fizeau/config.yaml`
4. Environment variables: `FIZEAU_PROVIDER`, `FIZEAU_BASE_URL`, `FIZEAU_API_KEY`,
   `FIZEAU_MODEL`
5. CLI flags: `--provider`, `--model`, `--model-ref`, `--max-iter`,
   `--work-dir`

Later sources override earlier ones.

### Config File Format

```yaml
provider: openai-compat
base_url: http://localhost:1234/v1
api_key: ""
model: qwen3.5-7b
max_iterations: 20
session_log_dir: .fizeau/sessions
```

### Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Agent completed successfully |
| 1 | Agent failed (error, iteration limit, provider error) |
| 2 | CLI usage error (bad flags, missing config) |

## System Decomposition

### `cmd/fiz/main.go`

- Parse flags and subcommand
- Load config (file → env → flags)
- Resolve prompt input and CLI defaults into a public `ServiceExecuteRequest`
- Call `FizeauService.Execute` / `TailSessionLog` / `List*` methods
- Decode events with `DecodeServiceEvent` or `DrainExecute`
- Print result, set exit code

### Config loader (internal to cmd)

- YAML parsing with `gopkg.in/yaml.v3`
- Env var overlay
- Flag overlay
- Produce service-owned routing/config inputs; provider construction stays in the service

### Session subcommands (internal to cmd)

- `log`: List session files from log directory, show summary
- `replay`: Render stored service session logs
- `usage`: Aggregate stored session logs with time filtering

### Boundary Rules

- `cmd/fiz` is a consumer of `FizeauService`, not a peer of the core loop.
- Routing intent belongs in public request fields (`Provider`, `Model`,
  `ModelRef`, `Profile`, `Harness`) or an explicit `ResolveRoute` ->
  `PreResolved` flow.
- Native provider construction, route failover, and session-log persistence are
  service responsibilities.
- Boundary tests must prevent `cmd/fiz` from importing or invoking core
  execution internals directly.

## Technology Rationale

| Layer | Choice | Why |
|-------|--------|-----|
| CLI framework | `flag` stdlib | Minimal, no dependency, sufficient for this scope |
| Config format | YAML | Human-readable, consistent with project conventions |
| YAML parser | `gopkg.in/yaml.v3` | De facto standard Go YAML library |

## Traceability

| Requirement | Component | Test Strategy |
|-------------|-----------|---------------|
| FEAT-006 FR-1..4 | main.go prompt handling | Functional: run binary with `run`, `-p`, and stdin |
| FEAT-006 FR-4 | main.go exit codes | Functional: check exit codes for success/failure/usage |
| FEAT-006 FR-5..6 | main.go output | Functional: text vs `--json` output |
| FEAT-006 FR-7..10 | config loader | Unit: config merging from file/env/flags |
| FEAT-006 FR-11..14 | session subcommands | Functional: run against test session logs |
| FEAT-006 FR-15..16 | main.go harness mode | Integration: harness invocation via CONTRACT-003 |
| CONTRACT-003 CLI boundary | boundary tests | Static/import tests proving CLI stays behind the service layer |

## Concern Alignment

- **Concerns used**: go-std (areas: all)
- **Project override applied**: `flag` stdlib instead of Cobra
- **Constraints honored**: `gofmt`, `go vet`, version metadata via `-ldflags`

## Risks

| Risk | Prob | Impact | Mitigation |
|------|------|--------|------------|
| `flag` stdlib too limited for subcommands | L | L | Subcommand dispatch is trivial; upgrade to Cobra later if needed |
| Config file format drift | L | L | Follow same YAML conventions |

## Review Checklist

- [x] Requirements mapping covers all FEAT-006 functional requirements
- [x] Command structure is clear and documented
- [x] Config resolution order is explicit
- [x] Exit codes defined
- [x] Technology choices justified
- [x] Traceability complete
- [x] Concern alignment verified
