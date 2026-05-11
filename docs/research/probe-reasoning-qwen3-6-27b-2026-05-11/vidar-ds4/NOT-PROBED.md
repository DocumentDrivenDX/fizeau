# vidar-ds4: Probe Not Run

vidar (http://vidar:1236) was unreachable from the execution environment on 2026-05-11
(curl returned HTTP 000 / connection refused). Probe could not be run.

**Model:** DeepSeek V4 Flash (not Qwen3.6-27B)  
**Lane:** vidar-ds4 (ds4/Anthropic-format wire)  
**Profile change:** `reasoning: 4096` → `reasoning: low` (ADR-010 policy alignment)

The ds4 transport uses `thinking: {type: enabled, budget_tokens: N}` (ThinkingMap/Anthropic
wire format). This is always budget-based; the `reasoning_wire` catalog value for
deepseek-v4-flash is a separate probe item from qwen3.6-27b.

Operator action: re-run `fizeau-probe-reasoning -profile scripts/benchmark/profiles/vidar-ds4.yaml`
once vidar is accessible.
