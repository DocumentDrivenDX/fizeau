# Provider Signal Matrix

Date: 2026-05-06

This note maps provider surfaces to the routing evidence Fizeau can actually
observe today. It separates:

- **Eligibility-critical signals**: hard gates that can disqualify a candidate
  or make it ineligible for automatic routing
- **Ranking-only signals**: evidence that improves ordering among already
  eligible candidates, but should not be treated as a hard rejection by itself

The matrix intentionally records missing coverage as a first-class result.
Unknown or absent evidence falls back to `FreshnessUnknown` unless a provider
package already caches a prior observation and can return `FreshnessStale`.

## Matrix

| Surface | Utilization evidence | Performance evidence | Context-length evidence | Cache evidence | Source endpoints | Freshness / fallback | Eligibility-critical | Ranking-only | Existing probe or capture |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| vLLM | `num_requests_running`, `num_requests_waiting` | Prometheus histograms/counters for TTFT, request latency, queue time, prefill time, inference time, output-token time | No dedicated provider probe in this step; context is not surfaced by the utilization probe | `vllm:kv_cache_usage_perc` or `vllm:gpu_cache_usage_perc`; OpenAI usage may also expose `prompt_tokens_details.cached_tokens` | `/metrics` on the server root, normalized from `/v1` | Fresh sample on success; stale cached sample on probe failure after one success; unknown on first failure | Context limits and exact pins are eligibility-critical when known; live queue/active pressure are not hard gates by themselves | Active/queued pressure, cache pressure, TTFT, latency histograms | `internal/provider/vllm/utilization_probe.go`, `internal/provider/vllm/utilization_probe_test.go`, `internal/provider/vllm/vllm_cassette_test.go` |
| Rapid-MLX | `num_running`, `num_waiting`, active request list | `ttft_s`, `tokens_per_second`, request phase, Metal memory | No dedicated context probe here | `cache_hit_type`, `cached_tokens`, `generated_tokens`, `cache.usage` / `cache_pressure`, Metal cache memory | `/v1/status`; `/v1/cache/stats` for vision-cache details | Unknown on successful samples; stale cached sample on failure after one success; unknown on first failure | Context limits are eligibility-critical when routed from catalog or discovery, not from the status probe | Queue depth, TTFT, tokens/s, cache hit type, memory pressure | `internal/provider/rapidmlx/utilization_probe.go`, `internal/provider/rapidmlx/utilization_probe_test.go` |
| oMLX | `total_requests`, `active_requests`, `waiting_requests` | `avg_prefill_tps`, `avg_generation_tps`, `uptime_seconds`, model load timing in the broader native API | `LookupModelLimits` reads `loaded_context_length` or `max_context_length` from `/api/v0/models/<id>` | `total_cached_tokens`, `cache_efficiency`, cached-token fields in response usage | `/api/status`; `/admin/api/stats` when admin auth is available; `/api/v0/models/<id>` for limits | Status probe returns unknown freshness on success; stale cached sample on failure after one success; unknown on first failure | Context length from model lookup is eligibility-critical for bounded routing | Queue depth, prefill/generation throughput, cached-token totals, memory pressure | `internal/provider/omlx/utilization_probe.go`, `internal/provider/omlx/utilization_probe_test.go`, `internal/provider/omlx/omlx_test.go`, `internal/provider/openai/discovery_integration_test.go` |
| llama-server | `requests_processing`, `requests_deferred`; slot processing state | Token/s derived from `/slots`; Prometheus metrics when available | No dedicated context probe here | `llamacpp:kv_cache_usage_ratio`; occupancy derived from slot count | `/metrics` on the server root when `--metrics` is enabled; `/slots` fallback | Fresh sample on success from either endpoint; stale cached sample on failure after one success; unknown on first failure | Context limits are eligibility-critical if the catalog or model metadata provides them; live queue signals are not hard gates | Queue depth, slot occupancy, cache ratio, token/s | `internal/provider/llamaserver/utilization_probe.go`, `internal/provider/llamaserver/utilization_probe_test.go`, `internal/provider/llamaserver/llamaserver_cassette_test.go` |
| LM Studio | No live utilization probe in the provider package; only model discovery and request success/failure evidence | TTFT and request timing are derived from request execution, not from a dedicated status endpoint here | `LookupModelLimits` reads `loaded_context_length` and falls back to `max_context_length` from `/api/v0/models/<id>` | OpenAI-compatible response usage may carry cached-token details when the backend returns them; there is no dedicated cache probe in this package | `/api/v0/models/<id>` for context; OpenAI-compatible `/v1/models` and `/v1/chat/completions` for discovery and requests | Context lookup is fresh when the HTTP call succeeds; otherwise the lookup returns zero values. No stale cache wrapper exists here today | Context length is eligibility-critical for exact pins and bounded routing | Model discovery order, request success, TTFT, any response-level cached-token telemetry | `internal/provider/lmstudio/lmstudio.go`, `internal/provider/lmstudio/lmstudio_test.go`, `internal/provider/openai/discovery_integration_test.go` |
| OpenRouter | No live utilization probe in the provider package | Gateway-reported usage cost is available through `usage.cost`; request timing is still derived from the request path | `LookupModelLimits` reads `context_length` and `top_provider.max_completion_tokens` from `/models` | Cache signals are pass-through from upstream usage objects: OpenAI-family cached tokens, Anthropic cache read/write fields, or `usage.cost` as a gateway-side billing signal | `/models` for limits; upstream chat endpoint through the OpenAI-compatible transport | Context lookup is fresh when the HTTP call succeeds; otherwise it returns zero values. No stale cache wrapper exists here today | Context length is eligibility-critical for bounded routing; cost can be a policy input but should not gate availability alone | Gateway-reported cost, upstream cached tokens, request timing | `internal/provider/openrouter/openrouter.go`, `internal/provider/openrouter/openrouter_test.go`, `internal/provider/openrouter/openrouter_cost_test.go` |
| Generic OpenAI-compatible providers | No live utilization probe in the provider package | Request latency and TTFT are derived from the execution path; response usage reports token counts | `openai.DiscoverModels` and model ranking are the discovery path; there is no dedicated context probe in this package | `prompt_tokens_details.cached_tokens` in usage when the upstream reports it | `/v1/models` and `/v1/chat/completions` on the configured OpenAI-compatible base URL | Discovery is fresh on success; otherwise model selection falls back to configured model or prior resolution. No stale utilization cache exists here today | Exact pins and discovered model availability are eligibility-critical; cached-token counts are not | Model discovery order, response usage, any upstream cached-token field, request timing | `internal/provider/openai/discovery.go`, `internal/provider/openai/discovery_test.go`, `internal/provider/openai/discovery_integration_test.go` |
| Ollama | No live utilization probe in the provider package | Request latency and TTFT are derived from the request path | No dedicated context probe in this package | No dedicated cache telemetry in the OpenAI-compatible surface; cache behavior is only inferable from repeated request timing or native Ollama APIs outside this package | OpenAI-compatible `/v1` surface | Unknown when the request succeeds but no utilization data exists; no stale cache wrapper exists today | Context length from model metadata would be eligibility-critical if added; today it is not probed here | Request timing, model residency inference, repeated-turn latency deltas | `internal/provider/ollama/ollama.go`, `internal/provider/ollama/ollama_test.go` |
| Lucebox | No live utilization probe in the provider package | Request latency and TTFT are derived from the request path | No dedicated context probe in this package | No dedicated cache telemetry in the OpenAI-compatible surface | OpenAI-compatible `/v1` surface | Unknown when the request succeeds but no utilization data exists | Context limits are eligibility-critical if sourced from catalog or discovery; the provider wrapper does not probe them today | Request timing and any upstream usage fields | `internal/provider/lucebox/lucebox.go` |
| Subprocess harnesses | No provider-level live utilization probe; routing evidence comes from CLI/TUI quota and usage capture | Native stream events, final event usage, and per-harness timing | Model discovery snapshots from `--help`, `--list-models`, or authenticated PTY discovery | `cache_read_tokens`, `cache_write_tokens`, `cache_tokens`, and provider-specific quota caches | CLI stdout JSON, PTY `/status` or `/usage`, stream JSON, model discovery commands | Fresh when captured during the current run; durable quota caches may return stale snapshots; missing capture should be treated as unknown | Exact model pins and quota exhaustion are eligibility-critical; missing live utilization is not by itself a hard rejection | Cached-token totals, reasoning-token totals, request timing, quota windows, model list order | `internal/harnesses/usage.go`, `internal/harnesses/codex/model_discovery.go`, `internal/harnesses/claude/model_discovery.go`, `internal/harnesses/gemini/model_discovery.go`, `internal/harnesses/pi/model_discovery.go`, `internal/harnesses/claude/quota_cache.go`, `internal/harnesses/codex/quota_cache.go`, `internal/harnesses/gemini/quota_cache.go` |

