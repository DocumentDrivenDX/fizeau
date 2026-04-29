---
ddx:
  id: harness-matrix-plan-2026-04-29
  created: 2026-04-29
  reviewed_by: codex (2026-04-29, v2 + v6)
status: DRAFT v7
exit_criterion: Filing of epic bead (bead-breakdown §NEW1). Subsequent revisions live in SD-010, not here.
---

# Plan: Harness × Model Matrix Benchmark

## Goal

Run TerminalBench-2 across **harness × model**: ddx-agent vs. OSS competitors **pi and opencode** (initial tranche). Frontier-reference cells (codex, claude-code) and `forge` are explicitly **deferred to a follow-up tranche** after the runner proves itself with three harnesses + one anchor model. Use one anchor model first; sweep models second.

### Terminology (codex v6 fix)

- **run** — one task execution: `(harness, profile, rep, task)` tuple. Locks, resume, status, and per-run budget apply at this level.
- **cell** — one matrix entry: `(harness, profile)` tuple, aggregating reps × tasks. Mean reward, SD, and reporting apply at this level.

**Central caveat (must restate in every published memo):** "Same model, different harness" is *not* a clean control — each harness ships its own system prompt, tool schema, retry policy, reasoning effort, context compaction, and default sampling. Results compare *(harness scaffolding + policy) over a shared model API*, not pure harness skill.

## Open question to resolve before Step 1: anchor model

User constraints:
- **$1–3 / Mtok output** bracket.
- **Recency rule (NEW v6):** model must be released or substantially refreshed within the last **~6 months** (cutoff: roughly 2025-11-01). Year-old models are excluded — there's no point benchmarking against ceilings the field has already moved past.

Sonnet 4.6 sits at ~$15/Mtok output and is **out of bracket** even though it has the best harness coverage. Year-old options (GPT-5-mini original, Haiku 4.5 original, Gemini 2.5 Flash) are also **out by recency** unless a refreshed snapshot lands in the bracket.

Candidate set (April 2026; **the Step 0 census must verify each** — do not anchor on this table):

| Model | Released / refreshed | Output $/Mtok | Harness coverage | Notes |
|---|---|---|---|---|
| GPT-5.3-mini | 2026-Q1 | ~$2 (verify) | codex native; opencode, ddx-agent, pi via OpenAI-compat | Recency + bracket likely match; tool-use solid. |
| Gemini 3 Flash | 2026-Q1 (verify) | ~$0.50 (verify) | gemini native; ddx-agent direct; OSS support uneven | Cheapest if available. |
| Haiku 4.6 / 5 | 2026 (verify) | verify bracket | claude-code native; ddx-agent direct | Include only if a 2026 refresh exists in bracket. |
| Qwen 3.6-plus (OpenRouter) | 2026-Q1 | ~$0.50–1 | OpenAI-compat everywhere | Reserved for Phase B (lesser-model pass). |
| DeepSeek V3.2+ (OpenRouter) | 2026 (verify) | ~$0.30–1 | OpenAI-compat everywhere | Reserve / alternate to Qwen for Phase B. |

**Excluded by recency:** GPT-4o, GPT-4.1-mini, Sonnet 4.5 / 4.6 (original), Haiku 4.5 (original), Gemini 2.5 Flash, Qwen 3.5 / earlier. **Excluded by bracket:** Sonnet 4.6, Opus, GPT-5 (full).

**Recommendation:** anchor Phase A.1 on **GPT-5.3-mini** if Step 0 census confirms availability + bracket. Phase A.2 second model picked from a different vendor for axis diversity (Gemini 3 Flash or refreshed Haiku).

### Selection fallback hierarchy (codex v6 fix)

Step 0 must apply this hierarchy to each candidate, in order, and document the outcome per row:

1. **Tool-use viable** — the model supports tool / function calling at a quality bar usable for TB-2. Fail this and the row is excluded outright.
2. **In bracket** — output ≤ $3/Mtok. Fail and excluded unless a signed-off exception is recorded in the census doc (e.g., user explicitly wants Sonnet 4.6 for harness coverage).
3. **Recency** — released or refreshed ≥ 2025-11-01. Fail and excluded unless a signed-off exception (e.g., the only viable model in a vendor axis is slightly older).

If after this filter no candidate exists for a vendor, **drop that vendor axis from Phase A** rather than substitute a non-compliant model.

