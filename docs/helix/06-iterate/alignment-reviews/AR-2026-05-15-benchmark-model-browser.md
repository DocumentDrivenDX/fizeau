# Alignment Review: Benchmark Model Browser

**Review Date**: 2026-05-15  
**Scope**: FEAT-008 benchmark workbench model-browser surface  
**Status**: complete  
**Review Epic**: fizeau-607adacc  
**Primary Governing Artifact**: docs/helix/01-frame/features/FEAT-008-benchmark-workbench.md

## Scope and Governing Artifacts

### Scope

- Result-bearing benchmark cell datatable and generated artifacts.
- Model picker, low/medium-cardinality filters, saved views, and active-filter affordances.
- Pairwise model-family gap analysis for questions such as Qwen versus Claude/Anthropic.
- Terminal-Bench task links from task cells.
- Benchmark workbench layout against `website/DESIGN.md`.
- Browser verification and HELIX follow-up coverage.

### Governing Artifacts

- `docs/helix/01-frame/features/FEAT-008-benchmark-workbench.md`
- `docs/helix/01-frame/user-stories/US-008-benchmark-workbench.md`
- `docs/helix/02-design/adr/ADR-015-browser-analytical-benchmark-workbench.md`
- `docs/helix/02-design/solution-designs/SD-015-benchmark-workbench.md`
- `website/DESIGN.md`
- `docs/helix/01-frame/concerns.md`

## Intent Summary

- **Vision**: The benchmark page should be a measurement instrument. The data is the headline, provenance is visible, and differences should be easier to read than absolute claims.
- **Requirements**: FEAT-008 requires a static, browser-side analytical workbench over the full result-bearing benchmark cell table. Every collected column remains available, invalid/non-result rows are excluded but audited, and common filters/aggregates answer viability and comparison questions.
- **Features / Stories**: US-008 covers inspection of all valid cells, passing-combination discovery, resource/cost investigation, curated-report preservation, and pairwise model-family gap discovery.
- **Architecture / ADRs**: ADR-015 chooses DuckDB-WASM over Parquet plus Perspective for a no-server analytical browser workload.
- **Technical Design**: SD-015 defines the Hugo page, DuckDB views, Perspective grid, filter controls, pairwise SQL, default columns, and design constraints.
- **Test Plans**: SD-015 requires local build plus browser verification. No checked-in browser smoke exists yet.
- **Implementation Plans**: Current implementation is direct: regenerate static benchmark data, bundle the browser module, render the Hugo page, and smoke-test the explorer in a browser.

## Planning Stack Findings

| Finding | Type | Evidence | Impact | Review Issue |
|---------|------|----------|--------|--------------|
| Pairwise gap story was missing before this pass and is now explicit. | underspecified resolved | `FEAT-008:79`, `US-008:71`, `SD-015:154`, `ADR-015:77` | The "where is the 20 percent gap?" question now has first-class requirements, selectors, dimensions, and SQL design. | fizeau-76d18e46 |
| Low/medium-cardinality filters are now promoted into native controls. | underspecified resolved | `FEAT-008:64`, `SD-015:126`, `website/content/benchmarks/explorer/_index.md:61` | Operators can answer common model/runtime/hardware questions without opening the grid configuration panel first. | fizeau-76d18e46 |
| Task metadata fallback was missing from the data design and builder. | incomplete resolved | `SD-015:84`, `scripts/website/build-benchmark-data.py:233`, `scripts/website/build-benchmark-data.py:623` | Pairwise grouping by task category/difficulty no longer collapses older reports into missing buckets. | fizeau-76d18e46 |
| The static site/data-pipeline stack is not declared in active HELIX concerns. | missing concern coverage | `docs/helix/01-frame/concerns.md`, `.ddx/plugins/helix/workflows/concerns/hugo-hextra/concern.md`, `.ddx/plugins/helix/workflows/concerns/python-uv/concern.md` | Future benchmark UI/data beads may miss the right Hugo, browser, and Python quality gates. | fizeau-96e8e4d6 |

## Implementation Map

- **Topology**: `scripts/website/build-benchmark-data.py` builds JSON/Parquet/manifest artifacts under `website/static/data/`; `website/assets/js/benchmark-workbench.js` owns the DuckDB/Perspective session; `website/content/benchmarks/explorer/_index.md` owns page markup; `website/assets/css/custom.css` owns site styling.
- **Entry Points**: `uv run --with PyYAML --with pyarrow --with duckdb python scripts/website/build-benchmark-data.py`, `npm run build:benchmark-workbench`, and `hugo --source website`.
- **Test Surfaces**: Python compile check, benchmark data regeneration, JS bundle build, Hugo build, and manual browser smoke against `/fizeau/benchmarks/explorer/`.
- **Unplanned Areas**: No orphaned workbench-specific code found. The remaining gap is test automation, not product behavior.

## Acceptance Criteria Status

