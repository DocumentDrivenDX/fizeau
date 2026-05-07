---
ddx:
  id: terminalbench-2-1-sweep-plan-2026-05-07
  spec-id: terminalbench-2.1-sweep-plan-2026-05-07
  created: 2026-05-07
  governs:
    - fizeau-85d36567   # runner implementation
    - fizeau-c770193a   # full sweep execution
  governed-by:
    - fizeau-1bbe2973   # this planning bead
    - fizeau-ee77ec7f   # TB-2.1 migration
---

# TerminalBench 2.1 Sweep Plan

**Dataset:** `terminal-bench/terminal-bench-2-1`

**Date:** 2026-05-07

**Status:** active — governs runner implementation (fizeau-85d36567) and sweep execution (fizeau-c770193a)

> **TB-2.0 preservation notice.** All TB-2.0 historical fixtures, subset manifests (`task-subset-v2.yaml`, `task-subset-v1.yaml`), evidence records, and research memos retain their `terminal-bench@2.0` labels. Nothing in this plan relabels or overwrites them. The machine-readable companion (`scripts/benchmark/terminalbench-2-1-sweep.yaml`) governs only TB-2.1 work.

---

## Goals

Three experimental questions drive this sweep:

1. **Provider/quant deltas.** How does the Qwen3.6-27B model family behave across local providers, inference runtimes, and quantization configurations? Rows must preserve `provider_surface`, `endpoint`, `quant_label`, `runtime`, and `hardware_label` so differences are not collapsed into a misleading single "local-qwen" number.

2. **Harness deltas.** How do different harness scaffoldings perform when the intended underlying model and provider are held approximately constant? The plan distinguishes true same-model comparisons (none in this sweep) from approximate same-provider-surface comparisons (Sonnet and GPT comparisons).

3. **Frontier harness-vs-fiz deltas.** How do frontier model+harness combinations (fiz Claude harness, fiz Codex harness) compare against fiz's native provider path using the same OpenRouter endpoint?

**Meta goal:** build evidence for model-selection calculus across cost, speed, quality, reliability, and invalid-run behavior; and make fiz the strongest agentic harness for supported workloads.

---

## Phases

The sweep is staged. Each phase can run and resume independently.

### Phase 1: canary

**Purpose:** Prove every intended lane can start, reach its provider, write session/trajectory artifacts, and classify invalid cells (quota/auth/setup/provider) separately from graded failures.

