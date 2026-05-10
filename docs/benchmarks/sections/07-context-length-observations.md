_This section is intended to be regenerated against the latest data — see `data/timing.json` and the charts below._

Per-turn TTFT (first-token latency) and steady-state decode tok/s, bucketed by **input-token length of that turn**. We bucket per turn rather than per task because the agent loop's input grows monotonically inside a single task — buckets reveal how each provider scales its prefill and decode under increasing context.

Buckets: 0–10k, 10–30k, 30–60k, 60–120k, 120k+ tokens. Buckets with fewer than 5 turns of data are dropped to avoid noise.

Read this as: a lane that holds steady across buckets has a working KV-cache / prefix-cache; a lane whose TTFT slopes up sharply is recomputing prefill on every turn.
