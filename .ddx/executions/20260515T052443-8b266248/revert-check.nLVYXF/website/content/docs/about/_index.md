---
title: About
weight: 90
---

## Mission

Fizeau exists for three reasons that build on each other:

1. **Facilitate agentic development.** A reusable, embeddable agent loop with the right primitives — tool-calling, planning, compaction, retries, sampling, reasoning, quotas, session logs — so building tools doesn't mean re-implementing the loop every time. Other tools embed `fizeau.New(...)` instead of writing their own agent harness.
2. **Make agentic work measurable.** Per-turn timing, prefill vs decode breakdown, cost-per-trial, subscription-quota accounting, route-attempt feedback — all first-class outputs, not bolted-on observability. You can't improve the prompts, agents, or providers you can't measure.
3. **Make local models a real option.** Local serving (vLLM, MLX, LM Studio, Ollama) on the same provider surface as cloud frontier models. The benchmarks compare them honestly. Self-hosted at the right quantization is often cheaper, sometimes faster, and rarely the right answer for everything — but you can pick per workload because the data is on the table.

Harness control from #1 enables #2 (we can instrument what we own) and makes #3 viable (one provider surface across cloud and local). Tools built on fizeau inherit that without writing their own loop, sampling controls, performance instrumentation, cost tracking, or subscription accounting.

## Why "Fizeau"

Named for **Armand Hippolyte Louis Fizeau** (1819–1896), the French physicist who made the first terrestrial measurement of the speed of light — and then, two years later, did something more interesting.

In 1849 Fizeau spun a toothed wheel on a rooftop in Suresnes and bounced a beam of light off a mirror 8.6 km away on Montmartre. By spinning the wheel fast enough that the returning beam was blocked by the *next* tooth instead of the gap that emitted it, he could solve for the round-trip transit time of light over a known distance — and from that, the speed of light. He measured 313,000 km/s, within a few percent of the modern value. The point is not the result. The point is that you can pin down a quantity that fast by making the *measurement chain itself* fast and precise enough to interrogate it.

In 1851 he sent two coherent light beams through a tube where water flowed in opposite directions, recombined them, and looked at the interference fringes. The fringes shifted with flow direction — proving that the speed of light through a moving medium depends on the medium's motion, but not by the simple sum a Newtonian intuition would predict. The "Fizeau drag coefficient" he derived matches the value Einstein arrived at from special relativity sixty years later.

Two ideas embedded there:

1. **The medium matters.** You cannot measure something travelling through a substrate without accounting for what the substrate is doing.
2. **Differences are easier to measure than absolutes.** Fizeau didn't measure the absolute speed of light through water; he measured the *difference* between light flowing with the current and light flowing against it.

Modern LLM agents are bytes flowing through a measurement chain — provider, network, harness, container, tools, and back. What we want to know about that chain is, mostly, *how is it running and where is it slow?* Fizeau, the software, treats those measurements as first-class output, not as observability bolted on. Every turn produces structured timing. Every provider runs through the same surface so deltas mean what they look like they mean. The rotating-wheel motif fits: a precision chronograph wrapped around a fast-moving signal, designed to interrogate a moving medium without distorting it.

## Read more

{{< cards >}}
  {{< card link="the-name" title="The Name (full story)" subtitle="Citations, the apparatus, and the design intent it informs." >}}
{{< /cards >}}
