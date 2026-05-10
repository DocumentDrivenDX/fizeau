- **pass@1** = (graded reps with reward > 0) / (total graded reps). **pass@k** = unique tasks where any rep solved / unique tasks attempted. With reps=5 we do not report best-of-N because the reps are deliberately identical.
- **Real runs** = trials with `turns > 0` AND any tokens flowed. Filters out `invalid_setup`, network, container-startup, and zero-turn timeouts so per-trial medians (turns, tokens, wall) reflect actual model interaction.
- **TTFT** = (first `llm.delta` event ts) − (matching `llm.request` ts) per turn, in seconds.
- **Decode tok/s** = `output_tokens / (response.ts − first_delta.ts)` per turn — post-prefill generation rate.
- Both timing metrics report as **median-of-per-task-medians** to dampen rep variance and outlier turns. Per-bucket timing requires ≥5 turns in the bucket to plot.
- Provider-side latency (TTFT including queue and prefill) and pure decode stay separate so wall-time can be attributed to prefill vs generation.
- External leaderboard data is the count of `reward.txt` files per submission per task on `harborframework/terminal-bench-2-leaderboard` on Hugging Face. We report `tasks_passed / tasks_attempted` rather than per-rep pass@1 because the leaderboard does not expose per-rep granularity uniformly.

### Regenerating the report

```sh
# full rebuild (data + charts + HTML)
.venv-report/bin/python scripts/benchmark/generate-report.py

# data only — useful before editing the narrative markdown:
.venv-report/bin/python scripts/benchmark/generate-report.py --emit-data-only

# refresh external leaderboard from Hugging Face:
.venv-report/bin/python scripts/benchmark/generate-report.py --refresh-leaderboard
```

The script reads from `benchmark-results/fiz-tools-v1/cells/` and writes to `docs/benchmarks/`. Narrative sections (`docs/benchmarks/sections/*.md`) are read at render time and are the only place to edit prose — do not edit the generated HTML directly.
