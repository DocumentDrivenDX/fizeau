---
ddx:
  id: SD-014
  created: 2026-05-12
  depends_on:
    - FEAT-006   # standalone CLI (fiz) — provides the snapshot data via fiz-bench
    - SD-009     # benchmark-mode runtime + preset
    - SD-010     # harness matrix benchmark (data shape this site consumes)
    - SD-012     # benchmark evidence ledger (validity story)
    - ADR-010    # reasoning wire form (drives provenance pillar)
---

# Solution Design: SD-014 — Benchmark Site Information Architecture

**Status:** Accepted
**Owner:** Fizeau Team
**Supersedes:** the planning record at
`docs/website/benchmark-redesign-plan-2026-05-12.md` (kept as the design
exploration; SD-014 is the resolved spec implementations cite).

## Summary

This spec defines the information architecture, data layer, and validity
rules for the `/benchmarks/` section of the Fizeau microsite. It is the
governing artifact for implementation beads under the `W-EPIC` thread.

The site serves two distinct stories with different audiences plus an
operator-facing data explorer and a provenance pillar that documents
"what it took to get here."

## 1. The two stories (top-level pillars)

**Story 1 — `/benchmarks/harness-comparison/`.** Audience: someone
evaluating which agent harness to use. They've heard of claude-code,
codex, opencode, pi, gemini-cli. The pillar answers "Is Fizeau a
viable agent for solving Terminal-Bench tasks, and how does it compare
to frontier harnesses?"

**Story 2 — `/benchmarks/model-landscape/`.** Audience: someone who's
chosen Fizeau and now needs to operationally tune. The pillar answers
"Given Fizeau, what model + provider + hardware combination should I
run? Local vs cloud? RTX 3090 Ti vs RTX 5090 mobile vs M2 Ultra vs M1
Max? oMLX vs Rapid-MLX vs llama.cpp vs ds4?"

These are different decision points and different visitors. The site
nav reflects them as distinct pillars; both are top-level and both
must have left-hand navigation populated by their child pages.

## 2. Information architecture

```
/benchmarks/                          PILLAR LANDING (frames both stories)
├── /harness-comparison/              STORY 1
│   ├── _index.md                     Headline + paired-comparison table
│   ├── vs-claude-code.md             Pair: fiz/anthropic-direct vs claude-code
│   ├── vs-codex.md                   Pair: fiz/openai-direct vs codex
│   ├── vs-opencode.md                Pair: fiz/X vs opencode/X
│   ├── vs-pi.md                      Pair: fiz/X vs pi/X
│   └── vs-gemini-cli.md              Pair: fiz/gemini-* vs gemini-cli
├── /model-landscape/                 STORY 2
│   ├── _index.md                     Headline matrix
│   ├── local-vs-cloud.md             Pareto frontier
│   ├── hardware.md                   GPU + Apple Silicon comparison
│   ├── backends.md                   Inference engine comparison
│   ├── reliability.md                invalid_class distribution per lane
│   └── reasoning-control.md          Wire-fix evidence + reasoning_tokens
├── /provenance/                      "WHAT IT TOOK TO GET HERE"
│   ├── _index.md                     Why this pillar exists
│   ├── changelog.md                  Date-ordered fixes + discoveries
│   ├── reasoning-control-saga.md     Wire-format dialect investigation
│   ├── timeout-calibration.md        QWEN_BASE × LANE_PENALTY framework
│   ├── token-accounting.md           reasoning_content fallback story
│   ├── classification.md             invalid_* taxonomy + evolution
│   └── stack-notes/                  PER-(engine, model) operator reference
│       ├── openrouter-qwen3.md       OR's effort flat-mapping; max_tokens honor
│       ├── llamacpp-qwen3.md         chat_template_kwargs envelope; REASONING_FORMAT=auto
│       ├── ds4-deepseek.md           alias collapse; flat reasoning_effort wire
│       └── ...                       (one per engine × model class, NOT physical machine)
├── /explorer/                        DYNAMIC raw-data viewer (DataTables.js)
├── /methodology/                     SHARED methodology (HOW we measure)
└── /reports/                         PER-RUN detail (raw audit trail)
    ├── /terminal-bench-2-1/
    └── /...
```

