# Benchmark Section Redesign — Plan (Design Exploration)

| Date | Status | Owner |
|---|---|---|
| 2026-05-12 | Resolved → SD-014 | (operator) |

> **Note:** This document is the design exploration record. The
> resolved spec that implementation cites is
> [SD-014](../helix/02-design/solution-designs/SD-014-benchmark-site-information-architecture.md).
> Open questions and discussion in this document have been resolved
> in §11 of SD-014. Kept here for the working record.

## The two stories

**Story 1 — Is Fizeau a viable agent?**
Audience: someone evaluating which agent harness to use. They've heard
of claude-code, codex, opencode, pi, gemini-cli. Why Fizeau?

**Story 2 — Given Fizeau, which model + provider + hardware do I run?**
Audience: someone who's chosen Fizeau and now needs to operationally
tune. Local vs cloud? RTX 5090 vs M2 Ultra? oMLX vs llama.cpp vs
Rapid-MLX? Honest cost/wall/quality data, not vendor claims.

The site nav reflects these as separate top-level pillars because they
serve different decision points and different audiences.

## Information architecture

```
/benchmarks/
├── _index.md                         Pillar landing — frames both stories
├── /harness-comparison/              STORY 1 pillar
│   ├── _index.md                     Headline + table + caveats
│   ├── vs-claude-code.md             Pair: fiz/anthropic-direct vs claude-code
│   ├── vs-codex.md                   Pair: fiz/openai-direct vs codex
│   ├── vs-opencode.md                Pair: fiz/X vs opencode/X
│   ├── vs-pi.md                      Pair: fiz/X vs pi/X
│   └── vs-gemini-cli.md              Pair: fiz/gemini-* vs gemini-cli
├── /model-landscape/                 STORY 2 pillar
│   ├── _index.md                     Headline matrix
│   ├── local-vs-cloud.md             Pareto frontier; wall × pass-rate scatter
│   ├── hardware.md                   M2 Ultra vs RTX 5090 vs RTX 3090
│   ├── backends.md                   oMLX vs Rapid-MLX vs llama.cpp vs ds4
│   ├── reliability.md                invalid_class distribution per lane
│   └── reasoning-control.md          Wire-fix evidence + reasoning_tokens
├── /provenance/                      "WHAT IT TOOK TO GET HERE" pillar
│   ├── _index.md                     Why this section exists
│   ├── changelog.md                  Date-ordered list of fixes + discoveries
│   ├── reasoning-control-saga.md     The wire-format dialect investigation
│   ├── timeout-calibration.md        QWEN_BASE × LANE_PENALTY derivation
│   ├── token-accounting.md           reasoning_content fallback story
│   ├── classification.md             invalid_* taxonomy + how it evolved
│   └── stack-notes/                  PER-(engine, model) operator reference
│       ├── openrouter-qwen3.md       OR's effort-tier flat-mapping; max_tokens honor
│       ├── llamacpp-qwen3.md         chat_template_kwargs envelope; REASONING_FORMAT=auto persistence
│       ├── ds4-deepseek.md           alias collapse; flat reasoning_effort wire; max requires --ctx≥393216
│       ├── omlx-qwen3.md             retired 2026-05-11 in favor of ds4
│       ├── rapid-mlx-qwen3.md        ...
│       └── ...                       (one per engine × model class, NOT per physical machine)
├── /explorer/                        DYNAMIC raw-data viewer (DataTables.js)
├── /methodology/                     SHARED methodology page (HOW we measure)
└── /reports/                         PER-RUN detail
    ├── /terminal-bench-2-1/          ← existing, moved here
    └── /...
```

Note the split between **methodology** (HOW we measure — invariant)
and **provenance** (WHAT it took — historical, evolving). They serve
different reader needs:

- Methodology: "I want to reproduce or extend these benchmarks." →
  formal definitions, schemas, validity rules.
- Provenance: "I want to understand why my own setup is misbehaving."
  → real war-stories, gotchas, what we discovered the hard way.

Hextra renders left-hand nav per section automatically. Pillar pages
(`harness-comparison/_index.md`, `model-landscape/_index.md`) get their
own subnav from the child pages. URLs flat under `/benchmarks/` to keep
SEO tidy.

## Lane identity — characteristics, not names

End users have never heard of `sindri-club-3090-llamacpp` and can't
reproduce or compare it without unpacking what each token means.
Internal lane names are operator vocabulary for routing benchmarks at
specific physical machines; they are NOT what end users want to read
or filter by.

Every public-facing surface (story pages, explorer columns, charts,
captions) describes lanes by their **characteristic descriptors**:

