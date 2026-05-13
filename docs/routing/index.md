# Routing

Fizeau routes automatically by evaluating `route(client_inputs, fiz_models_snapshot)`.
Configure provider sources and endpoints, then let the agent discover models,
join them with the model catalog, track availability/usage, and select the best
candidate from the snapshot facts.

The primary strength control is numeric power, but it is a scoring input rather
than the only gate:

- Power is a catalog score from 1 to 10.
- Higher means more capable for agent tasks.
- Power 0 means unknown or exact-pin-only.
- `--min-power` and `--max-power` apply only to unpinned automatic routing.
- Hard gates are limited to explicit user constraints and dispatchability.

If no power bound or hard pin is supplied, the agent selects the best-scoring
viable auto-routable model from the snapshot. Pay-per-token providers are
excluded from unpinned automatic routing unless provider default inclusion and
explicit metered-spend opt-in both allow them to create actual metered spend.
Subscription models score with PAYG-equivalent effective cost while keeping
`actual_cash_spend=false`.

Hard pins are exclusive:

- `--model qwen-3.6-27b` means only that model identity may be used.
- `--provider lmstudio` means only that provider source or selected endpoint
  may be used, depending on the request surface.
- `--harness codex` means only that harness may be used.

If a hard pin cannot be satisfied, routing fails with attempted-route and
candidate evidence. The agent does not substitute a broader model, source,
endpoint, or harness.

`fiz models` is the snapshot-first inspection surface. It stays quick and
returns stale data immediately when freshness is pending. `fiz models --refresh`
blocks on routing-relevant stale fields, and `fiz models --refresh-all` blocks
on all refreshable fields. If no DDx server or other long-running maintainer is
keeping the snapshot warm, stale output should point the operator toward
starting that freshness heartbeat or using `--refresh`.

Useful commands:

```bash
fiz policies
fiz harnesses
fiz models --format json
fiz --list-models --json
fiz run --policy cheap "prompt"
fiz run --policy smart "prompt"
fiz run --model qwen-3.6-27b "prompt"
fiz run --provider lmstudio "prompt"
```

The agent dispatches one selected candidate per request. Semantic retry or
escalation belongs to the caller: rerun with a higher `--min-power`, a lower
`--max-power`, or different hard pins when task evidence justifies it.

See also:

- [Candidate selection](best-provider.md)
- [Hard pins](override-precedence.md)
- [Policies and power bounds](policies.md)
