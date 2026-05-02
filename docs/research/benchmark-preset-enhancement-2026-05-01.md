---
ddx:
  id: benchmark-preset-enhancement-2026-05-01
  created: 2026-05-01
  reviewed_by: pending
status: DRAFT v1
exit_criterion: Filing of epic bead. Subsequent revisions live in child beads.
---

# Benchmark Preset Enhancement for terminal-bench-2

## Background and Diagnosis

The `benchmark` preset at `internal/prompt/presets.go` produces 0 passes on terminal-bench-2 with Qwen3.6-27B. Four documented failure modes:

**FM-1 — Tool-Call Loop** (`ErrToolCallLoop`): Agent issues same bash command 3× without adapting.  
**FM-2 — Missing Output File**: Agent reasons to a solution in `<think>` blocks and declares success, but never calls the `write` tool.  
**FM-3 — Reasoning Stall**: Agent fills context with extended thinking without emitting tool calls. The stall guard counts tool calls — pure reasoning turns are invisible to it.  
**FM-4 — No Verification**: Agent writes the file but exits without running the provided test script.

## Changes to `internal/prompt/presets.go`

### Base text change (trailing paragraph only)

```diff
-Work systematically: read relevant files first using the read tool, make changes using edit or write tools, verify with bash (builds/tests), and report concisely.
+Work systematically: (1) read task instructions and relevant files, (2) plan your approach in one short mental step, (3) implement using edit or write tools, (4) verify the output exists and passes any provided test, (5) report results.
```

### 5 new guidelines appended to existing 11

```go
"ERROR RECOVERY: If a tool call fails or returns an error, change your approach before retrying — never issue the same failing command twice in a row",
"OUTPUT VERIFICATION: Before reporting task complete, confirm the required output file exists by reading it or running bash to check; do not declare success until the file is on disk",
"ANTI-STALL: If you have been thinking for more than one turn without calling a tool, call a tool immediately in your next response — do not continue reasoning without acting",
"RUN THE TEST: If a test script is mentioned or visible (e.g. test_outputs.py, test.sh), run it with bash as your final verification step before reporting done",
"TOOL FIRST: Think through your plan in at most one reasoning paragraph, then call tools — extended thinking before acting wastes context and risks timeout",
```

## Token Count Estimate

| Component | Before | After | Delta |
|---|---|---|---|
| Base text | ~290 tokens | ~298 tokens | +8 |
| Guidelines (11→16 items) | ~210 tokens | ~305 tokens | +95 |
| **Total preset contribution** | **~500** | **~603** | **+103** |

The increase is ~17% but absolute count remains small. 30k+ tokens remain free for task context and trajectory with Qwen3.6-27B's 32k window.

## Test Strategy

Changes to `internal/prompt/presets_test.go`:

- `TestBenchmarkPreset_ErrorRecoveryGuideline`: assert all 5 new guidelines present by exact string.
- `TestBenchmarkPreset_VerificationStepInBase`: assert `p.Base` contains `"verify the output exists"` and `"pass any provided test"`.
- `TestBenchmarkPreset_GuidelineCount`: assert `len(GetPreset("benchmark").Guidelines) == 16`.
- Existing `TestPresets_NoDuplicateGuidelines` must continue to pass.

No golden fixtures affected. The harness golden integration test is independent of preset text.

## Bead Breakdown

| Bead | Title | Deps | Size |
|---|---|---|---|
| PRESET-1 (epic) | Benchmark preset: improve terminal-bench-2 pass rate | — | S |
| PRESET-2 | Add 5 new guidelines + update base text in `presets.go` | — | S |
| PRESET-3 | Structural tests for new guidelines in `presets_test.go` | PRESET-2 | S |

## Why S Overall

Single file change, text-only, no new functions, no interface changes. No harness adapter changes — `ddx_agent.py` already passes `--preset benchmark` verbatim. The only judgment call is exact wording (caps-prefix pattern matches existing BENCHMARK MODE RULES style).
