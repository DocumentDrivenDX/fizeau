# SD-013: Provider Signal Matrix for OpenRouter, Ollama, and OpenAI-Compatible Surfaces

This note inventories the routing-evidence surfaces that FEAT-004 and SD-005
already imply, with emphasis on OpenRouter and generic OpenAI-compatible
providers. The point is to separate:

- model inventory and context-limit discovery
- quota and cost evidence
- live utilization evidence

OpenRouter and the generic `openai` wrapper are intentionally not treated as
provider-owned live utilization sources in this step.

## Matrix

| Provider class | Context-limit discovery | Quota headers | Cost attribution | Utilization status |
|----------------|-------------------------|---------------|------------------|--------------------|
| `ollama` | `GET /api/ps` returns loaded-model inventory with per-model `context_length`; `POST /api/show` exposes model metadata including `model_info.<family>.context_length` and serialized `parameters` such as `num_ctx` for the selected model. | None documented here. Ollama does not currently publish a provider-owned quota-header surface in this layer. | Local/free inference; no provider billing surface is expected in the router. | `documentation-only`: `POST /api/chat` and `POST /api/generate` return native timing fields (`total_duration`, `load_duration`, `prompt_eval_count`, `prompt_eval_duration`, `eval_count`, `eval_duration`) in nanoseconds, but the router still lacks a live Ollama utilization probe. Cache pressure remains unknown because no verified native cache counter is exposed. Marked unavailable for live utilization until a follow-up probe bead lands. |
| `openrouter` | `internal/provider/openrouter.LookupModelLimits()` reads `GET /models` and maps `context_length` plus `top_provider.max_completion_tokens` into `limits.ModelLimits`. | `internal/provider/quotaheaders.ParseOpenRouter()` parses `x-ratelimit-limit`, `x-ratelimit-remaining`, `x-ratelimit-reset`, and `retry-after`. | `internal/provider/openrouter.UsageCostAttribution()` preserves gateway-reported `usage.cost` as billed USD cost. | `ranking-only`: model inventory, quota, and cost are available, but no provider-owned live utilization probe exists here. Marked unavailable for live utilization; tracked by bead `fizeau-4d01efdc`. |
| `openai` / generic OpenAI-compatible | No provider-owned limit probe in this layer. The shared OpenAI-compatible `/v1/models` discovery path is for model inventory and ranking, not live utilization. | `internal/provider/quotaheaders.ParseOpenAI()` is the default parser installed by `internal/provider/openai.New()` when a quota observer is wired. | Cost attribution is optional and provider-specific; the generic OpenAI-compatible wrapper defaults to `CostSourceUnknown` unless a concrete provider supplies a hook. | `documentation-only`: the shared protocol shape is transport plumbing, not a utilization surface. Marked unavailable for live utilization; tracked by bead `fizeau-4d01efdc`. |
| `vllm` | `LookupModelLimits()` is via the shared OpenAI-compat discovery path; context surfaces from configured catalog or model metadata, not the utilization probe. | Per-server quota headers are not standardised; depend on the deployment's auth gateway. | No native cost surface — local inference. | `live`: `internal/provider/vllm/utilization_probe.go` reads Prometheus `/metrics` (normalised from `/v1`) for `num_requests_running`, `num_requests_waiting`, TTFT/latency/queue/prefill/inference histograms, and `vllm:kv_cache_usage_perc` (or legacy `vllm:gpu_cache_usage_perc`). Fresh on success; stale cached after one prior success; unknown on first failure. |
| `rapid-mlx` | Context discovery from catalog or shared OpenAI-compat path; no dedicated probe field. | None native. | None native (local). | `live`: `internal/provider/rapidmlx/utilization_probe.go` reads `/v1/status` for `num_running`, `num_waiting`, active request list, `ttft_s`, `tokens_per_second`, request phase, Metal memory; `/v1/cache/stats` adds vision-cache `cache_hit_type`, `cached_tokens`, `generated_tokens`, `cache.usage` / `cache_pressure`. Unknown freshness on success; stale cached after one prior success; unknown on first failure. |
| `omlx` | `LookupModelLimits` reads `loaded_context_length` or `max_context_length` from `/api/v0/models/<id>`; eligibility-critical for bounded routing. | None native. | None native (local). | `live`: `internal/provider/omlx/utilization_probe.go` reads `/api/status` for `total_requests`, `active_requests`, `waiting_requests`, `avg_prefill_tps`, `avg_generation_tps`, `uptime_seconds`, `total_cached_tokens`, `cache_efficiency`; `/admin/api/stats` when admin auth is available. Unknown freshness on success; stale cached after one prior success; unknown on first failure. |
| `llama-server` | Context limits sourced from catalog or model metadata; no dedicated probe field. | None native. | None native (local). | `live`: `internal/provider/llamaserver/utilization_probe.go` reads `/metrics` (when `--metrics` is enabled) for `requests_processing`, `requests_deferred`, `llamacpp:kv_cache_usage_ratio`; falls back to `/slots` for slot processing state and token/s derivation. Fresh on either-endpoint success; stale cached after one prior success; unknown on first failure. |
| `lmstudio` | `LookupModelLimits` reads `loaded_context_length` then `max_context_length` from `/api/v0/models/<id>`. Native v1 model management uses `GET /api/v1/models` plus `POST /api/v1/models/load` and the documented unload/download variants. | None native. | None native (local). | `documentation-only`: native `POST /api/v1/chat` returns a `stats` object with `input_tokens`, `total_output_tokens`, `reasoning_output_tokens`, `tokens_per_second`, `time_to_first_token_seconds`, optional `model_load_time_seconds` — derived per-response, not from a probe. No verified native cache counter on the documented surfaces; OpenAI-compat response usage may carry cached-token details when the backend reports them. `lms server status --json` is a CLI status surface, not a provider utilization signal. |
| `lucebox` | Context limits sourced from catalog or shared OpenAI-compat discovery; not probed by the provider package. | None native. | None native. | `documentation-only`: OpenAI-compatible `/v1` surface only; no live utilization probe today. Request latency and TTFT derived from the request path. |
| Subprocess harnesses (`codex`, `claude`, `gemini`, `pi`) | Model discovery snapshots from `--help`, `--list-models`, or authenticated PTY discovery. | Provider-specific durable quota caches at `internal/harnesses/<name>/quota_cache.go` may return stale snapshots; eligibility-critical when quota is exhausted. | Captured per harness via `internal/harnesses/usage.go` from CLI stdout JSON / PTY `/status` / `/usage` / stream JSON; carries `cache_read_tokens`, `cache_write_tokens`, `cache_tokens`, reasoning-token totals, request timing. | `documentation-only` at the provider seam: no provider-level live probe. Routing evidence comes from per-run capture during execution; missing capture is treated as unknown rather than reused. |

