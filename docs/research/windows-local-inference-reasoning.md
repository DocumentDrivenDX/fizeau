# Windows Local Inference Reasoning Controls

Date: 2026-04-23

## Question

Agent needs a Windows-local inference path that satisfies both sides of the
execute-bead contract:

- tool-compatible chat for read/write/edit/bash style agent tools
- enforceable per-request reasoning control so local Qwen/GPT-OSS models do not
  spend the whole output budget in reasoning loops

The Mac recommendation is currently OMLX for Qwen models: Vidar OMLX probes show
Qwen `enable_thinking` / `thinking_budget` controls changing behavior. LM Studio
remains useful as a broad local provider, but Bragi's Qwen GGUF evidence showed
OpenAI-compatible `/v1/chat/completions` accepting multiple reasoning control
shapes without honoring them.

## Current Evidence

| Engine | Windows stance | Tool-compatible chat | Reasoning off | Named levels | Token budget | Separated reasoning | Notes |
| --- | --- | --- | --- | --- | --- | --- | --- |
| LM Studio OpenAI-compatible | Windows app available; already used on Bragi | Yes, via `/v1/chat/completions` | Not proven on Bragi Qwen GGUF | GPT-OSS via `/v1/responses` `reasoning.effort` only | Not proven | Yes for some models | Keep as supported provider, but classify Bragi Qwen as accepted/no-op until a new probe proves otherwise. |
| LM Studio native REST | Windows app available | Native `/api/v1/chat` docs say custom tools are not supported on that endpoint | Documented `reasoning: off` | `low`, `medium`, `high`, `on` | No numeric budget documented | `stats.reasoning_output_tokens` | Good probe surface; not yet an execute-bead surface because tools are missing. |
| Ollama | Windows-supported local server | Has chat API; tool compatibility still needs agent-specific probe | API `think: false` for most thinking models | GPT-OSS uses `low`, `medium`, `high` and cannot fully disable thinking | No numeric budget documented | `message.thinking` | Strong Windows candidate for Qwen off/on, weaker for exact budgets. |
| vLLM under WSL/Linux | Practical Windows route is likely WSL2, not native Windows | OpenAI-compatible server with tool-call examples | Qwen3 parser supports `enable_thinking=False` via chat template kwargs | Request-level template kwargs can override defaults | `thinking_token_budget` sampling parameter | `message.reasoning` | Best documented match for tool use plus numeric reasoning budgets, if WSL/GPU setup is acceptable. |
| llama.cpp / llama-server | Windows binaries exist | OpenAI-style function calling documented | Qwen docs and community issues discuss `enable_thinking=false`; behavior must be probed | No stable named-level contract found | Active `--reasoning-budget` work, but per-request API support is unclear | Reasoning chunks appear in server output | Promising Windows-native fallback, but needs live probe against the exact build/model. |

## Source Notes

- LM Studio REST overview documents `/api/v1/chat` and its endpoint comparison:
  native chat supports a request `context_length`, but custom tools are listed on
  OpenAI-compatible `/v1/responses`, `/v1/chat/completions`, and
  Anthropic-compatible `/v1/messages`, not native chat:
  <https://lmstudio.ai/docs/developer/rest-api>
- LM Studio native chat documents `reasoning` as
  `off|low|medium|high|on` and response `stats.reasoning_output_tokens`:
  <https://lmstudio.ai/docs/developer/rest/chat>
- LM Studio Responses documents `reasoning: { "effort": "low" }` for
  `openai/gpt-oss-20b`:
  <https://lmstudio.ai/docs/developer/openai-compat/responses>
- Ollama documents the `think` API field, Qwen 3 support, GPT-OSS
  `low|medium|high`, and separated `message.thinking`:
  <https://docs.ollama.com/capabilities/thinking>
- vLLM documents Qwen3 reasoning parser behavior, request-level
  `chat_template_kwargs`, tool-call examples with reasoning, and
  `thinking_token_budget`:
  <https://docs.vllm.ai/en/latest/features/reasoning_outputs/>
  <https://docs.vllm.ai/en/stable/api/vllm/reasoning/qwen3_reasoning_parser/>
- llama.cpp documents OpenAI-style function calling:
  <https://github.com/ggml-org/llama.cpp/blob/master/docs/function-calling.md>
  Its Qwen local-running docs and current project discussions indicate
  `enable_thinking` / reasoning-budget behavior is active but still needs
  build-specific validation:
  <https://qwen.readthedocs.io/en/latest/run_locally/llama.cpp.html>
  <https://github.com/ggml-org/llama.cpp/discussions/21445>

## Recommendation

Use a tiered Windows strategy:

1. Default Windows research target: vLLM in WSL2 for evidence-grade local
   execute-bead, because it is the best documented intersection of
   OpenAI-compatible tool calling, separated reasoning output, request-level
   Qwen thinking control, and numeric `thinking_token_budget`.
2. Windows-native fallback: Ollama for Qwen `think=false` / `think=true`
   comparisons when exact numeric budgets are not required. Treat GPT-OSS as
   level-only because Ollama documents that GPT-OSS cannot fully disable
   thinking.
3. Windows-native experimental path: llama-server, but only after a live probe
   proves the exact build can combine OpenAI-style tools with either per-request
   `enable_thinking=false` or a reasoning budget that is not only a server-level
   startup flag.
4. Keep LM Studio in the support matrix, but split its capability rows:
   OpenAI-compatible chat is tool-capable but Bragi Qwen reasoning control is
   no-op in current evidence; native chat has documented reasoning control but
   is not tool-capable enough for execute-bead.

## Live Probe Plan

For each Windows candidate, run the same four checks before promoting it to an
evidence-grade arm:

1. Tool call smoke: a small OpenAI-compatible chat request with one required
   JSON function/tool call.
2. Reasoning off: Qwen request with the candidate's documented off switch;
   require non-empty content and zero/near-zero separated reasoning.
3. Reasoning budget: request a small budget; require a bounded
   reasoning-token count or an explicit documented unsupported result.
4. Agent loop smoke: one read-only execute-bead task with `reasoning=off` and
   one write task with the intended default reasoning level.

Suggested first commands:

```bash
# Ollama
curl http://localhost:11434/api/chat -d '{"model":"qwen3","messages":[{"role":"user","content":"What is 37*42? Answer only the integer."}],"think":false,"stream":false}'

# vLLM
curl http://localhost:8000/v1/chat/completions -H 'Content-Type: application/json' -d '{"model":"Qwen/Qwen3-8B","messages":[{"role":"user","content":"What is 37*42? Answer only the integer."}],"extra_body":{"chat_template_kwargs":{"enable_thinking":false}}}'

# llama-server
curl http://localhost:8080/v1/chat/completions -H 'Content-Type: application/json' -d '{"model":"qwen3.5","messages":[{"role":"user","content":"What is 37*42? Answer only the integer."}],"chat_template_kwargs":{"enable_thinking":false}}'
```