Hugo+Hextra renders left-hand nav per section automatically. Pillar
pages that route to children must populate nav from those children.

**Methodology vs Provenance split:**
- **Methodology** = HOW we measure. Invariant. Reproducibility
  reference: schemas, validity rules, task subsets.
- **Provenance** = WHAT IT TOOK. Historical. Operator-facing
  war-stories, gotchas, per-(engine, model) setup notes.

## 3. Lane identity — characteristics, not internal names

**Invariant:** every public-facing surface (story pages, explorer
columns, charts, captions) describes a lane by its characteristic
descriptor, never by its internal slug. Hardware-coupled internal lane
names (`sindri-club-3090-llamacpp`, `vidar-ds4`, `bragi-qwen3-6-27b`,
etc.) survive only as URL anchors and as a default-hidden explorer
column for operator debugging.

**Required characteristic dimensions** for any lane that appears in
public documentation:

| Dimension | Source |
|---|---|
| Model | `metadata.model_display_name` |
| Quant | `metadata.quant_display` |
| Weight bits | `metadata.weight_bits` |
| KV cache config | `metadata.kv_cache_quant`, `metadata.kv_cache_disk` |
| Inference engine | `metadata.engine` (renamed from `runtime`) |
| Engine version | `metadata.engine_version` (from /props or build-time) |
| GPU | `machines.yaml.<host>.hardware_profile.chip` |
| Platform | `machines.yaml.<host>.hardware_profile.chip_family` |
| VRAM | `machines.yaml.<host>.hardware_profile.vram_gb` |
| Memory | `machines.yaml.<host>.hardware_profile.memory_gb` |
| TDP (operational) | `machines.yaml.<host>.tdp_watts_configured` (preferred) or `hardware_profile.tdp_watts_spec` |
| Sampling params | `sampling.{temperature, top_p, top_k}` |
| Reasoning | `sampling.reasoning` |

**Public descriptor format** (composed by aggregator):
```
Qwen3.6 27B · Q3_K_XL (Unsloth GGUF) · llama.cpp · RTX 3090 Ti (24 GB) · 225 W
DeepSeek V4 Flash · IQ2XXS-mixed-Q2K-Q8 · ds4-server · M2 Ultra (192 GB) · 200 W · KV on disk 128 GB
Qwen3.6 27B · cloud-hosted · OpenRouter
```

**Lanes excluded from public documentation** (per operator decision
2026-05-12): hardware-coupled internal lane names. They appear only as
internal IDs in URLs and the explorer's hidden column.

## 4. Validity rules — the noise filter

A lane qualifies for inclusion in story pages and the default
explorer view when ALL of:

1. **Coverage:** ≥3 reps on each task in the displayed task subset.
   Lane appears for tasks where it meets this bar; absent for those
   it doesn't.
2. **Reliability ceiling:** <20% of cells classified
   `invalid_class != null`. Above that, lane is infrastructure-broken,
   not model-failing; suppressed from public views.
3. **Reasoning-token coverage** (only for reasoning-capable models):
   ≥80% of cells have `reasoning_tokens > 0`. Catches lanes where the
   wire is wrong (silently no thinking).
4. **Valid-since gate:** Cells dated before the lane's `valid_since`
   timestamp are excluded. `valid_since` is per-lane, declared in
   the profile YAML, and reflects when fizeau started emitting the
   correct wire and capturing the correct telemetry for that lane.
5. **Required metadata complete:** profile YAML declares every
   characteristic dimension in §3. Profiles missing any required
   field are excluded with a structured "metadata incomplete" reason.

Lanes that fail any criterion appear in `aggregates-rejected.json`
with the reason; the reliability page surfaces the rejected list with
explanations so the absence is honest, not hidden.

## 5. Data layer

### Build-time aggregator

`scripts/website/build-benchmark-data.py` walks
`benchmark-results/fiz-tools-v1/cells/<suite>/<task>/<lane>/rep-N/report.json`
and produces:

- `microsite/static/data/cells.json` — every cell, normalized schema
- `microsite/static/data/cells-valid.json` — same, validity-filtered
- `microsite/static/data/aggregates.json` — pre-computed per-(engine,
  model, hardware) summaries for story-page tables/charts