| Story / Feature | Criterion | Test Reference | Status | Evidence |
|-----------------|-----------|----------------|--------|----------|
| FEAT-008 | Load `cells.parquet` directly in browser. | manual browser smoke | SATISFIED | `ADR-015:37`, `benchmark-workbench.js:323`, status reported 1,339 rows loaded. |
| FEAT-008 | Exclude invalid/non-result rows and count exclusions. | data-builder run | SATISFIED | `build-benchmark-data.py:518`, manifest has 1,339 rows and 2,672 excluded. |
| FEAT-008 | Expose all collected columns while keeping useful defaults. | manual browser smoke | SATISFIED | `benchmark-workbench.js:6`, `benchmark-workbench.js:633`; default columns omit `terminalbench_task_url`. |
| US-008-002 | Combine selected task/GPU/RAM/pass filters. | manual browser smoke | SATISFIED | `benchmark-workbench.js:557`, `benchmark-workbench.js:606`. |
| US-008-003 | Aggregate row count, pass rate, timeouts, tokens, cost, models, GPUs, p50 wall. | manual browser smoke | SATISFIED | `benchmark-workbench.js:718`; metrics updated from `workbench_cells`. |
| US-008-004 | Keep curated reports separate and link to explorer. | Hugo build | SATISFIED | `website/content/benchmarks/_index.md`; `hugo --source website` passed. |
| US-008-005 | Baseline/compare model-family selectors and comparison dimensions. | manual browser smoke | SATISFIED | `explorer/_index.md:221`, `benchmark-workbench.js:76`, `benchmark-workbench.js:747`. |
| US-008-005 | Pairwise table reports pass rates, gap, rows, fails, timeouts, tokens, p50 wall. | manual browser smoke | SATISFIED | `benchmark-workbench.js:829`; browser showed task-category gaps for Claude Sonnet 4 versus Qwen3-6-27B. |
| US-008-005 | Task rows link to Terminal-Bench task pages. | manual browser smoke | SATISFIED | `benchmark-workbench.js:230`, `benchmark-workbench.js:654`, `benchmark-workbench.js:885`, `benchmark-workbench.js:1030`. Pairwise task rows linked to `https://www.tbench.ai/registry/terminal-bench-core/head/<task>`. |
| SD-015 | Browser workbench smoke is repeatable and checked in. | none | UNTESTED | Manual smoke exists; tracked as fizeau-857f3b8e. |

## Gap Register

| Area | Classification | Planning Evidence | Implementation Evidence | Resolution Direction | Issue |
|------|----------------|-------------------|-------------------------|----------------------|-------|
| Raw result-bearing cells | ALIGNED | `FEAT-008:49`, `US-008:10`, `SD-015:66` | `build-benchmark-data.py:518`, `build-benchmark-data.py:793`, regenerated manifest | code-to-plan | fizeau-76d18e46 |
| Model picker and enum filters | ALIGNED | `FEAT-008:64`, `SD-015:126` | `explorer/_index.md:33`, `explorer/_index.md:61`, `benchmark-workbench.js:46`, `benchmark-workbench.js:557` | code-to-plan | fizeau-76d18e46 |
| Pairwise model-gap analysis | ALIGNED | `FEAT-008:79`, `US-008:71`, `SD-015:154` | `explorer/_index.md:221`, `benchmark-workbench.js:76`, `benchmark-workbench.js:747` | code-to-plan | fizeau-76d18e46 |
| Terminal-Bench task links | ALIGNED | `FEAT-008:86`, `US-008:88`, `SD-015:99` | `benchmark-workbench.js:230`, `benchmark-workbench.js:654`, `benchmark-workbench.js:885`, `benchmark-workbench.js:1030` | code-to-plan | fizeau-76d18e46 |
| Task category/difficulty coverage | ALIGNED | `SD-015:84`, `US-008:82` | `build-benchmark-data.py:233`, `build-benchmark-data.py:591`, regenerated data has 17 categories and 3 difficulty buckets with no missing category/difficulty values | code-to-plan | fizeau-76d18e46 |
| Workbench layout and density | ALIGNED | `FEAT-008:88`, `SD-015:200`, `website/DESIGN.md` | `custom.css:128`, `custom.css:686`, `custom.css:955`, `custom.css:989` | code-to-plan | fizeau-76d18e46 |
| Automated browser regression coverage | INCOMPLETE | `SD-015:233`, `ADR-015:118` | Manual WebDriver smoke only; no checked-in command | quality-improvement | fizeau-857f3b8e |
| HELIX concern coverage for site/data stack | INCOMPLETE | `docs/helix/01-frame/concerns.md`; available `hugo-hextra` and `python-uv` concerns | Workbench uses Hugo, npm/esbuild, DuckDB/Perspective, and Python/uv scripts | decision-needed | fizeau-96e8e4d6 |

### Quality Findings

