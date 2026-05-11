# vidar-qwen3-6-27b: Probe Not Run

vidar (http://vidar:1235) was unreachable from the execution environment on 2026-05-11
(curl returned HTTP 000 / connection refused). Probe could not be run.

**Model:** Qwen3.6-27B-MLX-8bit (oMLX 8-bit, legacy lane)  
**Lane:** vidar-qwen3-6-27b (omlx/ThinkingMap wire)  
**Status:** This lane was retired on 2026-05-11 in favor of vidar-ds4 (deepseek-v4-flash).
The profile is kept for historical reruns.

The oMLX transport uses Anthropic-format `thinking: {type: enabled, budget_tokens: N}`, which
is always budget-based. Wire form for Qwen3.6-27B-MLX-8bit on oMLX is effectively `tokens`
regardless of catalog setting.

Operator action: re-run `fizeau-probe-reasoning -profile scripts/benchmark/profiles/vidar-qwen3-6-27b.yaml`
once vidar is accessible and the oMLX server is running on port 1235.
