# Rapid-MLX Model-Harness Index Resource

Captured: 2026-05-06

Source commit:
<https://github.com/raullenchai/Rapid-MLX/commit/903487e82ad1998f0c20b721a7df66ec815ea673>

Commit title:
`feat: MHI benchmark, one-liner installer, auto Homebrew publish (#110)`

## Definition

Rapid-MLX defines MHI as Model-Harness Index: a model x harness score on a
0-100 scale.

Formula from the commit README:

```text
MHI = 0.50 * ToolCalling + 0.30 * HumanEval + 0.20 * MMLU
```

Dimensions:

| Dimension | Weight | Source in Rapid-MLX commit |
| --- | ---: | --- |
| Tool Calling | 50% | `rapid-mlx agents --test` harness tool-call scenarios |
| HumanEval | 30% | 10 HumanEval tasks |
| MMLU | 20% | 10 tinyMMLU tasks |

The commit message also describes an alternate script-level framing:
TAU-bench 50%, HumanEval 30%, and MMLU 20%. The committed
`scripts/mhi_eval.py` contains that TAU-bench path, while the README leaderboard
describes tool-calling results from `rapid-mlx agents --test`. Treat this as a
useful source pattern with naming drift, not as a fully stable external
standard.

## Relevant Files

- `README.md` — MHI definition, formula, leaderboard, and interpretation.
- `scripts/mhi_eval.py` — reproducible evaluation script for TAU-bench,
  HumanEval, and tinyMMLU subsets.
- `scripts/mhi_batch.sh` — batch runner, though it still references ACI in
  comments/output paths.
- `reports/mhi/*.json` — checked-in sample reports.

## Reported Examples

The commit README reports:

| Model + Harness | Tool Calling | HumanEval | MMLU | MHI |
| --- | ---: | ---: | ---: | ---: |
| Qwopus 27B + Hermes | 100% | 80% | 90% | 92 |
| Qwopus 27B + PydanticAI | 100% | 80% | 90% | 92 |
| Qwopus 27B + LangChain | 100% | 80% | 90% | 92 |
| Qwopus 27B + smolagents | 100% | 80% | 90% | 92 |
| Qwopus 27B + Anthropic SDK | 100% | 80% | 90% | 92 |

## Implications For Fizeau

MHI is directly relevant to Fizeau's benchmark evidence ledger because it
separates model x harness behavior from model-only capability.

For Fizeau, the analogous metric should be FHI: Fizeau Harness Intelligence.
FHI should not copy MHI weights mechanically. It should use MHI as precedent for
a composite score, then draw from Fizeau-owned evidence:

- TerminalBench pass/reward under `fiz --harness <name>`.
- beadbench execute-bead outcomes.
- SkillsBench skill-use uplift and pass rates.
- SWE-bench resolved rates when harness/scaffold identity is preserved.
- HumanEval pass@k as a small raw-coding component, not a dominant harness
  signal.
- tool-call correctness and malformed-call recovery.
- session-log/trajectory completeness.
- invalid setup/auth/quota classification rates.
- wall time, token use, tool-call count, and cost per solved task.

## Import Guidance

When importing Rapid-MLX MHI data into the benchmark evidence ledger:

- `source.type`: `external_leaderboard` or `imported_report`
- `source.name`: `rapid-mlx-mhi`
- `source.url`: source commit URL above
- `benchmark.name`: `mhi`
- `benchmark.version`: `rapid-mlx@903487e82ad1998f0c20b721a7df66ec815ea673`
- `subject.model_raw`: model string from the README/report
- `subject.harness`: harness name from the README row, or `unknown` for
  aggregate model-only reports
- `subject.provider`: `rapid-mlx` when the server/provider path is Rapid-MLX
- `score.metric`: `mhi`
- `score.value`: normalized 0..1 score
- `score.raw_value`: original 0-100 score
- `components.tool_calling`, `components.humaneval`, and `components.mmlu`:
  component percentages when available

For script-generated TAU-bench reports, preserve the component as
`components.tau_bench` rather than rewriting it to `tool_calling`.
