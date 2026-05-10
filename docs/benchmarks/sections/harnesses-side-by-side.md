### Side-by-side coverage today

The point of the harness page is to hold the model constant and read the difference between rows as harness loss. Two model families currently have enough lanes wired up to read that delta:

**Sonnet 4.6 (three paths, no clean comparison yet).**
- `claude-native-sonnet-4-6` — Claude Code's own CLI, no fiz involvement. 15 attempts, 0 real reps in the latest sweep (all `invalid_setup`).
- `fiz-harness-claude-sonnet-4-6` — fiz wraps the Claude Code CLI. 202 attempts, 0 real reps (same blocker).
- `fiz-openrouter-claude-sonnet-4-6` — fiz's built-in agent loop talking to Sonnet through OpenRouter. 199 attempts, **15 real reps**, 22.2 % pass@1 on the partial `openai-cheap` cell (3 of 11 unique tasks solved any-rep).

The only row that is currently producing graded data is the OpenRouter built-in path. Until the two CLI-wrapping lanes get past `invalid_setup`, we cannot put a number on Claude Code's harness loss versus fiz's loop on Sonnet — that is the comparison we most want to make and the one that is most blocked.

**GPT-5.4-mini (three paths, partial side-by-side).**
- `codex-native-gpt-5-4-mini` — Codex CLI native, only 1 of 3 canary tasks attempted but **100 % pass@k** on what it touched (extremely small sample).
- `fiz-harness-codex-gpt-5-4-mini` — fiz wraps Codex. 202 attempts, 0 real reps. Reports `pass@1 15.3%` from the binary success path even though no token-level data flowed.
- `fiz-openrouter-gpt-5-4-mini` — fiz's built-in loop direct to OpenRouter. 199 attempts, 14 real reps, **6.7 % pass@k** on the partial cell.
- `gpt-5-4-mini-openrouter` — older fiz built-in lane, 100 % pass@k on a 3-task canary only.

The reading here is more legible than Sonnet but still preliminary: native Codex looks much stronger on the canary than fiz's built-in loop on the same model on a wider task set, but the canary is 3 tasks and the fiz cell is 11 — the comparison only becomes diagnostic once the wrapped Codex lane stops invalidating.

### What to compare next

These are the comparisons we cannot make today because one side of the side-by-side is missing. Adding the listed fiz lane would close the gap.

| Model | Have | Missing fiz lane | Why it would matter |
| --- | --- | --- | --- |
| Claude Sonnet 4.6 | `claude-native-sonnet-4-6` (when un-blocked) and `fiz-openrouter-claude-sonnet-4-6` | A working `fiz-harness-claude-sonnet-4-6` cell with real reps | Direct read of Claude Code's harness loss versus a vendor-direct fiz loop on the same model and provider key. |
| GPT-5.4-mini | `codex-native-gpt-5-4-mini` (canary only) and `fiz-openrouter-gpt-5-4-mini` | `codex-native-gpt-5-4-mini` extended to the full `openai-cheap` 35-task cell | The Codex wrapper looks strong on the canary. Without the wider cell we cannot tell whether the canary picked easy tasks or whether the wrapper genuinely outperforms the OpenRouter loop. Cheap to do (≈ $9). |
| GPT-5.5 | `fiz-openai-gpt-5-5` (89 tasks, 24 % pass@k) | `fiz-openrouter-gpt-5-5` cell on the same `openai-cheap` subset | Lets us check whether OpenAI-native vs OpenRouter routing is responsible for any pass-rate delta on the same model, separate from harness. |
| Qwen3.6-27B (frontier reasoning lane) | All three Qwen provider rows on the providers page | A `fiz-harness-codex-qwen-3-6-27b` lane (Codex CLI configured against an OpenAI-compat Qwen endpoint) | Currently the only Qwen rows use fiz's built-in loop. A second harness on the same Qwen weights would let the providers-page numbers be re-read as model-loss vs harness-loss. |
| Opus 4.6 | External leaderboard (Crux, Judy, Capy, Droid, Mux, Terminus2 all reporting > 78 % on `all`) | Any fiz lane on Opus 4.6 (built-in loop or harness wrapper) | We currently have zero internal coverage of the model that tops the leaderboard. Without it the gap between fiz and the best external row mixes harness loss and model loss into a single unreadable number. |

Across these five rows the cheapest two (mini extended to 35 tasks, and getting the wrapped-Codex lane producing real data) are the highest-leverage additions: both are well under $20 in API spend and they unblock the only model where we have lanes on three different harnesses on the same model.
