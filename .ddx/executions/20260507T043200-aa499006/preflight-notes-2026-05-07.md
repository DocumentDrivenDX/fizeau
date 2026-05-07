# TerminalBench 2.1 Local Qwen Provider Preflight — 2026-05-07

Recorded per bead fizeau-169b19c4 AC 8: live model-list smoke checks documented
without committing secrets or machine-local config.

## Verified endpoints and model IDs

| Provider profile    | Endpoint                    | Model ID verified via /v1/models       | /health |
|---------------------|-----------------------------|----------------------------------------|---------|
| vidar-qwen3-6-27b   | http://vidar:1235/v1        | Qwen3.6-27B-MLX-8bit                   | n/a     |
| grendel-rapid-mlx   | http://grendel:8000/v1      | mlx-community/Qwen3.6-27B-8bit         | ready   |
| bragi-club-3090     | http://bragi:8020/v1        | qwen3.6-27b-autoround                  | n/a     |
| sindri-club-3090    | http://sindri:8020/v1       | qwen3.6-27b-autoround                  | n/a     |

Source: bead notes recorded at execution time; no API keys or machine-local
config are stored here.

## Smoke check commands (reference only — do not commit outputs)

```sh
# vidar native oMLX
curl -sf http://vidar:1235/v1/models | python3 -m json.tool

# grendel Rapid-MLX
curl -sf http://grendel:8000/v1/models | python3 -m json.tool
curl -sf http://grendel:8000/health

# bragi club-3090 vLLM
curl -sf http://bragi:8020/v1/models | python3 -m json.tool

# sindri club-3090 vLLM
curl -sf http://sindri:8020/v1/models | python3 -m json.tool
```

## Notes

- No API key required for oMLX (vidar); `OMLX_API_KEY` may be any non-empty string.
- `RAPID_MLX_API_KEY` and `VLLM_API_KEY` are placeholders; set to any non-empty
  string if the server does not enforce auth, or to the actual token if configured.
- The bragi LM Studio profile (`bragi-qwen3-6-27b`, port 1234) is retained as
  historical data but is not part of the verified 2026-05-07 local sweep. It
  should be re-preflighted before inclusion in a future run.
- Vidar is represented only via the native oMLX profile (`provider.type: omlx`).
  No OpenAI-compatible alias sweep lane exists for vidar.
