# overfull-hbox CA certificate evidence

Bead: `fizeau-5c185a8d`

## Code evidence

- `scripts/benchmark/harbor_agent.py:114` creates `/etc/ssl/certs` in the Harbor task container.
- `scripts/benchmark/harbor_agent.py:120` uploads a host CA bundle to `/etc/ssl/certs/ca-certificates.crt`.
- `scripts/benchmark/harbor_agent.py:126` makes that bundle readable and links `/etc/ssl/cert.pem`.
- `scripts/benchmark/harbor_agent.py:139` checks for an existing bundle before falling back.
- `scripts/benchmark/harbor_agent.py:142` installs `ca-certificates` with `apt-get` when no bundle exists.

`os` and `Path` are imported at the top of `scripts/benchmark/harbor_agent.py`, so `_find_host_ca_bundle` has its required imports.

## Verification commands

```bash
go test ./...
```

Result: pass.

```bash
FIZEAU_BENCH_SUBSET_FILE=.ddx/executions/20260504T035630-71fa954f/overfull-hbox-subset.yaml \
FIZEAU_BENCH_RESULTS_DIR=.ddx/executions/20260504T035630-71fa954f/benchmark-results \
FIZEAU_PROVIDER_NAME=openrouter \
FIZEAU_PROVIDER=openrouter \
FIZEAU_MODEL=anthropic/claude-sonnet-4.6 \
FIZEAU_BASE_URL=https://openrouter.ai/api/v1 \
FIZEAU_API_KEY_ENV=OPENROUTER_API_KEY \
FIZEAU_BENCH_PRESET=benchmark \
./scripts/benchmark/run_benchmark.sh
```

Result: `overfull-hbox` outcome `pass`, reward `1`, duration `475917ms`.

Report: `.ddx/executions/20260504T035630-71fa954f/benchmark-results/report-20260504T040144Z.json`

Reward file: `.ddx/executions/20260504T035630-71fa954f/benchmark-results/harbor-jobs/overfull-hbox-20260504T040144Z/overfull-hbox__pKqn7iQ/verifier/reward.txt`

## Container CA evidence

The Harbor trial log records the install-time container commands:

- `mkdir -p /etc/ssl/certs`
- `chmod 644 /etc/ssl/certs/ca-certificates.crt && ln -sf /etc/ssl/certs/ca-certificates.crt /etc/ssl/cert.pem`
- fallback command with `apt-get install -y --no-install-recommends ca-certificates`

The verifier stdout also records `ca-certificates (20240203)` being installed in the task container during solution verification and `Updating certificates in /etc/ssl/certs...`.

Note: Harbor recorded `NonZeroAgentExitCodeError` after the verifier pass because OpenRouter returned `402 Payment Required` after the task was solved. This was not the prior TLS failure: the session consumed `609430` input tokens and `8343` output tokens, and the verifier reward remained `1`.
