# Local Server Platform Metrics Capture

Date: 2026-05-06

This note summarizes the API surfaces Fizeau can use to capture request usage,
utilization, and performance metrics from local server platforms. It includes
the current implementation targets, vLLM and llama-server, plus oMLX,
Rapid-MLX, LM Studio, and Ollama.

## Summary

| Platform | Best usage source | Best utilization source | Performance fields | Capture confidence |
| --- | --- | --- | --- | --- |
| vLLM | OpenAI response usage; Prometheus token counters | Root `/metrics` on the OpenAI-compatible server | running/waiting counts, KV cache pressure, TTFT and request latency histograms, token counters | High |
| llama-server | OpenAI response usage; non-OAI completion timings when used | Root `/metrics` with `--metrics`; `/slots` fallback | processing/deferred request counts, cache ratio, per-slot processing state and token/s | High when metrics/slots are enabled |
| oMLX | OpenAI/Anthropic response usage; streaming usage chunks | `/api/status`; `/admin/api/stats` if admin auth is available | TTFT, total time, prefill/generation duration, cached tokens, tokens/s, model-load duration | High |
| Rapid-MLX | OpenAI/Anthropic response usage; streaming usage chunks | `/v1/status`; `/v1/cache/stats` for vision cache | token totals, active request `ttft_s`, tokens/s, Metal memory, cache hit details | High |
| LM Studio | Native `/api/v1/chat` response `stats`; OpenAI usage fields | Native model-management endpoints; stream events for in-flight progress | tokens/s, TTFT, model-load time, prompt-processing events | Medium-high |
| Ollama | Native `/api/generate` and `/api/chat` final response metrics | `/api/ps` for loaded models and VRAM footprint | total/load/prompt/generation durations, input/output tokens | High for request metrics, medium for live utilization |

## Provider Signal Matrix

Routing uses two different classes of evidence:

- **Eligibility-critical** signals can make an endpoint preferred or
  deprioritized for the current request because they reflect live load,
  freshness, or a hard capability boundary.
- **Ranking-only** signals improve ordering, telemetry, or display but do not
  by themselves make a healthy endpoint ineligible.

The matrix below maps the current provider surfaces to those classes and shows
where the existing Go probes already live. Missing live probes are listed as
tracked follow-up beads so the gap stays explicit.

