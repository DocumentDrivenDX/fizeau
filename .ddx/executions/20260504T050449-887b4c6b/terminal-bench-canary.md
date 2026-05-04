# Terminal-Bench Canary Verification

Bead: `fizeau-01248b3d`

## Task Directory

Canonical `--tasks-dir`:

```sh
scripts/benchmark/external/terminal-bench-2
```

Verified task directories:

```text
fix-git OK
log-summary-date-ranges OK
git-leak-recovery OK
```

The checkout was initialized with:

```sh
git submodule update --init scripts/benchmark/external/terminal-bench-2
```

Submodule commit:

```text
53ff2b87d621bdb97b455671f2bd9728b7d86c11
```

## Matrix Verification

Command:

```sh
HARBOR_AGENT_ARTIFACT="$PWD/bin/fiz-linux-arm64" ./bin/ddx-agent-bench matrix \
  --subset=scripts/beadbench/external/termbench-subset-canary.json \
  --profiles=gpt-5-mini \
  --harnesses=ddx-agent \
  --reps=1 \
  --tasks-dir=scripts/benchmark/external/terminal-bench-2 \
  --out=.ddx/executions/20260504T050449-887b4c6b/matrix-canary-ddx-agent-gpt-5-mini-final
```

Result file:

```text
.ddx/executions/20260504T050449-887b4c6b/matrix-canary-ddx-agent-gpt-5-mini-final/matrix.json
```

Final statuses:

| Task | final_status | reward |
| --- | --- | ---: |
| `fix-git` | `graded_pass` | 1 |
| `git-leak-recovery` | `graded_pass` | 1 |
| `log-summary-date-ranges` | `graded_fail` | 0 |