The first concrete deliverable (Step 0) is a one-page **model census** with verified prices and harness compatibility. Profiles and matrix anchors are picked from that — not from this plan's table.

### Smoke model vs anchor model

Step 0's anchor blocks Phase A.1 (Step 9) but **not** Steps 1–8. Plumbing work (egress, schema, adapters, runner, cost guards) needs *some* model to exercise the path. Use a **smoke model** — the cheapest tool-capable OpenAI-compat model on OpenRouter (≤ $1/Mtok output). The smoke model is **exempt from the recency rule** (its job is to make the rig green, not to publish numbers). The smoke profile is committed alongside the implementation; the anchor and second-model profiles are committed when Step 0 closes.

## Phases and exit criteria

| Phase | Scope | Exit |
|---|---|---|
| **A.0** Calibration | `noop` + `dumb-script` adapters; canary subset; **no model spend** | matrix.json with reward=0 (`noop`) and reward=1 only on `hello-world` (`dumb-script`) |
| **A.1** Single anchor model | anchor × {ddx-agent, pi, opencode}; canary; 3 reps | All 27 cells `graded_*`; per-cell SD < 0.4; cost report; published memo with caveats |
| **A.2** Add second model + frontier ref | + second model from census; + claude-code as reference | All cells complete; ddx-agent ≥ pi, opencode mean reward (north-star). If not → root-cause beads before expanding |
| **B** Lesser model | Qwen 3.6-plus, A.2 harness set | Memo with delta vs A.2 |
| **C** Local | ddx-agent only; provider × sampling sweep | Profile YAMLs only — **runner code unchanged** |

## Current state (verified 2026-04-29)

- `internal/benchmark/external/termbench/` — task loader (TB1+TB2), plan builder, ATIF v1.4 capture, verifier reader. Harness-agnostic.
- `cmd/bench/external_termbench.go` — single-harness, single-model, host tempdir, **no grading**.
- `internal/harnesses/registry.go` — codex, claude, gemini, opencode, agent, pi, virtual, script + provider harnesses. **No `forge`.**
- `scripts/benchmark/harbor_agent.py` — ddx-agent-only Harbor `BaseInstalledAgent`; reads `DDX_BENCH_PROVIDER_*` env (default model: `claude-haiku-4-5-20251001`).
- `scripts/benchmark/run_benchmark.sh` — Harbor wrapper, ddx-agent only.
- `scripts/beadbench/external/termbench-subset.json` — 20-task TB-2 subset @ `53ff2b87`.
- Submodule `scripts/benchmark/external/terminal-bench-2` — pinned, **uninitialized in current worktree**.
- Latest run: 2026-04-11, opus-4.6-fast, 2/15 (13.3%); 2026-04-27 baseline doc still `_pending_`.

## Architectural decisions

**D1. In-container only.** All harness cells run inside the upstream Harbor container. Mixing host-side and in-container layouts contaminates the comparison. Harnesses that can't be installed in-container are dropped from the matrix (documented), not run host-side.

**D2. Single CLI entry: `ddx-agent-bench matrix`.** Go subcommand replaces the parallel-shell-scripts approach. Reuses subset loader + registry; shells out to `harbor run` per run:

```
ddx-agent-bench matrix \
  --subset=scripts/beadbench/external/termbench-subset-canary.json \
  --profiles=gpt-5-3-mini \
  --harnesses=ddx-agent,pi,opencode \
  --reps=3 \
  --budget-usd=15 \
  --out=benchmark-results/matrix-<ts>/ \
  [--resume] [--force-rerun] [--retry-budget-halted]
```

No `--concurrency` flag in v1 — see D6.

**D3. Existing `cmd/bench --external=termbench` survives** as `--mode=smoke` — fast, ungraded, host tempdir, useful while iterating on ddx-agent code. The matrix subcommand is the only graded path.

**D4. Run state machine + telemetry (codex v6 fix).** A run has three orthogonal axes; `final_status` is derived from them deterministically. Adapters report the first two; the runner derives the third.

```
process_outcome  ∈ {completed, timeout, harness_crash, install_failed, harness_refused, budget_halted}
grading_outcome  ∈ {graded, ungraded}                # ungraded = no verifier output
reward           ∈ {0, 1, null}                      # null iff ungraded

final_status (derived)
  = graded_pass            if grading_outcome=graded ∧ reward=1
  = graded_fail            if grading_outcome=graded ∧ reward=0
  = budget_halted          if process_outcome=budget_halted
  = install_fail_permanent if process_outcome=install_failed ∧ retriable=false
  = install_fail_transient if process_outcome=install_failed ∧ retriable=true
  = ran_ungraded           if grading_outcome=ungraded ∧ process_outcome=completed
  = <process_outcome>      otherwise                 # timeout, harness_crash, harness_refused
```

