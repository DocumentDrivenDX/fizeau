---
title: Fizeau
layout: hextra-home
---

<div class="hero">
  <div class="hero__mark">{{< wheel-mark size=120 color="var(--accent-cyan)" >}}</div>

  <div class="hero__eyebrow">Embeddable Go agent runtime</div>

  <h1 class="hero__title">Fizeau</h1>

  <p class="hero__lede">
    An <b>agentic-development runtime</b> with its own measured loop. Other tools build on fizeau instead of writing their own — we own the harness, sampling, performance instrumentation, cost tracking, and subscription accounting so they don't have to. Local-model-first via vLLM, MLX, LM Studio, Ollama; cloud providers when you want them.
  </p>

  <div class="hero__cta">
    <a class="hero__btn hero__btn--primary" href="docs/getting-started/">Get started</a>
    <a class="hero__btn" href="benchmarks/">View benchmarks</a>
    <a class="hero__btn hero__btn--ghost" href="https://github.com/easel/fizeau">GitHub →</a>
  </div>
</div>

<div class="hero-readout-wrap">

{{< bench-readout >}}

</div>

<section class="what-is">
  <h2 class="what-is__title">Why fizeau exists</h2>

  <p>Fizeau exists for three reasons that build on each other:</p>

  <ol>
    <li><b>Facilitate agentic development.</b> A reusable, embeddable agent loop with the right primitives — tool-calling, planning, compaction, retries, session logging — so building tools doesn't mean re-implementing the loop every time.</li>
    <li><b>Make agentic work measurable.</b> Per-turn timing, prefill vs decode breakdown, cost-per-trial, subscription-quota accounting — first-class outputs, not bolted-on observability. You can't improve prompts, agents, or providers you can't measure.</li>
    <li><b>Make local models a real option.</b> Local serving (vLLM, MLX, LM Studio, Ollama) on the same provider surface as cloud frontier models. The benchmarks compare them honestly. Self-hosted at the right quantization is often cheaper, sometimes faster, and rarely the right answer for everything — but you can pick per workload because the data is on the table.</li>
  </ol>

  <h2 class="what-is__title" style="margin-top: 2.5rem;">What it is</h2>

  <p>Fizeau is an agent runtime with a built-in agent loop (the <code>fiz</code> harness): it manages the prompt, tool-call protocol, file/edit/bash tooling, planning, compaction, retries, sampling, reasoning, quotas, and session log. It is not an LLM serving runtime — it does not host weights. Upstream model traffic goes to whatever provider the lane points at (OpenAI, Anthropic, OpenRouter, vLLM, oMLX, RapidMLX, native local).</p>

  <p>Fizeau can also run <em>as a wrapper</em> around a different agent CLI (Claude Code, Codex, Pi, OpenCode) — the <code>fiz-harness-*</code> lanes in the <a href="benchmarks/profiles/">profile catalog</a> use this mode, where fiz handles configuration, environment, tool-call accounting, and session logging while delegating the reasoning loop to the wrapped agent. This isolates "is the agent loop hurting?" from "is the model hurting?" — same model, different harness, different lane.</p>

  <p>For <a href="benchmarks/">benchmark purposes</a>, each lane in the catalog holds either axis constant and varies the other. A delta between two lanes that share a model but differ in harness is <em>harness loss</em>; a delta between two lanes that share a harness but differ in provider is <em>provider/runtime loss</em>.</p>
</section>

<section class="features">
  <h2 class="features__title">Built for instrumented agent work</h2>
  <p class="features__lede">
    Every surface assumes you want to know <em>what the medium is doing</em>. There is no separate observability layer — the runtime emits structured per-turn timing as a first-class output.
  </p>

  <div class="features__grid">

  <div class="feature">
  <div class="feature__label">RUNTIME</div>
  <h3>Built-in agent loop</h3>
  <p>Tool-calling LLM loop with read, write, edit, bash, find, grep, ls, patch, task. Compaction, retry, sampler, reasoning, quotas — all wired through one provider-shaped surface.</p>
  </div>

  <div class="feature">
  <div class="feature__label">PROVIDERS</div>
  <h3>One surface, many backends</h3>
  <p>OpenAI, Anthropic, OpenRouter, vLLM, oMLX, RapidMLX, native local. Lane definitions are YAML; benchmark deltas reflect provider/runtime, not harness drift.</p>
  </div>

  <div class="feature">
  <div class="feature__label">MEASUREMENT</div>
  <h3>TTFT, decode, prefill — per turn</h3>
  <p>Every <code>llm.request → llm.delta → llm.response</code> chain is timed and recorded. No sampling, no aggregation loss. Bucket by context length; attribute wall-time to prefill vs generation.</p>
  </div>

  <div class="feature">
  <div class="feature__label">HARNESS-AS-WRAPPER</div>
  <h3>Wrap Claude Code, Codex, Pi, OpenCode</h3>
  <p><code>fiz-harness-*</code> lanes route through fiz as a measurement wrapper around another agent CLI. Holds the model constant; varies the harness; isolates "is the loop hurting?" from "is the model hurting?"</p>
  </div>

  <div class="feature">
  <div class="feature__label">SESSIONS</div>
  <h3>JSONL session logs, replayable</h3>
  <p>Every turn, every tool call, every cost figure on disk in line-delimited JSON. <code>fiz log</code> to list, <code>fiz replay</code> to render. Replays drive the per-turn timing analysis behind every chart on this site.</p>
  </div>

  <div class="feature">
  <div class="feature__label">EMBEDDABLE</div>
  <h3>Go library, no subprocess overhead</h3>
  <p><code>fizeau.New(...).Execute(ctx, request)</code>. Lives inside a build orchestrator (<a href="https://github.com/DocumentDrivenDX/ddx">DDx</a>) or any Go service that needs a tool-using model on its critical path.</p>
  </div>

  </div>
</section>