| Dimension | Examples |
|---|---|
| **Model** | Qwen3.6-27B, DeepSeek V4 Flash, GPT-5.5, Claude Sonnet 4.6 |
| **Quant** | Q3_K_XL (Unsloth GGUF), MLX-8bit, IQ2XXS-mixed-Q2K-Q8, BF16 native |
| **KV cache config** | default, on-disk 128GB (Samsung SSD), Q8 quant |
| **Inference engine** | llama.cpp (b9014-d4b0c22f9), vLLM 0.20.2rc1, oMLX, Rapid-MLX, ds4-server (antirez/ds4@ae302c2f), OpenRouter (cloud) |
| **GPU / Platform** | RTX 3090 (24GB), RTX 5090 (32GB), M2 Ultra (192GB unified), cloud |
| **Sampling** | T=0.6, top_p=0.95, top_k=20 (Qwen3 thinking-mode preset) |
| **Reasoning** | budget=4096 tokens (`reasoning.max_tokens`), effort=high (alias-collapsed), thinking off |

**Operator-only internal names** like `sindri-club-3090-llamacpp` and
`vidar-ds4` are URL-slug identifiers and explorer-default-hidden
columns. They never appear in headlines, table captions, or chart
legends.

### Required lane metadata (machine-readable)

Each profile must declare these fields so the aggregator can produce
the public descriptors. Profiles missing fields = lane is excluded
from public views (per the validity rule below). Today's
`scripts/benchmark/profiles/<lane>.yaml` files have most of this in
`metadata:` and `sampling:`; some need backfilling.

```yaml
metadata:
  # Model identity
  model_family: qwen3-6-27b           # for grouping
  model_display_name: "Qwen3.6 27B"   # public-facing
  model_id: Qwen3.6-27B-UD-Q3_K_XL.gguf  # provider-side ID

  # Quantization
  quant_label: gguf-q3-k-xl-unsloth         # internal slug
  quant_display: "Q3_K_XL (Unsloth GGUF)"   # public-facing
  weight_bits: 3                            # for power/size sort

  # KV cache
  kv_cache_quant: default                   # default | q8 | q4 | fp16
  kv_cache_disk: false                      # bool
  kv_cache_disk_budget_mb: null             # only if kv_cache_disk: true

  # Inference engine
  engine: llama.cpp                         # llama.cpp | vllm | mlx | omlx | rapid-mlx | ds4 | openrouter | openai-direct | anthropic-direct
  engine_version: b9014-d4b0c22f9           # build/commit; from /props or static

  # Hardware (resolved via machines.yaml hardware_profiles registry)
  hardware_profile: nvidia-rtx-3090-club    # references machines.yaml
  # → hardware_profile resolves to: { gpu: "RTX 3090", vram_gb: 24,
  #   platform: "Linux x86_64 CUDA", host_class: "workstation" }

# Sampling and reasoning already in sampling: block; aggregator pulls
# from there.
sampling:
  temperature: 0.6
  top_p: 0.95
  top_k: 20
  reasoning: 4096
```

### Public lane descriptor (derived, in cell records)

The aggregator composes these into a single human-readable descriptor
per lane:

```
Qwen3.6 27B · Q3_K_XL (Unsloth GGUF) · llama.cpp · RTX 3090 (24GB)
DeepSeek V4 Flash · IQ2XXS-mixed-Q2K-Q8 · ds4-server · M2 Ultra (192GB) · KV on disk 128GB
Qwen3.6 27B · cloud-hosted · OpenRouter
GPT-5.5 · cloud-hosted · OpenAI direct
```

The descriptor IS the lane identity in public surfaces. Internal lane
slugs (`sindri-club-3090-llamacpp`) appear only in:
- URL paths (so links remain stable)
- Explorer's "internal_lane_id" column (hidden by default; exposed
  for operator debugging)
- The reproducibility section of methodology page

### Lanes excluded from public documentation

Per operator direction: hardware-coupled lane names (anything
referencing a specific physical machine — `sindri-*`, `vidar-*`,
`bragi-*`, `grendel-*`) do not appear in any user-facing prose,
table caption, or chart legend. They appear only as internal IDs in
URLs and the explorer's hidden column.

The descriptor `RTX 3090 (24GB)` is what readers see; whether that
particular RTX 3090 is hosted on the operator's `sindri` machine
or someone else's identical hardware is irrelevant to the comparison.

## Data validity rules — the noise filter

Per operator direction: only "valid data" appears in story pages. Lanes
with insufficient coverage, infrastructure issues, or pre-fix wire bugs
are dropped (not greyed out, not "noise" — actually absent).