| Provider | Utilization signals | Performance signals | Context length / max tokens | Cache indicators | Source endpoint(s) | Freshness / unknown fallback | Signal class | Existing probe(s) | Follow-up bead |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| vLLM | `num_requests_running`, `num_requests_waiting`, KV cache pressure | Prometheus latency histograms, token counters, request-queue timing | Model inventory from `/v1/models`; otherwise catalog/context metadata | `vllm:kv_cache_usage_perc` or `vllm:gpu_cache_usage_perc` | Server-root `/metrics` after stripping `/v1` | Fresh on a successful metrics fetch; stale cache or `unknown` on failure | Eligibility-critical: active/queued/cache pressure. Ranking-only: latency histograms, counters, context inventory | `internal/provider/vllm/utilization_probe.go`, `internal/provider/vllm/utilization_probe_test.go`, cassette tests | None |
| Rapid-MLX | `num_running`, `num_waiting`, active-request snapshots | `ttft_s`, `tokens_per_second`, total prompt/completion counters | Model inventory / catalog metadata; no dedicated limit probe in this step | `cache_hit_type`, `cached_tokens`, `generated_tokens`, Metal active/peak/cache memory | `/v1/status` with root fallback when the base URL ends in `/v1` | Fresh on a successful status fetch; stale cache or `unknown` on failure | Eligibility-critical: active/waiting/cache pressure. Ranking-only: per-request timings, totals, memory | `internal/provider/rapidmlx/utilization_probe.go`, `internal/provider/rapidmlx/utilization_probe_test.go`, cassette tests | None |
| oMLX | `active_requests`, `waiting_requests`, `cache_efficiency`, model memory used/max | `avg_prefill_tps`, `avg_generation_tps`, total request/token counters, uptime, loaded models | `LookupModelLimits` from `/v1/models/status` for `max_context_window` and `max_tokens` | `total_cached_tokens`, `cache_efficiency`, cache-state fields | `/api/status`; optional `/admin/api/stats`; `/v1/models/status` for limits | Fresh on a successful status fetch; stale cache or `unknown` on failure | Eligibility-critical: active/waiting/cache efficiency and memory pressure. Ranking-only: totals, TPS, loaded-model inventory, limit discovery | `internal/provider/omlx/utilization_probe.go`, `internal/provider/omlx/utilization_probe_test.go`, `internal/provider/omlx/omlx.go`, `internal/provider/omlx/omlx_test.go` | None |
| llama-server | `requests_processing`, `requests_deferred`, slot occupancy | Per-slot `is_processing`, slot count, cache ratio | Slot metadata and `/v1/models` inventory; no dedicated context probe in this step | `llamacpp:kv_cache_usage_ratio`, `/slots` occupancy ratio | `/metrics` first, then `/slots` fallback on the server root | Fresh on a successful metrics or slots fetch; stale cache or `unknown` on failure | Eligibility-critical: processing/deferred requests and slot occupancy. Ranking-only: slot count / derived occupancy | `internal/provider/llamaserver/utilization_probe.go`, `internal/provider/llamaserver/utilization_probe_test.go`, cassette tests | None |
| LM Studio | No provider-owned live utilization probe yet; use per-request stats and model-state evidence | Native `/api/v1/chat` stats: `time_to_first_token_seconds`, `tokens_per_second`, `model_load_time_seconds` | `LookupModelLimits` from `/api/v0/models/{model}` using `loaded_context_length` or `max_context_length` | No dedicated cache indicator in the current surface | Native `/api/v1/chat`; `/api/v0/models/{model}`; OpenAI-compatible `/v1/chat/completions` for compatibility | Per-request stats are fresh; limit discovery is stale/unknown when the model endpoint is unreachable | Ranking-only for this step: context limits and per-request timing. Eligibility-critical utilization is not yet probed | `internal/provider/lmstudio/lmstudio.go`, `internal/provider/lmstudio/lmstudio_test.go`, `internal/provider/openai/discovery.go` | `fizeau-be3cb865` |
| OpenRouter / generic OpenAI-compatible providers | No provider-owned live utilization probe yet; quota headers and response usage are the available routing evidence | Response usage, OpenRouter gateway cost, rate-limit/quota headers | `LookupModelLimits` from `/models` for `context_length` and `top_provider.max_completion_tokens` | OpenRouter gateway cost attribution; no cache-usage surface in the generic OpenAI-compatible layer | `/v1/models`; OpenRouter response headers; OpenAI-compatible chat completions | Per-response and per-model-list data are fresh; utilization is `unknown` when no gateway probe exists | Ranking-only for this step: context limits, cost attribution, and quota evidence | `internal/provider/openrouter/openrouter.go`, `internal/provider/openrouter/openrouter_test.go`, `internal/provider/openai/discovery.go`, `internal/provider/quotaheaders/quotaheaders.go` | `fizeau-4d01efdc` |
| Ollama | No provider-owned live utilization probe yet; request metrics and loaded-model inventory are the observable surfaces | `total_duration`, `load_duration`, `prompt_eval_duration`, `eval_duration`, input/output token counts | `/api/ps` model inventory exposes `context_length`; model docs expose additional detail | No documented cache-hit signal in the current provider wrapper | Native `/api/chat`, `/api/generate`, `/api/ps` | Request metrics are fresh; live utilization is `unknown` without a dedicated probe | Ranking-only for this step: request timing, tokens, and model inventory. Eligibility-critical utilization is not yet probed | `internal/provider/ollama/ollama.go`, `internal/provider/ollama/ollama_test.go` | `fizeau-33ed2078` |
| Subprocess harnesses | No provider-owned endpoint utilization; use harness quota/account/model-evidence freshness instead | Request runtime, final text latency, progress events, cancellation timing | Harness-specific model discovery / quota cache / config metadata, not a live server probe | No cache indicator on the provider surface; route decisions rely on durable quota caches instead | PTY/subprocess execution, harness cassettes, quota/account discovery, session logs | Freshness comes from durable cache state; missing or stale evidence should fall back to service-owned lease counts or mark the harness secondary | Eligibility-critical: quota/account freshness and hard pins. Ranking-only: progress and latency evidence | `internal/harnesses/*`, `internal/serviceimpl/execute_subprocess.go`, `internal/provider/conformance/run.go`, harness-specific quota/model discovery tests | `fizeau-89312cef` |

The current implementation therefore splits cleanly into two groups:

1. Providers with concrete live utilization probes today: vLLM, Rapid-MLX,
   oMLX, and llama-server.
