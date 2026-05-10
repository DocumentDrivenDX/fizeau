---
title: Architecture
linkTitle: Architecture
weight: 3
description: "Package structure, design principles, and the decision record."
---

Fizeau is a Go library and CLI for driving language-model agents through
provider-agnostic tools, harnesses, and routing. This page describes the
shape of the codebase and the principles behind it; the
[ADR index](adr/) captures individual decisions, dated and numbered.

## Design principles

- **Public facade, private mechanics.** The root `fizeau` package exports
  service interfaces, request/response/event types, constructors, and errors.
  Concrete execution, transcript, routing-health, quota, and aggregation
  mechanics live behind `internal/` packages so they can change without
  breaking embedders.
- **Round-trip timing as a first-class signal.** Named for the physicist who
  measured the speed of light with a rotating wheel, the project treats
  per-turn timing, replay fidelity, and the gap between measured and rest
  frames as observability primitives, not afterthoughts. See
  [the name](../about/the-name/).
- **Replay first, analytics second.** JSONL session logs are the canonical
  replay artifact. OpenTelemetry GenAI spans and metrics are the analytics
  surface. Cost is recorded when reported, never guessed from stale tables
  ([ADR-001](adr/ADR-001/)).
- **Routing by power, overrides as failure signals.** Callers choose a named
  policy (`cheap`, `default`, `smart`, `air-gapped`) or numeric power bounds.
  Manual model/provider/harness pins are escape hatches — when overrides
  spike, that's a routing-quality regression, not a feature
  ([ADR-005](adr/ADR-005/), [ADR-006](adr/ADR-006/)).

## Package layout

### Public surface

| Package | Role |
|---------|------|
| `fizeau` (root) | Public facade: service interfaces, request/response/event types, constructors, errors, compatibility wrappers, and contract tests. |
| `agentcli/` | Mountable CLI command tree backed by the public service facade. |
| `cmd/fiz/` | Standalone CLI binary; gated behind a service-boundary import allowlist. |
| `telemetry/` | Runtime telemetry scaffolding for `invoke_agent` / `chat` / `execute_tool` spans. |

### Service implementation

| Package | Role |
|---------|------|
| `internal/serviceimpl/` | Concrete service execution and session-log dispatch used by the root facade. |
| `internal/transcript/` | Service-owned transcript, progress, session-log, and replay rendering helpers. |
| `internal/session/` | Session log writer, replay renderer, and usage aggregation. |
| `internal/sessionlog/` | Session log primitives. |

### Agent loop and tools

| Package | Role |
|---------|------|
| `internal/core/` | Reusable agent loop, provider/tool contracts, core events, stream consumption. |
| `internal/tool/` | Built-in tools (read, write, edit, bash). |
| `internal/skill/` | Skill loading and dispatch. |
| `internal/compaction/` & `internal/compactionctx/` | Conversation compaction and prefix-token accounting. |
| `internal/prompt/` | Prompt presets bundling tool sets and system prompts. |

### Providers and harnesses

| Package | Role |
|---------|------|
| `internal/provider/` | Native provider backends and provider registries. |
| `internal/harnesses/` | Subprocess harness adapters and quota/account discovery. |
| `internal/sdk/` | Provider-SDK glue. |

### Routing, quota, and modeling

| Package | Role |
|---------|------|
| `internal/routing/` | Power-based routing engine. |
| `internal/routehealth/` | Process-local route-attempt feedback, cooldown, TTL, reliability signals. |
| `internal/routingquality/` | Routing-quality ring, aggregation, and override-class pivot. |
| `internal/quota/` | Provider-level quota state machine and burn-rate prediction. |
| `internal/modelcatalog/` & `internal/modelmatch/` | Schema-versioned model catalog and matching. |
| `internal/sampling/` | Sampling profiles per model ([ADR-007](adr/ADR-007/)). |
| `internal/reasoning/` | Reasoning-effort modeling. |

### Terminal capture and benchmark

| Package | Role |
|---------|------|
| `internal/pty/` & `internal/ptytest/` | Direct PTY transport and rendering ([ADR-002](adr/ADR-002/), [ADR-003](adr/ADR-003/)). |
| `internal/benchmark/` & `internal/corpus/` | Benchmark harness and corpus loaders. |
| `internal/comparison/` | Cross-tool comparison primitives. |
| `internal/observations/` | Observation capture and projection. |

### Cross-cutting

| Package | Role |
|---------|------|
| `internal/config/` | Multi-provider YAML config. |
| `internal/safefs/` | Centralized wrappers for intentional filesystem reads/writes (scoped `gosec` suppressions). |
| `internal/buildinfo/` & `internal/productinfo/` | Build and product metadata. |
| `internal/lint/`, `internal/renamecheck/` | Repo-internal linters. |
| `internal/fiztools/` | Misc tooling support. |
| `internal/serverinstance/` | Long-running server lifecycle. |

## Provider interface pattern

Providers implement `internal/core.Provider` synchronously. Streaming is opt-in
through `internal/core.StreamingProvider`; the core loop detects it at runtime
with a type assertion. Adding a streaming-only provider does not require
changing the base `Provider` contract.

## Event surface

Fizeau owns public `ServiceEvent` construction, progress text, transcript
semantics, and session-log projections. Embedders should consume the public
event stream and projections rather than parsing harness-native streams or the
raw session-log JSONL ([ADR-008](adr/ADR-008/)).

The defined event types — `EventSessionStart`, `EventCompactionStart/End`,
`EventLLMRequest`, `EventLLMDelta`, `EventLLMResponse`, `EventToolCall`,
`EventSessionEnd` — flow through a single callback with monotonically
increasing sequence numbers.

## Decision record

The architectural decisions, with context, alternatives, consequences,
and risks, live in the [ADR index](adr/). Each ADR is dated and numbered;
superseded entries stay in the source tree but are hidden from the index.