**Source provenance for vLLM, Rapid-MLX, oMLX, llama-server, LM Studio,
Lucebox, and Subprocess rows**: extracted from
`docs/research/provider-signal-matrix-2026-05-06.md` (Matrix +
Probe-to-Matrix Mapping + Freshness Rules sections). The original three
rows (Ollama, OpenRouter, generic OpenAI-compatible) are unchanged from
this SD's first cut; the extended rows align with the same column
semantics.

### Freshness rules (lifted from research)

A successful live probe produces `FreshnessFresh`. A probe that can reuse a
prior observation after a failure should return `FreshnessStale`. A
first-time failure or a surface with no implemented probe returns unknown
evidence rather than pretending the signal exists. Ollama request metrics
are fresh per completed response and `/api/ps` inventory is fresh per
successful poll; neither surface has a stale-cache wrapper today, so
callers should treat failures as unknown rather than reusing older values.

## Existing Probe Map

The matrix above relies on existing code paths rather than new probes:

- `internal/provider/openrouter/openrouter.go`
  - model limit discovery: `LookupModelLimits()`
  - quota parsing: `QuotaHeaderParser: quotaheaders.ParseOpenRouter`
  - cost attribution: `UsageCostAttribution()`
- `internal/provider/openai/openai.go`
  - generic quota parsing: default `quotaheaders.ParseOpenAI`
  - generic cost handling: optional `UsageCostAttribution` hook, otherwise unknown
- `internal/provider/openai/discovery.go`
  - shared OpenAI-compatible model discovery and ranking helpers
- `internal/provider/quotaheaders/quotaheaders.go`
  - OpenAI and OpenRouter rate-limit header parsers

For contrast, the local provider families documented in FEAT-004/SD-005 already
have live utilization probes where applicable, or else documented native
metrics surfaces that a future probe can consume:

- `ollama` -> documented native chat/generate timing fields plus loaded-model
  inventory in `/api/ps`; live utilization probe still missing
- `vllm` -> `/metrics`
- `llama-server` -> `/metrics`, then `/slots`
- `omlx` -> `/api/status`
- `rapid-mlx` -> documented status surface

OpenRouter and generic OpenAI-compatible surfaces do not join that group in the
current design. Ollama's native metrics are documented above, but the live
utilization probe itself remains a follow-up item rather than an implemented
routing signal.

## Follow-Up Status

