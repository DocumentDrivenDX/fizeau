---
title: "Runtime Properties"
weight: 5
toc: false
---

<div class="br-body">
<div class="meta">Filterable grid of per-lane runtime properties. Click a column header to sort; shift-click a second column for a secondary sort. Use the filter row to narrow lanes. Empty cells (—) indicate the runtime_props field was absent for that lane — either a managed cloud lane where capture is not applicable, or capture has not run yet for this lane.</div>

<div class="narrative">
<p>Each row corresponds to one benchmark lane (a profile in <code>scripts/benchmark/profiles/</code>). The columns come from the <code>runtime_props</code> field on the evidence record, captured by the props-capture extractor at run time. Fields are all optional; lanes without capture data show <strong>—</strong> and sort last.</p>
<ul>
<li><strong>Extractor</strong> — which capture script populated this row.</li>
<li><strong>Base model</strong> — the model identifier as reported by the server (not the profile alias).</li>
<li><strong>Model quant</strong> — quantization format (e.g. <code>int4</code>, <code>Q3_K_XL</code>, <code>8-bit</code>).</li>
<li><strong>KV quant</strong> — KV-cache quantization applied by the runtime, if any.</li>
<li><strong>Draft model / mode</strong> — speculative decoding draft model and accept strategy, when enabled.</li>
<li><strong>Max ctx</strong> — maximum context window in tokens as reported by the server.</li>
<li><strong>MTP</strong> — whether multi-token prediction is enabled server-side.</li>
<li><strong>Temp</strong> — temperature from sampling_defaults (server-side default, may differ from per-request override).</li>
</ul>
</div>

{{< runtime-props-grid >}}

</div>
