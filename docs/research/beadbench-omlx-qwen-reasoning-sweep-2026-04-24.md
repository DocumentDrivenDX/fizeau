# Beadbench OMLX Qwen Reasoning Sweep (2026-04-24)

Goal: identify hard `execute-bead` tasks where a frontier baseline succeeds and
local `OMLX Qwen 3.6` fails, then measure whether changing the OMLX reasoning
budget changes the outcome.

## Reasoning-level mapping

The native agent maps named reasoning levels to Qwen `thinking_budget` tokens
through the portable reasoning policy in
`internal/reasoning/reasoning.go` and the Qwen serialization path in
`internal/provider/openai/openai.go`.

| effort | budget tokens |
| --- | ---: |
| `low` | `2048` |
| `medium` | `8192` |
| `high` | `32768` |

Manifest arm added for this sweep:

- `agent-vidar-omlx-qwen36-27b-high`

## Initial hard-task slice

Start with repo-local hard tasks so verifier/runtime noise stays low:

1. `agent-routing-profiles`

Planned follow-on hard tasks after the first labeled split lands:

1. `ddx-worker-lifecycle-controls`
2. `axon-auth-error-observability`
3. `niflheim-race-detection-scheduler`

## Commands

Frontier baseline:

```bash
python3 scripts/beadbench/run_beadbench.py \
  --task agent-routing-profiles \
  --arm claude-opus47 \
  --timeout-seconds 1200
```

Local OMLX low:

```bash
python3 scripts/beadbench/run_beadbench.py \
  --task agent-routing-profiles \
  --arm agent-vidar-omlx-qwen36-27b-low \
  --timeout-seconds 600
```

Local OMLX medium:

```bash
python3 scripts/beadbench/run_beadbench.py \
  --task agent-routing-profiles \
  --arm agent-vidar-omlx-qwen36-27b \
  --timeout-seconds 600
```

Local OMLX high:

```bash
python3 scripts/beadbench/run_beadbench.py \
  --task agent-routing-profiles \
  --arm agent-vidar-omlx-qwen36-27b-high \
  --timeout-seconds 600
```

## Results

### Task: `agent-routing-profiles`

| arm | run id | outcome | verifier | duration | notes |
| --- | --- | --- | --- | ---: | --- |
| `claude-opus47` | `run-20260424T014537Z-677067` | `success` | `pass` | `768079ms` | Preserved successful implementation commit `7756528ecc2e10a5c41025051b8ccfba5950d127`; full `go test ./...` verifier passed. |
| `agent-vidar-omlx-qwen36-27b-low` | `run-20260424T014835Z-690879` | `timeout` | `skipped` | `600472ms` | `progress_class=write_progress`; the run did mutate the sandbox before timing out, so this is not a pure no-progress stall. |
| `agent-vidar-omlx-qwen36-27b` | `run-20260424T015859Z-732183` | `timeout` | `skipped` | `600585ms` | `progress_class=write_progress`; increasing budget from `2048` to `8192` did not rescue the task. Live process inspection during the run showed the worker spending over a minute on `find / -name "models.yaml" -type f`, which is evidence of tool-choice waste in addition to inference/runtime cost. |
| `agent-vidar-omlx-qwen36-27b-high` | `run-20260424T020941Z-763803` | `timeout` | `skipped` | `600488ms` | `progress_class` still unresolved at note-write time, but the run hit the same 600s wall as `low` and `medium`. Raising the budget to `32768` still did not produce a completed verified attempt. |

## Initial read

One hard task is already labeled:

- `claude-opus47` can complete and verify `agent-routing-profiles`.
- `OMLX qwen36-27b low` cannot complete the same bead inside `600s`.
- `OMLX qwen36-27b medium` also cannot complete the same bead inside `600s`.
- `OMLX qwen36-27b high` also cannot complete the same bead inside `600s`.

That already makes `agent-routing-profiles` a valid complexity-estimator example
in the bucket “frontier subscription succeeds, local Qwen low fails.”

