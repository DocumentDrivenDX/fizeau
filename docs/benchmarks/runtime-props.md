# Runtime Props: Per-Cell Server Metadata

When the bench matrix runner starts a cell, it queries the inference server once
(after preflight, before the bench run) and persists the result as `runtime_props`
in the cell's evidence record. The field is optional: it is absent only for cells
produced before this feature was introduced (pre-`fizeau-c12e6241`).

## Why it exists

Many cell-cell deltas are **not** in the lane definition — they live in the
running server config: MTP on/off, speculative decoder configs, KV quant
variants, context window, etc. Capturing server-reported props per cell makes
the comparison grid self-describing and enables parameter-sweep experiments
where the same lane id is reused with different server configs.

## Field reference

| Field | Type | Example | Notes |
|---|---|---|---|
| `extractor` | string | `"llamacpp"` | Platform that filled this record |
| `extracted_at` | ISO-8601 datetime | `"2026-05-14T10:00:00Z"` | When extraction ran |
| `extraction_failed` | string | `"status 404"` | Only set on failure; absence = success |
| `base_model` | string | `"Qwen3.6-27B-UD-Q3_K_XL.gguf"` | Model id as reported by server |
| `model_quant` | string | `"autoround"` | Quantization method reported by server |
| `kv_quant` | string | `"q8_0"` | KV-cache quantization, if exposed |
| `draft_model` | string | `"z-lab/Qwen3.6-27B-DFlash"` | Draft model id for speculative runs |
| `draft_mode` | string | `"dflash"`, `"mtp"`, `"eagle"`, `"off"` | Speculative decoder variant |
| `max_context` | integer | `32768` | Server-reported context window (tokens) |
| `gpu_layers` | integer | `40` | GPU layers loaded (llama.cpp) |
| `mtp_enabled` | boolean | `true` | Multi-token prediction active (ds4) |
| `speculative_n` | integer | `5` | Speculative tokens per step |
| `server_version` | string | `"0.4.2"` | Server/engine version string |
| `build_info` | string | `"b3001 (commit abc1234)"` | Build metadata from server |
| `sampling_defaults.temperature` | number | `0.8` | Server default temperature |
| `sampling_defaults.top_p` | number | `0.95` | Server default top_p |
| `sampling_defaults.top_k` | integer | `40` | Server default top_k |
| `sampling_defaults.repeat_penalty` | number | `1.1` | Server default repeat penalty |
| `platform_raw` | object | `{...}` | Full server response for grep-ability |

## On extraction failure

If the extractor cannot reach the server or receives an unexpected response,
it writes:

```json
{
  "runtime_props": {
    "extractor": "ds4",
    "extracted_at": "2026-05-14T10:00:00Z",
    "extraction_failed": "ds4: fetch http://192.168.2.106:1236/props: connection refused"
  }
}
```

The cell still runs normally — the failure is logged to stderr and the bench
proceeds. The grid renders `—` for `runtime_props` fields in this case.

## Platform coverage

| Platform (`runtime:` field) | Endpoints queried | Status |
|---|---|---|
| `llama-server` / `llamacpp` | `GET /props` | Confirmed (llama.cpp canonical) |
| `vllm` | `GET /v1/models`, `GET /v1/server_info` | `/v1/models` confirmed; `/v1/server_info` optional |
| `ds4` | `GET /props`, `GET /v1/models` | Confirmed on live vidar:1236; `/props` exposes `model.mtp`, `model.mtp_draft_tokens`, and `runtime.ctx_size` |
| `omlx` | `GET /v1/models` | Confirmed at vidar:1235 |
| `lucebox` / `lucebox-dflash` | `GET /props`, `GET /v1/models` | `/v1/models` confirmed; `/props` TODO — verify on live sindri:1236 |
| `rapid-mlx` | `GET /v1/models` | Confirmed at grendel:8000 |
| `openrouter` / `openai` / cloud | _(none)_ | Cloud lanes get `extractor: "cloud"` with `base_model` from profile |

## Adding a new platform extractor

1. Create `internal/benchmark/runtimeprops/<platform>.go`.
2. Implement a function matching signature `func extractX(ctx context.Context, lane LaneInfo) (evidence.RuntimeProps, error)`.
3. Register the new function in `dispatchExtractor()` in `runtimeprops.go`.
4. Add a unit test in `runtimeprops_test.go` using `httptest.NewServer` with a canned response.
5. Update the platform coverage table above.

The extractor must use `fetchTimeout` (5 s) and must never panic — return
`(evidence.RuntimeProps{}, err)` on failure; the dispatcher wraps that into an
`ExtractionFailed` record automatically.
