---
ddx:
  id: helix.arch
  depends_on:
    - SD-001
    - SD-002
    - CONTRACT-003
---
# Architecture — Fizeau

## System Context

Fizeau is a library-first execution service. Callers submit intent
(`prompt`, `policy`, `model`, `provider`, `harness`, permissions, reasoning)
through the public `FizeauService` contract. The service
owns routing, provider construction, execution, event normalization, and
session-log persistence.

```
┌──────────────┐     ┌──────────────┐     ┌──────────────┐
│ Orchestrator │     │ CI / Worker  │     │ fiz CLI      │
│ (in-process) │     │ (in-process) │     │ (binary)     │
└──────┬───────┘     └──────┬───────┘     └──────┬───────┘
       │                    │                    │
       └────────────┬───────┴────────────────────┘
                    │
            ┌───────▼────────┐
            │ FizeauService  │
            │ service API    │
            │ Execute/List/* │
            └───────┬────────┘
                    │
        ┌───────────┴────────────┐
        │                        │
┌───────▼────────┐      ┌────────▼────────┐
│ Native path     │      │ Subprocess path │
│ route/provider  │      │ harness runners │
│ + core loop     │      │ (claude/codex/  │
│ + tools         │      │ gemini/pi/...)  │
└───────┬─────────┘      └────────┬────────┘
        │                         │
 ┌──────▼──────────┐      ┌───────▼─────────┐
 │ provider        │      │ PTY / subprocess │
 │ adapters        │      │ integration      │
 │ openai/omlx/... │      └──────────────────┘
 └─────────────────┘
```

## Module Boundaries

### 1. CLI module: `cmd/fiz` and the top-level `agentcli` package

Responsibilities:

- parse flags, stdin, env, and project working directory
- build public service requests
- call `fizeau.New`, `Execute`, `TailSessionLog`, `List*`, `ListPolicies`,
  `ResolveRoute`, and `RouteStatus`
- decode events with `DecodeServiceEvent` or `DrainExecute`
- render stdout/stderr/JSON and map status to process exit codes

Must not own:

- native provider construction
- route candidate ordering or failover
- direct `internal/core` loop invocation
- session lifecycle persistence by replaying service events into internal
  session-log types

### 2. Service module: root `fizeau` package and `service*.go`

Responsibilities:

- public contract for execution, route resolution, model/provider listing, and
  health/status
- request validation and route resolution
- native provider selection and construction from configured providers/endpoints
- subprocess harness dispatch
- event emission and typed event decoding
- session-log persistence and routing attribution
- failover policy for native-route execution

This is the only public execution boundary. `internal/core` is an implementation
detail used by the service for the native harness.

### 3. Provider adapter and routing modules: `internal/provider/*`,
`internal/routing`, model catalog, and service-owned native wrappers

Responsibilities:

- translate service/provider config into concrete provider implementations
- map public reasoning/model controls to provider-specific wire formats
- discover models and endpoint health
- rank and filter candidates
- execute native failover and report `routing_actual`

These modules are not consumer APIs. They exist to keep provider-specific and
routing-specific behavior behind the service boundary.

## Package View

```
fizeau/
├── *.go                        # public types and FizeauService methods
├── loop.go                     # native core loop implementation
├── stream_consume.go           # streaming helper for native providers
├── compaction/                 # conversation compaction
├── telemetry/                  # runtime telemetry scaffolding
├── tool/                       # built-in native tools
├── session/                    # log/replay/usage support
├── internal/
│   ├── provider/               # backend adapters
│   ├── routing/                # candidate ranking and routing policy
│   ├── harnesses/              # subprocess harness registry and runners
│   ├── modelcatalog/           # policy/model catalog
│   └── ...                     # config, safefs, prompt helpers, etc.
└── cmd/
    └── fiz/                    # first-party CLI consumer of the service
```

## Execution Flow

### Native (`fiz`) harness

```
CLI / caller
  -> FizeauService.Execute(req)
  -> service route resolution
  -> service-native provider construction
  -> core loop
  -> tools / compaction / telemetry
  -> service final event + session log
```

### Subprocess harnesses

```
CLI / caller
  -> FizeauService.Execute(req)
  -> service route resolution
  -> harness runner selection
  -> PTY / subprocess execution
  -> normalized service events
  -> service final event + session log
```

## Architectural Rules

1. The CLI is the first consumer of the service, not a parallel execution path.
2. `internal/core` is never called from `cmd/agent`.
3. Provider construction happens inside the service/provider-adapter layer.
4. Config-backed route planning and failover are service concerns.
5. Session logs are written by service-owned execution, not synthesized in the
   CLI from decoded events.
6. Any new CLI-visible execution/status behavior must be added to
   `CONTRACT-003` before the CLI reaches into internals to fetch it.

## Caching

DDx uses native provider prompt-caching where supported. The Anthropic provider
emits `cache_control: {type: "ephemeral"}` breakpoints on a stable request
prefix so multi-turn sessions hit Anthropic's prompt cache. The OpenAI-compatible
providers (gemini, lmstudio, omlx, openrouter) rely on the upstream's
auto-caching of stable prefixes; DDx's responsibility there is to keep request
prefixes byte-stable across turns.

- **Prefix order invariant.** The Anthropic request body is laid out as
  `tools` → `system` → conversation history → trailing user message, in that
  exact order. Anything before the trailing user message must remain byte-stable
  across consecutive turns within a session for the cache to hit.
- **Two-marker placement.** A `cache_control: ephemeral` breakpoint is stamped
  on the LAST tool definition and on the LAST `system` block. An Anthropic
  breakpoint marks the END of a cacheable prefix, so placing one marker at the
  end of the tool list caches `tools[*]`, and a second at the end of the system
  list caches `tools[*] + system[*]`. The conversation history that follows is
  not separately marked — the next turn re-uses these two boundaries while the
  trailing turn-specific user message remains uncached, which is the intended
  shape.
- **Compaction caveat.** `internal/compaction` rewrites retained messages to
  shrink the working context. A compaction event will change bytes inside the
  `messages` array and is therefore expected to break the cache prefix. The
  next turn pays a cache-write cost and subsequent turns re-cache against the
  new prefix. This is intentional, not a bug — compaction trades cache continuity
  for a smaller working context.
- **Tool-mutation caveat.** Any text inside a `Tool.Description` (or any other
  field of the tools/system prefix) that varies per turn silently kills caching:
  the prefix bytes change, so Anthropic treats it as a cold prefix. Tools
  registered via `agent.ToolDef` must be deterministic across turns within a
  session. If a tool needs per-turn state, surface it through tool input rather
  than baking it into the description.

The public opt-out is `Options.CachePolicy`. Values: `""` and `"default"` both
mean "cache as designed"; `"off"` suppresses BOTH the tool and system markers
on the wire and signals the cost-attribution layer to emit explicit zero
cache-amounts rather than nil.

## Key Design Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Public boundary | `DdxAgent` service contract | One execution API for CLI and embedders |
| Native execution | service-owned wrapper around `internal/core` | Preserve one core loop while hiding internals |
| Provider ownership | service/provider adapters | Keep backend-specific behavior out of consumers |
| Routing ownership | service + `internal/routing` | One place for candidate ranking and failover |
| Session logging | service-owned persistence | Avoid dual schemas and lifecycle drift |
| CLI framework | `flag` stdlib | Minimal binary surface |