2. Providers with useful routing evidence but no live utilization probe in this
   bead: LM Studio, OpenRouter/generic OpenAI-compatible providers, Ollama, and
   subprocess harnesses.

## Cross-Platform Capture Model

Fizeau should treat local server metric capture as three related but separate
channels:

1. **Per-request accounting** from the response payload or final stream chunk.
   This is the most portable source for prompt tokens, completion tokens,
   total tokens, request latency, prompt-eval duration, generation duration,
   and model-load duration.
2. **Live utilization snapshots** from status/admin/model endpoints. These are
   platform-specific and should be polled opportunistically, not required for a
   request to be valid.
3. **Derived metrics** computed in Fizeau when upstream gives primitives:
   generation tokens/s, prompt tokens/s, queue time, wall time, TTFT, cache hit
   ratio, and loaded-model memory pressure.

Normalize all durations internally to seconds. Ollama reports nanoseconds;
oMLX and LM Studio report seconds for their extended fields; Rapid-MLX exposes
seconds for status details and OpenAI-style usage counts in responses.

## vLLM

Sources:

- Production metrics docs: <https://docs.vllm.ai/en/v0.15.0/usage/metrics/>
- Fizeau governing ACs:
  `docs/helix/01-frame/features/FEAT-003-providers.md` AC-FEAT-003-13 and
  `docs/helix/01-frame/features/FEAT-004-model-routing.md` AC-FEAT-004-18

Relevant API support:

- vLLM's OpenAI-compatible API server exposes Prometheus metrics at the server
  root `/metrics`, not under `/v1`.
- A configured OpenAI-compatible base URL such as `http://host:8000/v1` should
  be normalized to root `http://host:8000` before probing metrics.
- Useful utilization gauges:
  `vllm:num_requests_running`, `vllm:num_requests_waiting`,
  `vllm:kv_cache_usage_perc`.
- Older deployments may expose `vllm:gpu_cache_usage_perc` instead of
  `vllm:kv_cache_usage_perc`; Fizeau should support both and prefer the newer
  name when both exist.
- Useful counters and histograms:
  `vllm:prompt_tokens`, `vllm:generation_tokens`,
  `vllm:prefix_cache_queries`, `vllm:prefix_cache_hits`,
  `vllm:time_to_first_token_seconds`,
  `vllm:e2e_request_latency_seconds`,
  `vllm:request_queue_time_seconds`,
  `vllm:request_prefill_time_seconds`,
  `vllm:request_inference_time_seconds`,
  `vllm:request_time_per_output_token_seconds`.

Per-request fields:

- OpenAI-compatible chat/completions responses provide normal `usage` fields
  when vLLM can compute them.
- Streaming usage should be requested with `stream_options.include_usage` when
  the client path supports it.
- Rich timing is best captured from Prometheus histograms or derived locally
  by Fizeau rather than assuming every OpenAI response includes timing fields.

Capture implications:

- vLLM meets the utilization-tracking requirement.
- Poll `/metrics` for active/queued requests and KV cache pressure.
- Use Prometheus counters/histograms for server-level performance trend
  capture, but keep per-request accounting anchored to OpenAI response usage
  and Fizeau wall-clock measurements.
- Record/replay tests should use real vLLM CPU server cassettes and parse
  running, waiting, and cache-pressure metrics from committed fixtures.

## llama-server

Sources:

- llama.cpp repository: <https://github.com/ggml-org/llama.cpp>
- Server README: <https://github.com/ggml-org/llama.cpp/blob/master/tools/server/README.md>
- Fizeau governing ACs:
  `docs/helix/01-frame/features/FEAT-003-providers.md` AC-FEAT-003-14 and
  `docs/helix/01-frame/features/FEAT-004-model-routing.md` AC-FEAT-004-18

Relevant API support:

- `llama-server` is an OpenAI-compatible HTTP server; default port is 8080 and
  the OpenAI chat endpoint is `/v1/chat/completions`.
- The server supports multiple concurrent requests with `--parallel` /
  `-np`, and continuous batching is enabled by default.
- Metrics are enabled with `--metrics` or `LLAMA_ARG_ENDPOINT_METRICS=1`.
- `/slots` is enabled by default and can be disabled with `--no-slots`.
  It returns per-slot state and can be called with `?fail_on_no_slot=1` to get
  HTTP 503 when no slot is available.
