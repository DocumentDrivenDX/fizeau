---
title: "Providers"
weight: 4
toc: true
---

<div class="br-body">
<div class="meta">Snapshot: 2026-05-12 13:20:24 UTC · Qwen3.6-27B across 5 provider/runtime combinations</div>
<div class="narrative">
<p>The model weights are the same across every row here — Qwen3.6-27B in some quantization. The variable is everything else: where the bytes get computed, which serving engine runs them, what sampling defaults the server applies, whether prefix-cache is hit, and how much round-trip latency the network adds.</p>
<p>Hostnames are abstracted to the substantive characteristics. The descriptive label captures engine + quantization + GPU/CPU + OS — enough to map to a known-good machine spec without leaking inventory.</p>
</div>
<h2>Pass-rate</h2>
<table><thead><tr><th>Profile / Submission</th><th>canary (3 tasks)</th><th>openai-cheap (35 tasks)</th><th>full (15 tasks)</th><th>all (89 tasks)</th><th>Provider</th></tr></thead><tbody><tr><td>local-vllm-rtx3090</td><td>33.3% <span class="meta">(1/3)</span></td><td>16.7% <span class="meta">(2/12)</span></td><td>6.7% <span class="meta">(1/15)</span></td><td>12.5% <span class="meta">(2/16)</span></td><td><span class="meta">vllm</span></td></tr><tr><td>local-lmstudio-qwen3-6-27b</td><td>0.0% <span class="meta">(0/3)</span></td><td>0.0% <span class="meta">(0/35)</span></td><td>0.0% <span class="meta">(0/15)</span></td><td>0.0% <span class="meta">(0/89)</span></td><td><span class="meta">lmstudio</span></td></tr><tr><td>fiz-openrouter-qwen3-6-27b</td><td>100.0% <span class="meta">(3/3)</span></td><td>85.7% <span class="meta">(30/35)</span></td><td>86.7% <span class="meta">(13/15)</span></td><td>61.8% <span class="meta">(55/89)</span></td><td><span class="meta">openrouter</span></td></tr><tr><td>local-rapidmlx-qwen3-6-27b</td><td>0.0% <span class="meta">(0/3)</span></td><td>8.3% <span class="meta">(1/12)</span></td><td>0.0% <span class="meta">(0/15)</span></td><td>6.2% <span class="meta">(1/16)</span></td><td><span class="meta">rapid-mlx</span></td></tr><tr><td>local-omlx-qwen3-6-27b</td><td>100.0% <span class="meta">(3/3)</span></td><td>61.8% <span class="meta">(21/34)</span></td><td>73.3% <span class="meta">(11/15)</span></td><td>38.3% <span class="meta">(31/81)</span></td><td><span class="meta">omlx</span></td></tr></tbody></table>
<h2>Detailed metrics</h2>
<table><thead><tr><th>Profile</th><th>Harness</th><th>Attempts</th><th>Real</th><th>pass@1</th><th>pass@k</th><th>med turns</th><th>med in</th><th>med out</th><th>med wall (s)</th><th>cost ($)</th><th>p50 TTFT (s)</th><th>p50 decode (tok/s)</th></tr></thead><tbody><tr><td>vLLM int4 / NVIDIA GeForce RTX 5090 Laptop GPU (24 GB) / Ubuntu 24.04.4 LTS (Noble Numbat) on WSL2 / Windows 11 host</td><td><span class="meta">fiz (built-in agent loop)</span></td><td>88</td><td>7</td><td>2.9%</td><td>12.5%</td><td>2</td><td>3,049</td><td>1,073</td><td>90</td><td>0.000</td><td>30.01</td><td>89.4</td></tr><tr><td>lmstudio / NVIDIA GeForce RTX 5090 Laptop GPU (24 GB) / Ubuntu 24.04.4 LTS (Noble Numbat) on WSL2 / Windows 11 host</td><td><span class="meta">fiz (built-in agent loop)</span></td><td>267</td><td>0</td><td>0.0%</td><td>0.0%</td><td>—</td><td>—</td><td>—</td><td>—</td><td>0.000</td><td>—</td><td>—</td></tr><tr><td>OpenRouter (cloud aggregator)</td><td><span class="meta">fiz (built-in agent loop)</span></td><td>334</td><td>293</td><td>64.4%</td><td>61.8%</td><td>15</td><td>98,989</td><td>5,697</td><td>541</td><td>0.147</td><td>0.91</td><td>46.6</td></tr><tr><td>RapidMLX 8-bit / Apple M1 Max (64 GB unified)</td><td><span class="meta">fiz (built-in agent loop)</span></td><td>70</td><td>0</td><td>3.6%</td><td>6.2%</td><td>—</td><td>—</td><td>—</td><td>—</td><td>0.000</td><td>30.02</td><td>15.7</td></tr><tr><td>oMLX 8-bit / Apple M2 Ultra (24-core CPU) (192 GB unified)</td><td><span class="meta">fiz (built-in agent loop)</span></td><td>179</td><td>109</td><td>38.8%</td><td>38.3%</td><td>14</td><td>95,916</td><td>5,175</td><td>930</td><td>0.000</td><td>10.15</td><td>15.4</td></tr></tbody></table>
<h2>Performance vs context length</h2>
<div class="narrative"><p><em>Regenerate this section against the latest per-turn timing aggregates and the charts below.</em></p>
<p>Per-turn TTFT (first-token latency) and steady-state decode tok/s, bucketed by <strong>input-token length of that turn</strong>. We bucket per turn rather than per task because the agent loop's input grows monotonically inside a single task — buckets reveal how each provider scales prefill and decode under increasing context.</p>
<p>Buckets: 0–10k, 10–30k, 30–60k, 60–120k, 120k+ tokens. Buckets with fewer than 5 turns of data are dropped to avoid noise.</p>
<p>Read this as: a lane that holds steady across buckets has a working KV-cache / prefix-cache; a lane whose TTFT slopes up sharply is recomputing prefill on every turn.</p></div>
<h3>TTFT (seconds, lower is better)</h3><div class="chart"><img src="/benchmarks/terminal-bench-2-1/charts/ttft-by-context.svg" alt="ttft-by-context.svg"></div>
<h3>Decode tok/s (higher is better)</h3><div class="chart"><img src="/benchmarks/terminal-bench-2-1/charts/decode-by-context.svg" alt="decode-by-context.svg"></div>
<h2>Provider details</h2>
<div class="narrative"><p><em>Provider details below use public lane labels. Raw hostnames, ports, and
machine inventory keys stay in the benchmark runner configuration; the website
only publishes the runtime, model, sampling, limits, and hardware class needed
to interpret results.</em></p>
<h3>OpenRouter (cloud aggregator)</h3>
<ul>
<li><strong>Lane:</strong> <code>fiz-openrouter-qwen3-6-27b</code></li>
<li><strong>Surface:</strong> OpenAI-compatible chat-completions API, model <code>qwen/qwen3-6-27b-instruct</code>.</li>
<li><strong>Engine version:</strong> TODO — OpenRouter does not surface the underlying provider's engine in the <code>/v1/models</code> payload, and we do not log the <code>OpenRouter-Provider</code> response header that would identify which back-end pool served each request.</li>
<li><strong>Sampling defaults (sent by fiz):</strong> <code>temperature=0.6</code>, <code>top_p=0.95</code>, <code>top_k=20</code>, reasoning <code>low</code> (Qwen3 thinking-mode recommended).</li>
<li><strong>KV / prefix cache:</strong> opaque — provider-side. OpenRouter's pricing page lists cached-input as <code>$0.0</code> for this model, suggesting either no cache or no rebate. Observed <code>p50 TTFT ≈ 0.79 s</code> is consistent with a warm pool.</li>
<li><strong>Quoted limits:</strong> 128k context advertised, 32k max output, no rate limit configured in our profile.</li>
<li><strong>Cost:</strong> observed <code>≈ $0.12 / run</code> at the <code>all</code> cell median (87k in / 5.2k out tokens × $0.10/Mtok in / $0.30/Mtok out — TODO confirm against the live OpenRouter price page snapshot).</li>
</ul>
<h3>sindri-vllm (vLLM int4, local CUDA)</h3>
<ul>
<li><strong>Lane:</strong> <code>sindri-vllm</code></li>
<li><strong>Engine:</strong> vLLM, model <code>qwen3.6-27b-autoround</code> (AutoRound int4 weights). <strong>Engine version: TODO</strong>.</li>
<li><strong>Sampling (sent by fiz):</strong> <code>temperature=0.6</code>, <code>top_p=0.95</code>, <code>top_k=20</code>, reasoning <code>low</code>.</li>
<li><strong>KV / prefix cache:</strong> vLLM launches <strong>without</strong> <code>--enable-prefix-caching</code> today — observed TTFT scales sharply with input length on the per-context-bucket chart above, the symptom. Turning it on is the single biggest performance lever for this lane (see overview "Open questions").</li>
<li><strong>Hardware class:</strong> local RTX-class CUDA workstation.</li>
<li><strong>Limits:</strong> 180k context advertised in the profile, 32k max output. Real usable context is lower in int4.</li>
<li><strong>Cost:</strong> $0 cash; observed wall-time roughly 2× the OpenRouter lane on the same task set, dominated by prefill.</li>
</ul>
<h3>sindri-llamacpp (llama.cpp Q3_K_XL, local CUDA)</h3>
<ul>
<li><strong>Lane:</strong> <code>sindri-llamacpp</code></li>
<li><strong>Engine:</strong> llama.cpp, Qwen3.6-27B Q3_K_XL quantization. <strong>Engine version: TODO</strong>.</li>
<li><strong>Sampling (sent by fiz):</strong> <code>temperature=0.6</code>, <code>top_p=0.95</code>, <code>top_k=20</code>; provider-specific reasoning hints are not sent to llama.cpp.</li>
<li><strong>Hardware class:</strong> same local RTX-class CUDA workstation as <code>sindri-vllm</code>; the runtime and quantization differ.</li>
<li><strong>Limits:</strong> advertised context and max-output values come from the profile; verify effective usable context after the next full sweep.</li>
</ul>
<h3>local-vllm-rtx3090 (vLLM int4, local CUDA laptop)</h3>
<ul>
<li><strong>Lane:</strong> <code>local-vllm-rtx3090</code></li>
<li><strong>Engine:</strong> vLLM, same <code>qwen3.6-27b-autoround</code> weights as <code>sindri-vllm</code>. <strong>Engine version: TODO.</strong> Same prefix-cache caveat — not enabled.</li>
<li><strong>Sampling:</strong> identical to <code>sindri-vllm</code>.</li>
<li><strong>Hardware class:</strong> local RTX-class CUDA laptop.</li>
<li><strong>Notes:</strong> Mobile inference host; only 7 of 196 attempts produced real reps in the latest sweep — most are <code>invalid_setup</code>. Treat the row as "lane wired up" rather than "lane producing comparable data."</li>
</ul>
<h3>local-omlx-qwen3-6-27b (oMLX 8-bit, Apple silicon)</h3>
<ul>
<li><strong>Lane:</strong> <code>local-omlx-qwen3-6-27b</code></li>
<li><strong>Engine:</strong> oMLX, model <code>Qwen3.6-27B-MLX-8bit</code>. <strong>oMLX version: TODO</strong>.</li>
<li><strong>Sampling:</strong> <code>temperature=0.6</code>, <code>top_p=0.95</code>, <code>top_k=20</code>, reasoning <code>low</code>.</li>
<li><strong>KV / prefix cache:</strong> MLX-side cache behaviour is TODO — we have not confirmed whether oMLX replays full conversation context per turn. The high median input-token count on this lane is consistent with replay rather than incremental decode and is one of the open questions on the overview page.</li>
<li><strong>Hardware class:</strong> local Apple Silicon workstation with large unified memory.</li>
<li><strong>Limits:</strong> 128k context advertised, 32k max output.</li>
</ul>
<h3>local-rapidmlx-qwen3-6-27b (RapidMLX 8-bit, Apple silicon)</h3>
<ul>
<li><strong>Lane:</strong> <code>local-rapidmlx-qwen3-6-27b</code></li>
<li><strong>Engine:</strong> Rapid-MLX, model <code>mlx-community/Qwen3.6-27B-8bit</code>. <strong>Engine version: TODO.</strong></li>
<li><strong>Sampling:</strong> identical to the oMLX lane.</li>
<li><strong>Hardware class:</strong> local Apple Silicon workstation class.</li>
<li><strong>Notes:</strong> 0 real reps of 178 attempts in the latest sweep; treat as not yet producing comparable data.</li>
</ul>
<h3>local-lmstudio-qwen3-6-27b (LM Studio alternate runtime)</h3>
<ul>
<li><strong>Lane:</strong> <code>local-lmstudio-qwen3-6-27b</code></li>
<li><strong>Engine:</strong> LM Studio. <strong>Version, exact model build, sampling defaults at the server: all TODO.</strong></li>
<li><strong>Notes:</strong> 0 real reps of 375 attempts in the latest sweep — a placeholder lane. Treat as not producing comparable data until the LM Studio request pipeline is fixed.</li>
</ul></div>
</div>