Telemetry schema each adapter MUST emit:

```json
{
  "process_outcome": "completed|timeout|harness_crash|install_failed|harness_refused|budget_halted",
  "grading_outcome": "graded|ungraded",
  "reward": int|null,
  "turns": int|null, "tool_calls": int|null, "tool_call_errors": int|null,
  "input_tokens": int|null, "output_tokens": int|null,
  "cached_input_tokens": int|null, "retried_input_tokens": int|null,
  "wall_seconds": float
}
```

`null` = unreported; aggregator drops it from means (denominator shrinks), reports `n/a` in matrix.md, and emits `n_reported` alongside. Adapters and aggregator agree by construction — there's one source of truth (the two outcome fields) and one derivation function.

**D5. Resumability.** On `--resume`, a run is **skipped** when `final_status ∈ {graded_pass, graded_fail, install_fail_permanent, budget_halted}`. All other statuses retry. `--force-rerun` overrides everything; `--retry-budget-halted` retries that one status only.

**D6. Concurrency = 1, hard.** v1 of the matrix runner serializes runs. **No `--concurrency` flag.** Reasons: (a) Docker resource pressure with multiple TB-2 containers; (b) provider rate limits hit faster than expected at concurrency > 1; (c) the rate-limit-aware scheduler is its own design problem and shouldn't gate v1. Concurrent execution is a follow-up bead.

## Plan

### Step 0 — Model census (≤ 1 day, no code)

Produce `docs/research/model-census-2026-04-29.md`: one row per candidate. Required columns: model, provider, **release/refresh date** (must be ≥ 2025-11-01 or excluded), output $/Mtok, input $/Mtok, cached-input $/Mtok if applicable, max output tokens, tool-use quality, harness compatibility for codex / claude-code / opencode / pi / ddx-agent, model-snapshot-id resolved at this date.

The doc must explicitly list models **excluded by recency** and **excluded by bracket** so the audit trail shows we considered them. Pick the Phase A.1 anchor and the Phase A.2 second model from the surviving rows; vendors must differ between A.1 anchor and A.2 second (axis diversity).

**Acceptance:** doc lands; user signs off on (anchor, second-model) pair. If no kept candidate exists for a vendor, that vendor is dropped from Phase A — not substituted with an out-of-bracket or year-old fallback.

### Step 1 — Initialize submodule + verify egress (smoke model)

`git submodule update --init scripts/benchmark/external/terminal-bench-2`. Run the existing `scripts/benchmark/run_benchmark.sh` against `hello-world` only, with `DDX_BENCH_PROVIDER_MODEL=<smoke>` (cheapest OpenAI-compat OpenRouter model). **Acceptance:** Harbor produces `reward=1`. If in-container egress to the provider doesn't work, fix that before any other step.

This is the **only** end-to-end "is the existing rig still alive" check. Step 7's per-harness `--help` smoke is not a separate egress test — it just confirms the new harness is installable.

### Step 2 — Profile schema + Go loader

`scripts/benchmark/profiles/<id>.yaml`:

```yaml
id: gpt-5-mini
provider:
  type: openai-compat        # anthropic | openai | openai-compat | google
  model: gpt-5-mini
  base_url: https://api.openai.com/v1
  api_key_env: OPENAI_API_KEY
pricing:
  input_usd_per_mtok: 0.60
  output_usd_per_mtok: 2.00
  cached_input_usd_per_mtok: 0.30
limits:
  max_output_tokens: 8192
  context_tokens: 200000
  rate_limit_rpm: 500
  rate_limit_tpm: 200000
sampling: { temperature: 0.0, reasoning: medium }
versioning: { resolved_at: 2026-04-29, snapshot: "" }
```

Loader at `internal/benchmark/profile/`. **Acceptance:** unit test loads each shipped profile; `ddx-agent-bench profiles list` prints a table.

### Step 3 — Adapter base + ddx-agent + calibration adapters

Refactor `scripts/benchmark/harbor_agent.py` into:

```
scripts/benchmark/harness_adapters/
  base.py              # install/log/secret-redact/telemetry helpers
  noop.py              # exit 0, do nothing
  dumb_script.py       # hard-codes hello-world solution
  ddx_agent.py         # current logic, lifted as-is
```