- OpenRouter live utilization probe: unavailable in this step; tracked by bead `fizeau-4d01efdc`.
- Generic OpenAI-compatible live utilization probe: unavailable in this step; tracked by bead `fizeau-4d01efdc`.
- Ollama live utilization probe: unavailable in this step; follow-up bead needed to consume `/api/chat` or `/api/generate` timings together with `/api/ps` inventory and any future cache counters.

If a later bead wants live utilization for either class, it needs a provider-
owned probe design rather than reuse of the shared OpenAI-compatible transport
layer.

## Per-provider cache wire format and telemetry

The matrix above lists *whether* each surface exposes cache evidence. This
section documents *what fizeau must send on the wire* to opt into caching
for each provider class, and what response field confirms a hit. Every cache
below keys on a byte-stable prefix; opt-in mechanics and telemetry surface
differ enough that conflating them costs hit-rate. Local servers expose
almost no standardised telemetry; for those, hit-rate is a TTFT inference,
not a returned counter.

### Anthropic Messages API

- **Opt-in**: explicit per-block `cache_control: {"type": "ephemeral"}` on
  the *last* block of any cacheable region. Allowed regions: `tools[i]`,
  `system[i]`, `messages[i].content[j]`. TTL options: default `"5m"`, or
  `"1h"` (`"ttl": "1h"`; gated by `extra-cache-ttl-2025-04-11` beta header).
- **Max breakpoints**: 4 explicit per request; top-level `cache_control` for
  automatic caching consumes a slot.
- **Telemetry**: `usage.cache_creation_input_tokens`,
  `usage.cache_read_input_tokens`, and (with mixed TTLs)
  `usage.cache_creation.ephemeral_5m_input_tokens` /
  `ephemeral_1h_input_tokens`. Total input = read + creation + input.
- **Min prompt length** (cumulative, at or after the breakpoint):
  Opus 4.5/4.6/4.7 and Haiku 4.5 = 4096 tokens; Sonnet 4.6 = 2048; older
  Sonnet/Opus/Haiku 3.5 = 1024. Below threshold caches silently no-op.
- **Lookback**: server walks back up to 20 blocks per breakpoint.
- **Workspace isolation (Feb 5 2026)**: caches are workspace-scoped, not
  org-scoped, on Claude API and Azure AI Foundry. Bedrock and Vertex remain
  org-scoped.
- **Trap**: thinking blocks cannot carry `cache_control` directly; they
  piggyback on the surrounding assistant turn.

### OpenAI Chat Completions

- **Opt-in**: none — server-side automatic prefix cache. Optional
  `prompt_cache_key` (string) acts as a routing shard hint that pins a
  session's hot prefixes to one backend (~15 RPM/prefix per machine before
  overflow). Recommended for any concurrent harness; single-user CLI can
  omit.
- **Telemetry**: `usage.prompt_tokens_details.cached_tokens` (int). Always
  present, zero on miss or sub-1024-token prompts.
- **Constraints**: minimum 1024 prompt tokens; cache hits then advance in
  128-token increments. Eligible on gpt-4o and newer.
- **Discount**: ~50% on cached input tokens.

### OpenRouter (gateway pass-through)

- **Anthropic upstreams**: send `cache_control` exactly as for Anthropic
  direct; OpenRouter forwards. 1h TTL supported.
- **OpenAI / Grok / Moonshot / Groq / DeepSeek upstreams**: automatic;
  nothing to send.
- **Gemini 2.5 upstreams**: implicit caching is on by default. Optional
  explicit `cache_control` breakpoints in content blocks; OpenRouter only
  forwards the **last** breakpoint.
- **Telemetry (normalised across upstreams)**:
  - `usage.cache_discount` (float, USD; sign-aware: negative on writes,
    positive on reads).
  - `usage.prompt_tokens_details.cached_tokens` (int) for OpenAI-family
    upstreams.
  - Pass-through `cache_creation_input_tokens` / `cache_read_input_tokens`
    for Anthropic upstreams.
- **Provider sticky routing**: OpenRouter pins subsequent requests in a
  session to the same upstream endpoint when caching is detected.
- **Trap**: top-level (automatic) `cache_control` only works when the
  request is routed to Anthropic-direct, not Bedrock/Vertex variants. Use
  per-block breakpoints to stay portable. Routing changes (provider
  preference, fallback) silently kill the cache.

### Google Gemini (native API)

- **Implicit cache**: automatic on Gemini 2.5+; min 2048 input tokens (2.5
  Flash, 2.5 Pro). TTL is short (~3-5 min) and not configurable.
- **Explicit cache**: pre-create a `cachedContents` resource via
  `POST /v1beta/cachedContents` (`model`, `contents`, optional `ttl`), then
  reference its `name` in the `cachedContent` field of `generateContent`.
