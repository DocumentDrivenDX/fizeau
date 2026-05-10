---
title: "Terminal-Bench 2.1"
weight: 1
toc: true
---

<div class="br-body">
<div class="meta">Snapshot: 2026-05-10 21:20:28 UTC · 3,652 trial reports · 20 active lanes</div>
<h2>How we run it</h2>
<div class="narrative"><p><a href="https://terminal-bench.dev/">Terminal-Bench</a> 2.1 is a public coding-agent benchmark of 89 long-form tasks. Each task ships a prompt, an isolated Docker container with the test environment, and a deterministic verifier. An agent reads the prompt, runs shell commands and edits files inside the container, then the verifier scores the resulting state. We use the arm64 preflight image of the dataset (commit <code>harbor-registry</code>).</p>
<p>Each lane runs through <a href="https://github.com/laude-institute/harbor">Harbor</a> 0.3.x's <code>BaseInstalledAgent</code> path. Harbor stages our agent runtime tarball into the task image, runs the agent inside the task's container with bind-mounted log directories, then runs the verifier separately. Our agent adapter (<code>scripts/benchmark/harbor_agent.py</code>) launches <code>fiz</code> with provider/model wired via per-lane env vars (<code>FIZEAU_PROVIDER</code>, <code>FIZEAU_BASE_URL</code>, <code>FIZEAU_MODEL</code>, …). Each task runs with <code>--reps 5</code> per lane; pass@1 (per-rep success rate) and pass@k (any-rep solve rate, for k=5 reps) are reported separately.</p>
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
<div class="narrative"><p><em>Editorial summary. Regenerate against the data in <code>data/aggregates.json</code> and <code>data/timing.json</code>. The numbers here go stale as more cells land — re-run <code>scripts/benchmark/generate-report.py</code> and refresh this section.</em></p>
<h3>Qwen3.6-27B across providers (the headline question)</h3>
<p>OpenRouter Qwen3.6-27B is the throughput reference. The local lanes bottleneck elsewhere:</p>
<ul>
<li><strong>sindri (vLLM int4 on a 3090)</strong>: best decode rate, worst prefill. On agent loops with 50–150k context per turn, prefill dominates wall — explaining why the median wall is roughly 2× OpenRouter despite faster decode.</li>
<li><strong>vidar (oMLX 8-bit on Apple silicon)</strong>: slow on both axes. MLX 8-bit at this model size is the rate limiter; only smaller quantization or a different runtime will move it.</li>
</ul>
<h3>Model-power signal vs harness loss</h3>
<p>The scatter in §6 mostly tracks the expected pattern — frontier-power models (Opus, GPT-5.5) sit at higher pass-rates than Qwen-class — but several Qwen lanes show distance below the trend that maps to harness loss, not model loss. The recently-fixed JSONL-bind-mount bug (commit <code>18a19a43</code>) closes one well-understood class of those.</p>
<h3>Cost / reliability frontier</h3>
<p>OpenRouter Qwen3.6-27B costs cash per run; sindri and vidar cost $0 in cash but cost in wall-time and reliability. For pure budget, OR Qwen wins; for ceiling-pass tasks where reliability matters, the frontier rows on the leaderboard remain ahead of any Qwen lane regardless of plumbing.</p>
<h3>Open questions</h3>
<ul>
<li>Vidar's input-token median runs higher than sindri on the same task set. Either the MLX server replays full conversation context where vLLM compacts, or the agent loop runs more turns on vidar before the model converges. Worth a focused trace.</li>
<li>Sindri's prefill latency is the single biggest performance lever: enabling vLLM <code>--enable-prefix-caching</code> (or boosting cache hit rate) should drop TTFT 5–10× and close most of the wall-time gap.</li>
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
</ul>
<h3>Regenerating the report</h3>
<pre><code class="language-sh"># full rebuild (data + charts + HTML)
.venv-report/bin/python scripts/benchmark/generate-report.py

# data only — useful before editing the narrative markdown:
.venv-report/bin/python scripts/benchmark/generate-report.py --emit-data-only

# refresh external leaderboard from Hugging Face:
.venv-report/bin/python scripts/benchmark/generate-report.py --refresh-leaderboard
</code></pre>
<p>The script reads from <code>benchmark-results/fiz-tools-v1/cells/</code> and writes to <code>docs/benchmarks/</code>. Narrative sections (<code>docs/benchmarks/sections/*.md</code>) are read at render time and are the only place to edit prose — do not edit the generated HTML directly.</p></div>
</div>