**Dataset:** `terminal-bench/terminal-bench-2-1`, canary subset (3–5 tasks; see [Profile Inventory](#profile-inventory))

**Reps:** 3 per cell

**Lanes (all 9):**
- `fiz-harness-claude-sonnet-4-6`
- `fiz-harness-codex-gpt-5-4-mini`
- `fiz-harness-pi-gpt-5-4-mini`
- `fiz-harness-opencode-gpt-5-4-mini`
- `fiz-openrouter-claude-sonnet-4-6`
- `fiz-openrouter-gpt-5-4-mini`
- `fiz-vidar-omlx-qwen3-6-27b`
- `fiz-bragi-lmstudio-qwen3-6-27b`
- `fiz-bragi-club-vllm-qwen3-6-27b` *(requires preflight; may be excluded if unreachable)*

**Gate:** canary must complete before starting phases 2–4.

### Phase 2: local-qwen

**Purpose:** Compare Qwen3.6-27B-family performance across all reachable local inference providers using fiz's native provider path.

**Comparison group:** `cg-local-qwen-provider-quant`

**Dataset:** `terminal-bench/terminal-bench-2-1`, full/expanded subset

**Reps:** 3 per cell

**Lanes (3):**
- `fiz-vidar-omlx-qwen3-6-27b` → `rg-vidar-omlx` (max 1 concurrent)
- `fiz-bragi-lmstudio-qwen3-6-27b` → `rg-bragi-lmstudio` (max 1 concurrent)
- `fiz-bragi-club-vllm-qwen3-6-27b` → `rg-bragi-club-vllm` (max 1 concurrent; requires preflight)

**Parallelism:** The three lanes use distinct resource groups (different servers); they may run in parallel.

### Phase 3: sonnet-comparison

**Purpose:** Measure how much harness scaffolding changes outcomes when provider surface is held approximately constant (both lanes route to Sonnet via OpenRouter).

**Comparison group:** `cg-sonnet-harness-fiz`

**Dataset:** `terminal-bench/terminal-bench-2-1`, full/expanded subset

**Reps:** 3 per cell

**Lanes (2):**
- `fiz-openrouter-claude-sonnet-4-6` — fiz native provider path
- `fiz-harness-claude-sonnet-4-6` — fiz Claude-harness path

**Parallelism:** Both lanes use `rg-openrouter`; cap concurrency at 2 for this phase.

### Phase 4: gpt-comparison

**Purpose:** Same question as sonnet-comparison for the OpenAI model family.

**Comparison group:** `cg-gpt-harness-fiz`

**Dataset:** `terminal-bench/terminal-bench-2-1`, full/expanded subset

**Reps:** 3 per cell

**Lanes (2):**
- `fiz-openrouter-gpt-5-4-mini` — fiz native provider path
- `fiz-harness-codex-gpt-5-4-mini` — fiz Codex-harness path

**Parallelism:** Both lanes use `rg-openrouter`; cap concurrency at 2 for this phase.

---

## Comparison Groups

### cg-local-qwen-provider-quant — Provider/quant delta

| Lane | Provider surface | Endpoint | Model served | Quant | Runtime | Hardware |
|------|-----------------|----------|-------------|-------|---------|----------|
| `fiz-vidar-omlx-qwen3-6-27b` | vidar-omlx | `http://vidar:1235/v1` | `Qwen3.6-27B-MLX-8bit` | mlx-8bit | oMLX | Apple Silicon M-class |
| `fiz-bragi-lmstudio-qwen3-6-27b` | bragi-lmstudio | `http://bragi:1234/v1` | `qwen/qwen3.6-27b` | q4-k-m-gguf | llama.cpp (LM Studio) | RTX 5090-mobile |
| `fiz-bragi-club-vllm-qwen3-6-27b` | bragi-club-vllm | `http://bragi-club:8000/v1` *(provisional)* | `<autoround-model-id>` *(verify)* | vllm-autoround | vLLM | RTX 3090 |

**Equivalence:** `approximate_same_family` — same Qwen3.6-27B Instruct weights lineage but different quant methods, runtimes, and server hardware. **Not a true same-model comparison.** Published memos must report per-lane breakdowns and must not average across these rows without disclosing the metadata differences.

### cg-sonnet-harness-fiz — Frontier harness-vs-fiz (Sonnet)

| Lane | Lane type | FIZEAU_HARNESS | FIZEAU_PROVIDER | Model | Base URL |
|------|-----------|---------------|----------------|-------|----------|
| `fiz-openrouter-claude-sonnet-4-6` | fiz_provider_native | *(not set)* | openrouter | `anthropic/claude-sonnet-4.6` | `https://openrouter.ai/api/v1` |
| `fiz-harness-claude-sonnet-4-6` | fiz_harness_pinned | claude | *(not set)* | `anthropic/claude-sonnet-4.6` | `https://openrouter.ai/api/v1` |

**Equivalence:** `approximate_same_provider_surface` — same model ID and base_url, but the Claude-harness lane delegates to claude-code which applies its own system prompt, tool schema, permission semantics, and context compaction. **Not a pure model control.** Published memos must carry the caveat from `terminalbench-fiz-wrapper-comparison-2026-05-06`.

### cg-gpt-harness-fiz — Frontier harness-vs-fiz (GPT-5.4-mini)

| Lane | Lane type | FIZEAU_HARNESS | FIZEAU_PROVIDER | Model | Base URL |
|------|-----------|---------------|----------------|-------|----------|
| `fiz-openrouter-gpt-5-4-mini` | fiz_provider_native | *(not set)* | openrouter | `openai/gpt-5.4-mini` | `https://openrouter.ai/api/v1` |
| `fiz-harness-codex-gpt-5-4-mini` | fiz_harness_pinned | codex | *(not set)* | `openai/gpt-5.4-mini` | `https://openrouter.ai/api/v1` |

**Equivalence:** `approximate_same_provider_surface` — same model ID and base_url, but the Codex-harness lane delegates to Codex CLI scaffolding. Same caveat as sonnet comparison.

---

## Lane Definitions

All lane details (profile IDs, FIZEAU_* inputs, metadata) are normative in `scripts/benchmark/terminalbench-2-1-sweep.yaml`. The table below summarizes the key fields for human reference.

| Lane ID | Profile ID | lane_type | Phases | FIZEAU_HARNESS | FIZEAU_PROVIDER | FIZEAU_MODEL | FIZEAU_BASE_URL | Resource group |
|---------|-----------|-----------|--------|---------------|----------------|-------------|----------------|----------------|
| `fiz-harness-claude-sonnet-4-6` | `fiz-harness-claude-sonnet-4-6` | fiz_harness_pinned | canary, sonnet-comparison | claude | *(not set)* | `anthropic/claude-sonnet-4.6` | `https://openrouter.ai/api/v1` | rg-openrouter |
| `fiz-harness-codex-gpt-5-4-mini` | `fiz-harness-codex-gpt-5-4-mini` | fiz_harness_pinned | canary, gpt-comparison | codex | *(not set)* | `openai/gpt-5.4-mini` | `https://openrouter.ai/api/v1` | rg-openrouter |
| `fiz-harness-pi-gpt-5-4-mini` | `fiz-harness-pi-gpt-5-4-mini` | fiz_harness_pinned | canary | pi | *(not set)* | `openai/gpt-5.4-mini` | `https://openrouter.ai/api/v1` | rg-openrouter |
| `fiz-harness-opencode-gpt-5-4-mini` | `fiz-harness-opencode-gpt-5-4-mini` | fiz_harness_pinned | canary | opencode | *(not set)* | `openai/gpt-5.4-mini` | `https://openrouter.ai/api/v1` | rg-openrouter |
| `fiz-openrouter-claude-sonnet-4-6` | `fiz-openrouter-claude-sonnet-4-6` | fiz_provider_native | canary, sonnet-comparison | *(not set)* | openrouter | `anthropic/claude-sonnet-4.6` | `https://openrouter.ai/api/v1` | rg-openrouter |
| `fiz-openrouter-gpt-5-4-mini` | `fiz-openrouter-gpt-5-4-mini` | fiz_provider_native | canary, gpt-comparison | *(not set)* | openrouter | `openai/gpt-5.4-mini` | `https://openrouter.ai/api/v1` | rg-openrouter |
| `fiz-vidar-omlx-qwen3-6-27b` | `vidar-qwen3-6-27b` | fiz_provider_native | canary, local-qwen | *(not set)* | omlx | `Qwen3.6-27B-MLX-8bit` | `http://vidar:1235/v1` | rg-vidar-omlx |
| `fiz-bragi-lmstudio-qwen3-6-27b` | `bragi-qwen3-6-27b` | fiz_provider_native | canary, local-qwen | *(not set)* | lmstudio | `qwen/qwen3.6-27b` | `http://bragi:1234/v1` | rg-bragi-lmstudio |
| `fiz-bragi-club-vllm-qwen3-6-27b` | `bragi-club-3090-vllm-qwen3-6-27b` | fiz_provider_native | canary, local-qwen | *(not set)* | openai-compat | `<autoround-model-id>` *(verify)* | `http://bragi-club:8000/v1` *(verify)* | rg-bragi-club-vllm |

### Harness-pinned vs native: what changes

**`fiz_harness_pinned` lanes** set `FIZEAU_HARNESS=<harness>` and do not set `FIZEAU_PROVIDER`. fiz delegates the model call to the named harness subprocess. The harness controls its own system prompt, tool schema, permission flags, context handling, and retry logic. The harness may apply its own model aliasing.

**`fiz_provider_native` lanes** set `FIZEAU_PROVIDER=<type>` (and optionally `FIZEAU_MODEL`, `FIZEAU_BASE_URL`) and do not set `FIZEAU_HARNESS`. fiz routes the call directly through its own provider client. fiz's own scaffolding, tool schema, and session logging apply.

---

## Resource Groups and Concurrency

### Local resource groups (max_concurrency=1)

| Group | Server | Base URL | Provider type | Hardware | Max concurrent cells |
|-------|--------|----------|--------------|---------|---------------------|
| `rg-vidar-omlx` | vidar | `http://vidar:1235/v1` | omlx | Apple Silicon M-class | **1** |
| `rg-bragi-lmstudio` | bragi | `http://bragi:1234/v1` | lmstudio | RTX 5090-mobile | **1** |
| `rg-bragi-club-vllm` | bragi-club | `http://bragi-club:8000/v1` *(provisional)* | openai-compat (vLLM) | RTX 3090 | **1** |

Local resource groups serialize all cells within their group. Cells from different resource groups (different servers) may run in parallel.

### Managed provider resource group (rate/budget caps)

| Group | Base URL | Max concurrent cells | RPM | TPM | Per-run cap | Per-phase cap | Per-sweep cap |
|-------|----------|---------------------|-----|-----|-------------|---------------|---------------|
| `rg-openrouter` | `https://openrouter.ai/api/v1` | **4** | 500 | 200,000 | $5.00 | $50.00 | $150.00 |

OpenRouter lanes may run in parallel across comparison groups, subject to the rate and budget caps. For single-comparison-group phases (sonnet-comparison, gpt-comparison), cap concurrency at 2. For the canary, all six OpenRouter lanes may use full max_concurrency.

Honor `Retry-After` headers on 429 responses. On 429, the cell is not marked invalid — it is retried after the back-off.

---

## Resume Semantics

The runner must implement resume mode by default for all phases.

**Skip (do not rerun):** cells whose `final_status` is in:
- `graded_pass`
- `graded_fail`
- `install_fail_permanent`
- `budget_halted`

**Retry (rerun on resume):** cells whose `final_status` is in:
- `install_fail_transient`
- `timeout`
- `harness_crash`
- `ran_ungraded`
- `harness_refused`

**Always run:** cells with no `report.json` (missing cells).

**Force rerun:** only when `--force-rerun` is explicitly set on the command. Force rerun must be auditable in the sweep command output; it must not silently discard prior graded results. Never auto-force-rerun completed graded or invalid cells between sweep restarts.

---

## Metrics for Model-Selection Calculus

The following metrics are required for every cell and run. The aggregator (`matrix-aggregate`) must expose them so comparative claims can be grounded in evidence.

### Per-run metrics

| Metric | Type | Notes |
|--------|------|-------|
| `reward` | int or null | 1=pass, 0=fail, null=ungraded |
| `final_status` | string | graded_pass / graded_fail / invalid_* / timeout / ... |
| `invalid_class` | string or null | invalid_quota / invalid_auth / invalid_setup / invalid_provider |
| `wall_time_seconds` | float | total wall-clock time from container start to verifier exit |
| `input_tokens` | int or null | |
| `output_tokens` | int or null | |
| `cached_input_tokens` | int or null | |
| `retried_input_tokens` | int or null | |
| `cost_usd` | float or null | derived from pricing × tokens; null if tokens unreported |
| `session_log_path` | string or null | fiz JSONL session log path (trajectory provenance) |
| `trajectory_path` | string or null | ATIF trajectory artifact path |

### Per-cell aggregate metrics

| Metric | Type | Notes |
|--------|------|-------|
| `mean_reward` | float or null | mean over `graded_*` runs only; null if n_valid=0 |
| `reward_sd` | float or null | |
| `n_runs` | int | total attempts |
| `n_valid` | int | graded_pass + graded_fail runs (valid denominator) |
| `n_invalid` | int | |
| `invalid_class_counts` | object | `{invalid_quota: N, ...}` |
| `total_cost_usd` | float or null | |
| `effective_cost_per_valid_run` | float or null | total_cost_usd / n_valid |
| `effective_cost_per_pass` | float or null | total_cost_usd / n_pass (null if n_pass=0) |

### Per-cell metadata (must appear in matrix.json)

- `lane_id`, `profile_id`, `comparison_group`, `phase`
- `model_family`, `model_id` (exact string served, from `/v1/models` or profile)
- `quant_label`, `provider_surface`, `runtime`, `hardware_label`, `endpoint`
- `resource_group`, `sampling_params`, `context_window`
- `fizeau_env` (FIZEAU_* pins; API key values must be redacted)
- `session_log_path`, `trajectory_path`

---

## Profile Inventory

### Existing profiles (reusable)

| Profile ID | Path | Used in phases |
|-----------|------|----------------|
| `fiz-harness-claude-sonnet-4-6` | `scripts/benchmark/profiles/fiz-harness-claude-sonnet-4-6.yaml` | canary, sonnet-comparison |
| `fiz-harness-codex-gpt-5-4-mini` | `scripts/benchmark/profiles/fiz-harness-codex-gpt-5-4-mini.yaml` | canary, gpt-comparison |
| `fiz-harness-pi-gpt-5-4-mini` | `scripts/benchmark/profiles/fiz-harness-pi-gpt-5-4-mini.yaml` | canary |
| `fiz-harness-opencode-gpt-5-4-mini` | `scripts/benchmark/profiles/fiz-harness-opencode-gpt-5-4-mini.yaml` | canary |
| `fiz-openrouter-claude-sonnet-4-6` | `scripts/benchmark/profiles/fiz-openrouter-claude-sonnet-4-6.yaml` | canary, sonnet-comparison |
| `fiz-openrouter-gpt-5-4-mini` | `scripts/benchmark/profiles/fiz-openrouter-gpt-5-4-mini.yaml` | canary, gpt-comparison |
| `vidar-qwen3-6-27b` | `scripts/benchmark/profiles/vidar-qwen3-6-27b.yaml` | canary, local-qwen |
| `bragi-qwen3-6-27b` | `scripts/benchmark/profiles/bragi-qwen3-6-27b.yaml` | canary, local-qwen |

### New profiles required

| Profile ID | Path | Status | Action needed |
|-----------|------|--------|---------------|
| `bragi-club-3090-vllm-qwen3-6-27b` | `scripts/benchmark/profiles/bragi-club-3090-vllm-qwen3-6-27b.yaml` | created, provisional | Run preflight; update `model` and `base_url` fields; update `versioning.snapshot` |

### New subset manifests required

| Manifest ID | Path | Status | Action needed |
|------------|------|--------|---------------|
| `terminalbench-2-1-canary` | `scripts/benchmark/task-subset-tb21-canary.yaml` | missing | Create 3–5 task canary subset from `terminal-bench/terminal-bench-2-1` catalog; do not reuse `task-subset-v2.yaml` (that is a TB-2.0 artifact) |
| `terminalbench-2-1-full` | `scripts/benchmark/task-subset-tb21-full.yaml` | missing | Generate stratified full/expanded subset from the 2.1 catalog using the same selection criteria as `task-subset-v2.yaml`; keep it clearly labeled as a TB-2.1 artifact |

---

## Equivalence Classification

| Comparison | Equivalence level | What differs | Claim ceiling |
|-----------|-----------------|--------------|---------------|
| Local qwen: vidar-omlx vs bragi-lmstudio vs bragi-club-vllm | `approximate_same_family` | quant method, runtime, server hardware, context window | "Qwen3.6-27B-family across local runtimes" — not "same model" |
| Sonnet: fiz-native vs fiz-claude-harness | `approximate_same_provider_surface` | harness scaffolding (system prompt, tool schema, context handling, permission policy) | "same model API, different harness" — not "pure model control" |
| GPT: fiz-native vs fiz-codex-harness | `approximate_same_provider_surface` | harness scaffolding | same caveat as Sonnet |

No comparison in this sweep is a **true same-model control** (identical model weights, identical provider surface, identical scaffolding, differing only on one dimension). Every memo must state the applicable equivalence level and what actually differs.

---

## Invalid Run Classification

Cells that never reached a meaningful model attempt must be classified as invalid rather than graded failures, and excluded from mean reward and cost denominators. They remain visible in `matrix.md` with cause and log path.

| Class | Trigger |
|-------|---------|
| `invalid_quota` | Rate limit, usage exhausted, credits exhausted, quota window closed (e.g., 429 with no Retry-After, or `out_of_credits`) |
| `invalid_auth` | Missing or rejected credentials |
| `invalid_setup` | Harness installation, binary architecture, permission-mode, or task environment failure before agent work starts |
| `invalid_provider` | Provider transport failure (network unreachable, DNS failure, server error) before any response is produced |

Invalid cells are never counted in `n_valid` or `mean_reward` denominators. `effective_cost_per_valid_run` and `effective_cost_per_pass` are computed over valid runs only.

---

## Implementation Notes for Runner (fizeau-85d36567)

The runner implementation agent should read `scripts/benchmark/terminalbench-2-1-sweep.yaml` as its primary input. Key behaviors required:

1. **Phase orchestration.** The runner must accept `--phase <name>` to run one phase independently, and `--phase all` to run all four in order.

2. **Resource-group scheduling.** Before starting any cell, acquire a per-resource-group slot. Release the slot when the cell completes or fails. Local resource groups (`rg-vidar-omlx`, `rg-bragi-lmstudio`, `rg-bragi-club-vllm`) cap at 1. `rg-openrouter` caps at 4 globally, 2 per single-comparison-group phase.

3. **Dry-run/plan output.** `--dry-run` must print: phase, lane IDs, comparison groups, task count, reps, resource groups, max parallelism per group, and the exact `fiz run` or `harbor run` command for each cell — without invoking Harbor.

4. **Metadata capture.** Each `report.json` must include all per-cell metadata fields listed in [Metrics](#metrics-for-model-selection-calculus).

5. **bragi-club preflight.** Before any cell in `rg-bragi-club-vllm`, run the preflight command from the resource group definition. If it fails, mark all cells in the group `invalid_provider` and continue with other groups.

6. **Evidence import compatibility.** Matrix artifacts must be importable via the existing `go run ./cmd/bench evidence import-terminalbench` workflow. Preserve all fields required by that importer.
