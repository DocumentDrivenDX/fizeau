---
title: "Explorer"
weight: 2
toc: false
---

<div class="bench-workbench" data-benchmark-workbench data-cells-url="../../data/cells.parquet" data-combinations-url="../../data/task-combinations.parquet" data-manifest-url="../../data/benchmark-data-manifest.json" data-duckdb-base="../../vendor/duckdb/">
  <section class="bench-workbench__header" aria-labelledby="benchmark-workbench-title">
    <div>
      <p class="bench-workbench__eyebrow">Raw Benchmark Cells</p>
      <h1 id="benchmark-workbench-title">Benchmark Workbench</h1>
    </div>
    <div class="bench-workbench__status" data-bw-status role="status" aria-live="polite">Initializing browser database...</div>
  </section>

  <section class="bench-workbench__controls" aria-label="Benchmark filters">
    <label class="bench-workbench__control bench-workbench__control--wide">
      <span>Search</span>
      <input type="search" data-bw-search placeholder="model, GPU, harness, outcome, descriptor">
    </label>
    <label class="bench-workbench__control">
      <span>Outcome</span>
      <select data-bw-result-state>
        <option value="">All outcomes</option>
      </select>
    </label>
    <label class="bench-workbench__control">
      <span>Test / task</span>
      <select data-bw-task>
        <option value="">All tests</option>
      </select>
    </label>
    <label class="bench-workbench__control">
      <span>Engine</span>
      <select data-bw-engine>
        <option value="">All engines</option>
      </select>
    </label>
    <label class="bench-workbench__control">
      <span>GPU</span>
      <select data-bw-gpu>
        <option value="">All GPUs</option>
      </select>
    </label>
    <label class="bench-workbench__control bench-workbench__control--number">
      <span>Max GPU RAM</span>
      <input type="number" data-bw-max-ram min="0" step="0.25" placeholder="GB">
    </label>
    <label class="bench-workbench__toggle">
      <input type="checkbox" data-bw-passed-only>
      <span>Passed only</span>
    </label>
  </section>

  <section class="bench-workbench__presets" aria-label="Saved views">
    <button type="button" data-bw-preset="all">All cells</button>
    <button type="button" data-bw-preset="passing-test">Passing selected test</button>
    <button type="button" data-bw-preset="passing-test-gpu">Passing selected test on GPU</button>
    <button type="button" data-bw-preset="passing-test-ram">Passing selected test under RAM</button>
    <button type="button" data-bw-preset="recent-failures">Recent failures</button>
  </section>

  <section class="bench-workbench__metrics" aria-label="Current aggregate readouts">
    <div class="bench-workbench__metric">
      <span>Rows</span>
      <strong data-bw-metric="rows">-</strong>
    </div>
    <div class="bench-workbench__metric">
      <span>Pass rate</span>
      <strong data-bw-metric="pass_rate">-</strong>
    </div>
    <div class="bench-workbench__metric">
      <span>Timeouts</span>
      <strong data-bw-metric="timeouts">-</strong>
    </div>
    <div class="bench-workbench__metric">
      <span>Models</span>
      <strong data-bw-metric="models">-</strong>
    </div>
    <div class="bench-workbench__metric">
      <span>GPUs</span>
      <strong data-bw-metric="gpus">-</strong>
    </div>
    <div class="bench-workbench__metric">
      <span>Tokens</span>
      <strong data-bw-metric="tokens">-</strong>
    </div>
    <div class="bench-workbench__metric">
      <span>Known cost</span>
      <strong data-bw-metric="cost">-</strong>
    </div>
    <div class="bench-workbench__metric">
      <span>Wall p50</span>
      <strong data-bw-metric="wall_p50">-</strong>
    </div>
  </section>

  <section class="bench-workbench__grid-panel" aria-label="Benchmark cell datagrid">
    <div class="bench-workbench__panel-head">
      <div>
        <span class="bench-workbench__eyebrow">Cell Datatable</span>
      </div>
      <button type="button" data-bw-open-config>Columns / filters</button>
    </div>
    <perspective-viewer data-bw-viewer></perspective-viewer>
  </section>

  <section class="bench-workbench__aggregate-panel" aria-label="Combination aggregates">
    <div class="bench-workbench__panel-head">
      <div>
        <span class="bench-workbench__eyebrow">Combination Aggregates</span>
      </div>
    </div>
    <div class="bench-workbench__aggregate-table" data-bw-combinations></div>
  </section>
</div>
