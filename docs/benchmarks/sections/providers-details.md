Provider details below use public lane labels. They publish enough information to interpret the benchmark results: provider surface, runtime family, model or quantization, sampling policy, context limits, and broad hardware class for self-hosted lanes.

### OpenRouter Qwen3.6-27B

- **Lane:** `fiz-openrouter-qwen3-6-27b`
- **Surface:** managed OpenAI-compatible API through OpenRouter.
- **Model:** `qwen/qwen3-6-27b-instruct`.
- **Sampling:** `temperature=0.6`, `top_p=0.95`, `top_k=20`, reasoning `low`.
- **Limits:** 128k advertised context, 32k max output.
- **Cost profile:** low cash cost per run; current medians put it near the budget end of the hosted lanes.
- **Notes:** provider-side routing and caching are opaque, so this lane is best read as the managed throughput reference for Qwen3.6-27B.

### sindri-vllm

- **Lane:** `sindri-vllm`
- **Surface:** self-hosted vLLM on a local RTX-class CUDA workstation.
- **Model:** Qwen3.6-27B AutoRound int4.
- **Sampling:** `temperature=0.6`, `top_p=0.95`, `top_k=20`, reasoning `low`.
- **Limits:** 180k advertised context, 32k max output; effective usable context depends on runtime memory pressure.
- **Notes:** decode throughput is strong, but TTFT rises with long context. Prefix caching is the main performance lever for this lane.

### sindri-llamacpp

- **Lane:** `sindri-llamacpp`
- **Surface:** self-hosted llama.cpp on the same local CUDA hardware class as `sindri-vllm`.
- **Model:** Qwen3.6-27B Q3_K_XL quantization.
- **Sampling:** `temperature=0.6`, `top_p=0.95`, `top_k=20`; provider-specific reasoning hints are not sent to llama.cpp.
- **Notes:** this lane isolates runtime and quantization differences against the same broad hardware class as the vLLM lane.

### local-vllm-rtx3090

- **Lane:** `local-vllm-rtx3090`
- **Surface:** self-hosted vLLM on a mobile RTX-class CUDA system.
- **Model:** Qwen3.6-27B AutoRound int4.
- **Sampling:** same as `sindri-vllm`.
- **Notes:** this lane is wired up but not yet producing enough real reps for a comparable benchmark read.

### local-omlx-qwen3-6-27b

- **Lane:** `local-omlx-qwen3-6-27b`
- **Surface:** self-hosted oMLX on an Apple silicon workstation class.
- **Model:** Qwen3.6-27B MLX 8-bit.
- **Sampling:** `temperature=0.6`, `top_p=0.95`, `top_k=20`, reasoning `low`.
- **Limits:** 128k advertised context, 32k max output.
- **Notes:** current results show slower TTFT and decode than the CUDA lanes at this model size.

### local-rapidmlx-qwen3-6-27b

- **Lane:** `local-rapidmlx-qwen3-6-27b`
- **Surface:** self-hosted Rapid-MLX on an Apple silicon workstation class.
- **Model:** Qwen3.6-27B MLX 8-bit.
- **Sampling:** same as the oMLX lane.
- **Notes:** this lane is not yet producing comparable real reps.

### local-lmstudio-qwen3-6-27b

- **Lane:** `local-lmstudio-qwen3-6-27b`
- **Surface:** LM Studio alternate runtime.
- **Model:** Qwen3.6-27B class local model.
- **Notes:** this lane is a placeholder until it produces real reps.