**A lane qualifies for inclusion when ALL of:**

1. **Coverage:** ≥3 reps on each task in the displayed task subset.
   Lane appears in tables only for the tasks where it meets this bar.
2. **Reliability ceiling:** <20% of cells classified `invalid_class !=
   null` (any invalid). Above that = lane is infrastructure-broken,
   not model-failing.
3. **Reasoning-token coverage** (only for reasoning-capable models):
   ≥80% of cells have `reasoning_tokens > 0`. Catches lanes where the
   wire is wrong (silently no thinking).
4. **Valid-since gate:** Cells dated before the lane's `valid_since`
   timestamp are excluded. Per-lane:
   - `fiz-openrouter-qwen3-6-27b`: valid_since `2026-05-11 22:00 UTC`
     (post-wire-fix `cfdcdcc4`, post-token-extraction `fizeau-8f62bcbb`)
   - `fiz-sindri-club-3090-llamacpp-qwen3-6-27b`: same
   - `fiz-vidar-ds4`: same
   - `fiz-vidar-omlx-qwen3-6-27b`: deprecated lane (replaced by ds4);
     valid_since `null` → excluded entirely from current views, kept
     in explorer for historical
   - `fiz-openai-gpt-5-5`: valid_since `2026-05-08` (no wire-fix
     dependency for native OpenAI)
   - `fiz-bragi-club-3090-qwen3-6-27b`: TBD — needs a fresh run with
     post-wire-fix codepath
   - `fiz-grendel-rapid-mlx-qwen3-6-27b`: TBD — same

The aggregator script applies these filters when producing
`aggregates.json`. Lanes that fail any criterion are emitted to a
separate `aggregates-rejected.json` with the reason, so the reliability
page can show "lanes excluded due to infrastructure issues" in a
transparent way (this is meta-data about the data, not noise in the
data).

**Where lanes appear regardless of validity:**
- `/explorer/` shows all cells; user filters as desired. Default filter
  is "valid lanes only" but a single click toggles to all.
- `/reports/` per-suite pages are the raw, ungated audit trail.

## Per-page outlines

### `/benchmarks/_index.md` — pillar landing

- One paragraph framing each story
- Two big cards linking to the two pillars
- "Look at the raw data" link to `/explorer/`
- Footer chip: "<N> cells across <M> tasks. <H> hours of compute. Last
  rebuild: <date>."

### `/benchmarks/harness-comparison/_index.md` — Story 1 root

**Headline section.** One-sentence answer:
> Fizeau passes X% of TB-2.1 tasks; claude-code passes Y%; codex Z%;
> opencode W%; pi V%.

**Headline table** — paired comparisons (model held constant):
| Harness | Model | Pass rate | Median wall | Median cost | % invalid |
|---|---|---|---|---|---|
| fiz / anthropic-direct | claude-sonnet-4-6 | … | … | … | … |
| claude-code | claude-sonnet-4-6 | … | … | … | … |
| fiz / openai-direct | gpt-5.5 | … | … | … | … |
| codex | gpt-5.5 | … | … | … | … |
| ... | ... |

**Per-task delta chart** (SVG): bar chart of (fiz pass rate – harness
pass rate) per task, sorted descending. Shows where each harness wins.

**Caveats section.** What's apples-to-apples and what isn't. Tool
surface differences. System prompt differences. Retry semantics.

**Sub-pages.** One per harness pairing for deep dives.

### `/benchmarks/harness-comparison/vs-claude-code.md`

Detailed pairing:
- Same task, same model (claude-sonnet-4-6), different harness
- Pass-rate by task (table)
- Wall distribution (boxplot or violin SVG)
- Failure-mode breakdown: where each harness gets stuck
- Tool-call counts and error rates
- System-prompt diff (text block)

Same template for vs-codex, vs-opencode, vs-pi, vs-gemini-cli.

### `/benchmarks/model-landscape/_index.md` — Story 2 root

**Headline matrix.** Sortable table (server-rendered, not the
explorer). Lane identity = full descriptor; internal lane name absent:

| Model | Quant | Engine | Hardware | Pass rate | Median wall | Cost/task | Median decode tok/s |
|---|---|---|---|---|---|---|---|
| GPT-5.5 | native | OpenAI direct | cloud | … | … | … | n/a |
| Claude Sonnet 4.6 | native | Anthropic direct | cloud | … | … | … | n/a |
| Qwen3.6 27B | native | OpenRouter | cloud | … | … | … | n/a |
| Qwen3.6 27B | Q3_K_XL (Unsloth GGUF) | llama.cpp | RTX 3090 (24GB) | … | … | $0 | … |
| Qwen3.6 27B | MLX-8bit | oMLX | M2 Ultra (192GB) | … | … | $0 | … |
| Qwen3.6 27B | MLX-4bit | Rapid-MLX | RTX 5090 (32GB) | … | … | $0 | … |
| DeepSeek V4 Flash | IQ2XXS-mixed-Q2K-Q8 | ds4-server | M2 Ultra (192GB), KV on disk 128GB | … | … | $0 | … |

Rows that fail the validity gate are absent (per #4).

**Pareto card.** Brief discussion: which (model, provider) wins on
which axis. Honest discussion of tradeoffs.

**Three drill-down cards** linking to /local-vs-cloud/, /hardware/,
/backends/.

### `/benchmarks/model-landscape/local-vs-cloud.md`

- Scatter plot SVG: x=median wall (log), y=pass rate. Color: cloud
  (blue) vs local (amber). Size: cost.
- Discussion: "the local Pareto frontier vs the cloud Pareto frontier"
- Cost-normalized chart: pass rate per dollar (cloud) vs per hour of
  GPU time (local, with a notional $/hr conversion factored)

### `/benchmarks/model-landscape/hardware.md`

Same model (qwen3.6-27b) across hardware:
- Bar chart: median decode tok/s per hardware (M2 Ultra, RTX 5090,
  RTX 3090)
- Bar chart: median prefill tok/s per hardware
- Heatmap: pass rate per task per hardware
- Discussion: where memory bandwidth wins vs compute wins

### `/benchmarks/model-landscape/backends.md`

Same model + same hardware, different backend:
- e.g., qwen3.6-27b on M2 Ultra: oMLX vs Rapid-MLX (vidar-omlx vs
  vidar-rapid-mlx if available)
- e.g., qwen3.6-27b on RTX 3090: vLLM vs llama.cpp (sindri-club-3090
  vs sindri-club-3090-llamacpp)
- Bar charts: tok/s, wall median, cost, pass rate
- Discussion: backend correctness deltas (do they produce equivalent
  outputs? — same task pass rate at p95?)

### `/benchmarks/model-landscape/reliability.md`

- Stacked-bar per lane: graded_pass / graded_fail / invalid_setup /
  invalid_provider / invalid_quota / invalid_auth percentages
- Insight: which lanes are model-failing (high graded_fail) vs infra-
  failing (high invalid_*). Catches the "ds4 invalid_setup chain" we
  saw and similar.
- "Lanes excluded from main views" callout: shows lanes that didn't
  pass the validity gate, with reason

### `/benchmarks/model-landscape/reasoning-control.md`

The recent investigation made tangible:
- Per-lane: was reasoning_tokens populated? With what budget?
- Probe artifacts (linked to docs/research/probe-reasoning-2026-05-11-v2/)
- Observation: where named tier translates to budget vs where it gets
  flat-mapped
- Why this matters: caller-intent portability (link to ADR-010)

### `/benchmarks/explorer/`

Single-page DataTables.js interface. Loads `cells.json` (full feed,
pre-filtered by validity by default; toggle to show all).

Columns:
- task | suite | harness | provider | model | model_family | tier |
  hardware | backend | rep | final_status | invalid_class |
  wall_seconds | reasoning_tokens | input_tokens | output_tokens |
  cost_usd | turns | started_at | finished_at

Default sort: `finished_at` desc.
Default filter: `valid_lane = true` (toggle to show all).

Built-in features (DataTables provides):
- Per-column sort
- Per-column filter (text, dropdown for enums)
- Multi-column search
- Export to CSV
- Pagination

**Pre-built saved views** (linked from story pages):
- "Story 1 raw data" → filter to harness comparisons
- "Story 2 raw data" → filter to model landscape lanes
- "Recent failures" → filter to graded_fail OR invalid_*, last 7 days
- "By lane" → grouped view

### `/benchmarks/methodology/`

- TerminalBench-2.1 overview: 89 tasks, what they test, grading method
- Harbor wrapper: how Fizeau/agents are invoked per cell
- Cell schema: what fields each cell record contains
- Task-base wall budget rationale (QWEN_BASE × LANE_PENALTY framework)
- Validity rules (the four criteria above), rationale per
- Reproducibility: how to reproduce a single cell, how to reproduce a
  lane sweep
- Provider-specific notes: ds4 alias-collapse, sindri thinking_budget
  hint-only, OR `reasoning.max_tokens` honor, etc.
- Versioning: cells include `fiz_tools_version` and `profile_snapshot`
  so historical comparisons are tractable
- ADR cross-references: ADR-010 reasoning wire, ADR-007 sampling
  catalog, ADR-009 routing surface

### `/benchmarks/provenance/_index.md` — provenance pillar root

Frames why this section exists separately from methodology:

> The benchmark numbers on this site reflect months of iteration on
> Fizeau, the providers we've integrated with, and the TerminalBench
> wrapping. Many of those numbers were either wrong or impossible to
> get six weeks ago. This section documents what changed, in what
> order, and why — both as honest disclosure and as operator guidance
> for anyone trying to reproduce or extend our setup.

Then a card grid linking to the four story pages and the lane-notes.

### `/benchmarks/provenance/changelog.md`

Date-ordered, audience = "operator who wants to reproduce" or "future
us trying to remember what state was good when." Format:

```
## 2026-05-12

- **OR full TB-2.1 sweep launched** with QWEN_BASE=2.5×, post-wire-fix.
  Targets the 36 task-gaps OR hadn't passed at fresh wire. Expected
  pass rate climb from baseline 60% as previously-truncated cells now
  fit budget. (Bead: fizeau-ee77ec7f rerun.)

## 2026-05-11

- **Wire-format dialect fixes landed** (commit `cfdcdcc4`,
  fizeau-073fe18b). Sindri now uses `chat_template_kwargs.{enable_thinking,
  thinking_budget}` envelope; ds4 switches to flat top-level
  `reasoning_effort`. Previously both lanes silently no-thinking'd.
  Verified end-to-end via cmd/fizeau-probe-reasoning v2 and live
  benchmark cells (sindri 281 cumulative reasoning_tokens on a
  financial-document-processor cell; ds4 18,181 on fix-ocaml-gc).
- **Token extraction fallback shipped** (fizeau-8f62bcbb). When
  `usage.completion_tokens_details.reasoning_tokens` is absent (ds4
  case) but `message.reasoning_content` is populated, we derive
  reasoning_tokens via len(reasoning_content)/4 and flag
  `reasoning_tokens_approx=true`. ds4 cells now report non-zero
  reasoning_tokens.
- **Catalog `reasoning_wire: tokens` for qwen3.6-27b on OR** (commit
  `6e0f3c8c`). Replaced wrong-inherited `model_id` value. OR-Qwen3 now
  receives `reasoning.max_tokens: 4096` (the only knob OR honors for
  Qwen3 — `effort` is silently flat-mapped at upstream).
- **Surface-ID lookup bug fixed** in `internal/config/config.go`. The
  catalog's `reasoning_wire` map was keyed by catalog ID but providers
  query by surface ID; OR-Qwen3 (`qwen/qwen3.6-27b` vs `qwen3.6-27b`)
  silently missed the lookup pre-fix.
- **ADR-010 amendment** documenting the L1 (live introspection) → L2
  (catalog) → L3 (static defaults) source-of-truth chain for wire
  selection.
- **`REASONING_FORMAT=auto` persisted in sindri's docker-compose**
  (operator action). Without this, container restart silently reverts
  to `none` → think blocks left inline in `content` rather than
  extracted to `reasoning_content` → fizeau extractor reports zero.

## 2026-05-08

- **`reasoning: <int>` accepted as token budget** in profile YAML
  (fizeau translator routes through `KindTokens` policy). Lets us
  carry an explicit budget per-profile until the catalog claims that
  responsibility (ADR-011).

## 2026-05-07

- **Initial OR-Qwen3 timing analysis.** Discovered 48% of `graded_fail`
  cells were truncated mid-work (`terminated_mid_work=True`), not
  model failures. Walls clustered at p90=1832s, max=2025s vs Harbor's
  900s base → led to QWEN_BASE = 2.5×.

## 2026-04-30 → 2026-05-04

- **Initial sweep ran with hardcoded `effort: low` for OR-Qwen3.**
  Reasoning_tokens flat at ~5555 across all tiers. Believed to be
  Qwen3's "low effort" budget. Investigation later revealed OR
  ignores `effort` for Qwen3 entirely; the 5555 is OR's default
  upstream budget.
```

### `/benchmarks/provenance/reasoning-control-saga.md`

The full investigation narrative. Sections:

1. **What we thought reasoning was doing.** `reasoning: low` → effort
   tier sent to provider → upstream model thinks accordingly. Wrong on
   multiple lanes.
2. **What it was actually doing on each lane.** OR flat-mapped Qwen3
   tiers. ds4 silently ignored `thinking: {type, budget_tokens}`.
   Sindri silently dropped top-level `enable_thinking`. Same caller
   intent, three different upstream behaviors, all wrong.
3. **The introspection moment.** Discovered ds4's `/props.reasoning.aliases`
   declares `{low: high, medium: high, xhigh: high}` — they really do
   collapse. Discovered sindri's chat template requires
   `chat_template_kwargs` envelope. The introspection endpoints
   *already document* the wire shape; we just weren't reading them.
4. **The fix architecture.** L1 (live introspection at provider
   construction) → L2 (catalog `reasoning_wire`) → L3 (static
   defaults). Per-provider wire dialects: OR's nested
   `reasoning.max_tokens`, sindri's `chat_template_kwargs`, ds4's flat
   `reasoning_effort`, Anthropic's `thinking: {type, budget_tokens}`.
5. **Capability honesty.** Where lanes can't honor caller intent (ds4
   has no token budget; sindri's budget is a soft hint), we record
   what we asked for AND what was actually emitted, so cross-lane
   analysis is honest at read time.

Cross-references: ADR-010, ADR-010 Amendment 2026-05-11, the probe
artifacts at `docs/research/probe-reasoning-2026-05-11-v2/`.

### `/benchmarks/provenance/timeout-calibration.md`

The QWEN_BASE × LANE_PENALTY framework:

- **QWEN_BASE = 2.5×.** Derived from OR-Qwen3's truncated-fail wall
  distribution: p95 ≈ 1832s, max ≈ 2025s vs Harbor's 900s base. 2.5×
  rescues p95 truncations with safety margin. Empirically validated:
  timing-baseline OR pass walls show max=2254s, exactly at the 2250s
  budget. Cannot be safely reduced.
- **LANE_PENALTY per-provider.** Empirical p75 of
  median(local_pass_wall / OR_pass_wall) across joint-pass tasks:
  - sindri-llamacpp Q3_K_XL on RTX 3090: 2.03×
  - vidar-omlx 8-bit on M2 Ultra: 3.23×
  - vidar-ds4 mixed-quant on M2 Ultra: TBD (started conservative 5×
    pending more joint-pass cells)
- **Profile multiplier = QWEN_BASE × LANE_PENALTY.** Composes cleanly:
  cells exit early on `finish_reason=stop` so over-provisioning has
  bounded cost; under-provisioning truncates real model work.
- **terminated_mid_work classification.** A `graded_fail` with
  `finish_reason ∈ {tool_calls, length}` AND wall ≥ 0.95 × budget is
  classified as truncation, not model failure. Filed under
  `invalid_*` instead of `graded_fail` so per-model pass rates aren't
  contaminated by infra-side timeout decisions.

Includes the framework's evolution: started at 1.0× with no per-lane
penalty (Apr 2026), introduced LANE_PENALTY after observing local
lanes truncate disproportionately (May 2026).

### `/benchmarks/provenance/token-accounting.md`

The reasoning-token reporting story:

- **OpenAI shape:** `usage.completion_tokens_details.reasoning_tokens`.
  Honored by OR, OpenAI-direct, some llama-server builds.
- **Missing from:** ds4 (puts thinking in `message.reasoning_content`,
  no separate count); some older llama-server builds.
- **Fizeau resolution chain** (per ADR-010 §8): usage path first; else
  fall back to `len(reasoning_content) / 4` with `_approx=true` flag
  in the cell record. Honest absence (zero with no flag) is also a
  valid outcome.
- **Why approx is OK.** Char/4 estimator is rough but bounded. The
  `_approx` flag tells analysts which numbers are estimated. A future
  bead can replace with a real tokenizer if approximation error
  matters in a specific analysis.

### `/benchmarks/provenance/classification.md`

The `invalid_*` taxonomy and how it evolved:

- `invalid_quota` — provider rejected with rate-limit / quota
  exhaustion. Distinct from model failure.
- `invalid_auth` — auth failure. Operator config issue.
- `invalid_setup` — harness adapter setup failed (e.g., docker pull,
  cert install, runtime extract). Pre-LLM failure.
- `invalid_provider` — upstream provider error mid-stream
  (connection-refused, 5xx, hang). Distinct from model giving up.
- `terminated_mid_work` — LLM was producing tokens (tool_calls or
  length finish_reason) but hit wall budget. Infra-side, not model.
- `harness_crash` — harbor/agent crashed unexpectedly.

Each was added to differentiate genuine model-quality signals from
infra/operator noise, so per-model pass rates are honest. The classifier
lives at `cmd/bench/matrix_invalid.go` and continues to evolve as new
failure modes surface.

### `/benchmarks/provenance/stack-notes/` — per-(engine, model) reference

One file per **engine × model-class** combination, NOT per physical
machine. The audience is an end-user thinking "I want to run Qwen3.6
on llama.cpp" or "I want to run DeepSeek V4 Flash via ds4-server" —
the operator behind a specific physical machine is irrelevant.

Each file documents:

- **The stack:** engine + model + recommended quant + KV-cache notes.
  Hardware-class generalizations (e.g., "needs ≥24GB VRAM for Q3_K_XL")
  rather than specific machines.
- **Wire format quirks:** what the engine actually accepts vs what
  fizeau emits (link to wire-format table).
- **Known limitations:** alias collapses, budget-non-binding hints,
  template-config dependencies, etc.
- **Operator setup checklist:** env vars, server-config persistence
  requirements, model-load-time gotchas — anything an operator
  reproducing this stack on their own hardware needs to know.
- **Timeout multiplier:** what we observed; rationale (link to
  timeout-calibration.md).
- **Status as of last benchmark:** working / wire-broken / deprecated.

Example: `stack-notes/llamacpp-qwen3.md`:
```
# Qwen3.6 27B on llama.cpp (llama-server)

| Field | Value |
|---|---|
| Engine | llama.cpp llama-server (build tested: b9014-d4b0c22f9) |
| Model | Qwen3.6-27B (Unsloth Q3_K_XL GGUF; ~14GB on disk) |
| Hardware class | NVIDIA GPU with ≥24GB VRAM (verified on RTX 3090) |
| Auth | Any non-empty value via LLAMACPP_API_KEY env (server ignores it) |
| Status | Working (post 2026-05-11 wire-fix) |
| Timeout multiplier observed | ~5× the harbor task base on TB-2.1 |

## Wire format quirks

llama-server requires `chat_template_kwargs.enable_thinking: true`
to activate Qwen3 thinking. Top-level `enable_thinking` is silently
dropped. Thinking output is extracted into `message.reasoning_content`
when server-side `--reasoning-format=auto` is set; otherwise the
`<think>` block stays inline in `message.content`.

## Known limitations

`thinking_budget` in `chat_template_kwargs` is a soft template hint
only — it does NOT cap thinking length. `max_tokens` is the only
hard cap on output (and applies to the combined reasoning + content).

## Operator setup checklist

1. Start llama-server with `--reasoning-format auto` (NOT `none`,
   which leaves `<think>` inline in `content`).
2. Persist `REASONING_FORMAT=auto` in your container env vars
   (compose / systemd unit) — runtime-only flags revert on restart.
3. Configure fizeau profile with provider type `llama-server`,
   `chat_template_kwargs` envelope is automatic via fizeau's
   ThinkingWireFormatQwen.
4. Verify with cmd/fizeau-probe-reasoning — non-zero
   reasoning_tokens on every non-off matrix row indicates correct
   wiring.

## Status as of last benchmark

Active. Post-wire-fix llamacpp Qwen3.6 lane produces graded_pass with
proper reasoning_tokens. See /benchmarks/reports/terminal-bench-2-1/
for the most recent full sweep.
```

### `/benchmarks/reports/terminal-bench-2-1/`

Existing 8-section TB-2.1 report, moved here unchanged. Becomes the
"raw audit trail" for this benchmark suite. Story pages link here for
the reproducible original analysis.

## Data plumbing

### Build-time aggregator

Script: `scripts/website/build-benchmark-data.py`. Walks
`benchmark-results/fiz-tools-v1/cells/<suite>/<task>/<lane>/rep-N/report.json`.
Emits:

- `microsite/static/data/cells.json` — every cell, normalized schema
- `microsite/static/data/cells-valid.json` — same, filtered by
  validity rules
- `microsite/static/data/aggregates.json` — pre-computed per-(harness,
  provider, model) summaries for story-page tables/charts
- `microsite/static/data/aggregates-rejected.json` — lanes excluded
  from main views with reasons

Schema for each cell record (proposed):
```json
{
  "schema_version": 1,
  "task": "fix-ocaml-gc",
  "suite": "terminal-bench-2-1",
  "harness": "fiz",
  "provider": "openrouter",
  "model": "qwen/qwen3.6-27b",
  "model_family": "qwen3-6-27b",
  "tier": "smart",
  "hardware": "cloud",
  "backend": "cloud-hosted",
  "rep": 3,
  "final_status": "graded_pass",
  "invalid_class": null,
  "wall_seconds": 2254,
  "reasoning_tokens": 41162,
  "reasoning_tokens_approx": false,
  "input_tokens": 142000,
  "output_tokens": 8200,
  "cost_usd": 0.092,
  "turns": 35,
  "started_at": "...",
  "finished_at": "...",
  "valid_lane": true,
  "lane_validity_reasons": []
}
```

`aggregates.json` keyed on `(harness, provider, model)` with computed:
- `n_cells`, `n_pass`, `n_fail`, `n_invalid`
- `pass_rate`, `invalid_rate`
- `wall_p50`, `wall_p95`
- `cost_p50`, `cost_total`
- `reasoning_tokens_p50`
- `tasks_covered` (set), `tasks_with_min_3_reps` (set)
- `valid_lane`, `validity_reasons` (per the 4 criteria)

### Hugo template usage

Story pages use Hugo `getJSON` to consume aggregates at build time.
Tables and headline numbers render server-side. Charts: Python script
generates SVG from aggregate data, output to
`microsite/static/charts/`.

### Explorer assets

- `microsite/static/data/cells-valid.json` (default explorer feed)
- `microsite/static/data/cells.json` (full toggle)
- `microsite/themes/hextra-overrides/layouts/explorer.html` — page
  template that loads DataTables.js + the JSON
- DataTables.js + jQuery via CDN (small dep; can vendor if offline use
  matters)

## Implementation phasing

Suggest splitting into ~5 beads under one EPIC:

| # | Title | Scope |
|---|---|---|
| **W-EPIC** | Benchmark website redesign — two stories + provenance + dynamic explorer | parent |
| **W1** | Build-time aggregator + validity rules | scripts/website/build-benchmark-data.py + tests; cells.json, aggregates.json |
| **W2** | Methodology page | self-contained; documents validity rules + reproducibility |
| **W3** | Story 1 pages — harness comparison pillar + 1-2 sub-pages | depends on W1 |
| **W4** | Story 2 pages — model landscape pillar + sub-pages | depends on W1 |
| **W5** | Explorer with DataTables.js | depends on W1 |
| **W6** | Provenance pillar — changelog + saga pages + per-lane notes | self-contained; documents what we discovered along the way |

W1 is the foundation; W2-W6 can land in parallel once W1 ships. W6
(provenance) and W2 (methodology) are documentation-heavy and don't
depend on the aggregator's data feed — they could even land first.

## Data we're missing

Story 1 pairings — need fresh sweeps for:
- fiz/anthropic-direct vs claude-code on claude-sonnet-4-6 across TB-2.1
- fiz/openai-direct vs codex on gpt-5.5 across TB-2.1
- fiz/openai-direct vs codex on gpt-5.4-mini across TB-2.1
- fiz/X vs opencode/X (need to pick X)
- fiz/X vs pi/X
- fiz/gemini-2.5 vs gemini-cli

Story 2 hardware — need fresh sweeps with post-wire-fix codepath:
- bragi-rapid-mlx (RTX 5090) on qwen3.6-27b — re-run with new wire
- grendel-rapid-mlx on qwen3.6-27b — same
- vidar-omlx — DEPRECATED, replace with vidar-ds4 in current views

Story 2 backends — need:
- vidar-omlx vs vidar-rapid-mlx vs vidar-ds4 (same hardware, different
  backends) — only ds4 currently active
- sindri-club-3090 (vLLM) vs sindri-club-3090-llamacpp on same model

These collection sweeps are tracked separately as bench beads (not
website beads) but the website pages will note "data pending" until
they land.

## Open questions for operator

1. **Decide on "Story 0":** should there be an even-higher-level
   landing that frames "what is fizeau and why benchmark it?" Or is
   that the project README's job and `/benchmarks/` jumps straight to
   the two stories? Lean: keep `/benchmarks/_index.md` as the framing,
   project README handles "what is fizeau."

2. **Existing TB-2.1 page disposition:** keep at current URL with a
   redirect to `/benchmarks/reports/terminal-bench-2-1/`? Or hard-move?
   Lean: redirect for SEO + bookmarks.

3. **Charts as commit artifacts vs build-time generated?** Pre-generated
   SVGs in git let reviewers see them in PRs. Build-time generation
   keeps them fresh with data. Lean: build-time, with a
   `make site-charts` target that operators can run + review locally.

4. **Cell-data publication policy:** publishing `cells.json` openly
   exposes per-task pass/fail data. Any tasks where the underlying
   prompt is sensitive? Lean: TB-2.1 tasks are public, no concern;
   document the policy for future suites.

5. **The "validity" criteria values (≥3 reps, <20% invalid, ≥80%
   reasoning_tokens):** are these the right thresholds? We can
   adjust per-data-distribution after seeing the first build.
EOF
)