One adapter file per harness; **no env-var-based dispatch**. The cell invocation passes the adapter module path explicitly to Harbor:

```
harbor run \
  --agent scripts/benchmark/harness_adapters/ddx_agent.py:Agent \
  --task <task-dir> \
  --output <cell-out>/<task-id>/ \
  --env DDX_BENCH_PROFILE_PATH=<profile.yaml>
```

Each adapter declares: `install()`, `command()` (non-interactive, no-TTY, stdin /dev/null), `apply_profile()` (YAML → harness-native config), `parse_telemetry()` (D4 schema), `redact_secrets()`.

A `FakeProvider` test helper (`harness_adapters/_test/fake_provider.py`) returns canned responses; adapters under test point at it via `base_url`. This enables paid-API-free coverage of `apply_profile`, `command`, and `parse_telemetry`. The helper is shared across adapter tests, not reimplemented per adapter.

**Acceptance:** pytest tests for each adapter assert correct command + env without invoking the harness or paid APIs. `noop`/`dumb-script` calibration produces the expected reward pattern.

### Step 4 — `ddx-agent-bench matrix` subcommand

Go subcommand under `cmd/bench/`. Per cell `(harness, profile, rep, task)`:

1. Compute `<out>/<harness>__<profile>__rep<N>/<task-id>/`
2. If `--resume` and prior `report.json` has terminal status (per D5), skip
3. Acquire per-cell lockfile (so concurrent runners can't double-spend)
4. Translate profile + harness → `harbor run` invocation (per Step 3)
5. Honor `Retry-After` from provider; track tokens from D4 telemetry
6. Write `report.json` with D4 + reward + status from the failure taxonomy:

```
install_fail_permanent | install_fail_transient | auth_fail | provider_refusal |
timeout | malformed_command | verifier_fail | harness_crash | budget_halted |
ran | graded_pass | graded_fail
```

`ddx-agent-bench matrix-aggregate <out>/` produces:

- `matrix.json` — every cell, every rep, every task, every field
- `matrix.md` — exact format below
- `costs.json` — input/output/cached/retry tokens × pricing → $

Target `matrix.md` format:

```markdown
## Reward (mean ± SD across 3 reps)

| Harness     | gpt-5-mini      | claude-code-anchor |
|-------------|-----------------|--------------------|
| ddx-agent   | 0.67 ± 0.33     | —                  |
| pi          | 0.33 ± 0.47     | —                  |
| opencode    | 0.50 ± 0.41     | —                  |
| claude-code | —               | 0.83 ± 0.24        |

## Per-task pass count (out of 3 reps)

| Task                       | ddx-agent / gpt-5-mini | pi / gpt-5-mini | opencode / gpt-5-mini |
|----------------------------|------------------------|-----------------|------------------------|
| hello-world                | 3/3                    | 3/3             | 3/3                    |
| log-summary-date-ranges    | 2/3                    | 0/3             | 1/3                    |
| git-leak-recovery          | 1/3                    | 0/3             | 0/3                    |

## Costs

| Cell                       | Input tok | Output tok | Cached tok | Cost ($) |
|----------------------------|-----------|------------|------------|----------|
| ddx-agent / gpt-5-mini     | 1.2M      | 0.4M       | 0.3M       | 1.42     |
...
```

**Acceptance:** matrix runs against `noop`/`dumb-script` produce the expected calibration pattern; resume after `kill -9` produces the same matrix.

### Step 5 — Cost guardrails (codex v6 fix: caps from observation, not formula)

Track all four token streams (input, output, cached-input, retried-input). The per-run hard cap is **derived from observation, not a worst-case formula**. Procedure (executed once before Phase A.1):

1. Run a single `(ddx-agent, smoke-anchor, 1 rep, 1 task)` end-to-end with the canary task that has the highest expected token burn (`git-leak-recovery`).
2. Record observed tokens × pricing → measured cost.
3. Per-run cap = `p95-of-3-observed-runs × 2.0` safety factor, **floor $1.00**, **ceiling $5.00** absolute.
4. Per-matrix cap = `per-run cap × n_runs × 1.2`.
5. The numbers are written into the matrix output's `costs.json` so memos can cite the derivation.

This step is itself a **prerequisite bead** before the A.1 matrix runs (NEW21). Over-budget runs → `process_outcome=budget_halted`, **not** silent truncation. **Acceptance:** test with a stub provider that overshoots → run marked `budget_halted`, matrix continues; observation procedure is documented in the cost guard package's README.

### Step 6 — Canary task subset

`scripts/beadbench/external/termbench-subset-canary.json`:

- `hello-world` (smoke; canonical)
- `log-summary-date-ranges` (data-processing, medium; file edits + reasoning)
- `git-leak-recovery` (software-engineering, medium; multi-step shell)

Exclusion rules documented inline: no `hard` tasks (variance dominates with 3 reps); no tasks failing under our own adapter pre-existing.

**Acceptance:** matrix runs end-to-end on canary with ddx-agent + anchor + 1 rep; rewards present (any value).

### Step 7 — OSS harness install spike

Output: `docs/research/oss-harness-install-2026-XX-XX.md`. Per harness (pi, opencode, forge) and per frontier reference (codex, claude-code):

- Install method, version pinned (binary URL + sha256 OR pinned pip/npm)
- Non-interactive flags (no TTY assumption; stdin /dev/null behavior; explicit exit-on-done)
- Profile mapping (env / config / CLI flags)
- Custom-base-url support? (OpenAI-compat / OpenRouter routing)
- License + benchmarking permission
- Known limitations

**Forge decision rule:** if `forge` is not openly installable in-container, drop from the matrix and document. Don't keep mentioning it as a competitor.

**Frontier reference rule:** codex and claude-code are reference cells, **not OSS competitors**. ToS / fair-use review required before any **public** memo. Internal memos are fine.

**Acceptance:** doc lands; for each kept harness, an in-container `--help` runs under Harbor; adapter unit tests pass.

### Step 8 — Adapters for kept harnesses

Implement one adapter per kept harness following Step 3's contract. **Acceptance:** unit tests pass; calibration matrix from Step 4 still passes alongside.

### Step 9 — Phase A.1 matrix

`ddx-agent-bench matrix --subset=canary --profiles=<anchor> --harnesses=ddx-agent,pi,opencode --reps=3 --budget-usd=<from Step 5>`. **27 runs.**

Memo `docs/research/matrix-baseline-phase-a1-2026-XX-XX.md`:

- Full pinning: harness commits, Harbor commit, TB-2 commit, Docker image digests, CLI versions, model snapshot at resolution, provider routing
- Caveat block (D1, D4, 3-rep variance)
- Raw matrix + per-task pass count + per-run final_status
- Cost breakdown (observed vs cap from Step 5)
- Known limitations including any `budget_halted` runs

**Acceptance (codex v6 fix):**
- All 27 runs reach a terminal `final_status` (any of: `graded_pass`, `graded_fail`, `budget_halted`, `install_fail_permanent`, `harness_refused`).
- ≥ 24 of 27 runs are `graded_*` (allow ≤ 3 non-graded, which must be itemized in the memo with cause).
- SD per cell is **reported, not gated**. The memo discusses cells with high SD but high SD does not block bead closure.
- Cost report is present and reconciles to observed token streams.

### Step 10 — Phase A.2

Add second model from census; add `claude-code` as frontier reference. Re-run on canary. 4 × 2 × 3 × 3 = **72 runs**. Cost $15–$50.

### Steps 11+ (deferred)

Phase B (Qwen) and Phase C (local) reuse all of A.1/A.2. **No runner code changes.** Documented as follow-up beads, not part of this plan body.

## Test plan

| Test | Cost | Cadence |
|---|---|---|
| Adapter unit tests (pytest + FakeProvider, no paid APIs) | free | every PR |
| Calibration matrix (`noop` + `dumb_script`) | free | on-demand (nightly is follow-up epic) |
| Cost-cap observation runs (Step 5, single task) | one-shot ~$1 | once before Phase A.1 |
| Phase A.1 matrix | derived from observation × 27 runs × safety | on demand |

## Risks

| Risk | Mitigation |
|---|---|
| Same-model-different-harness contaminated by hidden defaults | Caveat in every memo; D4 makes shape visible |
| Sonnet 4.6 (out of $1–3 bracket) is what user actually wants | Step 0 census surfaces the trade-off; user decides anchor |
| Forge not OSS-installable | Step 7 drop rule; documented |
| Container egress to providers blocked | Validated Step 1 before any other work |
| Pi / opencode license disallows benchmarking | Confirmed Step 7 |
| Frontier reference (codex, claude-code) ToS for public benchmarks | Reviewed before public memo; internal use unrestricted |
| Cost runaway | Per-cell + per-matrix cap; `budget_halted`; all four token streams tracked |
| Variance with 3 reps | Canary only for first matrix; expand subset only after rig stable |
| Provider rate-limit hit at concurrency > 1 | Default `--concurrency=1`; profile RPM/TPM caps on opt-in |
| Adapter bugs masquerade as harness skill differences | Calibration adapters in CI; adapter unit tests required |
| Telemetry shape gaming | D4 is descriptive only; reward is the first-class metric |

## Spec updates required (HELIX governing artifacts)

The existing specs (SD-008, SD-009) assume **one harness (ddx-agent), one provider config per run**. The matrix breaks both assumptions, so we file one new SD and amend two existing ones before any code lands. Per the project's HELIX evolve practice, **specs lead implementation**; do not start Step 2 without SD-010 in `Status: Approved`.

### NEW: SD-010 — Multi-Harness × Model Matrix Benchmark

`docs/helix/02-design/solution-designs/SD-010-harness-matrix-benchmark.md`. Section outline:

1. Summary + relation to SD-008/SD-009 (extends, does not replace)
2. **Architecture decisions D1–D6** (lifted from this plan: in-container only; single CLI entry; smoke vs matrix split; D4 telemetry schema; resumability policy; concurrency default)
3. **Profile schema (normative)** — the YAML in Step 2 of this plan, frozen as the v1 schema
4. **Adapter contract (normative)** — the Python protocol from Step 3; adapter unit-test requirements
5. **Failure taxonomy (normative)** — the 11-status enum from Step 4
6. **Aggregator output format (normative)** — the matrix.md / matrix.json / costs.json shapes from Step 4
7. **Same-model-different-harness caveat (publication policy)** — wording every memo must include
8. **ToS / fair-use policy** for frontier-reference cells (codex, claude-code) before public memos
9. Open questions: anchor model selection (deferred to Step 0 census)

Acceptance for SD-010: peer review by codex (or equivalent), user sign-off on the anchor-model open question, no implementation beads claimed before status flips to Approved.

### AMEND: SD-008 — Terminal-Bench / Harbor Integration Path

Add Section 6 "Multi-Harness Extension":
- In-container installability checklist for non-ddx-agent harnesses (Section 1 of SD-008 currently covers ddx-agent's `BaseInstalledAgent` flow only)
- Per-harness adapter file location (`scripts/benchmark/harness_adapters/`)
- Egress requirements for harnesses calling provider APIs from inside the container
- Drop rule: harnesses that can't be installed in-container are documented and excluded, NOT run host-side

### AMEND: SD-009 — Benchmark Mode

Three additions:
- **§5 (Metrics)**: state explicitly that thresholds (resolved-task floor 0.55, target 0.7) apply to ddx-agent's own runs only. Cross-harness cells use **mean reward + SD over reps**, not the floor/target. The floor/target was set against a single-arm baseline; reusing it for cross-harness comparison is an apples/oranges error.
- **§7 (Evidence-Grade Protocol)**: add multi-harness extension referencing SD-010. Same-model-different-harness comparison rules; required caveat block; minimum 3 reps per cell.
- **NEW §9 — Resumability and Failure Taxonomy** lifting D5 + the 11-status enum from this plan as normative.

### AMEND: FEAT-005 — Logging and Cost

Acknowledge the four token streams (input, output, cached-input, retried) tracked under SD-010. Profile pricing schema referenced as the source of $-per-Mtok numbers. Cost-cap enforcement is a feature requirement, not just a benchmark-runner detail.

### NO change required

- FEAT-001/002/003/004/006/007 — unaffected
- PRD / concerns — unchanged; the matrix is an evaluation tool, not a product feature
- SD-001 through SD-007 — unchanged

## Bead breakdown

One epic, four amend-spec tasks, one new-spec task, then implementation tasks ordered by the dependency graph below. Status, acceptance, and dependencies are normative; implementation beads cannot be claimed until their parent spec bead is closed.

```
EPIC agent-NEW1: Harness × Model Matrix Benchmark (initial tranche)
├── SPEC agent-NEW2: Author SD-010 (NEW spec) ─────────────┐
├── SPEC agent-NEW3: Amend SD-008 §6 (multi-harness)       │ (parallel; all four
├── SPEC agent-NEW4: Amend SD-009 §5/§7/§9                  │  close before any
└── SPEC agent-NEW5: Amend FEAT-005 (token streams)        ─┘  TASK is claimed)
    │
    ├── SPIKE agent-NEW6  Step 0    Model census (no code)
    ├── TASK  agent-NEW7  Step 1    Submodule init + egress canary (smoke model)
    ├── TASK  agent-NEW8  Step 2    Profile schema + Go loader (smoke + noop profiles)
    ├── TASK  agent-NEW9  Step 3    Adapter base + ddx-agent + noop + dumb_script
    ├── TASK  agent-NEW10 Step 4a   ddx-agent-bench matrix subcommand
    ├── TASK  agent-NEW11 Step 4b   matrix-aggregate subcommand
    ├── TASK  agent-NEW12 Step 5    Cost-cap observation procedure + guardrails
    ├── TASK  agent-NEW13 Step 6    Canary subset manifest
    ├── SPIKE agent-NEW14 Step 7    OSS harness install spike (pi, opencode)
    ├── TASK  agent-NEW15 Step 8a   Adapter: pi
    ├── TASK  agent-NEW16 Step 8b   Adapter: opencode
    ├── TASK  agent-NEW17           CI: adapter unit tests on PR
    ├── TASK  agent-NEW18 Step 5b   Anchor profile YAML (post-NEW6)
    └── TASK  agent-NEW19 Step 9    Phase A.1 matrix run + memo
```

**Deferred to follow-up tranche** (separate epic, filed after NEW19 closes):
- `forge` adapter (only if NEW14's expanded re-spike confirms installability)
- `codex` adapter (frontier reference; ToS approval for public memo)
- `claude-code` adapter (frontier reference; ToS approval for public memo)
- Phase A.2 matrix (anchor + second model + frontier-ref harnesses)
- Phase B matrix (lesser open models like Qwen 3.6-plus)
- Phase C matrix (local models / sampling sweep)
- Nightly calibration matrix in CI (initial: on-demand only)
- Concurrent execution scheduler

This is the codex-v6 cut: prove the runner with `ddx-agent + pi + opencode + one anchor` first; everything else moves to a later epic.

### Dependency / ordering rules

- All four spec beads (NEW2–NEW5) close before any TASK is claimed.
- NEW6 (census) blocks **only** NEW18 (anchor profile) and NEW19 (A.1 memo). It does NOT block NEW7–NEW17.
- NEW7 (egress canary) uses the **smoke model** — explicitly does not need NEW6.
- NEW8 (profile schema + Go loader) uses **smoke + noop test profiles** — explicitly does not need NEW6. Anchor profile is filed separately as NEW18 after NEW6.
- NEW7 blocks NEW9.
- NEW8 blocks NEW9 and NEW10.
- NEW9 (base + ddx-agent + calibration adapters) blocks NEW10.
- NEW10 (matrix runner) blocks NEW11 (aggregator), NEW12 (cost guards integrate here), NEW13 (canary subset) — NEW13 may draft in parallel.
- NEW14 (install spike, scoped to pi + opencode only) blocks NEW15 + NEW16. Either harness may be dropped → bead closed-with-reason=dropped.
- NEW17 (CI) lands after NEW9.
- NEW18 (anchor profile) needs NEW6 + NEW8.
- NEW19 (A.1 memo) is gated on NEW9 + NEW10 + NEW11 + NEW12 + NEW13 + NEW15 + NEW16 + NEW18.

**Frontier-ref ToS handling moves to the follow-up epic (codex v6 fix):** adapter implementation and publication approval are split there, not entangled in this initial tranche.

### Bead acceptance criteria

Per HELIX practice, every bead has a deterministic acceptance criterion. Examples (full text goes in the bead body when filed):

| Bead | Acceptance |
|---|---|
| NEW2 (SD-010) | Spec lands at `docs/helix/02-design/solution-designs/SD-010-harness-matrix-benchmark.md` covering D1–D6, profile schema, adapter contract, failure taxonomy, aggregator format, caveat policy, ToS policy. Status `Approved` after codex peer review + user sign-off on anchor-model. |
| NEW3 (Amend SD-008) | New §6 lands; cross-references SD-010. |
| NEW4 (Amend SD-009) | §5 thresholds scoped to ddx-agent only; §7 multi-harness extension cross-refs SD-010; §9 (new) resumability + failure taxonomy lifted from SD-010. |
| NEW5 (Amend FEAT-005) | Four-stream cost requirement documented; profile-pricing source cross-ref. |
| NEW6 (census) | `docs/research/model-census-2026-04-XX.md` lands; selection-fallback hierarchy applied per row; user signs off on (anchor, second-model). |
| NEW7 (egress canary) | Harbor produces `reward=1` on `hello-world` with the **smoke** model; result archived under `benchmark-results/egress-canary-<ts>/`. |
| NEW8 (profile schema) | Loader + smoke profile + noop test profile land; unit test passes; `ddx-agent-bench profiles list` prints a table. **Anchor profile lands separately as NEW18.** |
| NEW9 (base + 3 adapters) | `harness_adapters/{base,noop,dumb_script,ddx_agent}.py`; pytest passes (no paid APIs via FakeProvider); calibration matrix produces expected reward pattern. |
| NEW10 (matrix subcommand) | `ddx-agent-bench matrix --harnesses=noop,dumb_script --profiles=noop --reps=2` produces a valid matrix; `kill -9 + --resume` produces same matrix. |
| NEW11 (aggregator) | `matrix.json`, `matrix.md`, `costs.json` produced in documented schemas; aggregator unit-tested against a fixture matrix; null handling verified. |
| NEW12 (cost guards) | Observation procedure documented and run; per-run + per-matrix caps derived from observed p95 with $1/$5 floor/ceiling; synthetic over-budget stub triggers `process_outcome=budget_halted`; matrix continues. |
| NEW13 (canary subset) | Subset JSON lands with 3 task IDs + exclusion comments; loader test passes. |
| NEW14 (OSS install spike, pi+opencode) | Doc lands; each harness either kept (with pinned install + non-interactive flags + profile mapping documented) or dropped with rationale. |
| NEW15, NEW16 (pi, opencode adapters) | Adapter file + pytest; calibration matrix still passes. Dropped → closed-with-reason=dropped. |
| NEW17 (CI) | GitHub Actions runs adapter pytest on every PR. (Nightly calibration is deferred.) |
| NEW18 (anchor profile) | `profiles/<anchor>.yaml` lands with all schema fields + resolved snapshot ID; loader test passes for it. |
| NEW19 (A.1 memo) | Memo at `docs/research/matrix-baseline-phase-a1-2026-XX-XX.md`; all 27 runs reach a terminal `final_status`; ≥ 24/27 are `graded_*`; SD reported but not gated; cost reconciled to observation; caveat block per SD-010. If criteria fail, **file root-cause beads in the follow-up epic and close NEW19 with reason=blocked**, do not retry indefinitely. |

### Filing notes

- All beads inherit `labels: [helix, kind:benchmark, area:cli, area:tooling]`.
- Spec beads add `phase:design`. Implementation beads add `phase:build`. Spike beads add `kind:spike`.
- Epic ID assigned by `ddx beads create`; the `NEW#` placeholders above get replaced with real `agent-XXXXXXXX` IDs at filing time.
- Each bead links its parent and (for implementation beads) its blocking spec bead via the `parent` and `depends_on` fields.
- **This planning doc closes when NEW1 (epic) is filed.** Any further refinement after that point lives in SD-010, child beads, or memos — not in v7+ of this file. Keep the file as the historical artifact behind the epic.

## Deliverables (initial tranche only)

- `docs/helix/02-design/solution-designs/SD-010-harness-matrix-benchmark.md` (NEW2)
- Amendments to SD-008, SD-009, FEAT-005 (NEW3–NEW5)
- `docs/research/model-census-2026-04-29.md` (NEW6)
- `scripts/benchmark/profiles/{smoke,noop,<anchor>}.yaml` + `internal/benchmark/profile/` Go loader (NEW8 + NEW18)
- `scripts/benchmark/harness_adapters/{base,noop,dumb_script,ddx_agent,pi,opencode}.py` + pytest with FakeProvider (NEW9 + NEW15 + NEW16)
- `cmd/bench/matrix.go`, `matrix_aggregate.go`, tests (NEW10 + NEW11)
- `scripts/beadbench/external/termbench-subset-canary.json` (NEW13)
- `docs/research/oss-harness-install-2026-XX-XX.md` (NEW14)
- `docs/research/matrix-baseline-phase-a1-2026-XX-XX.md` (NEW19)
- CI: adapter pytest on PR (NEW17)

Phase A.2, B, C, frontier-ref adapters, forge, nightly calibration, and concurrency live in a follow-up epic filed after NEW19 closes.
