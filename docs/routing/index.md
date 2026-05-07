# Routing

Fizeau routes automatically. Configure provider sources and endpoints, then
let the agent discover models, join them with the model catalog, track
availability/usage, and select the best candidate for the request.

The primary strength control is numeric power:

- Power is a catalog score from 1 to 10.
- Higher means more capable for agent tasks.
- Power 0 means unknown or exact-pin-only.
- `--min-power` and `--max-power` apply only to unpinned automatic routing.

If no power bound or hard pin is supplied, the agent selects the best
lowest-cost viable auto-routable model from discovered inventory.

Hard pins are exclusive:

- `--model qwen-3.6-27b` means only that model identity may be used.
- `--provider lmstudio` means only that provider source or selected endpoint
  may be used, depending on the request surface.
- `--harness codex` means only that harness may be used.

If a hard pin cannot be satisfied, routing fails with attempted-route and
candidate evidence. The agent does not substitute a broader model, source,
endpoint, or harness.

Useful commands:

```bash
fiz --list-models --json
fiz run --min-power 5 "prompt"
fiz run --min-power 8 "prompt"
fiz run --model qwen-3.6-27b "prompt"
fiz run --provider lmstudio "prompt"
```

The agent dispatches one selected candidate per request. Semantic retry or
escalation belongs to the caller: rerun with a higher `--min-power`, a lower
`--max-power`, or different hard pins when task evidence justifies it.

See also:

- [Candidate selection](best-provider.md)
- [Hard pins](override-precedence.md)
- [Profiles and power bounds](profiles.md)
