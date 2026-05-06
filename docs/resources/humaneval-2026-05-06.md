# HumanEval Resource

Captured: 2026-05-06

Source:
<https://github.com/openai/human-eval>

## Definition

HumanEval is an evaluation harness for the HumanEval problem-solving dataset
from OpenAI's "Evaluating Large Language Models Trained on Code" paper.

The official repository evaluates generated completions stored as JSONL rows
with `task_id` and `completion`. The evaluator reports `pass@k` metrics and
writes per-completion results that indicate whether a completion passed, timed
out, or failed.

## Relevance To FHI

HumanEval is useful, but it is not a primary FHI benchmark.

It measures isolated function-level code generation. It does not meaningfully
exercise:

- multi-step repository navigation
- terminal/tool-loop discipline
- harness permission and subprocess behavior
- context management
- long-horizon retry and debugging
- session-log observability

It should therefore contribute as a low-cost coding component or model-power
anchor, not as a dominant harness-intelligence signal.

## Import Guidance

When importing HumanEval rows into the benchmark evidence ledger:

- `source.type`: `external_leaderboard`, `imported_report`, or `fizeau_runner`
- `source.name`: `humaneval`
- `source.url`: `https://github.com/openai/human-eval`
- `benchmark.name`: `humaneval`
- `benchmark.version`: dataset commit or package version
- `subject.model_raw`: model string used to generate completions
- `subject.harness`: harness used for generation, or `none` for direct API
- `subject.provider`: provider used for generation, or `unknown`
- `scope.task_id`: HumanEval task id for atomic records
- `score.metric`: `pass_at_1`, `pass_at_10`, `pass_at_100`, or `passed`
- `runtime.outcome`: `passed`, `timed_out`, or `failed` for completion-level
  records

For FHI, use HumanEval as a small component that helps distinguish raw model
coding capability from harness effects observed on TerminalBench, beadbench,
SkillsBench, and SWE-bench.