- `/slots` exposes per-slot fields such as `id`, `id_task`, `n_ctx`,
  `is_processing`, sampling parameters, and token/progress details. This is
  the best fallback when metrics are not available.

Fizeau metric targets:

- From `/metrics`, parse request pressure and cache pressure where available:
  `llamacpp:requests_processing`, `llamacpp:requests_deferred`, and
  `llamacpp:kv_cache_usage_ratio`.
- From `/slots`, derive:
  active slot count from `is_processing`, max concurrency from slot count,
  and fallback occupancy ratio.

Per-request fields:

- OpenAI-compatible responses provide standard usage when llama-server emits
  OpenAI usage fields for the requested endpoint/model path.
- The non-OAI `/completion` endpoint has richer native options and can expose
  prompt-cache behavior, but Fizeau's provider path should stay on
  `/v1/chat/completions` for normal provider compatibility.

Capture implications:

- llama-server meets utilization tracking when started with `--metrics`; it
  remains partially trackable through `/slots` when metrics are unavailable.
- A configured OpenAI-compatible base URL ending in `/v1` should be normalized
  to the server root before probing `/metrics` and `/slots`.
- Tests should cover both `/metrics` and `/slots` fallback parsing because
  operators can disable slots and may forget `--metrics`.
- Record/replay tests should start a real CPU llama-server with `--metrics`,
  `/slots`, and constrained parallelism, then record `/v1/models`, `/metrics`,
  `/slots`, chat, and busy-slot evidence.

## oMLX

Sources:

- Repository: <https://github.com/jundot/omlx>
- README API compatibility and admin dashboard:
  <https://github.com/jundot/omlx/tree/7269c757fb9d703dc775c4d43cdeec68bb3d42e8>
- Code inspected at commit `7269c757fb9d703dc775c4d43cdeec68bb3d42e8`:
  `omlx/server.py`, `omlx/server_metrics.py`, `omlx/admin/routes.py`,
  `tests/test_stream_usage.py`, `tests/test_status_endpoint.py`

Relevant API support:

- OpenAI-compatible endpoints: `/v1/chat/completions`, `/v1/completions`,
  `/v1/embeddings`, `/v1/rerank`, `/v1/models`.
- Anthropic-compatible endpoint: `/v1/messages`.
- Streaming usage is explicitly supported through
  `stream_options.include_usage`.
- `/api/status` is a lightweight status endpoint protected by the same API key
  policy as serving endpoints. It includes:
  `total_requests`, `active_requests`, `waiting_requests`,
  `total_prompt_tokens`, `total_completion_tokens`, `total_cached_tokens`,
  `cache_efficiency`, `avg_prefill_tps`, `avg_generation_tps`,
  loaded model IDs, and model memory used/max.
- `/admin/api/stats` exposes a richer dashboard snapshot when admin auth is
  available. It adds active model details, per-model/session/all-time scope,
  runtime cache observability, host/port, and engine information.

Per-request fields:

- Standard usage: `prompt_tokens`, `completion_tokens`, `total_tokens`.
- Extended OpenAI-style usage fields: `prompt_tokens_details.cached_tokens`,
  `model_load_duration`, `time_to_first_token`, `total_time`,
  `prompt_eval_duration`, `generation_duration`,
  `prompt_tokens_per_second`, `generation_tokens_per_second`.
- Anthropic usage maps to input/output token counts and cache read/create
  style fields where applicable.

Capture implications:

- Prefer response usage for per-request accounting.
- If streaming, request `stream_options.include_usage: true` and parse the
  terminal usage chunk with `choices: []`.
- Poll `/api/status` before/after request batches for server-wide counters and
  active/waiting request pressure.
- Treat `/admin/api/stats` as optional because it is admin-panel scoped and may
  require cookie or admin authentication beyond normal serving API keys.
- oMLX is one of the strongest targets for local utilization capture because it
  tracks cached tokens, queue/request counts, model memory, session/all-time
  totals, and persisted serving stats.

## Rapid-MLX

Sources:

- Repository: <https://github.com/raullenchai/Rapid-MLX>
- README and server guide:
  <https://github.com/raullenchai/Rapid-MLX/tree/cbb0bab0f1bb35fc8d240afad3c267e10ada1d35>
- Code inspected at commit `cbb0bab0f1bb35fc8d240afad3c267e10ada1d35`:
  `vllm_mlx/routes/chat.py`, `vllm_mlx/routes/completions.py`,
  `vllm_mlx/routes/health.py`, `docs/guides/server.md`