The low-effort timeout is also not a pure cold-start wait. Beadbench classified
it as `write_progress`, which means the worker changed the sandbox before the
timeout. Medium showed the same timeout class and, during live inspection,
spent measurable wall time on a full-filesystem `find / -name "models.yaml"`
search. That points at the native agent’s tool-use policy as part of the
failure mode. `High` did not rescue the run either, so this task currently
belongs in the bucket “frontier harness succeeds; local Qwen fails across all
tested reasoning budgets.”

## Harness-control follow-up

To separate model quality from harness quality, run a known-good Sonnet bead on
both the Claude harness and the native agent through OpenRouter:

```bash
python3 scripts/beadbench/run_beadbench.py \
  --task helix-contract001-audit \
  --arm claude-sonnet46 \
  --arm agent-openrouter-sonnet46 \
  --timeout-seconds 900
```

That control answers a different question from the OMLX sweep:

- if `claude-sonnet46` passes and `agent-openrouter-sonnet46` fails, the gap is
  almost certainly in the native agent loop, tools, or prompting;
- if both pass, then the native loop can handle at least this long-context
  evidence-extraction task when the underlying model is strong enough.

### Task: `helix-contract001-audit`

| arm | run id | outcome | verifier | duration | notes |
| --- | --- | --- | --- | ---: | --- |
| `claude-sonnet46` | `run-20260424T020827Z-762055` | `success` | `pass` | `545354ms` | Produced preserved result `8eacc2f381751309d1b331981e62cd7d50664e39`; verifier `test -f docs/helix/02-design/contracts/CONTRACT-001-audit.md && ! rg -n 'TBD|investigating' ...` passed. |
| `agent-openrouter-sonnet46` | `run-20260424T020827Z-762055` | `execution_failed` | `skipped` | `220390ms` | Native agent + OpenRouter Sonnet 4.6 hit `iteration_limit` after `2,098,037` tokens and `$6.409526999999999` cost. This is the cleanest harness-control failure in the current set: same model family, different harness, very different outcome. |
| `agent-openrouter-sonnet46` | `run-20260424T022504Z-769145` | `execution_failed` | `skipped` | `304477ms` | Rerun after native benchmark-mode bash hardened to block shell `find` and `ls -R`. Failure mode stayed `iteration_limit`; tokens dropped slightly to `1,984,867` and cost to `$6.127209`, but the task still did not complete. The native harness problem is therefore deeper than one bad shell-discovery pattern. |

## Pi comparison points

Pi's default CLI tool surface is narrower than the native benchmark preset:

```text
--tools <tools> ... (default: read,bash,edit,write)
Available: read, bash, edit, write, grep, find, ls
```

That matters for harness comparison:

- Pi can use `find`, `grep`, and `ls`, but they are off by default.
- The native benchmark preset exposes `read`, `write`, `edit`, `bash`, `find`,
  `grep`, `ls`, and `patch` by default.
- After the 2026-04-24 native hardening pass, benchmark-mode `bash` now blocks
  shell `find` and `ls -R` and directs the model back to structured tools.

The Sonnet control rerun shows that this tool-level hardening is necessary but
not sufficient. It removed one obvious anti-pattern, but the native
OpenRouter/Sonnet path still burns roughly two million tokens and dies on
`iteration_limit` where the Claude harness completes in one pass.

## Current read

The benchmark evidence now supports two distinct claims:

1. `agent-routing-profiles` is a valid hard-task separator for reasoning-budget
   work: `claude-opus47` succeeds while `OMLX qwen36-27b` fails at low,
   medium, and high.
2. The native-agent harness has its own quality gap independent of local-model
   quality: `claude-sonnet46` succeeds on `helix-contract001-audit`, while the
   native OpenRouter Sonnet 4.6 arm fails twice on `iteration_limit` at very
   high token cost.

That makes the next optimization targets concrete:

- system/execute-bead prompt compression and loop discipline for the native
  agent path;
- tool-surface and tool-result shaping so strong cloud models do not waste
  iterations inside the native loop;
- richer native execution logging, because the kept rerun sandbox still
  persisted only the final embedded session line rather than a full tool/event
  trace.
