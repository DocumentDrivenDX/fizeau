# Route-Status Sanity

Date: 2026-05-15

Workdir: `.ddx/executions/20260515T084338-0031e2a5/route-status-sanity`

Equivalent CLI invocation inside that workdir:

```sh
fiz route-status --model mlx-community/Qwen3.6-27B-8bit --json
```

Executed command in this bead worktree:

```sh
HOME=$PWD/.ddx/executions/20260515T084338-0031e2a5/route-status-home \
FIZEAU_CACHE_DIR=$PWD/.ddx/executions/20260515T084338-0031e2a5/route-status-cache \
go run ./cmd/fiz --work-dir \
  $PWD/.ddx/executions/20260515T084338-0031e2a5/route-status-sanity \
  route-status --model mlx-community/Qwen3.6-27B-8bit --json
```

Observed result:

- Winner: `vidar`
- `selected_server_instance`: `127.0.0.1:53703`
- `grendel` candidate: `eligible=false`
- `grendel.filter_reason`: `unhealthy`
- `grendel.reason`: `provider grendel known unreachable (last dial failure 0s ago)`

Supporting evidence:

- `.ddx/executions/20260515T084338-0031e2a5/route-status-sanity/.fizeau/route-health-main.json`
  recorded a successful probe for `vidar` and a failed probe for `grendel`
  at `2026-05-15T08:58:00.759300686Z`.
- The `grendel` rejection therefore came from real failed probe/discovery
  evidence, not from a startup-skipped unknown-health state.
