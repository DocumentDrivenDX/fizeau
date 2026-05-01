# Hard Pins

Pins narrow the candidate set before scoring. They are hard constraints, not
preferences.

Precedence:

1. `--harness`
2. `--provider`
3. `--model`
4. `--model-ref`
5. `--min-power` / `--max-power`

`--min-power` and `--max-power` never widen or override exact pins. They only
filter unpinned automatic routing.

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
fiz run --min-power 8 "prompt"
```

Only auto-routable models with catalog power at or above 8 may be selected.

If constraints cannot be met, the command fails before broadening the request.
Use `fiz --list-models --json` to inspect available models, power, cost,
speed, availability, endpoint/host identity, catalog reference,
auto-routable state, and exact-pin-only state.
