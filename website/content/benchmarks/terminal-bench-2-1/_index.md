---
title: "Terminal-Bench 2.1"
weight: 1
toc: true
---

<div class="br-body">
<div class="meta">Snapshot: 2026-05-13 15:38:48 UTC · 2,717 trial reports · 23 active profiles</div>
<h2>How we run it</h2>
<div class="narrative"><p><a href="https://terminal-bench.dev/">Terminal-Bench</a> 2.1 is a public coding-agent benchmark of 89 long-form tasks. Each task ships a prompt, an isolated Docker environment, and a deterministic verifier. An agent reads the prompt, runs shell commands, edits files inside the container, and is scored against the resulting state.</p>
<p>Each Fizeau profile runs through <a href="https://github.com/laude-institute/harbor">Harbor</a> 0.3.x's installed-agent path. Harbor installs the agent runtime in the task container, runs the attempt, and then invokes the verifier separately. Profile configuration selects the provider, model, runtime, and harness without publishing private service locations. Each task runs five reps per profile; pass@1 is the per-rep success rate, and pass@k reports whether any of the five reps solved the task.</p>
<p>We slice the 89-task set into nested benchmarks of decreasing scope. The subset YAMLs are under <code>scripts/benchmark/task-subset-tb21-*.yaml</code>:</p></div>
<table><thead><tr><th>Subset</th><th>Tasks</th><th>Selection rule</th></tr></thead><tbody><tr><td>canary</td><td>3</td><td>3-5 task canary covering SE, data-processing, and system-administration; one task per category; deterministic sort by difficulty desc then id asc</td></tr><tr><td>openai-cheap</td><td>35</td><td>observed native OpenAI GPT-5.5 average cost &lt;= ~$0.90 per run where available; otherwise OpenRouter Qwen3.6 27B token count projected at GPT-5.5 pricing &lt;= ~$1.00 per run; exclude known multi-dollar cells</td></tr><tr><td>full</td><td>15</td><td>filtered TB-2.1 tasks with fixed category quotas SE=5 security=3 file-ops=2 sysadmin=2 data-processing=2 debugging=1; difficulty-desc then id-asc</td></tr><tr><td>all</td><td>89</td><td>all 89 tasks from the Harbor terminal-bench/terminal-bench-2-1 task catalog</td></tr></tbody></table>
<h2>Three perspectives on the same data</h2>
<div class="narrative">
<p>The trial set runs each task many ways. Each of the three sub-pages slices the data along a different axis:</p>
<ul>
<li><a href="models/"><b>Models</b></a> — fiz with its built-in agent loop across multiple models, on the cheap subset where cost lets us run real reps.</li>
<li><a href="harnesses/"><b>Harnesses</b></a> — same model, different agent loop. Includes external leaderboard rows for the same models in other harnesses.</li>
<li><a href="providers/"><b>Providers</b></a> — Qwen3.6-27B held constant, varying the host (cloud aggregator vs local CUDA vs Apple silicon). The harness/runtime story.</li>
</ul>
</div>
<h2>Headline observations</h2>
<div class="narrative"><h3>Qwen3.6-27B across providers (the headline question)</h3>
<p>OpenRouter Qwen3.6-27B is the throughput reference. The local profiles bottleneck elsewhere:</p>
<ul>
<li><strong>sindri-vllm (vLLM int4 on local CUDA)</strong>: best decode rate, worst prefill. On agent loops with 50–150k context per turn, prefill dominates wall — explaining why the median wall is roughly 2× OpenRouter despite faster decode.</li>
<li><strong>local-omlx-qwen3-6-27b (oMLX 8-bit on Apple silicon)</strong>: slow on both axes. MLX 8-bit at this model size is the rate limiter; only smaller quantization or a different runtime will move it.</li>
</ul>
<h3>Model-power signal vs harness loss</h3>
<p>The scatter in section 6 mostly tracks the expected pattern: frontier-power models (Opus, GPT-5.5) sit at higher pass-rates than Qwen-class models. Several Qwen profiles still sit below the trend, which points to harness/runtime loss in addition to model capability.</p>
<h3>Cost / reliability frontier</h3>
<p>OpenRouter Qwen3.6-27B costs cash per run; local profiles cost $0 in cash but cost in wall-time and reliability. For pure budget, OR Qwen wins; for ceiling-pass tasks where reliability matters, the frontier rows on the leaderboard remain ahead of any Qwen profile regardless of plumbing.</p>
<h3>Open questions</h3>
<ul>
<li>The oMLX profile's input-token median runs higher than the vLLM profile on the same task set. Either the MLX server replays full conversation context where vLLM compacts, or the agent loop runs more turns before the model converges. Worth a focused trace.</li>
<li><code>sindri-vllm</code> prefill latency is the single biggest performance lever: enabling vLLM <code>--enable-prefix-caching</code> (or boosting cache hit rate) should drop TTFT 5–10× and close most of the wall-time gap.</li>
</ul></div>
<h2>Method notes</h2>
<div class="narrative"><ul>
<li><strong>pass@1</strong> = (graded reps with reward &gt; 0) / (total graded reps). <strong>pass@k</strong> = unique tasks where any rep solved / unique tasks attempted. With reps=5 we do not report best-of-N because the reps are deliberately identical.</li>
<li><strong>Real runs</strong> = trials with <code>turns &gt; 0</code> AND any tokens flowed. Filters out <code>invalid_setup</code>, network, container-startup, and zero-turn timeouts so per-trial medians (turns, tokens, wall) reflect actual model interaction.</li>
<li><strong>TTFT</strong> = (first <code>llm.delta</code> event ts) − (matching <code>llm.request</code> ts) per turn, in seconds.</li>
<li><strong>Decode tok/s</strong> = <code>output_tokens / (response.ts − first_delta.ts)</code> per turn — post-prefill generation rate.</li>
<li>Both timing metrics report as <strong>median-of-per-task-medians</strong> to dampen rep variance and outlier turns. Per-bucket timing requires ≥5 turns in the bucket to plot.</li>
<li>Provider-side latency (TTFT including queue and prefill) and pure decode stay separate so wall-time can be attributed to prefill vs generation.</li>
<li>External leaderboard data is the count of <code>reward.txt</code> files per submission per task on <code>harborframework/terminal-bench-2-leaderboard</code> on Hugging Face. We report <code>tasks_passed / tasks_attempted</code> rather than per-rep pass@1 because the leaderboard does not expose per-rep granularity uniformly.</li>
</ul></div>
</div>
