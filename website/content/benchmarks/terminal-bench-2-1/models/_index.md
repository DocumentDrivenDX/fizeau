---
title: "Models"
weight: 2
toc: true
---

<div class="br-body">
<div class="meta">Snapshot: 2026-05-11 15:37:18 UTC · 10 model lanes shown</div>
<div class="narrative">
<p>Each row is fiz running its own built-in agent loop against a different model. Where possible we report on the <code>openai-cheap</code> subset (35 tasks) so the cost gate doesn't bias the model selection — frontier hosted models are typically too expensive to run with k=5 reps across all 89 TB-2.1 tasks.</p>
</div>
<h2>Pass-rate</h2>
<table><thead><tr><th>Profile / Submission</th><th>canary (3 tasks)</th><th>openai-cheap (35 tasks)</th><th>full (15 tasks)</th><th>all (89 tasks)</th><th>Provider</th></tr></thead><tbody><tr><td>claude-native-sonnet-4-6</td><td>0.0% <span class="meta">(0/1)</span></td><td>0.0% <span class="meta">(0/3)</span></td><td>0.0% <span class="meta">(0/2)</span></td><td>0.0% <span class="meta">(0/3)</span></td><td><span class="meta">anthropic</span></td></tr><tr><td>claude-sonnet-4-6</td><td>33.3% <span class="meta">(1/3)</span></td><td>8.6% <span class="meta">(3/35)</span></td><td>13.3% <span class="meta">(2/15)</span></td><td>3.4% <span class="meta">(3/89)</span></td><td><span class="meta">openrouter</span></td></tr><tr><td>codex-native-gpt-5-4-mini</td><td>100.0% <span class="meta">(1/1)</span></td><td>100.0% <span class="meta">(3/3)</span></td><td>100.0% <span class="meta">(2/2)</span></td><td>100.0% <span class="meta">(3/3)</span></td><td><span class="meta">openai</span></td></tr><tr><td>fiz-openai-gpt-5-5</td><td>100.0% <span class="meta">(3/3)</span></td><td>42.9% <span class="meta">(15/35)</span></td><td>100.0% <span class="meta">(15/15)</span></td><td>24.7% <span class="meta">(22/89)</span></td><td><span class="meta">openai</span></td></tr><tr><td>fiz-openrouter-claude-sonnet-4-6</td><td>100.0% <span class="meta">(3/3)</span></td><td>27.3% <span class="meta">(3/11)</span></td><td>20.0% <span class="meta">(3/15)</span></td><td>20.0% <span class="meta">(3/15)</span></td><td><span class="meta">openrouter</span></td></tr><tr><td>fiz-openrouter-gpt-5-4-mini</td><td>66.7% <span class="meta">(2/3)</span></td><td>18.2% <span class="meta">(2/11)</span></td><td>13.3% <span class="meta">(2/15)</span></td><td>13.3% <span class="meta">(2/15)</span></td><td><span class="meta">openrouter</span></td></tr><tr><td>gpt-5-3-mini</td><td>0.0% <span class="meta">(0/1)</span></td><td>0.0% <span class="meta">(0/2)</span></td><td>0.0% <span class="meta">(0/2)</span></td><td>0.0% <span class="meta">(0/2)</span></td><td><span class="meta"></span></td></tr><tr><td>gpt-5-4-mini-openrouter</td><td>100.0% <span class="meta">(1/1)</span></td><td>100.0% <span class="meta">(3/3)</span></td><td>100.0% <span class="meta">(2/2)</span></td><td>100.0% <span class="meta">(3/3)</span></td><td><span class="meta">openrouter</span></td></tr><tr><td>gpt-5-mini</td><td>0.0% <span class="meta">(0/1)</span></td><td>0.0% <span class="meta">(0/3)</span></td><td>0.0% <span class="meta">(0/2)</span></td><td>0.0% <span class="meta">(0/3)</span></td><td><span class="meta">openai-compat</span></td></tr><tr><td>vidar-qwen3-6-27b-openai-compat</td><td>33.3% <span class="meta">(1/3)</span></td><td>8.6% <span class="meta">(3/35)</span></td><td>13.3% <span class="meta">(2/15)</span></td><td>3.4% <span class="meta">(3/89)</span></td><td><span class="meta"></span></td></tr></tbody></table>
<h2>Detailed metrics</h2>
<table><thead><tr><th>Profile</th><th>Harness</th><th>Attempts</th><th>Real</th><th>pass@1</th><th>pass@k</th><th>med turns</th><th>med in</th><th>med out</th><th>med wall (s)</th><th>cost ($)</th><th>p50 TTFT (s)</th><th>p50 decode (tok/s)</th></tr></thead><tbody><tr><td>claude-native-sonnet-4-6</td><td><span class="meta">Claude Code (native CLI)</span></td><td>15</td><td>0</td><td>0.0%</td><td>0.0%</td><td>—</td><td>—</td><td>—</td><td>—</td><td>0.000</td><td>—</td><td>—</td></tr><tr><td>claude-sonnet-4-6</td><td><span class="meta">fiz (built-in agent loop)</span></td><td>102</td><td>0</td><td>14.0%</td><td>3.4%</td><td>—</td><td>—</td><td>—</td><td>—</td><td>0.000</td><td>1.95</td><td>824.5</td></tr><tr><td>codex-native-gpt-5-4-mini</td><td><span class="meta">Codex (native CLI)</span></td><td>15</td><td>0</td><td>91.7%</td><td>100.0%</td><td>—</td><td>—</td><td>—</td><td>—</td><td>0.000</td><td>—</td><td>—</td></tr><tr><td>fiz-openai-gpt-5-5</td><td><span class="meta">fiz (built-in agent loop)</span></td><td>521</td><td>98</td><td>24.9%</td><td>24.7%</td><td>12</td><td>42,581</td><td>2,398</td><td>179</td><td>0.840</td><td>0.07</td><td>292.5</td></tr><tr><td>fiz-openrouter-claude-sonnet-4-6</td><td><span class="meta">fiz (built-in agent loop)</span></td><td>199</td><td>15</td><td>22.2%</td><td>10.0%</td><td>11</td><td>166,505</td><td>2,182</td><td>135</td><td>0.574</td><td>1.89</td><td>1474.7</td></tr><tr><td>fiz-openrouter-gpt-5-4-mini</td><td><span class="meta">fiz (built-in agent loop)</span></td><td>199</td><td>14</td><td>6.9%</td><td>6.7%</td><td>8</td><td>32,542</td><td>886</td><td>108</td><td>0.053</td><td>0.78</td><td>177.7</td></tr><tr><td>gpt-5-3-mini</td><td><span class="meta">fiz (built-in agent loop)</span></td><td>22</td><td>0</td><td>—</td><td>0.0%</td><td>—</td><td>—</td><td>—</td><td>—</td><td>0.000</td><td>—</td><td>—</td></tr><tr><td>gpt-5-4-mini-openrouter</td><td><span class="meta">fiz (built-in agent loop)</span></td><td>15</td><td>0</td><td>46.7%</td><td>100.0%</td><td>—</td><td>—</td><td>—</td><td>—</td><td>0.000</td><td>1.04</td><td>194.4</td></tr><tr><td>gpt-5-mini</td><td><span class="meta">fiz (built-in agent loop)</span></td><td>21</td><td>0</td><td>—</td><td>0.0%</td><td>—</td><td>—</td><td>—</td><td>—</td><td>0.000</td><td>—</td><td>—</td></tr><tr><td>vidar-qwen3-6-27b-openai-compat</td><td><span class="meta">fiz (built-in agent loop)</span></td><td>276</td><td>0</td><td>5.4%</td><td>3.4%</td><td>—</td><td>—</td><td>—</td><td>—</td><td>0.000</td><td>9.44</td><td>18.1</td></tr></tbody></table>
<h2>Cost to extend coverage</h2>
<div class="narrative"><p><em>Estimates below use observed per-run costs from lanes that already produced real reps; see <code>data/aggregates.json</code> for the underlying numbers. Prices come from <code>scripts/benchmark/profiles/*.yaml</code>. Where a lane has no real reps yet, the estimate is a back-of-envelope <code>pricing × median-tokens</code> from a comparable lane on the same model.</em></p>
<h3>What it would cost to close the gaps</h3>
<p>Three of the model rows on the table above carry zero real reps because their setups never produced a non-<code>invalid_setup</code> trial in the latest sweep (<code>claude-native-sonnet-4-6</code>, <code>claude-sonnet-4-6</code> openrouter built-in, <code>gpt-5-mini</code>). Two other rows (<code>fiz-openai-gpt-5-5</code>, <code>fiz-openrouter-claude-sonnet-4-6</code>) have partial coverage on the 35-task <code>openai-cheap</code> subset but have not run <code>--reps 5</code> against the full subset. The estimates below are what it would take in pure model spend to bring each lane to a full <code>--reps 5 × 35-task</code> cell on the cheap subset.</p>
<table>
<thead>
<tr>
<th>Lane</th>
<th>Source $/run</th>
<th>Subset cost (35 × 5 reps)</th>
<th>Notes</th>
</tr>
</thead>
<tbody>
<tr>
<td><code>fiz-openai-gpt-5-5</code></td>
<td>$0.84 (98 real reps observed)</td>
<td><strong>≈ $147</strong></td>
<td>Most expensive; <code>openai-cheap</code> was sized to keep this under ~$1/run, real number landed close.</td>
</tr>
<tr>
<td><code>fiz-openrouter-gpt-5-5</code></td>
<td>not yet run</td>
<td><strong>≈ $147</strong> est.</td>
<td>Same model + pricing as above; assume identical token use until measured.</td>
</tr>
<tr>
<td><code>fiz-openrouter-claude-sonnet-4-6</code></td>
<td>$0.57 (15 real reps observed)</td>
<td><strong>≈ $100</strong></td>
<td>Median in-tokens 166k drives the cost; cached-input pricing ($0.30/Mtok) would cut this if prefix-cache hits land.</td>
</tr>
<tr>
<td><code>fiz-harness-claude-sonnet-4-6</code></td>
<td>unknown (0 real reps)</td>
<td><strong>≈ $100</strong> est.</td>
<td>Wrapper path; assume same token profile as the direct OpenRouter Sonnet lane. Currently blocked on <code>invalid_setup</code>, not on cost.</td>
</tr>
<tr>
<td><code>fiz-openrouter-gpt-5-4-mini</code></td>
<td>$0.053 (14 real reps observed)</td>
<td><strong>≈ $9</strong></td>
<td>Already cheap; the bottleneck is reliability, not budget.</td>
</tr>
<tr>
<td><code>fiz-harness-codex-gpt-5-4-mini</code></td>
<td>unknown (0 real reps)</td>
<td><strong>≈ $9</strong> est.</td>
<td>Same model + pricing as the direct mini lane. Reliability gating, not cost.</td>
</tr>
</tbody>
</table>
<p>To extend everything outside Qwen to a full <code>--reps 5 × 35-task</code> cell on <code>openai-cheap</code>, total spend is <strong>≈ $510</strong> in API costs (Sonnet ≈ $200 across two paths, GPT-5.5 ≈ $295 across two paths, mini ≈ $18 across two paths). The <code>all</code> (89-task) subset for the GPT-5.5 / Sonnet rows would be roughly 2.5× that — <strong>≈ $1.3k</strong> for the frontier hosted models, which is why those rows stay on the cheap subset by default.</p>
<p>The cheap rows (mini, Qwen via OpenRouter) are not budget-gated; they are blocked on the same <code>invalid_setup</code> issues that produced 108 of 199 attempts in the table above. Fixing that infrastructure issue unlocks coverage, not money.</p></div>
<h2>Model power vs pass-rate</h2>
<div class="narrative"><p><em>Regenerate this section against the latest data — see <code>data/aggregates.json</code> and the chart below. Open in a pull request when refreshed.</em></p>
<p>Each marker is a lane (or external submission) plotted at its model-power score (1 = weak, 10 = frontier per <code>scripts/benchmark/terminalbench_model_power.json</code>) against pass@k on the <code>all</code> subset. Marker size scales with median turns (larger = the agent worked harder before converging or giving up). Distance below the trend line at a given x-value is the <em>harness loss</em> for that lane: how much pass-rate the lane leaves on the floor relative to what the underlying model delivers elsewhere.</p></div>
<div class="chart"><img src="/benchmarks/terminal-bench-2-1/charts/model-power-scatter.svg" alt="model-power-scatter.svg"></div>
</div>
