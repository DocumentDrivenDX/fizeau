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
      <span>Model</span>
      <select data-bw-model>
        <option value="">All models</option>
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

  <details class="bench-workbench__filter-drawer" open>
    <summary>Comparison filters</summary>
    <div class="bench-workbench__filter-grid">
      <label class="bench-workbench__control">
        <span>Model family</span>
        <select data-bw-filter-field="model_family"><option value="">All families</option></select>
      </label>
      <label class="bench-workbench__control">
        <span>Model quant</span>
        <select data-bw-filter-field="model_quant"><option value="">All model quants</option></select>
      </label>
      <label class="bench-workbench__control">
        <span>KV cache</span>
        <select data-bw-filter-field="kv_cache_quant"><option value="">All KV caches</option></select>
      </label>
      <label class="bench-workbench__control">
        <span>K quant</span>
        <select data-bw-filter-field="k_quant"><option value="">All K quants</option></select>
      </label>
      <label class="bench-workbench__control">
        <span>V quant</span>
        <select data-bw-filter-field="v_quant"><option value="">All V quants</option></select>
      </label>
      <label class="bench-workbench__control">
        <span>MTP</span>
        <select data-bw-filter-field="runtime_mtp_enabled"><option value="">All MTP states</option></select>
      </label>
      <label class="bench-workbench__control">
        <span>Provider</span>
        <select data-bw-filter-field="provider_type"><option value="">All providers</option></select>
      </label>
      <label class="bench-workbench__control">
        <span>Surface</span>
        <select data-bw-filter-field="provider_surface"><option value="">All surfaces</option></select>
      </label>
      <label class="bench-workbench__control">
        <span>Harness</span>
        <select data-bw-filter-field="harness_label"><option value="">All harnesses</option></select>
      </label>
      <label class="bench-workbench__control">
        <span>Lane</span>
        <select data-bw-filter-field="lane_label"><option value="">All lanes</option></select>
      </label>
      <label class="bench-workbench__control">
        <span>Task category</span>
        <select data-bw-filter-field="task_category"><option value="">All categories</option></select>
      </label>
      <label class="bench-workbench__control">
        <span>Difficulty</span>
        <select data-bw-filter-field="task_difficulty"><option value="">All difficulties</option></select>
      </label>
      <label class="bench-workbench__control">
        <span>Deployment</span>
        <select data-bw-filter-field="deployment_class"><option value="">All deployments</option></select>
      </label>
      <label class="bench-workbench__control">
        <span>Machine</span>
        <select data-bw-filter-field="machine"><option value="">All machines</option></select>
      </label>
      <label class="bench-workbench__control">
        <span>GPU vendor</span>
        <select data-bw-filter-field="gpu_vendor"><option value="">All vendors</option></select>
      </label>
      <label class="bench-workbench__control">
        <span>Chip family</span>
        <select data-bw-filter-field="hardware_chip_family"><option value="">All chip families</option></select>
      </label>
      <label class="bench-workbench__control">
        <span>Memory type</span>
        <select data-bw-filter-field="hardware_memory_type"><option value="">All memory types</option></select>
      </label>
      <label class="bench-workbench__control">
        <span>Backend</span>
        <select data-bw-filter-field="backend"><option value="">All backends</option></select>
      </label>
      <label class="bench-workbench__control">
        <span>Draft mode</span>
        <select data-bw-filter-field="runtime_draft_mode"><option value="">All draft modes</option></select>
      </label>
      <label class="bench-workbench__control">
        <span>Draft model</span>
        <select data-bw-filter-field="runtime_draft_model"><option value="">All draft models</option></select>
      </label>
      <label class="bench-workbench__control">
        <span>KV disk</span>
        <select data-bw-filter-field="kv_cache_disk"><option value="">All KV disk states</option></select>
      </label>
      <label class="bench-workbench__control">
        <span>Reasoning</span>
        <select data-bw-filter-field="sampling_reasoning"><option value="">All reasoning</option></select>
      </label>
      <label class="bench-workbench__control">
        <span>Temperature</span>
        <select data-bw-filter-field="sampling_temperature"><option value="">All temperatures</option></select>
      </label>
      <label class="bench-workbench__control">
        <span>Top-p</span>
        <select data-bw-filter-field="sampling_top_p"><option value="">All top-p values</option></select>
      </label>
      <label class="bench-workbench__control">
        <span>Top-k</span>
        <select data-bw-filter-field="sampling_top_k"><option value="">All top-k values</option></select>
      </label>
      <label class="bench-workbench__control">
        <span>Context</span>
        <select data-bw-filter-field="context_tokens"><option value="">All contexts</option></select>
      </label>
      <label class="bench-workbench__control">
        <span>Output cap</span>
        <select data-bw-filter-field="max_output_tokens"><option value="">All output caps</option></select>
      </label>
    </div>
  </details>

  <section class="bench-workbench__presets" aria-label="Saved views">
    <button type="button" data-bw-preset="all">All cells</button>
    <button type="button" data-bw-preset="passing-test">Passing selected test</button>
    <button type="button" data-bw-preset="passing-test-gpu">Passing selected test on GPU</button>
    <button type="button" data-bw-preset="passing-test-ram">Passing selected test under RAM</button>
    <button type="button" data-bw-preset="recent-failures">Recent failures</button>
    <button type="button" data-bw-clear-filters>Clear filters</button>
  </section>

  <div class="bench-workbench__active-filters" data-bw-active-filters hidden></div>

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

  <section class="bench-workbench__compare-panel" aria-label="Pairwise model comparison">
    <div class="bench-workbench__panel-head">
      <div>
        <span class="bench-workbench__eyebrow">Pairwise Gap</span>
      </div>
      <div class="bench-workbench__compare-controls">
        <label class="bench-workbench__control">
          <span>Baseline</span>
          <select data-bw-compare-a><option value="">Baseline family</option></select>
        </label>
        <label class="bench-workbench__control">
          <span>Compare</span>
          <select data-bw-compare-b><option value="">Compare family</option></select>
        </label>
        <label class="bench-workbench__control">
          <span>Group by</span>
          <select data-bw-compare-dimension>
            <option value="task_category">Task category</option>
            <option value="task_difficulty">Task difficulty</option>
            <option value="task">Task</option>
            <option value="result_state">Outcome</option>
            <option value="engine">Engine</option>
            <option value="model_quant">Model quant</option>
            <option value="deployment_class">Deployment</option>
            <option value="gpu_vendor">GPU vendor</option>
            <option value="effective_gpu_model">GPU</option>
            <option value="sampling_reasoning">Reasoning</option>
            <option value="provider_type">Provider</option>
            <option value="harness_label">Harness</option>
          </select>
        </label>
      </div>
    </div>
    <div class="bench-workbench__comparison-table" data-bw-comparison></div>
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
