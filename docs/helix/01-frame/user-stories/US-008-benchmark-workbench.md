---
ddx:
  id: US-008
  created: 2026-05-15
  depends_on:
    - FEAT-008
---
# User Stories: Benchmark Workbench

## US-008-001 - Inspect Every Result-Bearing Cell

As a benchmark operator, I want one table where every timeout, graded
pass, and graded failure is available, so that I can audit benchmark
outcomes without reverse-engineering which curated report omitted a
field.

**Acceptance Criteria**

- The explorer loads `cells.parquet`, not the large JSON artifact, for
  the primary workbench.
- The grid contains every column in the Parquet cell table.
- The Parquet cell table excludes invalid and non-result-bearing
  source reports; the manifest exposes exclusion counts for audit.
- The default visible columns are useful for outcome, model, quant,
  runtime, hardware, resource, and provenance inspection.
- Users can add hidden columns without rebuilding the site.
- The raw database pane can be opened directly through a URL hash and
  contains the controls that affect the raw grid.

## US-008-002 - Find Passing Combinations

As an operator comparing local runtimes, I want to filter to passing
cells for a selected test, selected GPU, or maximum GPU RAM, so that I
can identify viable model/runtime/hardware combinations.

**Acceptance Criteria**

- A selected test/task filter can be combined with pass-only.
- A selected GPU filter can be combined with pass-only.
- A maximum GPU RAM filter can be combined with pass-only.
- The aggregate combination table updates from its colocated task,
  model, GPU, and pass-only controls and reports passes, failures,
  timeouts, pass rate, cost, tokens, and wall p50.
- The combination table's column headers sort the aggregate rows in
  place.

## US-008-003 - Investigate Cost and Resource Use

As an operator planning a sweep, I want token, cost, and wall-time
aggregates to update as filters change, so that I can estimate the
budget and runtime of comparable configurations.

**Acceptance Criteria**

- Current row count, pass rate, timeout count, total tokens, known cost
  total, distinct models, distinct GPUs, and wall p50 are visible.
- Unknown costs remain absent from cost totals rather than coerced to
  zero.
- Runtime and token fields remain available as sortable/filterable grid
  columns.
- The summary pane shows unfiltered dataset shape and distribution
  charts before users begin narrowing the raw database.

## US-008-004 - Preserve Curated Reports

As a reader of the public benchmark pages, I want the workbench to live
alongside the curated benchmark story, so that narrative pages stay
readable while raw data remains available for deeper inspection.

**Acceptance Criteria**

- `/benchmarks/` links to `/benchmarks/explorer/`.
- Existing `/benchmarks/terminal-bench-2-1/` report pages still render.
- The legacy static table enhancer remains scoped to report tables and
  does not interfere with the workbench grid.

## US-008-005 - Locate Pairwise Model Gaps

As a curious benchmark reader, I want to compare two model families and
see where their pass-rate gap is largest, so that I can understand
whether a headline delta comes from task type, task difficulty,
individual tasks, runtime, hardware, provider, or harness effects.

**Acceptance Criteria**

- The explorer has independent baseline and compare selectors for
  model families.
- The comparison can group by task category, task difficulty,
  individual task, outcome, engine, model quant, deployment class,
  GPU/GPU vendor, provider, and harness.
- The comparison table reports baseline pass rate, comparison pass
  rate, pass-rate gap in percentage points, row counts, graded-fail
  counts, timeout counts, token totals, and p50 wall time.
- Individual task rows link to the canonical Terminal-Bench task page.
- The comparison table's column headers sort the displayed buckets in
  place.
- The comparison pane can be deep-linked and keeps comparison controls
  colocated with the pairwise table.

## US-008-006 - Navigate Analytical Panes

As a benchmark reader, I want the explorer to have stable local
navigation for summary, raw data, comparison, and combinations, so that
I can move among related analytical modes without hunting through one
long stacked page.

**Acceptance Criteria**

- The explorer defaults to the Summary pane.
- Summary, Raw database, Comparison, and Combinations panes are addressable
  via URL hashes.
- The left-hand local navigation remains visible across pane changes on
  desktop and collapses to a compact top nav on narrow screens.
- Each pane contains the controls for the table or charts it changes.
