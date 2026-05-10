_Estimates below use observed per-run costs from the lanes that already produced real reps; see `data/aggregates.json` for the underlying numbers. Prices come from `scripts/benchmark/profiles/*.yaml`. Where a lane has no real reps yet, the estimate is a back-of-envelope from `pricing × median-tokens` for a comparable lane on the same model._

### What it would cost to close the gaps

Three of the model rows on the table above carry zero real reps because their setups never produced a non-`invalid_setup` trial in the latest sweep (`claude-native-sonnet-4-6`, `claude-sonnet-4-6` openrouter built-in, `gpt-5-mini`). Two other rows (`fiz-openai-gpt-5-5`, `fiz-openrouter-claude-sonnet-4-6`) have partial coverage on the 35-task `openai-cheap` subset but have not been run with `--reps 5` against the full subset. The estimates below are what it would take in pure model spend to bring each lane to a full `--reps 5 × 35-task` cell on the cheap subset.

| Lane | Source $/run | Subset cost (35 × 5 reps) | Notes |
| --- | --- | --- | --- |
| `fiz-openai-gpt-5-5` | $0.84 (98 real reps observed) | **≈ $147** | Most expensive; `openai-cheap` was sized to keep this under ~$1/run, real number landed close. |
| `fiz-openrouter-gpt-5-5` | not yet run | **≈ $147** est. | Same model + pricing as above; assume identical token use until measured. |
| `fiz-openrouter-claude-sonnet-4-6` | $0.57 (15 real reps observed) | **≈ $100** | Median in-tokens 166k drives the cost; cached-input pricing ($0.30/Mtok) would cut this if prefix-cache hits land. |
| `fiz-harness-claude-sonnet-4-6` | unknown (0 real reps) | **≈ $100** est. | Wrapper path; assume same token profile as the direct OpenRouter Sonnet lane. Currently blocked on `invalid_setup`, not on cost. |
| `fiz-openrouter-gpt-5-4-mini` | $0.053 (14 real reps observed) | **≈ $9** | Already cheap; the bottleneck is reliability, not budget. |
| `fiz-harness-codex-gpt-5-4-mini` | unknown (0 real reps) | **≈ $9** est. | Same model + pricing as the direct mini lane. Reliability gating, not cost. |

To extend everything outside Qwen to a full `--reps 5 × 35-task` cell on `openai-cheap`, total spend is **≈ $510** in API costs (Sonnet ≈ $200 across two paths, GPT-5.5 ≈ $295 across two paths, mini ≈ $18 across two paths). The `all` (89-task) subset for the GPT-5.5 / Sonnet rows would be roughly 2.5× that — call it **≈ $1.3k** for the frontier hosted models, which is why those rows stay on the cheap subset by default.

The cheap rows (mini, Qwen via OpenRouter) are not budget-gated; they are blocked on the same `invalid_setup` issues that produced 108 of 199 attempts in the table above. Fixing that infrastructure issue is what unlocks coverage, not money.
