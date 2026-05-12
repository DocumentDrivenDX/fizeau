_Provider details below use public lane labels. Raw hostnames, ports, and
machine inventory keys stay in the benchmark runner configuration; the website
only publishes the runtime, model, sampling, limits, and hardware class needed
to interpret results._

### OpenRouter (cloud aggregator)

- **Lane:** `fiz-openrouter-qwen3-6-27b`
- **Surface:** OpenAI-compatible chat-completions API, model `qwen/qwen3-6-27b-instruct`.
- **Engine version:** TODO — OpenRouter does not surface the underlying provider's engine in the `/v1/models` payload, and we do not log the `OpenRouter-Provider` response header that would identify which back-end pool served each request.
- **Sampling defaults (sent by fiz):** `temperature=0.6`, `top_p=0.95`, `top_k=20`, reasoning `low` (Qwen3 thinking-mode recommended).
- **KV / prefix cache:** opaque — provider-side. OpenRouter's pricing page lists cached-input as `$0.0` for this model, suggesting either no cache or no rebate. Observed `p50 TTFT ≈ 0.79 s` is consistent with a warm pool.
- **Quoted limits:** 128k context advertised, 32k max output, no rate limit configured in our profile.
- **Cost:** observed `≈ $0.12 / run` at the `all` cell median (87k in / 5.2k out tokens × $0.10/Mtok in / $0.30/Mtok out — TODO confirm against the live OpenRouter price page snapshot).

### sindri-vllm (vLLM int4, local CUDA)

- **Lane:** `sindri-vllm`
- **Engine:** vLLM, model `qwen3.6-27b-autoround` (AutoRound int4 weights). **Engine version: TODO**.
- **Sampling (sent by fiz):** `temperature=0.6`, `top_p=0.95`, `top_k=20`, reasoning `low`.
- **KV / prefix cache:** vLLM launches **without** `--enable-prefix-caching` today — observed TTFT scales sharply with input length on the per-context-bucket chart above, the symptom. Turning it on is the single biggest performance lever for this lane (see overview "Open questions").
- **Hardware class:** local RTX-class CUDA workstation.
- **Limits:** 180k context advertised in the profile, 32k max output. Real usable context is lower in int4.
- **Cost:** $0 cash; observed wall-time roughly 2× the OpenRouter lane on the same task set, dominated by prefill.

### sindri-llamacpp (llama.cpp Q3_K_XL, local CUDA)

- **Lane:** `sindri-llamacpp`
- **Engine:** llama.cpp, Qwen3.6-27B Q3_K_XL quantization. **Engine version: TODO**.
- **Sampling (sent by fiz):** `temperature=0.6`, `top_p=0.95`, `top_k=20`; provider-specific reasoning hints are not sent to llama.cpp.
- **Hardware class:** same local RTX-class CUDA workstation as `sindri-vllm`; the runtime and quantization differ.
- **Limits:** advertised context and max-output values come from the profile; verify effective usable context after the next full sweep.

### local-vllm-rtx3090 (vLLM int4, local CUDA laptop)

- **Lane:** `local-vllm-rtx3090`
- **Engine:** vLLM, same `qwen3.6-27b-autoround` weights as `sindri-vllm`. **Engine version: TODO.** Same prefix-cache caveat — not enabled.
- **Sampling:** identical to `sindri-vllm`.
- **Hardware class:** local RTX-class CUDA laptop.
- **Notes:** Mobile inference host; only 7 of 196 attempts produced real reps in the latest sweep — most are `invalid_setup`. Treat the row as "lane wired up" rather than "lane producing comparable data."

### local-omlx-qwen3-6-27b (oMLX 8-bit, Apple silicon)

- **Lane:** `local-omlx-qwen3-6-27b`
- **Engine:** oMLX, model `Qwen3.6-27B-MLX-8bit`. **oMLX version: TODO**.
- **Sampling:** `temperature=0.6`, `top_p=0.95`, `top_k=20`, reasoning `low`.
- **KV / prefix cache:** MLX-side cache behaviour is TODO — we have not confirmed whether oMLX replays full conversation context per turn. The high median input-token count on this lane is consistent with replay rather than incremental decode and is one of the open questions on the overview page.
- **Hardware class:** local Apple Silicon workstation with large unified memory.
- **Limits:** 128k context advertised, 32k max output.

### local-rapidmlx-qwen3-6-27b (RapidMLX 8-bit, Apple silicon)

- **Lane:** `local-rapidmlx-qwen3-6-27b`
- **Engine:** Rapid-MLX, model `mlx-community/Qwen3.6-27B-8bit`. **Engine version: TODO.**
- **Sampling:** identical to the oMLX lane.
- **Hardware class:** local Apple Silicon workstation class.
- **Notes:** 0 real reps of 178 attempts in the latest sweep; treat as not yet producing comparable data.

### local-lmstudio-qwen3-6-27b (LM Studio alternate runtime)

- **Lane:** `local-lmstudio-qwen3-6-27b`
- **Engine:** LM Studio. **Version, exact model build, sampling defaults at the server: all TODO.**
- **Notes:** 0 real reps of 375 attempts in the latest sweep — a placeholder lane. Treat as not producing comparable data until the LM Studio request pipeline is fixed.
