Each card below is one (provider, model, harness) tuple. Cards are colored by provider family for grouping; the same color carries into subsequent charts. Lane definitions live in `scripts/benchmark/profiles/<id>.yaml`.

The catalog spans four kinds of provider surface:

- **OpenRouter / OpenAI / Anthropic** — managed API providers used as throughput and reliability references.
- **vLLM** (`sindri-vllm`, local RTX-class CUDA) — self-hosted with int4 AutoRound quantization.
- **llama.cpp** (`sindri-llamacpp`, same local CUDA class) — self-hosted Q3_K_XL runtime.
- **oMLX / RapidMLX** — Apple-silicon MLX runtimes at 8-bit quantization.

Lanes whose `id` starts with `fiz-harness-` route through fiz-as-a-harness wrapping a different agent CLI (e.g. claude or codex) to isolate "is the agent loop hurting?" from "is the model hurting?".

Self-hosted lanes are reported with stable public lane labels and broad hardware classes, so the results are comparable without exposing private service details.
