---
title: "Explorer"
weight: 2
toc: false
---
<div class="bench-workbench" data-benchmark-workbench data-cells-url="../../data/cells.parquet" data-combinations-url="../../data/task-combinations.parquet" data-manifest-url="../../data/benchmark-data-manifest.json" data-duckdb-base="../../vendor/duckdb/">
  <section class="bench-workbench__header" aria-labelledby="benchmark-workbench-title">
    <div>
      <p class="bench-workbench__eyebrow">Browser Analytical Workbench</p>
      <h1 id="benchmark-workbench-title">Benchmark Explorer</h1>
    </div>
    <div class="bench-workbench__status" data-bw-status role="status" aria-live="polite">Initializing browser database...</div>
  </section>
  <div class="bench-workbench__shell">
    <nav class="bench-workbench__nav" aria-label="Explorer sections">
      <a href="#summary" data-bw-route="summary">
        <span>01</span>
        <strong>Summary</strong>
        <em>Shape, distribution, timing</em>
      </a>
      <a href="#data" data-bw-route="data">
        <span>02</span>
        <strong>Raw database</strong>
        <em>Search, sort, filter cells</em>
      </a>
      <a href="#compare" data-bw-route="compare">
        <span>03</span>
        <strong>Comparison</strong>
        <em>Pairwise model gaps</em>
      </a>
      <a href="#combinations" data-bw-route="combinations">
        <span>04</span>
        <strong>Combinations</strong>
        <em>Viable model/runtime/GPU sets</em>
      </a>
    </nav>
    <div class="bench-workbench__panes">
      <section class="bench-workbench__pane" data-bw-pane="summary" aria-labelledby="benchmark-summary-title">
        <div class="bench-workbench__pane-head">
          <div>
            <p class="bench-workbench__eyebrow">Summary</p>
            <h2 id="benchmark-summary-title">What Was Collected</h2>
          </div>
        </div>
        <div class="bench-workbench__summary-copy">
          <p>
            The explorer loads the valid benchmark cell dataset directly in the browser with DuckDB-WASM.
            A cell is one result-bearing run: an explicit timeout, a completed graded pass, or a completed graded failure.
            Setup/auth/provider-invalid rows are excluded from the Parquet table and counted in the manifest.
          </p>
        </div>
        <section class="bench-workbench__metrics bench-workbench__metrics--summary" aria-label="Dataset summary readouts">
          <div class="bench-workbench__metric">
            <span>Rows</span>
            <strong data-bw-summary-metric="rows">-</strong>
          </div>
          <div class="bench-workbench__metric">
            <span>Pass rate</span>
            <strong data-bw-summary-metric="pass_rate">-</strong>
          </div>
          <div class="bench-workbench__metric">
            <span>Timeouts</span>
            <strong data-bw-summary-metric="timeouts">-</strong>
          </div>
          <div class="bench-workbench__metric">
            <span>Tasks</span>
            <strong data-bw-summary-metric="tasks">-</strong>
          </div>
          <div class="bench-workbench__metric">
            <span>Models</span>
            <strong data-bw-summary-metric="models">-</strong>
          </div>
          <div class="bench-workbench__metric">
            <span>GPUs</span>
            <strong data-bw-summary-metric="gpus">-</strong>
          </div>
          <div class="bench-workbench__metric">
            <span>Tokens</span>
            <strong data-bw-summary-metric="tokens">-</strong>
          </div>
          <div class="bench-workbench__metric">
            <span>Wall p50</span>
            <strong data-bw-summary-metric="wall_p50">-</strong>
          </div>
        </section>
        <div class="bench-workbench__summary-grid" aria-label="Dataset distribution charts">
          <section class="bench-workbench__chart-panel">
            <div class="bench-workbench__panel-head">
              <p class="bench-workbench__eyebrow">Task Type Distribution</p>
            </div>
            <div data-bw-chart="task_category"></div>
          </section>
          <section class="bench-workbench__chart-panel">
            <div class="bench-workbench__panel-head">
              <p class="bench-workbench__eyebrow">GPU Distribution</p>
            </div>
            <div data-bw-chart="gpu"></div>
          </section>
          <section class="bench-workbench__chart-panel">
            <div class="bench-workbench__panel-head">
              <p class="bench-workbench__eyebrow">Wall Time by GPU</p>
            </div>
            <div data-bw-chart="wall_by_gpu"></div>
          </section>
          <section class="bench-workbench__chart-panel">
            <div class="bench-workbench__panel-head">
              <p class="bench-workbench__eyebrow">Wall Time by Model</p>
            </div>
            <div data-bw-chart="wall_by_model"></div>
          </section>
        </div>
      </section>
      <section class="bench-workbench__pane" data-bw-pane="data" aria-labelledby="benchmark-data-title" hidden>
        <div class="bench-workbench__pane-head">
          <div>
            <p class="bench-workbench__eyebrow">Raw Database</p>
            <h2 id="benchmark-data-title">Benchmark Cells</h2>
          </div>
          <button type="button" data-bw-open-config>Columns / filters</button>
        </div>
        <section class="bench-workbench__controls" aria-label="Raw benchmark filters">
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
        <details class="bench-workbench__filter-drawer">
          <summary>Raw enum filters</summary>
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
        <section class="bench-workbench__presets" aria-label="Saved raw database views">
          <button type="button" data-bw-preset="all">All cells</button>
          <button type="button" data-bw-preset="passing-test">Passing selected test</button>
          <button type="button" data-bw-preset="passing-test-gpu">Passing selected test on GPU</button>
          <button type="button" data-bw-preset="passing-test-ram">Passing selected test under RAM</button>
          <button type="button" data-bw-preset="recent-failures">Recent failures</button>
          <button type="button" data-bw-clear-filters>Clear filters</button>
        </section>
        <div class="bench-workbench__active-filters" data-bw-active-filters hidden></div>
        <section class="bench-workbench__metrics" aria-label="Current raw database aggregate readouts">
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
        <perspective-viewer data-bw-viewer></perspective-viewer>
      </section>
      <section class="bench-workbench__pane" data-bw-pane="compare" aria-labelledby="benchmark-compare-title" hidden>
        <div class="bench-workbench__pane-head">
          <div>
            <p class="bench-workbench__eyebrow">Pairwise Gap</p>
            <h2 id="benchmark-compare-title">Model Comparison</h2>
          </div>
        </div>
        <section class="bench-workbench__compare-controls" aria-label="Pairwise model comparison controls">
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
          <label class="bench-workbench__control bench-workbench__control--number">
            <span>Max GPU RAM</span>
            <input type="number" data-bw-compare-max-ram min="0" step="0.25" placeholder="GB">
          </label>
        </section>
        <details class="bench-workbench__filter-drawer" open>
          <summary>Comparison scope filters</summary>
          <div class="bench-workbench__filter-grid">
            <label class="bench-workbench__control">
              <span>Task category</span>
              <select data-bw-compare-filter-field="task_category"><option value="">All categories</option></select>
            </label>
            <label class="bench-workbench__control">
              <span>Difficulty</span>
              <select data-bw-compare-filter-field="task_difficulty"><option value="">All difficulties</option></select>
            </label>
            <label class="bench-workbench__control">
              <span>Task</span>
              <select data-bw-compare-filter-field="task"><option value="">All tasks</option></select>
            </label>
            <label class="bench-workbench__control">
              <span>Engine</span>
              <select data-bw-compare-filter-field="engine"><option value="">All engines</option></select>
            </label>
            <label class="bench-workbench__control">
              <span>GPU</span>
              <select data-bw-compare-filter-field="effective_gpu_model"><option value="">All GPUs</option></select>
            </label>
            <label class="bench-workbench__control">
              <span>Provider</span>
              <select data-bw-compare-filter-field="provider_type"><option value="">All providers</option></select>
            </label>
            <label class="bench-workbench__control">
              <span>Harness</span>
              <select data-bw-compare-filter-field="harness_label"><option value="">All harnesses</option></select>
            </label>
            <label class="bench-workbench__control">
              <span>Model quant</span>
              <select data-bw-compare-filter-field="model_quant"><option value="">All model quants</option></select>
            </label>
            <label class="bench-workbench__control">
              <span>KV cache</span>
              <select data-bw-compare-filter-field="kv_cache_quant"><option value="">All KV caches</option></select>
            </label>
            <label class="bench-workbench__control">
              <span>MTP</span>
              <select data-bw-compare-filter-field="runtime_mtp_enabled"><option value="">All MTP states</option></select>
            </label>
            <label class="bench-workbench__control">
              <span>GPU vendor</span>
              <select data-bw-compare-filter-field="gpu_vendor"><option value="">All vendors</option></select>
            </label>
            <label class="bench-workbench__control">
              <span>Reasoning</span>
              <select data-bw-compare-filter-field="sampling_reasoning"><option value="">All reasoning</option></select>
            </label>
          </div>
        </details>
        <div class="bench-workbench__comparison-table" data-bw-comparison></div>
      </section>
      <section class="bench-workbench__pane" data-bw-pane="combinations" aria-labelledby="benchmark-combinations-title" hidden>
        <div class="bench-workbench__pane-head">
          <div>
            <p class="bench-workbench__eyebrow">Combination Aggregates</p>
            <h2 id="benchmark-combinations-title">Passing Configurations</h2>
          </div>
        </div>
        <section class="bench-workbench__combo-controls" aria-label="Combination aggregate controls">
          <label class="bench-workbench__control">
            <span>Task</span>
            <select data-bw-combo-task><option value="">All tests</option></select>
          </label>
          <label class="bench-workbench__control">
            <span>Model</span>
            <select data-bw-combo-model><option value="">All models</option></select>
          </label>
          <label class="bench-workbench__control">
            <span>GPU</span>
            <select data-bw-combo-gpu><option value="">All GPUs</option></select>
          </label>
          <label class="bench-workbench__toggle">
            <input type="checkbox" data-bw-combo-passed-only>
            <span>Passed only</span>
          </label>
        </section>
        <div class="bench-workbench__aggregate-table" data-bw-combinations></div>
      </section>
    </div>
  </div>
</div>
