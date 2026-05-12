# Reasoning Wire Form Runbook - 2026-05-11

## Findings

The 2026-05-11 reasoning probe confirmed that reasoning control is a
model/catalog property, not just a provider transport property. OpenRouter
`qwen/qwen3.6-27b` now uses the token-budget wire (`reasoning.max_tokens:
2048` on the low row) while Sindri's llama.cpp Qwen endpoint requires the
`chat_template_kwargs` envelope; top-level Qwen fields do not activate
thinking. Vidar ds4 exposes DeepSeek V4 Flash through a flat OpenAI-compatible
`reasoning_effort` field, with `reasoning_content` present in responses but no
native `usage.completion_tokens_details.reasoning_tokens`, so Fizeau must use
the reasoning-content fallback token estimate.

## Probe Artifacts

Artifacts are under `docs/research/probe-reasoning-2026-05-11-v2/`.

| Lane | Introspection | Matrix | Verdict |
|---|---|---|---|
| `fiz-openrouter-qwen3-6-27b` | `fiz-openrouter-qwen3-6-27b/introspection.json` | `fiz-openrouter-qwen3-6-27b/matrix.md` | reasoning emitted; catalog keeps Qwen behavior encoded separately from plain provider defaults |
| `sindri-club-3090-llamacpp` | `sindri-club-3090-llamacpp/introspection.json` | `sindri-club-3090-llamacpp/matrix.md` | `chat_template_kwargs.{enable_thinking,thinking_budget}` activates reasoning |
| `vidar-ds4` | `vidar-ds4/introspection.json` | `vidar-ds4/matrix.md` | `reasoning_effort` is the active ds4 wire form; tokens are approximate |
| `vidar-qwen3-6-27b` | `vidar-qwen3-6-27b/NOT-IN-USE.md` | n/a | retired; port 1235 was not listening during the v2 check |

Verification reports live under
`docs/research/probe-reasoning-2026-05-11-v2/verification/`. Each active lane
has a `report.json` carrying `reasoning_tokens > 0` and an explicit
`reasoning_tokens_approx` flag.

## Operator Runbook

When adding a new model, first capture the endpoint's introspection surface
(`/props`, `/v1/models`, or the provider's model metadata endpoint) into the
probe artifact directory. Run `cmd/fizeau-probe-reasoning` against the exact
benchmark profile and inspect both the request bodies and the response token
accounting. Add the catalog entry only after choosing the measured
`reasoning_wire`: use `tokens` when token budgets are the knob that changes
reasoning volume, `effort` when named effort is the actual wire contract,
`model_id` when the model identifier encodes the behavior, and `none` only when
all non-off rows fail to produce reasoning. Finally, run one short verification
cell and confirm the resulting `report.json` records nonzero reasoning tokens
with `reasoning_tokens_approx` matching the source of the count.
