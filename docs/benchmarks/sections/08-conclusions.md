### Qwen3.6-27B across providers (the headline question)

OpenRouter Qwen3.6-27B is the throughput reference. The local lanes bottleneck elsewhere:

- **sindri-vllm (vLLM int4 on local CUDA)**: best decode rate, worst prefill. On agent loops with 50–150k context per turn, prefill dominates wall — explaining why the median wall is roughly 2× OpenRouter despite faster decode.
- **local-omlx-qwen3-6-27b (oMLX 8-bit on Apple silicon)**: slow on both axes. MLX 8-bit at this model size is the rate limiter; only smaller quantization or a different runtime will move it.

### Model-power signal vs harness loss

The scatter in section 6 mostly tracks the expected pattern: frontier-power models (Opus, GPT-5.5) sit at higher pass-rates than Qwen-class models. Several Qwen lanes still sit below the trend, which points to harness/runtime loss in addition to model capability.

### Cost / reliability frontier

OpenRouter Qwen3.6-27B costs cash per run; local lanes cost $0 in cash but cost in wall-time and reliability. For pure budget, OR Qwen wins; for ceiling-pass tasks where reliability matters, the frontier rows on the leaderboard remain ahead of any Qwen lane regardless of plumbing.

### Open questions

- The oMLX lane's input-token median runs higher than the vLLM lane on the same task set. Either the MLX server replays full conversation context where vLLM compacts, or the agent loop runs more turns before the model converges. Worth a focused trace.
- `sindri-vllm` prefill latency is the single biggest performance lever: enabling vLLM `--enable-prefix-caching` (or boosting cache hit rate) should drop TTFT 5–10× and close most of the wall-time gap.
