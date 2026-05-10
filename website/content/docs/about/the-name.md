---
title: The Name
linkTitle: The Name
weight: 1
---

## Hippolyte Fizeau

The project is named for **Armand Hippolyte Louis Fizeau** (1819–1896), the French physicist who made the first terrestrial measurement of the speed of light — and then, two years later, did something more interesting.

### 1849 — the toothed wheel

In September 1849 Fizeau set up a rotating toothed wheel on the rooftop of his father's house in Suresnes, near Paris, and a mirror eight and a half kilometres away on Montmartre. He shone a beam of light through a gap between teeth, off the distant mirror, and back. By spinning the wheel fast enough that the returning beam was blocked by the *next* tooth instead of the gap that emitted it, he could solve for the round-trip transit time of light over a known distance — and from that, the speed of light. He measured it at roughly 313,000 km/s, within a few percent of the modern value.

The apparatus is a measurement instrument, not a model. The point is not the result; the point is that it works at all — that you can pin down a quantity that fast by making the *measurement chain itself* fast and precise enough to interrogate it.

### 1851 — light in moving water

Two years later Fizeau did the experiment that has his name on it more permanently. He sent two coherent light beams through a tube where water was flowing rapidly in opposite directions, recombined them, and looked at the interference fringes. The fringes shifted depending on the water's flow direction — proving that the speed of light through a moving medium depends on the medium's motion, but not by the simple sum a Newtonian intuition would predict. The "Fizeau drag coefficient" he derived turned out to be the same value Einstein arrived at from special relativity sixty years later.

Two ideas embedded in that experiment:

1. **The medium matters.** You cannot measure something travelling through a substrate without accounting for what the substrate is doing.
2. **Differences are easier to measure than absolutes.** Fizeau didn't measure the absolute speed of light through water; he measured the *difference* between light flowing with the current and light flowing against it, then differenced them out.

## Why this name for an agent runtime

Modern LLM agents are bytes flowing through a measurement chain — provider, network, harness, container, tools, and back. The thing we want to know about that chain is, mostly, *how is it running and where is it slow?* — round-trip latency per turn, prefill vs decode rate, throughput under context pressure, where compute is being spent, where it is being wasted.

Fizeau, the software, treats those measurements as first-class output, not as observability bolted on. Every turn produces structured timing. Every provider runs through the same surface so deltas mean what they look like they mean. The rotating-wheel motif is the right one: a precision chronograph wrapped around a fast-moving signal, designed to interrogate a moving medium without distorting it.

That is the genesis of the name, and the design intent behind the project.

### Reading

- Fizeau, H. (1849). "Sur une expérience relative à la vitesse de propagation de la lumière." *Comptes Rendus*, 29, 90–92.
- Fizeau, H. (1851). "Sur les hypothèses relatives à l'éther lumineux, et sur une expérience qui paraît démontrer que le mouvement des corps change la vitesse avec laquelle la lumière se propage dans leur intérieur." *Comptes Rendus*, 33, 349–355.
- Wikipedia: [Hippolyte Fizeau](https://en.wikipedia.org/wiki/Hippolyte_Fizeau), [Fizeau experiment](https://en.wikipedia.org/wiki/Fizeau_experiment).
