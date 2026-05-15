# Hard Pins

Pins narrow the candidate set before scoring. They are explicit user
constraints, not preferences.

Precedence:

1. `--harness`
2. `--provider`
3. `--model`
4. `--policy`
5. `--min-power` / `--max-power`

`--policy`, `--min-power`, and `--max-power` never widen or override exact
pins. They only shape automatic routing and scoring.

Pins override provider `include_by_default` and metered opt-in, so a
deliberately pinned pay-per-token provider can be considered even when it is
excluded from unpinned automatic routing. Pins do not override policy
requirements or dispatchability: `--policy air-gapped --provider openrouter`
fails because `air-gapped` requires `no_remote`, and a pin still fails if the
chosen source cannot actually serve the requested model.

Hard gates are limited to explicit user constraints and dispatchability. Cost,
quality, health risk, latency, utilization, and power fit are scoring inputs
after the snapshot is assembled.

Examples:

```bash
fiz run --model qwen-3.6-27b "prompt"
```

Only that model identity may be used. The router may choose among available
sources/endpoints that serve it, but it must not substitute a different model.

```bash
fiz run --provider lmstudio "prompt"
```

Only that provider source, or the selected endpoint when the surface supports
endpoint selection, may be used.

```bash
fiz run --harness codex "prompt"
```

Only that harness may be used.

```bash
fiz run --policy cheap "prompt"
```

Prefer low marginal spend. Local/fixed candidates are favored when they are
eligible.

```bash
fiz run --policy smart "prompt"
```

Prefer high-capability candidates. Local/fixed candidates are not allowed by
the policy default.

```bash
fiz run --policy air-gapped "prompt"
```

Require local-only execution. Remote providers and account harnesses are
rejected, including when pinned.

```bash
fiz run --min-power 8 "prompt"
```

Prefer models at or above catalog power 8. Candidates below the requested
minimum are penalized more than candidates above the maximum because weak
models are more likely to fail the task.

If constraints cannot be met, the command fails before broadening the request.
Use `fiz --list-models --json` to inspect available models, power, cost,
speed, availability, endpoint/host identity, catalog reference,
auto-routable state, exact-pin-only state, and the snapshot freshness needed to
decide whether `fiz models --refresh` or `fiz models --refresh-all` is more
appropriate.