Relevant API support:

- OpenAI-compatible server at `http://localhost:8000/v1`.
- Endpoints include `/v1/chat/completions`, `/v1/completions`,
  `/v1/messages`, `/v1/embeddings`, plus model and health/status endpoints.
- Streaming usage is supported through `stream_options.include_usage`.
- `/v1/status` returns server-wide and active-request details:
  `status`, `model`, `uptime_s`, `steps_executed`, `num_running`,
  `num_waiting`, `total_requests_processed`, `total_prompt_tokens`,
  `total_completion_tokens`, `metal.active_memory_gb`,
  `metal.peak_memory_gb`, `metal.cache_memory_gb`, cache statistics, and
  active `requests`.
- Active request entries can include `request_id`, `phase`,
  `tokens_per_second`, `ttft_s`, `progress`, `cache_hit_type`,
  `cached_tokens`, `generated_tokens`, and `max_tokens`.
- `/v1/cache/stats` exists for vision cache details; text prompt cache details
  are mostly internal unless surfaced through `/v1/status`.

Per-request fields:

- Non-streaming chat/completions include standard OpenAI usage fields.
- Streaming chat emits normal content chunks and, when requested, a terminal
  usage chunk with `prompt_tokens`, `completion_tokens`, and `total_tokens`.
- Rapid-MLX logs per-request tokens/s, but the API-level performance surface is
  stronger through `/v1/status` than through the OpenAI response itself.

Capture implications:

- Prefer OpenAI response usage for token accounting.
- Poll `/v1/status` at request start/end or periodically during long requests
  to collect TTFT, active queue pressure, cache hit type, and Metal memory.
- For single-request streaming, Fizeau can derive wall-clock TTFT locally while
  using `/v1/status` as a corroborating platform metric.
- Rapid-MLX is a good match for a vLLM-like local status collector because
  status includes active request state, queue depth, cache details, and Metal
  memory in one unaffiliated endpoint.

## LM Studio

Sources:

- Organization: <https://github.com/lmstudio-ai>
- Server docs:
  <https://github.com/lmstudio-ai/docs/blob/main/1_developer/0_core/0_server/index.md>
- CLI docs: <https://lmstudio.ai/docs/cli>
- Native REST API overview: <https://lmstudio.ai/docs/developer/rest>
- Native chat endpoint: <https://lmstudio.ai/docs/developer/rest/chat>
- Streaming events: <https://lmstudio.ai/docs/developer/rest/streaming-events>
- REST API v0 reference: <https://lmstudio.ai/docs/developer/rest/endpoints>

Relevant API support:

- LM Studio exposes native REST APIs, OpenAI-compatible endpoints, and
  Anthropic-compatible endpoints.
- Native v1 REST API endpoints include `/api/v1/chat`, `/api/v1/models`,
  `/api/v1/models/load`, `/api/v1/models/unload`,
  `/api/v1/models/download`, and `/api/v1/models/download/status`.
- `/api/v1/chat` response `stats` contains `input_tokens`,
  `total_output_tokens`, `reasoning_output_tokens`, `tokens_per_second`,
  `time_to_first_token_seconds`, and optional `model_load_time_seconds`.
- Native streaming emits named SSE events, including model-load events and
  prompt-processing events. The stream ends with `chat.end`, whose `result`
  includes the same `stats` object as non-streaming responses.
- The older `/api/v0/*` REST API was documented as including enhanced stats
  such as tokens/second and TTFT, but LM Studio now recommends v1 for new
  integrations.
- OpenAI-compatible chat completions include standard `usage` fields in the
  OpenAI-shaped response.

Capture implications:

- Prefer LM Studio's native `/api/v1/chat` when Fizeau controls the request
  shape and wants performance telemetry. It exposes richer stats than generic
  OpenAI compatibility.
- Use OpenAI-compatible `/v1/chat/completions` when compatibility matters, but
  expect mostly token usage rather than the richer native stats object.
- For streaming native v1 chat, parse named SSE events:
  `model_load.start/progress/end`, `prompt_processing.start/progress/end`,
  deltas, and terminal `chat.end`.
- Use `/api/v1/models` and the CLI/server status commands for model lifecycle
  and loaded/unloaded state. The public docs do not show a v1 equivalent of
  oMLX/Rapid's aggregate `/status` endpoint with live queue depth and memory,
  so Fizeau should treat utilization as model-state plus per-request stats
  unless a version-specific endpoint is verified.

