# Probe Reasoning v2 - Post-Wire-Fix Verification

**Run:** 2026-05-12 UTC for OR and sindri refresh; ds4 complete matrix retained
from the 2026-05-11 v2 run because the 2026-05-12 live refresh exceeded the
probe session window after the medium row.  
**Probe binary:** `cmd/fizeau-probe-reasoning` with Qwen
`chat_template_kwargs`, ds4 `openai_effort`, and reasoning-content fallback
support.

## Verdict per lane

| Lane | Wire | Reasoning observed | Notes |
|---|---|---:|---|
| `fiz-openrouter-qwen3-6-27b` | OpenRouter `reasoning` object | yes, direct usage tokens | Catalog/code verification selects token-shaped wire for `qwen/qwen3.6-27b`; `reasoning: low` maps to the emitted cap `reasoning.max_tokens: 2048`. The smoke report records direct provider usage of `reasoning_tokens: 352`, which is valid observed spend under that cap. |
| `sindri-club-3090-llamacpp` | `chat_template_kwargs.{enable_thinking,thinking_budget}` | yes, approximate | 2026-05-12 probe confirms the kwargs envelope activates Qwen thinking across every non-off row. |
| `vidar-ds4` | top-level `reasoning_effort` plus `think:false` off path | yes, approximate | Complete 2026-05-11 matrix confirms ds4 emits `reasoning_content`; the 2026-05-12 `/props` snapshot is present. |
| `vidar-qwen3-6-27b` | legacy oMLX | retired | Port 1235 was not listening on 2026-05-12; use `vidar-ds4` for current vidar verification. |

## Verification Reports

Single-cell reasoning telemetry reports are under `verification/`:

- `verification/fiz-openrouter-qwen3-6-27b/report.json`
- `verification/sindri-club-3090-llamacpp/report.json`
- `verification/vidar-ds4/report.json`

All active reports carry `reasoning_tokens > 0` and an explicit
`reasoning_tokens_approx` flag. OpenRouter is direct usage accounting; sindri
and ds4 are reasoning-content fallback estimates.

PortableBudgets are verified as emitted controls, not expected observed spend.
For OpenRouter Qwen, the deterministic property is that the selected catalog
wire is `tokens` and named `low` intent emits the 2048-token cap. The observed
`reasoning_tokens` value is whatever the provider actually consumed and
reported. It must be truthful and correctly marked as direct or approximate,
but it is not required to equal 2048.

The earlier probe matrix `fiz-openrouter-qwen3-6-27b/low-request.json` is
pre-final evidence and shows the effort-shaped request that motivated this
fix. It is not used as the final captured wire artifact for the smoke report.

## Catalog Conclusion

`qwen/qwen3.6-27b` is cataloged as `reasoning_wire: tokens` with the portable
budget table (`low: 2048`, `medium: 8192`, `high: 32768`), while
`deepseek-v4-flash` is cataloged as `reasoning_wire: effort` with
`reasoning_levels: [high, max]` and `reasoning_default: high`. These match the
probe findings, focused wire-form tests, and the observed telemetry contracts.
