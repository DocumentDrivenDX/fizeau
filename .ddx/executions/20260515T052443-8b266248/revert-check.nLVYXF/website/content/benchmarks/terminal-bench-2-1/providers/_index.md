---
title: "Providers"
weight: 4
toc: true
---

<div class="br-body">
<div class="meta">Snapshot: 2026-05-13 15:38:48 UTC · Qwen3.6-27B across 6 provider/runtime combinations</div>
<div class="narrative">
<p>The model weights are the same across every row here — Qwen3.6-27B in some quantization. The variable is everything else: where the bytes get computed, which serving engine runs them, what sampling defaults the server applies, whether prefix-cache is hit, and how much round-trip latency the network adds.</p>
<p>Hostnames are abstracted to the substantive characteristics. The descriptive label captures engine + quantization + GPU/CPU + OS — enough to map to a known-good machine spec without leaking inventory.</p>
</div>
<h2>Pass-rate</h2>
<table><thead><tr><th>Profile / Submission</th><th>canary (3 tasks)</th><th>openai-cheap (35 tasks)</th><th>full (15 tasks)</th><th>all (89 tasks)</th><th>Provider</th></tr></thead><tbody><tr><td>local-vllm-rtx3090</td><td>33.3% <span class="meta">(1/3)</span></td><td>16.7% <span class="meta">(2/12)</span></td><td>6.7% <span class="meta">(1/15)</span></td><td>12.5% <span class="meta">(2/16)</span></td><td><span class="meta">vllm</span></td></tr><tr><td>local-lmstudio-qwen3-6-27b</td><td>0.0% <span class="meta">(0/3)</span></td><td>0.0% <span class="meta">(0/35)</span></td><td>0.0% <span class="meta">(0/15)</span></td><td>0.0% <span class="meta">(0/89)</span></td><td><span class="meta">lmstudio</span></td></tr><tr><td>fiz-openrouter-qwen3-6-27b</td><td>100.0% <span class="meta">(3/3)</span></td><td>85.7% <span class="meta">(30/35)</span></td><td>86.7% <span class="meta">(13/15)</span></td><td>61.8% <span class="meta">(55/89)</span></td><td><span class="meta">openrouter</span></td></tr><tr><td>local-rapidmlx-qwen3-6-27b</td><td>0.0% <span class="meta">(0/3)</span></td><td>8.3% <span class="meta">(1/12)</span></td><td>0.0% <span class="meta">(0/15)</span></td><td>6.2% <span class="meta">(1/16)</span></td><td><span class="meta">rapid-mlx</span></td></tr><tr><td>sindri-llamacpp</td><td>66.7% <span class="meta">(2/3)</span></td><td>51.6% <span class="meta">(16/31)</span></td><td>40.0% <span class="meta">(6/15)</span></td><td>32.0% <span class="meta">(24/75)</span></td><td><span class="meta">llama-server</span></td></tr><tr><td>local-omlx-qwen3-6-27b</td><td>100.0% <span class="meta">(3/3)</span></td><td>61.8% <span class="meta">(21/34)</span></td><td>73.3% <span class="meta">(11/15)</span></td><td>38.3% <span class="meta">(31/81)</span></td><td><span class="meta">omlx</span></td></tr></tbody></table>
<h2>Detailed metrics</h2>
<table><thead><tr><th>Profile</th><th>Harness</th><th>Attempts</th><th>Real</th><th>pass@1</th><th>pass@k</th><th>med turns</th><th>med in</th><th>med out</th><th>med wall (s)</th><th>cost ($)</th><th>p50 TTFT (s)</th><th>p50 decode (tok/s)</th></tr></thead><tbody><tr><td>vLLM int4 / NVIDIA GeForce RTX 5090 Laptop GPU (24 GB) / Ubuntu 24.04.4 LTS (Noble Numbat) on WSL2 / Windows 11 host</td><td><span class="meta">fiz (built-in agent loop)</span></td><td>88</td><td>7</td><td>2.9%</td><td>12.5%</td><td>2</td><td>3,049</td><td>1,073</td><td>90</td><td>0.000</td><td>30.01</td><td>89.4</td></tr><tr><td>lmstudio / NVIDIA GeForce RTX 5090 Laptop GPU (24 GB) / Ubuntu 24.04.4 LTS (Noble Numbat) on WSL2 / Windows 11 host</td><td><span class="meta">fiz (built-in agent loop)</span></td><td>267</td><td>0</td><td>0.0%</td><td>0.0%</td><td>—</td><td>—</td><td>—</td><td>—</td><td>0.000</td><td>—</td><td>—</td></tr><tr><td>OpenRouter (cloud aggregator)</td><td><span class="meta">fiz (built-in agent loop)</span></td><td>334</td><td>315</td><td>64.5%</td><td>61.8%</td><td>15</td><td>98,989</td><td>5,948</td><td>569</td><td>0.143</td><td>0.89</td><td>46.8</td></tr><tr><td>RapidMLX 8-bit / Apple M1 Max (64 GB unified)</td><td><span class="meta">fiz (built-in agent loop)</span></td><td>70</td><td>0</td><td>3.6%</td><td>6.2%</td><td>—</td><td>—</td><td>—</td><td>—</td><td>0.000</td><td>30.02</td><td>15.7</td></tr><tr><td>llama-server / NVIDIA RTX 3090 Ti (24 GB) / Ubuntu 24.04.4 LTS (Noble Numbat) on WSL2</td><td><span class="meta">fiz (built-in agent loop)</span></td><td>75</td><td>67</td><td>32.4%</td><td>32.0%</td><td>13</td><td>85,260</td><td>3,361</td><td>487</td><td>0.000</td><td>1.96</td><td>18.2</td></tr><tr><td>oMLX 8-bit / Apple M2 Ultra (24-core CPU) (192 GB unified)</td><td><span class="meta">fiz (built-in agent loop)</span></td><td>179</td><td>109</td><td>38.8%</td><td>38.3%</td><td>14</td><td>95,916</td><td>5,175</td><td>930</td><td>0.000</td><td>10.15</td><td>15.4</td></tr></tbody></table>
<h2>Performance vs context length</h2>
<div class="narrative"><p>Per-turn TTFT (first-token latency) and steady-state decode tok/s, bucketed by <strong>input-token length of that turn</strong>. We bucket per turn rather than per task because the agent loop's input grows monotonically inside a single task — buckets reveal how each provider scales prefill and decode under increasing context.</p>
<p>Buckets: 0–10k, 10–30k, 30–60k, 60–120k, 120k+ tokens. Buckets with fewer than 5 turns of data are dropped to avoid noise.</p>
<p>Read this as: a lane that holds steady across buckets has a working KV-cache / prefix-cache; a lane whose TTFT slopes up sharply is recomputing prefill on every turn.</p></div>
<h3>TTFT (seconds, lower is better)</h3><div class="chart"><img src="/benchmarks/terminal-bench-2-1/charts/ttft-by-context.svg" alt="ttft-by-context.svg"></div>
<h3>Decode tok/s (higher is better)</h3><div class="chart"><img src="/benchmarks/terminal-bench-2-1/charts/decode-by-context.svg" alt="decode-by-context.svg"></div>
<h2>Provider details</h2>
<div class="narrative"><p>Provider details below use public lane labels. They publish enough information to interpret the benchmark results: provider surface, runtime family, model or quantization, sampling policy, context limits, and broad hardware class for self-hosted lanes.</p>
<h3>OpenRouter Qwen3.6-27B</h3>
<ul>
<li><strong>Lane:</strong> <code>fiz-openrouter-qwen3-6-27b</code></li>
<li><strong>Surface:</strong> managed OpenAI-compatible API through OpenRouter.</li>
<li><strong>Model:</strong> <code>qwen/qwen3-6-27b-instruct</code>.</li>
<li><strong>Sampling:</strong> <code>temperature=0.6</code>, <code>top_p=0.95</code>, <code>top_k=20</code>, reasoning <code>low</code>.</li>
<li><strong>Limits:</strong> 128k advertised context, 32k max output.</li>
<li><strong>Cost profile:</strong> low cash cost per run; current medians put it near the budget end of the hosted lanes.</li>
<li><strong>Notes:</strong> provider-side routing and caching are opaque, so this lane is best read as the managed throughput reference for Qwen3.6-27B.</li>
</ul>
<h3>sindri-vllm</h3>
<ul>
<li><strong>Lane:</strong> <code>sindri-vllm</code></li>
<li><strong>Surface:</strong> self-hosted vLLM on a local RTX-class CUDA workstation.</li>
<li><strong>Model:</strong> Qwen3.6-27B AutoRound int4.</li>
<li><strong>Sampling:</strong> <code>temperature=0.6</code>, <code>top_p=0.95</code>, <code>top_k=20</code>, reasoning <code>low</code>.</li>
<li><strong>Limits:</strong> 180k advertised context, 32k max output; effective usable context depends on runtime memory pressure.</li>
<li><strong>Notes:</strong> decode throughput is strong, but TTFT rises with long context. Prefix caching is the main performance lever for this lane.</li>
</ul>
<h3>sindri-llamacpp</h3>
<ul>
<li><strong>Lane:</strong> <code>sindri-llamacpp</code></li>
<li><strong>Surface:</strong> self-hosted llama.cpp on the same local CUDA hardware class as <code>sindri-vllm</code>.</li>
<li><strong>Model:</strong> Qwen3.6-27B Q3_K_XL quantization.</li>
<li><strong>Sampling:</strong> <code>temperature=0.6</code>, <code>top_p=0.95</code>, <code>top_k=20</code>; provider-specific reasoning hints are not sent to llama.cpp.</li>
<li><strong>Notes:</strong> this lane isolates runtime and quantization differences against the same broad hardware class as the vLLM lane.</li>
</ul>
<h3>local-vllm-rtx3090</h3>
<ul>
<li><strong>Lane:</strong> <code>local-vllm-rtx3090</code></li>
<li><strong>Surface:</strong> self-hosted vLLM on a mobile RTX-class CUDA system.</li>
<li><strong>Model:</strong> Qwen3.6-27B AutoRound int4.</li>
<li><strong>Sampling:</strong> same as <code>sindri-vllm</code>.</li>
<li><strong>Notes:</strong> this lane is wired up but not yet producing enough real reps for a comparable benchmark read.</li>
</ul>
<h3>local-omlx-qwen3-6-27b</h3>
<ul>
<li><strong>Lane:</strong> <code>local-omlx-qwen3-6-27b</code></li>
<li><strong>Surface:</strong> self-hosted oMLX on an Apple silicon workstation class.</li>
<li><strong>Model:</strong> Qwen3.6-27B MLX 8-bit.</li>
<li><strong>Sampling:</strong> <code>temperature=0.6</code>, <code>top_p=0.95</code>, <code>top_k=20</code>, reasoning <code>low</code>.</li>
<li><strong>Limits:</strong> 128k advertised context, 32k max output.</li>
<li><strong>Notes:</strong> current results show slower TTFT and decode than the CUDA lanes at this model size.</li>
</ul>
<h3>local-rapidmlx-qwen3-6-27b</h3>
<ul>
<li><strong>Lane:</strong> <code>local-rapidmlx-qwen3-6-27b</code></li>
<li><strong>Surface:</strong> self-hosted Rapid-MLX on an Apple silicon workstation class.</li>
<li><strong>Model:</strong> Qwen3.6-27B MLX 8-bit.</li>
<li><strong>Sampling:</strong> same as the oMLX lane.</li>
<li><strong>Notes:</strong> this lane is not yet producing comparable real reps.</li>
</ul>
<h3>local-lmstudio-qwen3-6-27b</h3>
<ul>
<li><strong>Lane:</strong> <code>local-lmstudio-qwen3-6-27b</code></li>
<li><strong>Surface:</strong> LM Studio alternate runtime.</li>
<li><strong>Model:</strong> Qwen3.6-27B class local model.</li>
<li><strong>Notes:</strong> this lane is a placeholder until it produces real reps.</li>
</ul></div>
</div>