- `microsite/static/data/aggregates-rejected.json` — lanes excluded
  with reasons

### Cell record schema

```json
{
  "schema_version": 1,
  "task": "fix-ocaml-gc",
  "suite": "terminal-bench-2-1",
  "harness": "fiz",
  "internal_lane_id": "sindri-club-3090-llamacpp",
  "descriptor": "Qwen3.6 27B · Q3_K_XL (Unsloth GGUF) · llama.cpp · RTX 3090 Ti (24 GB) · 225 W",
  "model": "Qwen3.6-27B-UD-Q3_K_XL.gguf",
  "model_display_name": "Qwen3.6 27B",
  "model_family": "qwen3-6-27b",
  "tier": "smart",
  "quant_display": "Q3_K_XL (Unsloth GGUF)",
  "weight_bits": 3,
  "kv_cache_quant": "default",
  "kv_cache_disk": false,
  "engine": "llama.cpp",
  "engine_version": "b9014-d4b0c22f9",
  "hardware_chip": "nvidia-rtx-3090-ti",
  "hardware_chip_family": "nvidia-cuda",
  "hardware_vram_gb": 24,
  "hardware_memory_gb": 96,
  "hardware_tdp_watts": 225,
  "hardware_tdp_source": "configured",
  "rep": 3,
  "final_status": "graded_pass",
  "invalid_class": null,
  "wall_seconds": 1989,
  "reasoning_tokens": 10734,
  "reasoning_tokens_approx": false,
  "input_tokens": 78000,
  "output_tokens": 3200,
  "cost_usd": 0,
  "turns": 28,
  "started_at": "...",
  "finished_at": "...",
  "valid_lane": true,
  "lane_validity_reasons": []
}
```

Hardware fields (`hardware_*`) come from joining the cell's profile
YAML against `scripts/benchmark/machines.yaml`. Internal lane id stays
in the record but is NOT a public column.

### Aggregate record schema

