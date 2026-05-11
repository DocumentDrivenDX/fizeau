---
title: "Providers"
weight: 4
toc: true
---

<div class="br-body">
<div class="meta">Snapshot: 2026-05-11 15:37:18 UTC · Qwen3.6-27B across 7 provider/runtime combinations</div>
<div class="narrative">
<p>The model weights are the same across every row here — Qwen3.6-27B in some quantization. The variable is everything else: where the bytes get computed, which serving engine runs them, what sampling defaults the server applies, whether prefix-cache is hit, and how much round-trip latency the network adds.</p>
<p>Hostnames are abstracted to the substantive characteristics. The descriptive label captures engine + quantization + GPU/CPU + OS — enough to map to a known-good machine spec without leaking inventory.</p>
</div>
<h2>Pass-rate</h2>
<table><thead><tr><th>Profile / Submission</th><th>canary (3 tasks)</th><th>openai-cheap (35 tasks)</th><th>full (15 tasks)</th><th>all (89 tasks)</th><th>Provider</th></tr></thead><tbody><tr><td>bragi-club-3090</td><td>33.3% <span class="meta">(1/3)</span></td><td>16.7% <span class="meta">(2/12)</span></td><td>6.7% <span class="meta">(1/15)</span></td><td>12.5% <span class="meta">(2/16)</span></td><td><span class="meta">vllm</span></td></tr><tr><td>bragi-qwen3-6-27b</td><td>0.0% <span class="meta">(0/3)</span></td><td>0.0% <span class="meta">(0/35)</span></td><td>0.0% <span class="meta">(0/15)</span></td><td>0.0% <span class="meta">(0/89)</span></td><td><span class="meta">lmstudio</span></td></tr><tr><td>fiz-openrouter-qwen3-6-27b</td><td>100.0% <span class="meta">(3/3)</span></td><td>90.6% <span class="meta">(29/32)</span></td><td>85.7% <span class="meta">(12/14)</span></td><td>75.4% <span class="meta">(52/69)</span></td><td><span class="meta">openrouter</span></td></tr><tr><td>grendel-rapid-mlx</td><td>0.0% <span class="meta">(0/3)</span></td><td>8.3% <span class="meta">(1/12)</span></td><td>0.0% <span class="meta">(0/15)</span></td><td>6.2% <span class="meta">(1/16)</span></td><td><span class="meta">rapid-mlx</span></td></tr><tr><td>sindri-club-3090</td><td>100.0% <span class="meta">(3/3)</span></td><td>69.7% <span class="meta">(23/33)</span></td><td>66.7% <span class="meta">(10/15)</span></td><td>34.1% <span class="meta">(28/82)</span></td><td><span class="meta">vllm</span></td></tr><tr><td>sindri-club-3090-llamacpp</td><td>66.7% <span class="meta">(2/3)</span></td><td>70.0% <span class="meta">(7/10)</span></td><td>66.7% <span class="meta">(4/6)</span></td><td>70.6% <span class="meta">(12/17)</span></td><td><span class="meta">llama-server</span></td></tr><tr><td>vidar-qwen3-6-27b</td><td>100.0% <span class="meta">(3/3)</span></td><td>60.0% <span class="meta">(21/35)</span></td><td>73.3% <span class="meta">(11/15)</span></td><td>37.8% <span class="meta">(31/82)</span></td><td><span class="meta">omlx</span></td></tr></tbody></table>
<h2>Detailed metrics</h2>
<table><thead><tr><th>Profile</th><th>Harness</th><th>Attempts</th><th>Real</th><th>pass@1</th><th>pass@k</th><th>med turns</th><th>med in</th><th>med out</th><th>med wall (s)</th><th>cost ($)</th><th>p50 TTFT (s)</th><th>p50 decode (tok/s)</th></tr></thead><tbody><tr><td>vLLM int4 / NVIDIA RTX 5090 24 GB (Laptop) / Windows 11 + WSL2</td><td><span class="meta">fiz (built-in agent loop)</span></td><td>196</td><td>7</td><td>2.9%</td><td>6.5%</td><td>2</td><td>3,049</td><td>1,073</td><td>90</td><td>0.000</td><td>30.01</td><td>89.4</td></tr><tr><td>lmstudio / NVIDIA RTX 5090 24 GB (Laptop) / Windows 11 + WSL2</td><td><span class="meta">fiz (built-in agent loop)</span></td><td>375</td><td>0</td><td>0.0%</td><td>0.0%</td><td>—</td><td>—</td><td>—</td><td>—</td><td>0.000</td><td>—</td><td>—</td></tr><tr><td>OpenRouter (cloud aggregator)</td><td><span class="meta">fiz (built-in agent loop)</span></td><td>204</td><td>169</td><td>87.3%</td><td>75.4%</td><td>16</td><td>96,239</td><td>4,493</td><td>297</td><td>0.117</td><td>0.82</td><td>55.8</td></tr><tr><td>RapidMLX 8-bit</td><td><span class="meta">fiz (built-in agent loop)</span></td><td>178</td><td>0</td><td>3.6%</td><td>3.2%</td><td>—</td><td>—</td><td>—</td><td>—</td><td>0.000</td><td>30.02</td><td>15.7</td></tr><tr><td>vLLM int4 / NVIDIA RTX 5090 Ti / Windows + WSL2</td><td><span class="meta">fiz (built-in agent loop)</span></td><td>305</td><td>104</td><td>28.7%</td><td>28.9%</td><td>11</td><td>62,940</td><td>3,623</td><td>845</td><td>0.000</td><td>18.98</td><td>66.3</td></tr><tr><td>llama-server / NVIDIA RTX 5090 Ti / Windows + WSL2</td><td><span class="meta">fiz (built-in agent loop)</span></td><td>17</td><td>16</td><td>70.6%</td><td>70.6%</td><td>27</td><td>272,981</td><td>3,936</td><td>436</td><td>0.000</td><td>2.44</td><td>21.2</td></tr><tr><td>oMLX 8-bit / Apple M2 Ultra (192 GB unified)</td><td><span class="meta">fiz (built-in agent loop)</span></td><td>292</td><td>109</td><td>37.7%</td><td>32.0%</td><td>14</td><td>95,916</td><td>5,175</td><td>930</td><td>0.000</td><td>10.11</td><td>15.4</td></tr></tbody></table>
<h2>Performance vs context length</h2>
<div class="narrative"><p><em>Regenerate this section against the latest data — see <code>data/timing.json</code> and the charts below.</em></p>
<p>Per-turn TTFT (first-token latency) and steady-state decode tok/s, bucketed by <strong>input-token length of that turn</strong>. We bucket per turn rather than per task because the agent loop's input grows monotonically inside a single task — buckets reveal how each provider scales prefill and decode under increasing context.</p>
<p>Buckets: 0–10k, 10–30k, 30–60k, 60–120k, 120k+ tokens. Buckets with fewer than 5 turns of data are dropped to avoid noise.</p>
<p>Read this as: a lane that holds steady across buckets has a working KV-cache / prefix-cache; a lane whose TTFT slopes up sharply is recomputing prefill on every turn.</p></div>
<h3>TTFT (seconds, lower is better)</h3><div class="chart"><img src="/benchmarks/terminal-bench-2-1/charts/ttft-by-context.svg" alt="ttft-by-context.svg"></div>
<h3>Decode tok/s (higher is better)</h3><div class="chart"><img src="/benchmarks/terminal-bench-2-1/charts/decode-by-context.svg" alt="decode-by-context.svg"></div>
<h2>Provider details</h2>
<div class="narrative"><p><em>Provider details below come from <code>scripts/benchmark/profiles/*.yaml</code> (sampling defaults, advertised limits, runtime label) and <code>scripts/benchmark/machines.yaml</code> (machine spec for self-hosted lanes). Fields marked <strong>TODO</strong> are not captured anywhere in the registry; they are flagged here so they stand out when <code>scripts/benchmark/capture-machine-info.sh</code> lands and starts populating real values.</em></p>
<h3>OpenRouter (cloud aggregator)</h3>
<ul>
<li><strong>Profile id:</strong> <code>fiz-openrouter-qwen3-6-27b</code></li>
<li><strong>Endpoint:</strong> <code>https://openrouter.ai/api/v1</code> — OpenAI-compatible chat-completions surface, model <code>qwen/qwen3-6-27b-instruct</code>.</li>
<li><strong>Engine version:</strong> TODO — OpenRouter does not surface the underlying provider's engine in the <code>/v1/models</code> payload, and we do not log the <code>OpenRouter-Provider</code> response header that would identify which back-end pool served each request.</li>
<li><strong>Sampling defaults (sent by fiz):</strong> <code>temperature=0.6</code>, <code>top_p=0.95</code>, <code>top_k=20</code>, reasoning <code>low</code> (Qwen3 thinking-mode recommended).</li>
<li><strong>KV / prefix cache:</strong> opaque — provider-side. OpenRouter's pricing page lists cached-input as <code>$0.0</code> for this model, suggesting either no cache or no rebate. Observed <code>p50 TTFT ≈ 0.79 s</code> is consistent with a warm pool.</li>
<li><strong>Quoted limits:</strong> 128k context advertised, 32k max output, no rate limit configured in our profile.</li>
<li><strong>Cost:</strong> observed <code>≈ $0.12 / run</code> at the <code>all</code> cell median (87k in / 5.2k out tokens × $0.10/Mtok in / $0.30/Mtok out — TODO confirm against the live OpenRouter price page snapshot).</li>
</ul>
<h3>Sindri (vLLM int4, RTX 5090 Ti)</h3>
<ul>
<li><strong>Profile id:</strong> <code>sindri-club-3090</code></li>
<li><strong>Endpoint:</strong> <code>http://sindri:8020/v1</code> (Tailscale).</li>
<li><strong>Engine:</strong> vLLM, model <code>qwen3.6-27b-autoround</code> (AutoRound int4 weights). <strong>Engine version: TODO</strong> — capture from <code>vllm --version</code> on the host; not in <code>machines.yaml</code>.</li>
<li><strong>Sampling (sent by fiz):</strong> <code>temperature=0.6</code>, <code>top_p=0.95</code>, <code>top_k=20</code>, reasoning <code>low</code>.</li>
<li><strong>KV / prefix cache:</strong> vLLM launches <strong>without</strong> <code>--enable-prefix-caching</code> today — observed TTFT scales sharply with input length on the per-context-bucket chart above, the symptom. Turning it on is the single biggest performance lever for this lane (see overview "Open questions").</li>
<li><strong>Hardware (<code>machines.yaml#sindri</code>):</strong> Custom desktop · AMD Ryzen 9 5950X · NVIDIA RTX 5090 Ti · Windows + WSL2. Memory <strong>TODO</strong>.</li>
<li><strong>Limits:</strong> 180k context advertised in the profile, 32k max output. Real usable context is lower in int4.</li>
<li><strong>Cost:</strong> $0 cash; observed wall-time roughly 2× the OpenRouter lane on the same task set, dominated by prefill.</li>
</ul>
<h3>Bragi (vLLM int4, RTX 5090 24 GB Laptop)</h3>
<ul>
<li><strong>Profile id:</strong> <code>bragi-club-3090</code></li>
<li><strong>Endpoint:</strong> <code>http://bragi:8020/v1</code> (Tailscale).</li>
<li><strong>Engine:</strong> vLLM, same <code>qwen3.6-27b-autoround</code> weights as sindri. <strong>Engine version: TODO.</strong> Same prefix-cache caveat as sindri — not enabled.</li>
<li><strong>Sampling:</strong> identical to sindri.</li>
<li><strong>Hardware (<code>machines.yaml#bragi</code>):</strong> Lenovo Legion 7i Pro laptop · NVIDIA RTX 5090 24 GB (Laptop) · Windows 11 + WSL2. CPU and memory <strong>TODO</strong>.</li>
<li><strong>Notes:</strong> Mobile inference host; only 7 of 196 attempts produced real reps in the latest sweep — most are <code>invalid_setup</code>. Treat the row as "lane wired up" rather than "lane producing comparable data."</li>
</ul>
<h3>Vidar (oMLX 8-bit, Apple M2 Ultra)</h3>
<ul>
<li><strong>Profile id:</strong> <code>vidar-qwen3-6-27b</code></li>
<li><strong>Endpoint:</strong> <code>http://vidar:1235/v1</code> (Tailscale).</li>
<li><strong>Engine:</strong> oMLX, model <code>Qwen3.6-27B-MLX-8bit</code>. <strong>oMLX version: TODO</strong> — not captured.</li>
<li><strong>Sampling:</strong> <code>temperature=0.6</code>, <code>top_p=0.95</code>, <code>top_k=20</code>, reasoning <code>low</code>.</li>
<li><strong>KV / prefix cache:</strong> MLX-side cache behaviour is TODO — we have not confirmed whether oMLX replays full conversation context per turn. The high median input-token count on this lane (104k vs sindri's 87k on the same task subset) is consistent with replay rather than incremental decode and is one of the open questions on the overview page.</li>
<li><strong>Hardware (<code>machines.yaml#vidar</code>):</strong> Apple Mac Studio · Apple M2 Ultra · 192 GB unified memory · macOS. Specific macOS version <strong>TODO</strong>.</li>
<li><strong>Limits:</strong> 128k context advertised, 32k max output.</li>
</ul>
<h3>Grendel (RapidMLX 8-bit, Apple silicon)</h3>
<ul>
<li><strong>Profile id:</strong> <code>grendel-rapid-mlx</code></li>
<li><strong>Endpoint:</strong> <code>http://grendel:8000/v1</code> (Tailscale).</li>
<li><strong>Engine:</strong> Rapid-MLX, model <code>mlx-community/Qwen3.6-27B-8bit</code>. <strong>Engine version: TODO.</strong></li>
<li><strong>Sampling:</strong> identical to vidar.</li>
<li><strong>Hardware (<code>machines.yaml#grendel</code>):</strong> chassis, CPU, GPU and memory all <strong>TODO</strong> — <code>machines.yaml</code> notes only "Apple Silicon RapidMLX backend at :8000 (full hardware spec TBD)". High priority for <code>capture-machine-info.sh</code>.</li>
<li><strong>Notes:</strong> 0 real reps of 178 attempts in the latest sweep; treat as not yet producing comparable data.</li>
</ul>
<h3>lmstudio (Bragi alternate runtime)</h3>
<ul>
<li><strong>Profile id:</strong> <code>bragi-qwen3-6-27b</code></li>
<li><strong>Endpoint:</strong> alternate port on the same Bragi host (<code>:1234</code> per machine notes).</li>
<li><strong>Engine:</strong> LM Studio. <strong>Version, exact model build, sampling defaults at the server: all TODO.</strong></li>
<li><strong>Notes:</strong> 0 real reps of 375 attempts in the latest sweep — a placeholder lane. Treat as not producing comparable data until the LM Studio request pipeline is fixed.</li>
</ul>
<h3>TODO checklist for <code>capture-machine-info.sh</code></h3>
<p>When the capture script lands, the fields that should populate (and replace the <strong>TODO</strong> markers above) are:</p>
<ul>
<li><code>vllm --version</code> / <code>omlx --version</code> / <code>rapid-mlx --version</code> per host.</li>
<li><code>nvidia-smi --query-gpu=name,memory.total,driver_version --format=csv</code> for CUDA hosts.</li>
<li><code>system_profiler SPHardwareDataType</code> (or <code>sysctl hw.memsize</code>) for Apple silicon hosts.</li>
<li>vLLM launch flags (<code>--enable-prefix-caching</code>, <code>--max-model-len</code>, <code>--gpu-memory-utilization</code>) so prefix-cache state is captured rather than inferred.</li>
<li>Server-side sampling defaults (temperature/top_p/top_k applied when the client omits them) — today we only know the client-side values fiz sends.</li>
</ul></div>
</div>
