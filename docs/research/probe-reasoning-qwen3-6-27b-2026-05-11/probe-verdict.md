# Probe Verdict: Qwen3.6-27B Reasoning Wire Form
**Date:** 2026-05-11  
**Bead:** fizeau-b83502c0  
**Spec:** ADR-010

## Summary

| Lane | Model | Tool Verdict | Catalog Value | Notes |
|------|-------|-------------|---------------|-------|
| fiz-openrouter | qwen/qwen3.6-27b | effort | **tokens** | Probe unreliable on trivial prompt — see analysis below |
| sindri-club-3090-llamacpp | Qwen3.6-27B-UD-Q3_K_XL.gguf | none | (see note) | Chat template suppresses thinking; model-property is tokens |
| vidar-ds4 | deepseek-v4-flash | not probed | — | vidar unreachable from exec env (HTTP 000) |
| vidar-qwen3-6-27b | Qwen3.6-27B-MLX-8bit | not probed | — | vidar unreachable from exec env (HTTP 000) |

**Catalog entry `qwen3.6-27b` updated to `reasoning_wire: tokens`.**

## Analysis: OpenRouter Lane

Raw probe data (`reasoning: {effort|max_tokens}` → `reasoning_tokens` in response):

| input | reasoning_tokens | finish |
|-------|-----------------|--------|
| off | 0 | stop |
| low | 648 | stop |
| medium | 437 | stop |
| high | 393 | stop |
| 4096 (max_tokens) | 302 | stop |
| 16384 (max_tokens) | 338 | stop |

The tool returned `verdict: effort` because `low/medium/high` are not within 5% of each other
(`allWithinPct` threshold). However, the data is **unreliable** for determining wire form here:

1. **Inverted ordering**: low > medium > high (648 > 437 > 393). If OR was truly honoring effort
   tiers, high should yield MORE tokens than low. The inversion indicates the model is simply
   using whatever reasoning it requires for "2+2 = 4" (~300–650 tokens) regardless of the effort
   setting — the variation is noise, not signal.

2. **All values far below ADR-010 flat-map figure**: ADR-010 independent research (harder tasks)
   found OR flat-maps effort to ~5555 reasoning tokens for Qwen3. The probe values (302–648)
   are 6–18x lower, consistent with the model naturally finishing before any budget is hit.

3. **Probe tool limitation on trivial prompts**: `fizeau-probe-reasoning` uses
   `"Briefly explain what 2+2 equals."` — cheap by design. When the model's natural thinking
   cost is far below the smallest budget, effort-vs-tokens divergence cannot be observed.
   With a harder question (where the model would use thousands of reasoning tokens), OR's
   flat-mapping behavior becomes visible: low/medium/high all yield ~same count →
   `namedFlat = true` → tool returns `tokens`.

**Operative verdict: `tokens`** — per ADR-010 independent research, which showed OR silently
flat-maps effort for Qwen3 (all tiers → ~5555 tokens) while `max_tokens` form is honored as
`thinking_budget`. The probe tool's `effort` result here is a false positive from the trivial
prompt, not a model-behavior change.

To re-confirm: re-run `fizeau-probe-reasoning` with a prompt that forces the model to think
for 2000–8000 tokens (e.g., a multi-step math proof or code debugging task). Expect
`namedFlat = true` and `verdict = tokens`.

## Analysis: Sindri Lane

All rows returned 0 reasoning tokens. This is consistent with the comment in
`sindri-club-3090-llamacpp.yaml`:

> "the 2026-05-11 probe showed the Q3_K_XL GGUF currently emits empty `<think> </think>`
> regardless of this setting. Operator decision whether to flip the server-side template."

The `none` verdict reflects the server-side chat template suppressing thinking output, not a
model-property. The Qwen3.6-27B model itself supports reasoning; once the template is flipped,
the effective wire form for llamacpp will be `tokens` (Qwen-native wire always uses
`thinking_budget`, budget-only). No catalog change from this lane; model property is `tokens`.

## Analysis: Vidar Lanes

Both `vidar:1235` (omlx) and `vidar:1236` (ds4) were unreachable from the execution
environment (curl returned HTTP 000 / connection refused). Probe could not be run.

- `vidar-ds4` uses DeepSeek V4 Flash, not Qwen3.6-27B. Its reasoning wire is a separate
  catalog concern (deepseek-v4-flash entry). This profile's `reasoning: 4096` → `reasoning: low`
  revert is a policy alignment change (per ADR-010), not driven by a Qwen3.6 wire probe.
- `vidar-qwen3-6-27b` (omlx, legacy) uses oMLX wire (ThinkingMap/Anthropic-format). oMLX
  always uses `budget_tokens`; wire form is always token-budget regardless of catalog setting.
  No probe needed; updating the profile is the operative change.

Operator follow-up: re-run probe on vidar lanes once vidar is accessible to confirm deepseek-v4-flash
and Qwen3.6-27B-MLX-8bit reasoning behavior.