## Probe-to-Matrix Mapping

- `internal/provider/vllm/utilization_probe.go` maps to the vLLM row and
  proves fresh/stale/unknown behavior for metrics-based utilization capture.
- `internal/provider/rapidmlx/utilization_probe.go` maps to the Rapid-MLX row
  and proves status parsing, cache-pressure normalization, and stale fallback.
- `internal/provider/omlx/utilization_probe.go` maps to the oMLX row and
  proves status parsing, cache efficiency, and stale fallback.
- `internal/provider/llamaserver/utilization_probe.go` maps to the llama-server
  row and proves metrics-first, slots-fallback utilization capture.
- `internal/provider/lmstudio/lmstudio.go` maps to the LM Studio row for
  context-length discovery, but there is no live utilization probe today.
- `internal/provider/openrouter/openrouter.go` maps to the OpenRouter row for
  context-length discovery and gateway cost attribution, but there is no live
  utilization probe today.
- `internal/provider/openai/discovery.go` maps to the generic OpenAI-compatible
  row for discovery and model ranking, but not for utilization snapshots.
- `internal/harnesses/usage.go` and the harness discovery/PTY helpers map to
  the subprocess row for usage, quota, and model-discovery evidence.

## Follow-up Beads

The following missing probes should be tracked explicitly as later beads:

- `follow-up: add live utilization probe for LM Studio / OpenAI-compatible local servers`
- `follow-up: add live utilization probe for OpenRouter gateway surfaces`
- `follow-up: add live utilization probe for generic OpenAI-compatible providers`
- `follow-up: add context or status probe for Ollama if routing starts using live model limits`
- `follow-up: add harness-level utilization snapshot capture beyond usage/quota parsing`

## Freshness Rules

- A successful live probe produces `FreshnessFresh`.
- A probe that can reuse a prior observation after a failure should return
  `FreshnessStale`.
- A first-time failure or a surface with no implemented probe should return
  unknown evidence rather than pretending the signal exists.
- Context-length lookups without a stale cache wrapper are fresh-only; they
  return zero values on failure and should be treated as unknown by callers.

