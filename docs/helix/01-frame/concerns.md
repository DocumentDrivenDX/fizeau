# Project Concerns — DDX Agent

## Area Labels

| Area | Description |
|------|-------------|
| `all` | Every bead |
| `lib` | Core library packages (agent loop, tools, providers, logging) |
| `cli` | Standalone CLI binary |

## Active Concerns

- **go-std** — Go + Standard Toolchain (areas: all)
- **testing** — Multi-layer testing with property-based, fuzz, and E2E coverage (areas: all)

## Project Overrides

### go-std

- **CLI framework**: None. DDX Agent CLI is minimal enough for `flag` stdlib. Cobra
  is not needed.
- **Test framework**: Use `testing` stdlib + `testify/assert` for assertions.
  No external test runner.
- **Structured logging**: Use `log/slog` from stdlib. No third-party logger.
- **HTTP client**: Use provider SDKs (`openai-go`, `anthropic-sdk-go`) directly.
  No custom HTTP client abstraction.

### testing

- **Property-based testing**: Use `pgregory.net/rapid` for property-based tests
  in Go. Define properties for all serialization (session log events),
  tool-call round-trips, and provider message translation.
- **Fuzz testing**: Use Go's native `testing.F` fuzz support for parsers,
  config loading, and tool input handling.
- **E2E testing**: Full agent loop E2E tests run against LM Studio with a
  loaded model (build tag `e2e`). Verify a complete file-read-and-edit
  workflow end-to-end.
- **Integration tests**: Provider integration tests against real LM Studio and
  real Anthropic API using build tags (`integration`, `e2e`). Real subprocess
  harness tests use build tag `harness_integration` so they can be run and
  reported separately from provider HTTP tests and full E2E workflows.
- **Harness integration evidence**: A harness capability is not considered
  supported by tests unless real-binary integration evidence invokes the
  installed harness through each advertised public surface (`DdxAgent.Execute`
  and/or the standalone CLI path, tracked separately). Parser tests, fixture
  replay, and fake binaries are unit/contract evidence only; they must not be
  described as proving Claude Code, OpenAI Codex, Pi, OpenCode, or any other
  external harness works. A single happy-path real run is only a harness-level
  smoke gate; it does not prove any other capability.
- **Deterministic harness tests**: When deterministic behavior is needed, use a
  reusable virtual/deterministic harness with dictionary-driven prompts,
  stable events, stable token usage, and explicit slow/error/cancel modes.
  This harness verifies shared service infrastructure and event contracts, not
  external harness compatibility. Real harness integration tests must assert
  that the resolved harness is not marked `TestOnly`.
- **Harness capability matrix**: Every harness capability must be declared in a
  machine-checkable matrix with one of: `required`, `supported`,
  `unsupported`, or `experimental`. Every `supported` capability requires real
  integration evidence for that harness and public surface. Each matrix row
  must carry evidence IDs that name the integration tests or recorded
  golden-master cassettes that prove it. CI must fail when a `supported`
  capability lacks evidence, when a row points at missing evidence, or when a
  capability is promoted from `experimental` to `supported` without adding
  evidence in the same change. `unsupported` capabilities must not be
  advertised by the public API, and requests for them must fail loudly with a
  typed capability-unsupported error rather than being silently ignored.
  `experimental` capabilities are excluded from "fully supported" claims until
  promoted and covered by integration evidence.
- **Harness capability granularity**: Do not collapse distinct harness behavior
  into vague labels. Track default model reporting, exact model pinning,
  declared/catalog model support, live model discovery, reasoning selection,
  progress events, token usage, session logging, cancellation, permission mode
  handling, workdir/context use, and quota monitoring as separate capabilities.
  Define each capability's observable contract before marking it supported; for
  example, cancellation must specify subprocess termination, final event status,
  and session-log flushing requirements.
- **Harness golden masters**: Real harness integration tests should support a
  record/replay workflow. Record mode runs the real authenticated harness,
  fails fast when the binary, credentials, subscription, configured model, tmux
  dependency, or quota surface is unavailable, and writes sanitized
  golden-master cassettes containing command arguments, binary version, event
  sequence, final metadata, usage, quota probe output, and relevant session-log
  records. Replay mode must run deterministically from those cassettes and is
  contract evidence for parser/event/session behavior, but replay alone is not
  evidence that the external harness still works today.
- **Harness live-run policy**: Skipped live harness integration tests do not
  count as passing evidence. CI must distinguish absent credentials from
  failing behavior. If live tests cannot run on every PR, run them in a
  scheduled or manually triggered job with pinned harness versions and publish
  the evidence freshness. A capability with stale, repeatedly skipped, or
  version-mismatched live evidence must be downgraded to `experimental` or
  `unsupported` until a fresh record-mode run passes.
- **Inspectable harness execution**: Live subprocess harnesses should run
  through a persistent PTY execution transport when the harness has meaningful
  terminal state, interactive status, or quota commands. On Unix, tmux is the
  preferred implementation because it preserves an attachable session, lets
  operators inspect in-flight work, and allows post-failure pane capture.
  The transport must record the session name, pane transcript, command
  arguments, exit status, and cleanup outcome. Direct non-PTY execution remains
  valid for deterministic replay, CI environments without tmux, Windows, and
  harnesses whose supported capabilities do not require terminal inspection.
  Tests for PTY-backed capabilities must prove attachability, pane capture,
  cancellation, subprocess cleanup, and session-log consistency.
- **Quota tests**: Claude Code and OpenAI Codex quota monitoring requires
  parser, cache, public API, and real quota-probe integration coverage. Tests
  should prove probe/cache/API behavior without requiring measurable quota burn;
  before/after quota-consumption deltas are manual or optional unless they can
  be made cheap, stable, and account-safe. Record mode for quota tests must
  fail fast when the harness is not authenticated rather than silently writing a
  "no quota" cassette.
- **Test data**: Use `rapid` generators for structured test data (Messages,
  ToolCalls, TokenUsage). Factory functions with sensible defaults for complex
  types.
- **Performance ratchets**: Track agent loop overhead (<1ms per iteration
  excluding model inference) and tool execution overhead via benchmarks.
