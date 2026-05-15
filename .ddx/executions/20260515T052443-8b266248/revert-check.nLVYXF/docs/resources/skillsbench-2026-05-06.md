# SkillsBench Resource

Captured: 2026-05-06

Source:
<https://www.skillsbench.ai/>

## Definition

SkillsBench evaluates how agent skills work across expert-curated tasks. The
public site describes three abstraction layers:

- skills as domain-specific applications and workflows
- agent harnesses as operating systems that manage tools and I/O
- models as CPUs that provide the raw reasoning/generation substrate

This is a close conceptual match for Fizeau because Fizeau owns the harness
surface: routing, tool access, subprocess wrappers, permissions, transcript
projection, and session logs.

## Published Shape

The public leaderboard reports pass rates across agent-model configurations on
84 tasks with 5 trials per task. It compares runs with skills and without
skills and reports a normalized gain.

The task registry spans diverse high-value domains such as engineering,
architecture, control systems, networking, document processing, science,
security, data visualization, robotics, economics, and energy.

## Relevance To FHI

SkillsBench should be an FHI input if Fizeau can run or import it with explicit
model x harness x skill identity.

Useful FHI dimensions:

- skill-use uplift: with-skills pass rate minus without-skills pass rate
- harness portability: same model across Claude, Codex, pi, opencode, and fiz
- invalid-run rate: install, auth, setup, permission, and malformed output
- observability: whether trajectories identify skill invocation, tool calls,
  and failure causes
- efficiency: wall time, turns, tool calls, and cost per solved task

## Import Guidance

When importing SkillsBench rows into the benchmark evidence ledger:

- `source.type`: `external_leaderboard` or `imported_report`
- `source.name`: `skillsbench`
- `source.url`: `https://www.skillsbench.ai/`
- `benchmark.name`: `skillsbench`
- `benchmark.version`: site version/date or report commit if available
- `subject.model_raw`: model string from the row
- `subject.harness`: agent harness string from the row
- `subject.provider`: `unknown` unless explicitly reported
- `scope.n_tasks`: 84 for the public leaderboard shape
- `score.metric`: `pass_rate` for with-skills or without-skills rows
- `components.with_skills_pass_rate`: with-skills pass rate
- `components.without_skills_pass_rate`: without-skills pass rate
- `components.normalized_gain`: normalized gain when reported

For FHI, prefer paired rows where the same model/harness has both with-skills
and without-skills measurements.
