---
title: User Story → Website Coverage Map
status: research
date: 2026-05-10
note: |
  Fizeau's HELIX corpus does not have a separate `user-stories/` tree.
  The leaf-level concrete behaviors live as **Acceptance Criteria** rows
  in each `FEAT-00X-*.md`. Each AC row is treated as one "user story"
  here. Story IDs use the AC IDs directly (e.g. `AC-FEAT-001-01`).
---

# User-Story-to-Website Coverage Map

Each row is one acceptance-criterion-as-user-story (AC = US in this
project's vocabulary). `website page` = the URL on the public site that
documents the *feature* the AC describes; "GAP" means there is no
public-facing page for the feature.

`demo-able`: **Y** if the behavior produces a clear before/after, a
visible terminal artifact, or a `fiz` invocation whose output the
viewer can read in 30 seconds. **N** if it is internal plumbing
(library API, error type, telemetry payload).

## FEAT-001 — Agent Loop

| story_id | story title | feature | website page | demo-able |
|---|---|---|---|---|
| AC-FEAT-001-01 | Text-only response terminates Run() with success + tokens | Agent loop happy path | `/docs/getting-started/`, `/docs/embedding/` | Y |
| AC-FEAT-001-02 | Tool-calling turns execute in order, results feed next request | Tool-calling loop | `/docs/tools/` | Y |
| AC-FEAT-001-03 | Iteration limits, ctx cancel, retry exhaustion terminate cleanly | Loop termination | `/docs/embedding/` (callouts) | N (internal) |
| AC-FEAT-001-04 | Session lifecycle events emitted in seq order | Session log event ordering | `/docs/observability/` | N (telemetry internals) |
| AC-FEAT-001-05 | Streaming providers — delta assembly, NoStream fallback, timing | Streaming + first-token timing | `/benchmarks/` (TTFT chart) | Y (TTFT visible) |
| AC-FEAT-001-06 | Concurrent Run() calls do not share state; compaction no-fit fails closed | Concurrency + compaction safety | GAP | N |
| AC-FEAT-001-07 | Reasoning-content byte limit aborts with ErrReasoningOverflow | Reasoning overflow guard | GAP | Y (intentional failure visible at terminal) |
| AC-FEAT-001-08 | Reasoning stall timeout aborts with ErrReasoningStall | Reasoning stall guard | GAP | Y (visible failure) |
| AC-FEAT-001-09 | 3+ identical tool calls in a row → ErrToolCallLoop | Tool-call loop detector | GAP | Y (visible failure) |
| AC-FEAT-001-10 | reasoning_byte_limit configurable via config.yaml | Config knob | `/docs/getting-started/` (mentions config) | N |

## FEAT-002 — Tools

| story_id | story title | feature | website page | demo-able |
|---|---|---|---|---|
| AC-FEAT-002-01 | read/write/edit/bash core semantics | Built-in tools | `/docs/tools/` | Y |
| AC-FEAT-002-02 | Binary read rejected; grep skips binaries; oversized output truncated | Tool safety | `/docs/tools/` | Y (truncation marker visible) |
| AC-FEAT-002-03 | File path resolves chained symlinks; outside-workdir behavior | Path semantics | GAP | N |
| AC-FEAT-002-04 | find/grep/ls/patch implement documented behavior | Navigation tools | `/docs/tools/` | Y (each tool individually) |
| AC-FEAT-002-05 | task tool create/update/get/list, structured errors, concurrency-safe | task tool | `/docs/tools/#task` | Y (multi-step plan visible) |
| AC-FEAT-002-06 | Model-backed integration test exercises full tool surface | Integration test gate | GAP | N |
| AC-FEAT-002-07 | RTK proxy execution for allowlisted bash commands | RTK opt-in bash filter | GAP | Y (RTK on/off comparison) |

## FEAT-003 — Providers

| story_id | story title | feature | website page | demo-able |
|---|---|---|---|---|
| AC-FEAT-003-01 | Provider config rejects `openai-compat` as identity (see audit I-16) | Provider identity validation | GAP (and contradicts `/docs/getting-started/`) | N |
| AC-FEAT-003-02 | Inventory + attempt metadata reports provider system, endpoint, billing | Provider inventory | `/docs/cli/fiz_providers/` | Y (`fiz providers` table) |
| AC-FEAT-003-03 | BillingForProviderSystem classifies fixed/per-token/unknown | Billing classification | GAP | N |
| AC-FEAT-003-04 | Pay-per-token providers excluded from default routing unless opted in | Default-deny routing | `/docs/routing/` | Y (`fiz route-status` before/after include_by_default) |
| AC-FEAT-003-05 | Policy reqs beat pins: air-gapped + remote pin → ErrPolicyRequirementUnsatisfied | Policy enforcement | `/docs/routing/` | Y (visible failure with reason) |
| AC-FEAT-003-06 | Limit/utilization probes preserve unknown vs available state | Probe state preservation | GAP | N |

## FEAT-004 — Model Routing

| story_id | story title | feature | website page | demo-able |
|---|---|---|---|---|
| AC-FEAT-004-01 | Embedded manifest is schema v5, validates models/policies/providers | Catalog schema | `/docs/cli/fiz_catalog/` | N |
| AC-FEAT-004-02 | ListPolicies returns exactly cheap/default/smart/air-gapped | Canonical policies | `/docs/cli/fiz_policies/`, `/docs/routing/` | Y (`fiz policies`) |
| AC-FEAT-004-03 | Each policy produces documented local/sub/remote behavior | Policy semantics | `/docs/routing/` | Y (4-policy comparison) |
| AC-FEAT-004-04 | Pay-per-token default-deny in unpinned routing | (dup of FEAT-003-04) | `/docs/routing/` | Y |
| AC-FEAT-004-05 | Pins override default inclusion but not require[] | Pin precedence | `/docs/routing/` | Y |
| AC-FEAT-004-06 | Soft power scoring penalizes undershoot more than overshoot | Power scoring | GAP | N |
| AC-FEAT-004-07 | Route decisions expose typed rejection reasons + score components | Route evidence | `/docs/cli/fiz_route-status/` | Y (`fiz route-status --json`) |
| AC-FEAT-004-08 | Removed v0.10 names not advertised by policy listing/CLI/service | Surface hygiene | `/docs/cli/_index/` | N |

## FEAT-005 — Logging and Cost

| story_id | story title | feature | website page | demo-able |
|---|---|---|---|---|
| AC-FEAT-005-01 | JSONL session logs contain ordered events with stable session_id | Session log shape | `/docs/observability/` | Y (`fiz log` + tail of JSONL) |
| AC-FEAT-005-02 | Replay renders human-readable transcript without mutating log | `fiz replay` | `/docs/cli/fiz_replay/`, `/demos/` | Y (already in demos) |
| AC-FEAT-005-03 | Provider-reported cost wins; mixed/unknown forces session unknown | Cost attribution | `/docs/observability/` | Y (`fiz usage` vs known/unknown) |
| AC-FEAT-005-04 | OTel export conforms to CONTRACT-001 | OTel surface | `/docs/observability/` | N |
| AC-FEAT-005-05 | `fiz usage` supports table/JSON/CSV with time-window filters | `fiz usage` | `/docs/cli/fiz_usage/` | Y (`fiz usage --since 7d`) |
| AC-FEAT-005-06 | Unwritable log dir / OTel export failure → best-effort, run completes | Failure modes | GAP | N |
| AC-FEAT-005-07 | Per-run cost cap halts loop before next request, status budget_halted | Cost cap | GAP | Y (cap-halt visible at terminal) |
| AC-FEAT-005-08 | v0.11 session log preserves policy/power_policy attribution | Routing-attr persistence | GAP | N |

## FEAT-006 — Standalone CLI

| story_id | story title | feature | website page | demo-able |
|---|---|---|---|---|
| AC-FEAT-006-01 | Prompt input from `run`, -p, @file, stdin, DDx envelope | Prompt input modes | `/docs/cli/fiz_run/` | Y |
| AC-FEAT-006-02 | Exit codes 0/1/2 + deterministic stdout/stderr/JSON | Exit semantics | `/docs/cli/_index/` | Y |
| AC-FEAT-006-03 | Config precedence: defaults < global < project < env < flags | Config precedence | `/docs/getting-started/` | Y (env override demo) |
| AC-FEAT-006-04 | `fiz policies` and `fiz harnesses` table + JSON | Discovery commands | `/docs/cli/fiz_policies/`, `/docs/cli/fiz_harnesses/` | Y |
| AC-FEAT-006-05 | --policy/--min-power/--max-power feed ServiceExecuteRequest | Power flags | `/docs/cli/_index/` | Y (route-status comparison) |
| AC-FEAT-006-06 | --profile/--model-ref/--backend rejected with migration guidance | Removed-flag rejection | GAP (and website still shows ModelRef in code samples — see audit I-03) | Y (visible error message) |
| AC-FEAT-006-07 | log/replay/usage operate against effective session-log dir | Session-aware subcommands | `/docs/cli/fiz_log/`, `/docs/cli/fiz_replay/`, `/docs/cli/fiz_usage/` | Y |
| AC-FEAT-006-08 | DDx harness mode returns structured JSON | Harness output | `/docs/cli/fiz_run/` (--json), `/docs/embedding/` | Y (JSON output diff) |

## FEAT-007 — Self-update and installer

| story_id | story title | feature | website page | demo-able |
|---|---|---|---|---|
| AC-FEAT-007-01 | `fiz version` + `fiz update --check-only` semver compare, exit 0/1 | Version check | `/docs/cli/fiz_version/`, `/docs/cli/fiz_update/` | Y |
| AC-FEAT-007-02 | `fiz update` downloads, verifies, atomic-replaces binary | Self-update | `/docs/cli/fiz_update/` | Y (visible progress/no-partial) |
| AC-FEAT-007-03 | Network/perm/disk failures leave existing binary intact | Update failure modes | GAP | Y (intentional break) |
| AC-FEAT-007-04 | install.sh selects correct artifact, installs to target dir | Installer | `/docs/getting-started/` | Y (one-line curl install) |
| AC-FEAT-007-05 | install.sh updates bash/zsh/fish PATH idempotently | PATH wiring | GAP | Y (re-run idempotent) |
| AC-FEAT-007-06 | Acceptance tests run without live GitHub | Test isolation | GAP | N |

---

## Coverage rollup

| Bucket | Stories | Has website page | GAP |
|---|---|---|---|
| FEAT-001 | 10 | 5 | 5 |
| FEAT-002 | 7 | 4 | 3 |
| FEAT-003 | 6 | 3 | 3 |
| FEAT-004 | 8 | 6 | 2 |
| FEAT-005 | 8 | 4 | 4 |
| FEAT-006 | 8 | 7 | 1 |
| FEAT-007 | 6 | 3 | 3 |
| **Total** | **53** | **32 (60%)** | **21 (40%)** |

| Bucket | Demo-able | Not demo-able |
|---|---|---|
| Total | **35 (66%)** | 18 (34%) |

## Top website-coverage gaps (Part B output, top 5)

1. **Cost cap (`AC-FEAT-005-07`)** — `cost_cap` budget halt is fully implemented but
   completely unmentioned on the public site. High differentiator vs.
   competitors. → propose new `/docs/cost-control/` page.
2. **Reasoning guards (`AC-FEAT-001-07`, `-08`, `-09`)** — three different
   safety nets (overflow, stall, tool-call loop) protecting against
   model misbehavior; none discussed publicly. → consolidate under
   `/docs/safety-rails/` or expand `/docs/observability/`.
3. **RTK opt-in bash filtering (`AC-FEAT-002-07`)** — proxied bash for
   allowlisted commands is a real production safety story. → add
   `/docs/tools/#rtk` section.
4. **OTel surface details (`AC-FEAT-005-04`)** — page exists at
   `/docs/observability/` but the audit did not verify it actually
   covers CONTRACT-001 attribute semantics. → cross-check.
5. **Pin / power / policy interaction (`AC-FEAT-004-05`, `-06`)** —
   `/docs/routing/` exists but power-scoring penalty math (undershoot
   vs overshoot) is invisible. → add a worked example.
