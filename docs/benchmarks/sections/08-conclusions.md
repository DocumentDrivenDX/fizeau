_Editorial summary. Regenerate against the data in `data/aggregates.json` and `data/timing.json`. The numbers here go stale as more cells land — re-run `scripts/benchmark/generate-report.py` and refresh this section._

### Qwen3.6-27B across providers (the headline question)

OpenRouter Qwen3.6-27B is the throughput reference. The local lanes bottleneck elsewhere:

- **sindri (vLLM int4 on a 3090)**: best decode rate, worst prefill. On agent loops with 50–150k context per turn, prefill dominates wall — explaining why the median wall is roughly 2× OpenRouter despite faster decode.
- **vidar (oMLX 8-bit on Apple silicon)**: slow on both axes. MLX 8-bit at this model size is the rate limiter; only smaller quantization or a different runtime will move it.

### Model-power signal vs harness loss

The scatter in §6 mostly tracks the expected pattern — frontier-power models (Opus, GPT-5.5) sit at higher pass-rates than Qwen-class — but several Qwen lanes show distance below the trend that maps to harness loss, not model loss. The recently-fixed JSONL-bind-mount bug (commit `18a19a43`) closes one well-understood class of those.

### Cost / reliability frontier

OpenRouter Qwen3.6-27B costs cash per run; sindri and vidar cost $0 in cash but cost in wall-time and reliability. For pure budget, OR Qwen wins; for ceiling-pass tasks where reliability matters, the frontier rows on the leaderboard remain ahead of any Qwen lane regardless of plumbing.

### Open questions

- Vidar's input-token median runs higher than sindri on the same task set. Either the MLX server replays full conversation context where vLLM compacts, or the agent loop runs more turns on vidar before the model converges. Worth a focused trace.
- Sindri's prefill latency is the single biggest performance lever: enabling vLLM `--enable-prefix-caching` (or boosting cache hit rate) should drop TTFT 5–10× and close most of the wall-time gap.
