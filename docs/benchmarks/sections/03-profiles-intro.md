Each card below is one (provider, model, harness) tuple. Cards are colored by provider family for visual grouping; the same color is used in subsequent charts. Lane definitions are kept in `scripts/benchmark/profiles/<id>.yaml`.

The catalog spans four kinds of provider surface:

- **OpenRouter / OpenAI / Anthropic** — managed API providers used as throughput and reliability references.
- **vLLM** (sindri-club-3090, bragi-club-3090) — self-hosted on a 3090 with int4 AutoRound quantization.
- **oMLX** (vidar) — Apple-silicon MLX runtime at 8-bit quantization.
- **RapidMLX** (grendel-rapid-mlx) — alternative MLX backend.

Lanes whose `id` starts with `fiz-harness-` route through fiz-as-a-harness wrapping a different agent CLI (e.g. claude or codex), used to isolate "is the agent loop hurting?" from "is the model hurting?".

Self-hosted lanes (vLLM, oMLX, RapidMLX) reference a machine in the registry at `scripts/benchmark/machines.yaml` via the profile's `metadata.server`. The hardware block on each card is rendered from that single source of truth — update the YAML to add a machine or correct hardware specs, then re-run `generate-report.py`.