`aggregates.json` is keyed by `(engine, model_display_name,
hardware_chip)` (the public descriptor's load-bearing dimensions) with:

- counts: `n_cells`, `n_pass`, `n_fail`, `n_invalid`
- rates: `pass_rate`, `invalid_rate`
- walls: `wall_p50`, `wall_p95`, `wall_max`
- cost: `cost_p50`, `cost_total`
- reasoning: `reasoning_tokens_p50`
- coverage: `tasks_covered`, `tasks_with_min_3_reps`
- validity: `valid_lane` (bool), `validity_reasons`

## 6. Explorer

Single-page DataTables.js interface loading the JSON feeds.

**Default columns** (visible):
- task | suite | harness | model | quant | engine | hardware | rep |
  final_status | invalid_class | wall_seconds | reasoning_tokens |
  cost_usd | turns | finished_at

**Hidden columns** (toggleable):
- internal_lane_id | model_id (provider's canonical) | engine_version |
  weight_bits | kv_cache_quant | kv_cache_disk | hardware_tdp_watts |
  reasoning_tokens_approx | started_at

**Default filters:**
- `valid_lane = true` (toggle off to show all)
- Default sort: `finished_at` desc

**Pre-built saved views** (linkable from story pages): "Story 1 raw
data," "Story 2 raw data," "Recent failures," "By engine."

**Implementation:** DataTables.js + jQuery via CDN. Cell count is
bounded (~5K cells per benchmark suite); client-side handling fits.
Migrate to Datasette-lite (WASM SQLite) only if cell count outgrows
client-side capacity.

## 7. Methodology page

`/benchmarks/methodology/` documents:

- TerminalBench-2.1 overview: tasks, what they test, grading method
- Harbor wrapper: per-cell invocation contract
- Cell schema: every field documented
- Task-base wall budget rationale (QWEN_BASE × LANE_PENALTY framework)
- Validity rules from §4, with rationale per
- Reproducibility: how to reproduce a single cell, how to reproduce a
  lane sweep
- Versioning: cells include `fiz_tools_version` and
  `profile_snapshot` so historical comparisons are tractable
- ADR cross-references: ADR-010 reasoning wire, ADR-007 sampling
  catalog, ADR-009 routing surface, ADR-012 cache

## 8. Provenance pillar

`/benchmarks/provenance/` documents the historical record. Distinct
from methodology: methodology is the formal contract; provenance is
the war-stories and operator gotchas.

- **changelog.md** — date-ordered fixes + discoveries with cell-level
  evidence
- **reasoning-control-saga.md** — the wire-format dialect investigation
  (OR Qwen3 effort flat-mapping → kwargs envelope → ds4 alias collapse
  → introspection moment → L1/L2/L3 architecture per ADR-010)
- **timeout-calibration.md** — QWEN_BASE × LANE_PENALTY framework
  derivation, terminated_mid_work classification
- **token-accounting.md** — usage path vs reasoning_content fallback;
  why `_approx=true` is honest
- **classification.md** — invalid_* taxonomy + evolution
- **stack-notes/** — per-(engine, model-class) operator reference
  (NOT per physical machine). Each entry documents wire quirks, known
  limitations, operator setup checklist, current status.

## 9. Per-page data flow

| Page | Consumes | Build-time |
|---|---|---|
| Pillar landings (`_index.md`) | `aggregates.json` headline numbers | Hugo template + `getJSON` |
| Story 1 sub-pages | `aggregates.json` filtered to harness pairs | Hugo template + Python-generated SVG charts |
| Story 2 sub-pages | `aggregates.json` filtered to engine/hardware/backend cuts | Hugo template + SVG charts |
| Reliability page | `aggregates.json` + `aggregates-rejected.json` | Hugo template (transparent rejection list) |
| Methodology + provenance | Static markdown | None (pure docs) |
| Explorer | `cells-valid.json` (default) + `cells.json` (toggle) | DataTables.js client-side |
| Reports per-suite | Existing per-run reports | None — preserved as-is |

## 10. Implementation phasing

| # | Title | Scope |
|---|---|---|
| **W-EPIC** | Benchmark site redesign — two stories + provenance + dynamic explorer | parent |
| **W1** | Build-time aggregator + validity rules + cell schema | foundation; produces cells.json + aggregates.json |
| **W2** | Methodology page | documents §4 validity, §5 schemas, reproducibility |
| **W3** | Story 1 pages — harness comparison pillar + sub-pages | depends on W1 |
| **W4** | Story 2 pages — model landscape pillar + sub-pages | depends on W1 |
| **W5** | Explorer with DataTables.js | depends on W1 |
| **W6** | Provenance pillar + stack-notes | self-contained docs; can land first |
| **W7** | Profile YAML metadata backfill (per §3 required dimensions) | prerequisite to W1's aggregator passing the validity gate for all worth-including lanes |

Dependency: W7 → W1 → (W2, W3, W4, W5). W6 is documentation-heavy
and depends on none of the others.

## 11. Decisions captured (operator review 2026-05-12)

1. **Page granularity:** flat URLs under `/benchmarks/`; pillar pages
   must have left-hand nav.
2. **Methodology placement:** dedicated page (not inline-only).
3. **Comparison fairness lenses:** all measured lenses get coverage
   (model + tool surface + harness behavior + retry semantics).
4. **Explorer scope:** "valid data" only by default (per §4); toggle
   to show all.
5. **Update cadence:** manual `make site` rebuild for v1; CI later.
6. **Charts:** SVG for story-page headlines (server-rendered);
   interactive table for the explorer.
7. **Lane identity:** characteristic descriptors only in public
   surfaces (per §3); hardware-coupled internal lane names suppressed.
8. **Power tracking:** `tdp_watts_configured` per machine (operational)
   plus `tdp_watts_spec` per hardware profile (chip max). Aggregator
   uses configured if present, else spec.
9. **Future deferred:** per-run TDP capture via nvidia-smi /
   powermetrics — filed as a future bead.

## 12. Out of scope

- Real-time updates (operator triggers `make site`).
- Multi-machine cache sharing for the data layer (per-host build).
- OR sub-provider (`@<sub_provider>`) expansion in lane identity —
  deferred to the M5 bead in the model-snapshot epic; site uses the
  parent OR descriptor for now.
- Pre-rendered chart artifacts in git — charts are build-time
  generated, with `make site-charts` for local review.
- Authentication / private-data publication — TB-2.1 is public, this
  spec assumes public publication. Future suites with sensitive
  prompts get separate review.