| Area | Dimension | Concern | Severity | Resolution | Issue |
|------|-----------|---------|----------|------------|-------|
| Browser workbench | robustness | Current browser verification is manual and could regress invisible selectors, task links, or Perspective defaults. | medium | Add checked-in browser smoke/regression command. | fizeau-857f3b8e |
| HELIX concerns | maintainability | Active concerns do not cover the static microsite or Python benchmark-data builder even though HELIX has library concerns for both. | medium | Select/override site and data-pipeline concerns for benchmark work. | fizeau-96e8e4d6 |

## Traceability Matrix

| Vision | Requirement | Feature/Story | Arch/ADR | Design | Tests | Impl Plan | Code Status | Classification |
|--------|-------------|---------------|----------|--------|-------|-----------|-------------|----------------|
| Data is the headline. | Full valid cell datatable, all columns available. | US-008-001 | ADR-015 | SD-015 SQL Views, Default Columns | builder run, browser smoke | regenerate data and bundle workbench | implemented | ALIGNED |
| Medium matters. | Exclusion counts and generated artifact metadata visible. | US-008-001 | ADR-015 | SD-015 SQL Views | manifest inspection, browser smoke | manifest emitted with source counts | implemented | ALIGNED |
| Differences are easier to read than absolutes. | Pairwise gap table by task/runtime/hardware/provider. | US-008-005 | ADR-015 | SD-015 Pairwise Gap Query | browser smoke | DuckDB SQL comparison panel | implemented | ALIGNED |
| Dense scientific instrument UI. | Full-width grid, no marketing layout, stable readout panels. | FEAT-008 | ADR-015 | SD-015 Design Constraints | Hugo build, browser smoke | CSS scoped to workbench | implemented | ALIGNED |
| Validate your work. | Repeatable workbench browser verification. | SD-015 | ADR-015 | SD-015 Verification | manual only | follow-up test bead | not yet automated | INCOMPLETE |

## Execution Issues Generated

| Issue ID | Type | Labels | Goal | Dependencies | Verification |
|----------|------|--------|------|--------------|--------------|
| fizeau-857f3b8e | task | helix, area:benchmark, area:ui, kind:test, phase:build, spec:FEAT-008 | Add checked-in browser smoke covering row load, model picker, enum filters, pairwise gap table, task links, and no default `terminalbench_task_url` column. | fizeau-425aa611 | Repository command/script passes locally and is documented. |
| fizeau-96e8e4d6 | task | helix, phase:frame, kind:planning, area:benchmark, area:ui, area:data, spec:FEAT-008 | Decide and declare active HELIX concerns/practices for Hugo/Hextra site work and Python benchmark-data pipeline work. | fizeau-425aa611 | `concerns.md` and open benchmark beads reflect the selected practices and gates. |

## Issue Coverage

| Gap / Criterion | Covering Issue | Status |
|-----------------|----------------|--------|
| Checked-in workbench browser smoke | fizeau-857f3b8e | covered |
| Site/data-pipeline HELIX concern coverage | fizeau-96e8e4d6 | covered |

## Verification

- `uv run --with PyYAML --with pyarrow --with duckdb python scripts/website/build-benchmark-data.py`
  - Wrote 1,339 `cell_rows` and 657 `task_combinations`; manifest reports 2,672 excluded source reports.
- `npm run build:benchmark-workbench`
  - Passed; bundle emitted to `website/static/js/benchmark-workbench.js`.
- `hugo --source website`
  - Passed with the existing Hugo `.Site.Data` deprecation warning only.
- `python3 -m compileall -q scripts/website/build-benchmark-data.py`
  - Passed.
- Manual WebDriver smoke against `http://127.0.0.1:1314/fizeau/benchmarks/explorer/`
  - Status showed `1,339 rows loaded from 1,339 valid cells at 2026-05-15T03:57:36Z; 2,672 excluded`.
  - Default visible columns did not include `terminalbench_task_url`.
  - Model picker and category/difficulty filters populated.
  - Pairwise default compared `claude-sonnet-4` to `qwen3-6-27b`; task-category gaps included system-administration, software-engineering, and data-processing with no missing category bucket.
  - Pairwise task rows linked to canonical Terminal-Bench task pages.

## Execution Order

1. fizeau-857f3b8e - add the browser smoke first so subsequent UI changes have a regression gate.
2. fizeau-96e8e4d6 - declare site/data concerns and wire future benchmark beads to those practices.

**Critical Path**: browser smoke coverage before more interaction polish.  
**Parallel**: concern declaration can proceed in parallel with later feature polish.  
**Blockers**: none for the current model-browser implementation.

## Open Decisions

| Decision | Why Open | Governing Artifacts | Recommended Owner |
|----------|----------|---------------------|-------------------|
| Whether to activate `hugo-hextra`, `python-uv`, both, or project-specific overrides for benchmark site/data work. | The current concerns file only declares Go/testing, while benchmark work now has a durable frontend/data pipeline. | `docs/helix/01-frame/concerns.md`, fizeau-96e8e4d6 | Fizeau maintainers |