## Ollama

Sources:

- Repository: <https://github.com/ollama/ollama>
- API usage docs: <https://docs.ollama.com/api/usage>
- Generate endpoint: <https://docs.ollama.com/api/generate>
- Chat endpoint: <https://docs.ollama.com/api/chat>
- Running models endpoint: <https://docs.ollama.com/api/ps>
- Code inspected at commit `d319227df01254ba375dbabd1d42e851465e4476`:
  `api/types.go`, `server/routes.go`, `docs/api/usage.mdx`

Relevant API support:

- Native endpoints: `/api/generate`, `/api/chat`, `/api/embed`,
  `/api/tags`, `/api/ps`, `/api/show`, plus model-management endpoints.
- `/api/generate` and `/api/chat` response metrics include:
  `total_duration`, `load_duration`, `prompt_eval_count`,
  `prompt_eval_duration`, `eval_count`, and `eval_duration`.
- Ollama documents all timing values in nanoseconds.
- For streaming responses, usage fields are included in the final chunk where
  `done` is true.
- `/api/ps` returns loaded/running models with `size`, `size_vram`,
  `expires_at`, `context_length`, and model details such as family,
  parameter size, and quantization.

Capture implications:

- Prefer native `/api/chat` or `/api/generate` over OpenAI compatibility for
  metrics capture because native responses expose timing primitives.
- Convert:
  `prompt_tokens = prompt_eval_count`,
  `completion_tokens = eval_count`,
  `prompt_tokens_per_second = prompt_eval_count / prompt_eval_duration_s`,
  `generation_tokens_per_second = eval_count / eval_duration_s`,
  `total_time = total_duration_s`,
  `model_load_duration = load_duration_s`.
- Poll `/api/ps` for loaded-model inventory, context length, and VRAM size,
  but do not expect per-request queue depth, active request progress, or cache
  hit details from the documented API.
- Ollama request metrics are reliable and simple. Live utilization is shallower
  than oMLX/Rapid-MLX unless Fizeau supplements it with external process/GPU
  telemetry.

## Recommended Fizeau Integration Order

1. Add a native Ollama metric parser because the fields are stable, documented,
   and easy to map.
2. Add a generic OpenAI-compatible usage parser with `stream_options.include_usage`
   support. This covers oMLX, Rapid-MLX, and many LM Studio flows.
3. Add platform status collectors:
   - Rapid-MLX: `GET /v1/status`
   - oMLX: `GET /api/status`, optional `GET /admin/api/stats`
   - Ollama: `GET /api/ps`
   - LM Studio: `GET /api/v1/models`, plus native chat stats
4. Add LM Studio native `/api/v1/chat` support as a separate capture path when
   Fizeau wants richer stats than the OpenAI-compatible endpoint provides.
5. Keep provider adapters tolerant of missing metrics. Every platform can
   produce token usage, but live utilization depth varies substantially.

## Normalized Metric Names

Suggested internal fields:

- `provider_platform`: `omlx`, `rapid-mlx`, `lmstudio`, `ollama`
- `api_family`: `openai`, `anthropic`, `native`
- `request_id`
- `model`
- `prompt_tokens`
- `completion_tokens`
- `reasoning_tokens`
- `total_tokens`
- `cached_prompt_tokens`
- `wall_time_s`
- `model_load_time_s`
- `ttft_s`
- `prompt_eval_time_s`
- `generation_time_s`
- `prompt_tokens_per_s`
- `generation_tokens_per_s`
- `queue_depth`
- `active_requests`
- `waiting_requests`
- `loaded_models`
- `model_memory_bytes`
- `model_vram_bytes`
- `cache_hit_rate`
- `cache_hit_type`
- `cache_memory_bytes`

## Open Questions

- LM Studio may have version-specific status/debug surfaces beyond the public
  docs; verify against the installed server version before assuming the only
  utilization source is `/api/v1/models` plus per-request stats.
- oMLX `/admin/api/stats` is excellent for utilization but admin-auth scoped.
  Fizeau should first support `/api/status`, then add admin credentials only as
  an explicit opt-in.
- Rapid-MLX exposes status details under `/v1/status`, but the OpenAI response
  itself does not carry all timing fields. Long-running captures should poll
  status during the request if TTFT/cache/current memory are needed.
- Ollama has strong per-response timing but no documented cache-hit or queue
  endpoint. External process telemetry may be needed for full utilization.