- **Telemetry**: `usageMetadata.cachedContentTokenCount` (also
  `cached_content_token_count` in some SDKs).
- **Discount**: 90% on 2.5+ models, 75% on 2.0.
- **Trap**: explicit cache has minimum sizes per model (commonly 4096+
  tokens) and incurs storage cost.

### oMLX

- **Opt-in**: automatic. Two-tier KV cache (RAM hot + SSD cold), paged
  prefix sharing inspired by vLLM, persisted to disk in safetensors. No
  request fields, no headers.
- **Telemetry**: not standardised. Whether
  `prompt_tokens_details.cached_tokens` is populated on the OpenAI-compat
  surface is **unverified** — empirical probe required (send same prompt
  twice; diff usage block + wall-clock TTFT). Server logs / menubar admin
  dashboard are out-of-band.
- **Constraints**: prefix must match byte-for-byte; model must remain
  loaded; page-level LRU eviction. SSD persistence means cache *can*
  survive model unload/reload.
- **Trap**: mlx-lm prefix-cache silently recomputes for SWA / Mamba /
  hybrid-attention models (issue ml-explore/mlx-lm#980). Qwen3 dense and
  Llama-class are fine; Gemma3 / Qwen3-Next / similar hybrids need
  per-model verification.

### LM Studio

- **Opt-in**: automatic; KV cache reuse via the underlying llama.cpp / MLX
  backend. Unified KV cache (since 0.4.0) on by default. Continuous
  batching on by default.
- **Telemetry**: none on the OpenAI-compat response. TTFT is the only
  signal.
- **Trap**: the Anthropic-compat endpoint (`/v1/messages`) has been
  observed to fully reprocess prompts where `/v1/chat/completions` reuses
  correctly — prefer the OpenAI-compat surface. Some MoE models
  (Qwen3.5-A3B family) silently disable cache reuse on the llama.cpp
  backend.

### vLLM

- **Opt-in**: server-side `--enable-prefix-caching` flag (or
  `enable_prefix_caching=True`). No request field.
- **Telemetry**: vLLM's OpenAI-compat layer populates
  `usage.prompt_tokens_details.cached_tokens` when APC fires.
- **Constraints**: only meaningful during prefill; no decode-time effect.
  Default hash is `builtin`; switch to `sha256` for collision resistance.
- **Trap**: APC silently does nothing if not enabled at server start.

### llama.cpp / llama-server

- **Opt-in**: per-request `cache_prompt: true` (default true). Server flag
  `--cache-reuse N` sets the minimum chunk size for KV-shift reuse (0
  disables). Recent v1.70+ adds a host-memory disk-backed prompt cache.
- **Telemetry**: none on the OpenAI-compat response. Server logs only.
- **Trap**: known regressions where `--cache-reuse` stops firing for
  specific models (Qwen3-Next, Gemma 4 unless `-fa` + `--swa-full`). The
  `--prompt-cache` flag is CLI-only and is **not** exposed via
  `llama-server`.

### Ollama

- **Opt-in**: automatic. `keep_alive` (request field, e.g. `"60m"` or
  `-1`) keeps the model resident so the cache survives. `num_keep` pins a
  prefix during context shift.
- **Telemetry**: none on the response. `prompt_eval_count` vs
  `prompt_eval_duration` delta is the only proxy.
- **Trap**: any change to `num_ctx`, quantisation, or model parameters
  triggers reload and cache wipe. Default 5-min `keep_alive` discards
  cache between idle gaps. Some Gemma-3-class models bypass cache.

### Cross-cutting rules

1. Every cache above keys on a **byte-stable prefix**. Anywhere fizeau
   injects a timestamp, run-id, UUID, or non-deterministic tool ordering
   into the system prompt or tool list, full prefill is paid every turn.
2. Anthropic and Gemini-explicit are the only providers that meaningfully
   tax the client (place breakpoints; create `cachedContents`). Everything
   else is automatic or a server-side flag.
3. Local servers (oMLX, LM Studio, llama-server, Ollama, vLLM) do prefix
   reuse server-side once configured but expose **zero standardised
   telemetry** in their OpenAI-compat responses. Hit rate must be measured
   via TTFT or server logs, not response usage.

### Source provenance

Per-provider opt-in fields, telemetry surface, min-prompt thresholds,
discount tiers, and traps extracted from
`docs/research/provider-caching-survey-2026-04-27.md` (sections 1–9 plus
the Cross-cutting notes). Folded here rather than SD-005 because SD-013's
matrix already carries the per-provider Cache evidence column, and the
wire-format addendum is the natural complement.
