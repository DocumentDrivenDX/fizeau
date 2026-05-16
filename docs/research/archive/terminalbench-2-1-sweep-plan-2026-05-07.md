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

**Amended:** 2026-05-14 — sweep schema v2 (fizeau-596ff006). Phases were split into two orthogonal concepts: **Subsets** (pure task lists, see [Subsets](#subsets)) and **Recipes** (curated CLI bundles pairing one subset with a lane list, see [Recipes](#recipes)). Lanes no longer enroll in phases; the runner's executable matrix is `(subset, lane)`. Recipes are CLI sugar for invoking a pre-curated `(subset, lanes[])` pair. The historical phase ids are preserved verbatim as recipe ids; `--phase` remains as a deprecated CLI alias.

> **TB-2.0 preservation notice.** All TB-2.0 historical fixtures, subset manifests (`task-subset-v2.yaml`, `task-subset-v1.yaml`), evidence records, and research memos retain their `terminal-bench@2.0` labels. Nothing in this plan relabels or overwrites them. The machine-readable companion (`scripts/benchmark/terminalbench-2-1-sweep.yaml`) governs only TB-2.1 work.

---

## Goals

Three experimental questions drive this sweep:

1. **Provider/quant deltas.** How does the Qwen3.6-27B model family behave across local providers, inference runtimes, and quantization configurations? Rows must preserve `provider_surface`, `endpoint`, `quant_label`, `runtime`, and `hardware_label` so differences are not collapsed into a misleading single "local-qwen" number.

2. **Harness deltas.** How do different harness scaffoldings perform when the intended model family is held approximately constant? The plan distinguishes true same-provider comparisons (none in the subscription-harness phases) from approximate same-model-family comparisons.

3. **Frontier harness-vs-fiz deltas.** How do frontier model+harness combinations (fiz Claude harness, fiz Codex harness) compare against fiz's native provider path through OpenRouter?

**Meta goal:** build evidence for model-selection calculus across cost, speed, quality, reliability, and invalid-run behavior; and make fiz the strongest agentic harness for supported workloads.

---

## Subsets

A subset is a pure task list — the YAML lives at `subsets:` in `scripts/benchmark/terminalbench-2-1-sweep.yaml`. Each entry carries `id`, `path` (subset YAML file), and `default_reps`. Subsets carry no lane information; the runner pairs them with lanes via either a recipe (curated bundle) or an ad-hoc `--subset X --lanes Y` CLI invocation.

| Subset id                          | Path                                             | Default reps |
| ---------------------------------- | ------------------------------------------------ | ------------ |
| `terminalbench-2-1-canary`         | scripts/benchmark/task-subset-tb21-canary.yaml   | 3            |
| `terminalbench-2-1-full`           | scripts/benchmark/task-subset-tb21-full.yaml     | 3            |
| `terminalbench-2-1-all`            | scripts/benchmark/task-subset-tb21-all.yaml      | 3            |
| `terminalbench-2-1-openai-cheap`   | scripts/benchmark/task-subset-tb21-openai-cheap.yaml | 5        |
| `terminalbench-2-1-or-passing`     | scripts/benchmark/task-subset-tb21-or-passing.yaml | 3          |
| `terminalbench-2-1-timing-baseline`| scripts/benchmark/task-subset-tb21-timing-baseline.yaml | 3     |

---

## Recipes

A recipe is a curated CLI bundle — the YAML lives at `recipes:`. It pairs one `subset:` with a `lanes:` list and optional overrides (`reps`, `max_concurrency_override`, `parallel_policy`, `preflight`, `staged`). Recipes are pure sugar at runtime: the executable matrix is still `(subset, lane)`, and an ad-hoc `--subset X --lanes Y` invocation bypasses recipes entirely.

`staged: true` declares membership in the gating sequence iterated by `fiz-bench sweep --staged-recipes` (the historical `--phase all` behavior). Staged sequence runs in YAML order: canary → local-qwen → sonnet-comparison → gpt-comparison → medium-model-canary → medium-model. Non-staged recipes (timing-baseline, or-passing, tb21-all, openai-cheap) are operational one-offs — invoke them by name with `--recipe <id>` or include them via `--all-recipes` for an exhaustive pass. `recipe.reps` overrides `subset.default_reps` when set; e.g. `medium-model-canary` runs the `terminalbench-2-1-canary` subset at reps=1, while `canary` runs the same subset at reps=3.

Each recipe can run and resume independently.

### Recipe 1: canary

**Purpose:** Prove every intended lane can start, reach its provider, write session/trajectory artifacts, and classify invalid cells (quota/auth/setup/provider) separately from graded failures.

**Dataset:** `terminal-bench/terminal-bench-2-1`, canary subset (3–5 tasks; see [Profile Inventory](#profile-inventory))

**Reps:** 3 per cell

**Lanes (all 7 active):**
- `fiz-harness-claude-sonnet-4-6`
- `fiz-harness-codex-gpt-5-4-mini`
- `fiz-openrouter-claude-sonnet-4-6`
- `fiz-openrouter-gpt-5-4-mini`
- `fiz-vidar-omlx-qwen3-6-27b`
- `fiz-bragi-club-3090-qwen3-6-27b`
- `fiz-sindri-vllm-qwen3-6-27b`

**Gate:** canary must complete before starting phases 2–4.

### Recipe 2: local-qwen

**Purpose:** Compare Qwen3.6-27B-family performance across all reachable local inference providers using fiz's native provider path.

**Comparison group:** `cg-local-qwen-provider-quant`

**Dataset:** `terminal-bench/terminal-bench-2-1`, full/expanded subset

**Reps:** 3 per cell

**Lanes (3):**
- `fiz-vidar-omlx-qwen3-6-27b` → `rg-vidar-omlx` (max 1 concurrent)
- `fiz-bragi-club-3090-qwen3-6-27b` → `rg-bragi-club-3090` (max 1 concurrent)
- `fiz-sindri-vllm-qwen3-6-27b` → `rg-sindri-club-3090` (max 1 concurrent)

**Parallelism:** The three lanes use distinct resource groups (different servers); they may run in parallel.

### Recipe 3: sonnet-comparison

**Purpose:** Compare fiz's native provider path to Sonnet through OpenRouter against fiz delegating to the authenticated Claude Code subscription harness.

**Comparison group:** `cg-sonnet-harness-fiz`

**Dataset:** `terminal-bench/terminal-bench-2-1`, full/expanded subset

**Reps:** 3 per cell

**Lanes (2):**
- `fiz-openrouter-claude-sonnet-4-6` — fiz native provider path
- `fiz-harness-claude-sonnet-4-6` — fiz Claude-harness path

**Parallelism:** The native OpenRouter lane uses OpenRouter auth. The Claude-harness lane requires staged Claude Code CLI artifacts plus the authenticated `.claude` home. Cap concurrency conservatively because both lanes consume subscription or cloud quota.

### Recipe 4: gpt-comparison

**Purpose:** Compare fiz's native provider path to GPT-5.4-mini through OpenRouter against fiz delegating to the authenticated Codex subscription harness.

**Comparison group:** `cg-gpt-harness-fiz`

**Dataset:** `terminal-bench/terminal-bench-2-1`, full/expanded subset

**Reps:** 3 per cell

**Lanes (2):**
- `fiz-openrouter-gpt-5-4-mini` — fiz native provider path
- `fiz-harness-codex-gpt-5-4-mini` — fiz Codex-harness path

**Parallelism:** The native OpenRouter lane uses OpenRouter auth. The Codex-harness lane requires staged Codex CLI artifacts plus the authenticated `.codex` home. Cap concurrency conservatively because both lanes consume subscription or cloud quota.

---

## Comparison Groups

### cg-local-qwen-provider-quant — Provider/quant delta

| Lane | Provider surface | Endpoint | Model served | Quant | Runtime | Hardware |
|------|-----------------|----------|-------------|-------|---------|----------|
| `fiz-vidar-omlx-qwen3-6-27b` | vidar-omlx | `http://vidar:1235/v1` | `Qwen3.6-27B-MLX-8bit` | mlx-8bit | oMLX | Apple Silicon M-class |
| `fiz-bragi-club-3090-qwen3-6-27b` | bragi-club-vllm | `http://bragi:8020/v1` | `qwen3.6-27b-autoround` | vllm-autoround | vLLM | RTX 3090 |
| `fiz-sindri-vllm-qwen3-6-27b` | sindri-club-vllm | `http://sindri:8020/v1` | `qwen3.6-27b-autoround` | vllm-autoround | vLLM | RTX 3090 |

**Equivalence:** `approximate_same_family` — same Qwen3.6-27B Instruct weights lineage but different quant methods, runtimes, and server hardware. **Not a true same-model comparison.** Published memos must report per-lane breakdowns and must not average across these rows without disclosing the metadata differences.

### cg-sonnet-harness-fiz — Frontier harness-vs-fiz (Sonnet)

| Lane | Lane type | Execution auth/surface | Model |
|------|-----------|------------------------|-------|
| `fiz-openrouter-claude-sonnet-4-6` | fiz_provider_native | OpenRouter API key | `anthropic/claude-sonnet-4.6` |
| `fiz-harness-claude-sonnet-4-6` | fiz_harness_pinned | Claude Code CLI + authenticated `.claude` home | `claude-sonnet-4-6` |

**Equivalence:** `approximate_same_model_family` — the native lane uses OpenRouter, while the harness lane delegates to Claude Code using subscription auth/session state. **Not a same-provider or pure model control.** Published memos must carry the caveat from `terminalbench-fiz-wrapper-comparison-2026-05-06`.

### cg-gpt-harness-fiz — Frontier harness-vs-fiz (GPT-5.4-mini)

| Lane | Lane type | Execution auth/surface | Model |
|------|-----------|------------------------|-------|
| `fiz-openrouter-gpt-5-4-mini` | fiz_provider_native | OpenRouter API key | `openai/gpt-5.4-mini` |
| `fiz-harness-codex-gpt-5-4-mini` | fiz_harness_pinned | Codex CLI + authenticated `.codex` home | `gpt-5.4-mini` |

**Equivalence:** `approximate_same_model_family` — the native lane uses OpenRouter, while the harness lane delegates to Codex using subscription auth/session state. Same caveat as sonnet comparison.

---

## Lane Definitions

All lane details (profile IDs, FIZEAU_* inputs, metadata) are normative in `scripts/benchmark/terminalbench-2-1-sweep.yaml`. The table below summarizes the key fields for human reference.

| Lane ID | Profile ID | lane_type | Phases | FIZEAU_HARNESS | FIZEAU_PROVIDER | FIZEAU_MODEL | FIZEAU_BASE_URL | Resource group |
|---------|-----------|-----------|--------|---------------|----------------|-------------|----------------|----------------|
| `fiz-harness-claude-sonnet-4-6` | `fiz-harness-claude-sonnet-4-6` | fiz_harness_pinned | canary, sonnet-comparison | claude | openrouter bootstrap | `claude-sonnet-4-6` | `https://openrouter.ai/api/v1` | rg-openrouter |
| `fiz-harness-codex-gpt-5-4-mini` | `fiz-harness-codex-gpt-5-4-mini` | fiz_harness_pinned | canary, gpt-comparison | codex | openrouter bootstrap | `gpt-5.4-mini` | `https://openrouter.ai/api/v1` | rg-openrouter |
| `fiz-harness-pi-vidar-qwen3-6-27b` | `vidar-qwen3-6-27b` | fiz_harness_pinned | canary, local-qwen | pi | omlx | `Qwen3.6-27B-MLX-8bit` | `http://vidar:1235/v1` | rg-vidar-omlx |
| `fiz-harness-opencode-vidar-qwen3-6-27b` | `vidar-qwen3-6-27b` | fiz_harness_pinned | canary, local-qwen | opencode | omlx | `Qwen3.6-27B-MLX-8bit` | `http://vidar:1235/v1` | rg-vidar-omlx |
| `fiz-openrouter-claude-sonnet-4-6` | `fiz-openrouter-claude-sonnet-4-6` | fiz_provider_native | canary, sonnet-comparison | *(not set)* | openrouter | `anthropic/claude-sonnet-4.6` | `https://openrouter.ai/api/v1` | rg-openrouter |
| `fiz-openrouter-gpt-5-4-mini` | `fiz-openrouter-gpt-5-4-mini` | fiz_provider_native | canary, gpt-comparison | *(not set)* | openrouter | `openai/gpt-5.4-mini` | `https://openrouter.ai/api/v1` | rg-openrouter |
| `fiz-vidar-omlx-qwen3-6-27b` | `vidar-qwen3-6-27b` | fiz_provider_native | canary, local-qwen | *(not set)* | omlx | `Qwen3.6-27B-MLX-8bit` | `http://vidar:1235/v1` | rg-vidar-omlx |
| `fiz-bragi-club-3090-qwen3-6-27b` | `bragi-club-3090` | fiz_provider_native | canary, local-qwen | *(not set)* | openai-compat | `qwen3.6-27b-autoround` | `http://bragi:8020/v1` | rg-bragi-club-3090 |
| `fiz-sindri-vllm-qwen3-6-27b` | `sindri-vllm` | fiz_provider_native | canary, local-qwen | *(not set)* | vllm | `qwen3.6-27b-autoround` | `http://sindri:8020/v1` | rg-sindri-club-3090 |
| `fiz-sindri-llamacpp-qwen3-6-27b` | `sindri-llamacpp` | fiz_provider_native | canary, local-qwen, timing-baseline, or-passing, tb21-all | *(not set)* | llama-server | `Qwen3.6-27B-UD-Q3_K_XL.gguf` | `http://sindri:8020/v1` | rg-sindri-club-3090-llamacpp |

### Harness-pinned vs native: what changes

**`fiz_harness_pinned` lanes** set `FIZEAU_HARNESS=<harness>` and always enter through the `fiz` command surface. Claude/Codex lanes pass `FIZEAU_PROVIDER=openrouter` only as a fiz bootstrap input so route/provider resolution succeeds before delegation to authenticated subscription CLIs. Pi/OpenCode lanes pass the local Vidar oMLX provider/model so fiz can exercise its subprocess harness plumbing against a local model. The harness controls its own auth/session state, system prompt, tool schema, permission flags, context handling, and retry logic. The harness may apply its own model aliasing.

**`fiz_provider_native` lanes** set `FIZEAU_PROVIDER=<type>` (and optionally `FIZEAU_MODEL`, `FIZEAU_BASE_URL`) and do not set `FIZEAU_HARNESS`. fiz routes the call directly through its own provider client. fiz's own scaffolding, tool schema, and session logging apply.

---

## Resource Groups and Concurrency

### Local resource groups (max_concurrency=1)

| Group | Server | Base URL | Provider type | Hardware | Max concurrent cells |
|-------|--------|----------|--------------|---------|---------------------|
| `rg-vidar-omlx` | vidar | `http://vidar:1235/v1` | omlx | Apple Silicon M-class | **1** |
| `rg-bragi-club-3090` | bragi | `http://bragi:8020/v1` | vllm | RTX 3090 | **1** |
| `rg-sindri-club-3090` | sindri | `http://sindri:8020/v1` | vllm | RTX 3090 | **1** |

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
| `fiz-openrouter-claude-sonnet-4-6` | `scripts/benchmark/profiles/fiz-openrouter-claude-sonnet-4-6.yaml` | canary, sonnet-comparison |
| `fiz-openrouter-gpt-5-4-mini` | `scripts/benchmark/profiles/fiz-openrouter-gpt-5-4-mini.yaml` | canary, gpt-comparison |
| `vidar-qwen3-6-27b` | `scripts/benchmark/profiles/vidar-qwen3-6-27b.yaml` | canary, local-qwen, Pi/OpenCode wrapped lanes |
| `bragi-qwen3-6-27b` | `scripts/benchmark/profiles/bragi-qwen3-6-27b.yaml` | canary, local-qwen |

### New profiles required

| Profile ID | Path | Status | Action needed |
|-----------|------|--------|---------------|
| `bragi-club-3090-vllm-qwen3-6-27b` | `scripts/benchmark/profiles/bragi-club-3090-vllm-qwen3-6-27b.yaml` | created, provisional | Run preflight; update `model` and `base_url` fields; update `versioning.snapshot` |

### New subset manifests required

| Manifest ID | Path | Status | Action needed |
|------------|------|--------|---------------|
| `terminalbench-2-1-canary` | `scripts/benchmark/task-subset-tb21-canary.yaml` | present | 3-task canary subset from `terminal-bench/terminal-bench-2-1`; does not reuse TB-2.0 task IDs |
| `terminalbench-2-1-full` | `scripts/benchmark/task-subset-tb21-full.yaml` | present | Stratified 2.1 subset for the full sweep |

---

## Equivalence Classification

| Comparison | Equivalence level | What differs | Claim ceiling |
|-----------|-----------------|--------------|---------------|
| Local qwen: vidar-omlx vs bragi-club-3090 vs sindri-club-3090 | `approximate_same_family` | quant method, runtime, server hardware, context window | "Qwen3.6-27B-family across local runtimes" — not "same model" |
| Sonnet: fiz-native/OpenRouter vs fiz-Claude-Code-subscription | `approximate_same_model_family` | provider surface, auth/session state, harness scaffolding | "native OpenRouter vs delegated subscription harness" — not "same provider" or "pure model control" |
| GPT: fiz-native/OpenRouter vs fiz-Codex-subscription | `approximate_same_model_family` | provider surface, auth/session state, harness scaffolding | same caveat as Sonnet |

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

0. **Agent runtime bundle.** The repo-root `./bench/run` entrypoint builds a cached
   agent-runtime Docker image, exports `/installed-agent` as a tarball, and
   passes that tarball to Harbor. Harbor must still run every task in the
   official TerminalBench task image; the runtime bundle is only extracted into
   that container as agent tooling. Secrets and credentials (`.claude`,
   `.codex`, `.fizeau/config.yaml`, API keys) remain runtime-only and must not
   be baked into Docker layers.

1. **Phase orchestration.** The runner must accept `--phase <name>` to run one phase independently, and `--phase all` to run all four in order.

2. **Resource-group scheduling.** Before starting any cell, acquire a per-resource-group slot. Release the slot when the cell completes or fails. Local resource groups (`rg-vidar-omlx`, `rg-bragi-club-3090`, `rg-sindri-club-3090`) cap at 1. `rg-openrouter` caps cloud/subscription comparison lanes conservatively.

3. **Dry-run/plan output.** `--dry-run` must print: phase, lane IDs, comparison groups, task count, reps, resource groups, max parallelism per group, and the exact `fiz run` or `harbor run` command for each cell — without invoking Harbor.

4. **Metadata capture.** Each `report.json` must include all per-cell metadata fields listed in [Metrics](#metrics-for-model-selection-calculus).

5. **Local endpoint preflight.** Before any local-provider cell, verify the endpoint is reachable. If it fails, mark all cells in that group `invalid_provider` and continue with other groups.

6. **Provider sampling compatibility.** Native OpenAI GPT-5-family lanes (`provider.type: openai`, including `fiz-openai-gpt-5-5`) must use OpenAI's default sampling controls on the wire. The runner must not inject `FIZEAU_TEMPERATURE` or `FIZEAU_TOP_P` for those lanes, and the OpenAI provider must strip those fields if config still supplies them. OpenRouter and other OpenAI-compatible lanes must keep their profile sampling controls, including `temperature`, `top_p`, `top_k`, `min_p`, and `repetition_penalty` when the compatibility provider accepts them. This is a lane/provider compatibility rule, not a per-run operator override.

7. **Evidence import compatibility.** Matrix artifacts must be importable via the existing `go run ./cmd/bench evidence import-terminalbench` workflow. Preserve all fields required by that importer.
