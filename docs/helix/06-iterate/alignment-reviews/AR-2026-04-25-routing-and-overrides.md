# Alignment Review: routing redesign and override-as-failure-signal

## Review Metadata

**Review Date**: 2026-04-25
**Scope**: routing → caching → overrides — three connected design shifts that landed across v0.9.9 and v0.9.10 and culminated in ADR-006's reframe of how callers interact with the routing surface
**Status**: complete
**Releases**: v0.9.9, v0.9.10
**Primary Governing Artifacts**: ADR-005, ADR-006, CONTRACT-003, SD-005
**Active Principles**: HELIX defaults; specs+code pairing; routing-test-rigor

## Scope and Governing Artifacts

### Scope

- ADR-005 smart routing replaces the removed route-table field (v0.9.9 release work)
- v0.9.10 prompt-caching across the stack (CachePolicy contract, Anthropic
  cache_control, openai-compat prefix-stability gate, cache-aware cost
  attribution)
- ADR-006 manual overrides are auto-routing failure signals (drafted
  2026-04-25, bead chain in flight)
- Spec amendment pass (`9a04ad4`) closing eight ambiguities (A1-A10
  coverage audit) on selection precedence, escalation ladder, cache
  scope, signal keying

### Governing artifacts touched

- `docs/helix/02-design/contracts/CONTRACT-003-fizeau-service.md`
- `docs/helix/02-design/solution-designs/SD-005-provider-config.md`
- `docs/helix/02-design/architecture.md` (Caching section added)
- `docs/helix/02-design/adr/ADR-005-smart-routing-replaces-model-routes.md`
- `docs/helix/02-design/adr/ADR-006-overrides-as-routing-failure-signals.md`
- `AGENTS.md` (Review and Verification Discipline, Spec Amendment
  Discipline, Bead Sizing and Cross-Repo Triage sections — added in this
  review's commit)

## Findings

### F1. Routing as optimizer, pins as override hatches

**Originally framed:** pin precedence (`Harness` → `Provider` → `Model` → `ModelRef` → `Profile`) was a primary user surface.

**Reframed (ADR-006):** the system exists to complete prompts at minimum cost and time. Auto-routing is the optimizer. Manual pins exist *only* as override hatches when auto picks badly. **Therefore every override is by definition a routing-quality failure signal**, and override capture becomes the primary feedback loop for routing improvement.

`Profile` is intent (cost-vs-time preference declaration), not an override. `ModelRef` is intent (tier alias). Only `Harness`, `Provider`, `Model` qualify as overrides — each tracked as an independent axis. Coincidental agreement (pin matches what auto would have picked anyway) still emits an override event because the user *intended* to assert; that's a UX signal that auto's reasoning is opaque enough to invite defensive pinning.

ADR-006 soft-supersedes ADR-005's framing of §1; mechanics unchanged. CONTRACT-003 selection precedence is now demoted to "implementation reference" via a disclaimer paragraph.

### F2. Two metrics, two questions, distinct UI

`per-(provider, model) success rate` measures **provider reliability** ("given that we routed to X, did X work?"). `auto_acceptance_rate` and `override_disagreement_rate` measure **routing quality** ("given the prompt, did we route to the right place?"). The two compose: routing quality × provider reliability ≈ end-to-end completion rate.

The legacy UI conflated them. Bead 2 of the ADR-006 chain (`agent-017b043f`) renames or relabels the existing metric to keep them distinct.

### F3. Specs+code pairing prevents drift

Codex spec-review pass (gpt-5.5) caught multiple aspirational amendments where the proposed prose claimed behavior the code didn't have:

- Provider-as-hard-pin claim — code only hard-pinned under `Harness+Provider`.
- Pre-dispatch HealthCheck re-validation — not implemented; `Execute` resolves once and dispatches.
- Quota cost-ramp wording — actual implementation uses score-penalty, not effective-cost mutation.
- `tools_supported: false` field — actual manifest field is `no_tools: true`.

Discipline locked into AGENTS.md "Spec Amendment Discipline": amendments either describe what the code does (with file:line citations) or land in the same commit as the code change. No aspirational specs.

### F4. Reviewer fidelity is model-shaped

gpt-5.5 reviewer behavior:
- **Excellent** at catching fake tests (the override-bead's `TestOverrideEventCoincidentalAgreement` self-incriminated: "we instead synthesize the scenario by..." admitting it didn't synthesize it).
- **Excellent** at code-vs-spec mismatch in spec-review passes.
- **Strict** about AC test names — promised tests must exist and exercise the AC's stated assertion.
- **Bad** at distinguishing environmental test failures from real defects — issued false BLOCKs on three caching beads where `httptest.NewServer` couldn't bind in its sandbox.

Discipline locked into AGENTS.md "Review and Verification Discipline": always verify locally before accepting reviewer verdicts.

### F5. False-positive `already_satisfied` is a real harness failure mode

Three beads in this review's window closed with `already_satisfied` status while structural ACs were not met:
- `agent-9d120ece` (route resolution) — closed on regression-test pass; AC #1 structural requirement unmet.
- `agent-1023f072` (boundary tightening) — same pattern earlier.
- `agent-d9c358ba` (smart-routing wiring) — split by worker analysis instead of attempted, but still classified `already_satisfied`.

The harness heuristic — "regression tests pass → close" — is too lax. Recommendation: tighten upstream in the `ddx` repo to require the AC's named structural test functions actually run before classifying `already_satisfied`. Bead to be filed in the `ddx` tracker as a follow-up (not part of this review).

The local discipline (verify before accept, reopen with notes when defects found) caught all three this session.

### F6. Bead sizing matters for reviewability

Two beads in this window were too broad and the worker correctly returned `already_satisfied` with a split recommendation:
- `agent-9d120ece` (route resolution) — split into 4 sub-beads.
- `agent-d9c358ba` (smart-routing wiring) — split into 4 sub-beads.

Recurring trigger: scope crosses CLI ↔ service ↔ engine boundaries, or AC names ≥ 3 prescribed test files. Discipline locked into AGENTS.md "Bead Sizing and Cross-Repo Triage."

### F7. Cross-repo bug ownership

`agent-9465287e` (execute-loop drops `--harness` override on REQUEST_CHANGES retry) was filed in this repo but the actual fix surface lives in the `ddx` CLI repo. Re-filed as `ddx-c67969d5` upstream with a sharp principle in the description: **retries must use the same parameters as the initial work**. Closed locally as `tracked upstream`.

Pattern documented in AGENTS.md "Bead Sizing and Cross-Repo Triage."

## Releases

- **v0.9.9** (2026-04-25) — ADR-005 implementation + service-boundary work.
- **v0.9.10** (2026-04-25) — prompt caching across the stack.
- v0.9.11 (planned) — ADR-006 bead chain (override telemetry, routing-quality metrics, operator CLI).

## Outcomes and follow-ups

### Closed in this window

13 beads closed in the v0.9.9 → v0.9.10 → ADR-006-prep window. Includes the entire `agent-10f643cc` boundary epic and four caching beads.

### Open at review close

ADR-006 chain in flight (`agent-9fc2633c`, `agent-017b043f`, `agent-79aedde2`). Worker `worker-20260425T234911-5974` retrying bead 1 after a real REQUEST_CHANGES with four substantive defects (test rigor + implementation gap on `isExplicitPinError`).

### Deferred

- A1 hardening (`Provider` always-hard) — open design call.
- A7 cooldown re-keying to per-(provider, model) — open design call.
- A6 pre-dispatch HealthCheck re-validation — needs `serviceRouteProvider` ineligible-skip first.
- Automatic learning loop driven by `override_class_breakdown` — needs override-event data first; future ADR-007 candidate.
- Anthropic 1-hour TTL beta header — needs measurement.
- Gemini `cachedContentTokenCount` ingestion.
- OpenRouter live quota plumbing.

### Upstream

- `ddx-c67969d5` — execute-loop retry must reuse initial worker params.
- (To be filed) — tighten `already_satisfied` gate to require structural AC test name match.

## Sign-off

Specs current, code current, AGENTS.md updated. Two clean releases shipped. ADR-006 chain operationalizes the override-as-failure-signal feedback loop that this review codified.
