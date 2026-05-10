---
title: The Name
linkTitle: The Name
weight: 1
---

## Hippolyte Fizeau

The project is named for **Armand Hippolyte Louis Fizeau** (23 September 1819 – 18 September 1896), the French physicist who made the first reasonably accurate non-astronomical measurement of the speed of light — and then, two years later, did something more interesting.[^bio]

### 1849 — the toothed wheel

In 1849 Fizeau set up a rotating brass wheel pierced by 720 teeth on the rooftop of his father's house on the slopes of Mont Valérien in Suresnes, west of Paris, and a return mirror on the heights of Montmartre, roughly 8.6 km away. He shone a beam of light through a gap between teeth, off the distant mirror, and back. By spinning the wheel fast enough that the returning beam was blocked by the *next* tooth instead of the gap that emitted it (the first total eclipse occurred at about 12.6 rotations per second), he could solve for the round-trip transit time of light over a known distance — and from that, the speed of light. He reported a value of approximately 315,000 km/s, within a few percent of the modern value (≈299,792 km/s).[^speed][^enwiki]

The apparatus is a measurement instrument, not a model. The point is not the result; the point is that it works at all — that you can pin down a quantity that fast by making the *measurement chain itself* fast and precise enough to interrogate it.

### 1851 — light in moving water

Two years later Fizeau performed the experiment that has his name on it more permanently. He sent two coherent light beams through a U-shaped tube where water flowed rapidly in opposite directions, recombined them on an interferometer, and measured the shift in the resulting fringes. The fringes shifted depending on the water's flow direction — confirming Augustin-Jean Fresnel's empirical "dragging coefficient" *f* = 1 − 1/n², which says light travelling through a moving medium is partially dragged along but by less than a Newtonian sum would predict.[^fizeauexp]

That result remained mysterious for half a century. In 1907 Max von Laue showed the Fresnel drag coefficient is a natural consequence of the relativistic addition of velocities derived in Einstein's 1905 special-relativity paper. Einstein himself singled out the Fizeau measurements as among the experimental results that "were enough" to inform his thinking about relativity.[^laue]

Two ideas embedded in that experiment:

1. **The medium matters.** You cannot measure something travelling through a substrate without accounting for what the substrate is doing.
2. **Differences are easier to measure than absolutes.** Fizeau didn't measure the absolute speed of light through water; he measured the *difference* between light flowing with the current and light flowing against it, then differenced them out.

## Why this name for an agent runtime

Modern LLM agents are bytes flowing through a measurement chain — provider, network, harness, container, tools, and back. The thing we want to know about that chain is, mostly, *how is it running and where is it slow?* — round-trip latency per turn, prefill vs decode rate, throughput under context pressure, where compute is being spent, where it is being wasted.

Fizeau, the software, treats those measurements as first-class output, not as observability bolted on. Every turn produces structured timing. Every provider runs through the same surface so deltas mean what they look like they mean. The rotating-wheel motif is the right one: a precision chronograph wrapped around a fast-moving signal, designed to interrogate a moving medium without distorting it.

That is the genesis of the name, and the design intent behind the project.

### Sources

[^bio]: Biographical dates: [Hippolyte Fizeau](https://en.wikipedia.org/wiki/Hippolyte_Fizeau), English Wikipedia. Cross-checked against [Fizeau](https://fr.wikipedia.org/wiki/Hippolyte_Fizeau) on French Wikipedia.

[^speed]: Geographic and apparatus details (Mont Valérien · Suresnes · Montmartre, ≈8 000 m baseline, ≈315 300 km/s reported value): [Hippolyte Fizeau](https://fr.wikipedia.org/wiki/Hippolyte_Fizeau) (French Wikipedia, "Mesure de la vitesse de la lumière" section). The original paper is Fizeau, H. L. (1849). "Sur une expérience relative à la vitesse de propagation de la lumière." *Comptes rendus hebdomadaires des séances de l'Académie des sciences*, **29** (5–7 Septembre): 90–92. Scanned originals are searchable on [Gallica (BnF)](https://gallica.bnf.fr/ark:/12148/cb343486750/date.r=comptes+rendus+academie+des+sciences.langFR).

[^enwiki]: Tooth count (720), measured value (313,274,304 m/s in the value commonly cited from English-language reproductions of Fizeau's later refinement), and rotation rate at first eclipse (12.6 rps): [Hippolyte Fizeau](https://en.wikipedia.org/wiki/Hippolyte_Fizeau), English Wikipedia. Different secondary sources report values in the 313 000–315 300 km/s range because Fizeau revised the figure across publications; the modern accepted value is 299 792.458 km/s.

[^fizeauexp]: Apparatus design (U-tube interferometer, two beams in counter-flowing water), Fresnel drag coefficient *f* = 1 − 1/n²: [Fizeau experiment](https://en.wikipedia.org/wiki/Fizeau_experiment), English Wikipedia. Original paper: Fizeau, H. L. (1851). "Sur les hypothèses relatives à l'éther lumineux, et sur une expérience qui paraît démontrer que le mouvement des corps change la vitesse avec laquelle la lumière se propage dans leur intérieur." *Comptes rendus hebdomadaires des séances de l'Académie des sciences*, **33**: 349–355.

[^laue]: Special-relativity connection: Max von Laue (1907), "Die Mitführung des Lichtes durch bewegte Körper nach dem Relativitätsprinzip," *Annalen der Physik*, **328** (10): 989–990. Einstein's later acknowledgement of the Fizeau result's importance is documented and quoted in the Wikipedia [Fizeau experiment](https://en.wikipedia.org/wiki/Fizeau_experiment) article, "Influence on Einstein" section.
