_Provider details below are pulled from `scripts/benchmark/profiles/*.yaml` (sampling defaults, advertised limits, runtime label) and `scripts/benchmark/machines.yaml` (machine spec for self-hosted lanes). Fields explicitly marked **TODO** are not yet captured anywhere in the registry; they are flagged here so they jump out when `scripts/benchmark/capture-machine-info.sh` lands and starts populating real values._

### OpenRouter (cloud aggregator)

- **Profile id:** `fiz-openrouter-qwen3-6-27b`
- **Endpoint:** `https://openrouter.ai/api/v1` — OpenAI-compatible chat-completions surface, model `qwen/qwen3-6-27b-instruct`.
- **Engine version:** TODO — OpenRouter does not surface the underlying provider's engine in the `/v1/models` payload, and we do not currently log the `OpenRouter-Provider` response header that would identify which back-end pool served each request.
- **Sampling defaults (sent by fiz):** `temperature=0.6`, `top_p=0.95`, `top_k=20`, reasoning `low` (Qwen3 thinking-mode recommended).
- **KV / prefix cache:** opaque — provider-side. OpenRouter's pricing page lists cached-input as `$0.0` for this model, suggesting either no cache or no rebate. Observed `p50 TTFT ≈ 0.79 s` is consistent with a warm pool.
- **Quoted limits:** 128k context advertised, 32k max output, no rate limit configured in our profile.
- **Cost:** observed `≈ $0.12 / run` at the `all` cell median (87k in / 5.2k out tokens × $0.10/Mtok in / $0.30/Mtok out — TODO confirm against the live OpenRouter price page snapshot).

### Sindri (vLLM int4, RTX 5090 Ti)

- **Profile id:** `sindri-club-3090`
- **Endpoint:** `http://sindri:8020/v1` (Tailscale).
- **Engine:** vLLM, model `qwen3.6-27b-autoround` (AutoRound int4 weights). **Engine version: TODO** — capture from `vllm --version` on the host; not currently in `machines.yaml`.
- **Sampling (sent by fiz):** `temperature=0.6`, `top_p=0.95`, `top_k=20`, reasoning `low`.
- **KV / prefix cache:** vLLM is launched **without** `--enable-prefix-caching` today — observed TTFT scales sharply with input length on the per-context-bucket chart above, and that is the symptom. Turning it on is the single biggest performance lever for this lane (see overview "Open questions").
- **Hardware (`machines.yaml#sindri`):** Custom desktop · AMD Ryzen 9 5950X · NVIDIA RTX 5090 Ti · Windows + WSL2. Memory **TODO**.
- **Limits:** 180k context advertised in the profile, 32k max output. Real usable context will be lower in int4.
- **Cost:** $0 cash; observed wall-time roughly 2× the OpenRouter lane on the same task set, dominated by prefill.

### Bragi (vLLM int4, RTX 5090 24 GB Laptop)

- **Profile id:** `bragi-club-3090`
- **Endpoint:** `http://bragi:8020/v1` (Tailscale).
- **Engine:** vLLM, same `qwen3.6-27b-autoround` weights as sindri. **Engine version: TODO.** **Same prefix-cache caveat** as sindri — not enabled.
- **Sampling:** identical to sindri.
- **Hardware (`machines.yaml#bragi`):** Lenovo Legion 7i Pro laptop · NVIDIA RTX 5090 24 GB (Laptop) · Windows 11 + WSL2. CPU and memory **TODO**.
- **Notes:** Mobile inference host; only 7 of 196 attempts produced real reps in the latest sweep — most attempts are `invalid_setup`. Treat the row as "lane wired up" rather than "lane producing comparable data."

### Vidar (oMLX 8-bit, Apple M2 Ultra)

- **Profile id:** `vidar-qwen3-6-27b`
- **Endpoint:** `http://vidar:1235/v1` (Tailscale).
- **Engine:** oMLX, model `Qwen3.6-27B-MLX-8bit`. **oMLX version: TODO** — not currently captured.
- **Sampling:** `temperature=0.6`, `top_p=0.95`, `top_k=20`, reasoning `low`.
- **KV / prefix cache:** MLX-side cache behaviour is TODO — we have not confirmed whether oMLX is replaying full conversation context per turn. The high median input-token count on this lane (104k vs sindri's 87k on the same task subset) is consistent with replay rather than incremental decode and is one of the open questions on the overview page.
- **Hardware (`machines.yaml#vidar`):** Apple Mac Studio · Apple M2 Ultra · 192 GB unified memory · macOS. Specific macOS version **TODO**.
- **Limits:** 128k context advertised, 32k max output.

### Grendel (RapidMLX 8-bit, Apple silicon)

- **Profile id:** `grendel-rapid-mlx`
- **Endpoint:** `http://grendel:8000/v1` (Tailscale).
- **Engine:** Rapid-MLX, model `mlx-community/Qwen3.6-27B-8bit`. **Engine version: TODO.**
- **Sampling:** identical to vidar.
- **Hardware (`machines.yaml#grendel`):** chassis, CPU, GPU and memory all **TODO** — `machines.yaml` notes only "Apple Silicon RapidMLX backend at :8000 (full hardware spec TBD)". This is a high priority for `capture-machine-info.sh`.
- **Notes:** 0 real reps of 178 attempts in the latest sweep; treat as not yet producing comparable data.

### lmstudio (Bragi alternate runtime)

- **Profile id:** `bragi-qwen3-6-27b`
- **Endpoint:** alternate port on the same Bragi host (`:1234` per machine notes).
- **Engine:** LM Studio. **Version, exact model build, sampling defaults at the server: all TODO.**
- **Notes:** 0 real reps of 375 attempts in the latest sweep — currently a placeholder lane. Treat as not producing comparable data until the LM Studio request pipeline is fixed.

### TODO checklist for `capture-machine-info.sh`

When the capture script lands, the fields that should populate automatically (and replace the **TODO** markers above) are:

- `vllm --version` / `omlx --version` / `rapid-mlx --version` per host.
- `nvidia-smi --query-gpu=name,memory.total,driver_version --format=csv` for CUDA hosts.
- `system_profiler SPHardwareDataType` (or `sysctl hw.memsize`) for Apple silicon hosts.
- vLLM launch flags (`--enable-prefix-caching`, `--max-model-len`, `--gpu-memory-utilization`) so prefix-cache state is captured rather than inferred.
- Server-side sampling defaults (temperature/top_p/top_k applied when the client omits them) — currently we only know the client-side values fiz sends.
