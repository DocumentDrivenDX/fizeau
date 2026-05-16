Metrics aggregate over the canonical `all` subset.

- **Real runs** filter excludes `invalid_setup`, `invalid_provider`, and zero-turn timeouts so per-trial medians (turns, tokens, wall) reflect actual model interaction.
- **TTFT** (time to first token) and **decode tok/s** are p50 across per-task medians of per-turn measurements — see method notes for definitions.
- Profiles with no real runs show `—` in throughput columns.
