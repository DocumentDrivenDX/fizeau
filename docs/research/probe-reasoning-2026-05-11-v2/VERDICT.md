# Probe Reasoning v2 — Post-Wire-Fix Verification

**Run:** 2026-05-11 22:45 UTC
**Probe binary:** `cmd/fizeau-probe-reasoning` after probe-tool wire-format
patches (mirror commit `cfdcdcc4` for sindri kwargs envelope + `OpenAIEffort`
for ds4) and reasoning-content fallback extractor (mirrors `fizeau-8f62bcbb`).

## Verdict per lane

| Lane | Wire | All tiers think? | Notes |
|---|---|---|---|
| `fiz-openrouter-qwen3-6-27b` | `openrouter` (`reasoning.{effort\|max_tokens}`) | ✅ off=0; non-off=246–523 toks | Regression check — unchanged from prior. Probe verdict heuristic recommends `effort` but catalog correctly has `tokens` per ADR-010 evidence (effort flat-mapped upstream for Qwen3 on OR). |
| `sindri-club-3090-llamacpp` | `qwen` (`chat_template_kwargs.{enable_thinking, thinking_budget}`) | ✅ off=0; non-off=223–477 toks | Wire fix landed. `low` row truncated (probe's 512 max-token cap; benchmark profiles use 65536). |
| `vidar-ds4` | `openai_effort` (top-level `reasoning_effort` + `think:false` off) | ✅ off=0; non-off=57–75 toks | Wire fix landed. Flat distribution across low/medium/high/4096/16384 confirms `/props.reasoning.aliases` collapse (only `max` is distinct, untested). |

## Cross-lane observations

- All three lanes now emit thinking when fizeau-aligned wire is sent.
  Pre-fix probe (preserved at
  `docs/research/probe-reasoning-qwen3-6-27b-2026-05-11/`) showed sindri+ds4
  at zero across all rows.
- Ds4's reasoning_tokens come from the char-count fallback on
  `message.reasoning_content` (fizeau-8f62bcbb ext); ds4 doesn't populate
  `usage.completion_tokens_details.reasoning_tokens`.
- Sindri shows budget-non-binding behavior consistent with prior probe
  (request `thinking_budget=2048` does not produce 2048 reasoning tokens).
  Treat as soft hint; `max_tokens` is the only hard cap.
- Probe verdict heuristic is overconfident for OR — recommends `effort`,
  but ADR-010 evidence shows OR-Qwen3 flat-maps effort tiers and only
  honors `max_tokens`. The catalog already has `tokens`; ignore the
  probe's recommendation in that case.

## Known probe limitations (not blocking benchmark sweep)

- Probe duplicates wire-construction logic from `internal/provider/openai`
  rather than driving fizeau's actual provider stack. Future maintenance
  drift is a risk; tracked as part of `fizeau-7c4b7647`.
- 512 max-token cap inside the probe truncates `low` rows on lanes that
  produce >512 tokens of thinking. Doesn't affect benchmark cells.
- Verdict logic is heuristic only — token-budget vs effort recommendation
  doesn't reflect upstream-routing nuances (OR's per-model knob choice).

## Conclusion

Wire emission verified across all three benchmark lanes. Safe to launch
the timing-baseline sweep.
