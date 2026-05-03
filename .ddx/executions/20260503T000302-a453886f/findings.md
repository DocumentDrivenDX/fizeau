# Investigation: bench/overfull-hbox — TLS cert error in container

Bead: `fizeau-4ef5962f`
Date: 2026-05-02
Failing runs (all 0-reward, `NonZeroAgentExitCodeError`):
- `smoke-overfull-hbox-20260502T203307Z` (Sonnet 4.6 via openrouter)
- `smoke-overfull-hbox-20260502T203525Z`, `…T203725Z`, `…T204315Z` (Sonnet 4.6 retries)
- `smoke-overfull-hbox-20260502T230536Z`, `…T230907Z`, `…T231139Z` (claude-opus-4-7 / claude-code harness)
- `benchmark-results/iteration-v1/sonnet-runs/.../overfull-hbox__5dJuRue` (earlier Sonnet 4.6 run)

## 1. Confirmed observations

Every `agent/fiz.txt` for overfull-hbox shows the identical failure shape:

```
{
  "status": "failed",
  "tokens": {"input": 0, "output": 0, "total": 0},
  "duration_ms": 38000000–59000000,
  "error": "agent: provider error: Post \"https://openrouter.ai/api/v1/chat/completions\": tls: failed to verify certificate: x509: certificate signed by unknown authority",
  ...
}
[failed] tokens: 0 in / 0 out
```

(Examples: `benchmark-results/harbor-jobs/smoke-overfull-hbox-20260502T203307Z/overfull-hbox__XLuNGwi/agent/fiz.txt`,
`benchmark-results/iteration-v1/sonnet-runs/cells/fiz/claude-sonnet-4-6/rep-001/overfull-hbox/fiz-overfull-hbox-rep1/overfull-hbox__5dJuRue/agent/fiz.txt`.)

Tokens are zero because the very first `POST /api/v1/chat/completions` aborts during the TLS handshake — the agent never gets a turn off the ground.

`exception.txt` shows harbor surfacing a non-zero exit from the wrapped fiz invocation; the failure originates inside the container, not in harbor's verifier.

By contrast, other terminal-bench-2 tasks running the same fiz binary against the same OpenRouter endpoint pass cleanly (e.g. `smoke-cobol-modernization-20260502T202221Z`, `smoke-break-filter-js-from-html-20260502T*`). That isolates the regression to the **task environment**, not the binary, the model, or the harbor adapter.

## 2. Root cause: confirmed

`scripts/benchmark/external/terminal-bench-2/overfull-hbox/environment/Dockerfile`:

```Dockerfile
FROM ubuntu:24.04
WORKDIR /app

RUN apt-get update && \
    apt-get install -y --no-install-recommends texlive-latex-base=2023.20240207-1 && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*

COPY tests/main.tex .
COPY tests/input.tex .
COPY tests/synonyms.txt .
```

Two facts together explain every observed failure:

1. The base image `ubuntu:24.04` does **not** ship `ca-certificates`. Verified locally:

   ```
   $ docker run --rm ubuntu:24.04 sh -c 'ls /etc/ssl/certs/ca-certificates.crt; dpkg -l ca-certificates'
   ls: cannot access '/etc/ssl/certs/ca-certificates.crt': No such file or directory
   un  ca-certificates <none> <none> (no description available)   # uninstalled
   ```

2. The Dockerfile installs `texlive-latex-base` with `--no-install-recommends`, and texlive-latex-base does **not** Depend (only Recommends) on `ca-certificates`, so the suppression skips it. After the apt step, `apt-get clean && rm -rf /var/lib/apt/lists/*` makes a later install much harder for the agent to recover from inside the trial.

The fiz binary is a static-ish Go executable; Go's `crypto/tls` walks SSL_CERT_FILE / SSL_CERT_DIR / `/etc/ssl/certs/ca-certificates.crt` to assemble the system root pool (see `crypto/x509/root_linux.go` in upstream Go). With none of those present, every `https://` Dial returns `x509: certificate signed by unknown authority` — exactly the recorded error.

The contrasting tasks that pass use `python:3.13-slim-bookworm`, which does ship the bundle:

```
$ docker run --rm python:3.13-slim-bookworm sh -c 'ls /etc/ssl/certs/ca-certificates.crt; dpkg -l ca-certificates'
/etc/ssl/certs/ca-certificates.crt
ii  ca-certificates 20230311+deb12u1 all  Common CA certificates
```

So this is not "model X can't solve overfull-hbox" — it is **fiz cannot make any HTTPS call from inside the overfull-hbox container**, which deterministically zeroes every attempt regardless of model.

## 3. Why this is overfull-hbox specifically

Of the 89 task Dockerfiles under `scripts/benchmark/external/terminal-bench-2/*/environment/Dockerfile`:

- Only 4 mention `ca-certificates` explicitly (`git-multibranch`, `qemu-alpine-ssh`, `qemu-startup`, `winning-avg-corewars`).
- Most other tasks inherit `python:*-slim*` or similar images that already include the bundle.
- overfull-hbox is the rare combination of "ubuntu:24.04 base" + "no ca-certificates installed" + "agent must reach a remote API for the task to make progress."

Tasks that already passed (cobol-modernization, break-filter-js-from-html, etc.) use `python:3.13-slim-bookworm` — which is why fiz's openrouter calls have worked there end-to-end despite the same binary, harness, and provider config.

## 4. Recommended fix (single line, scoped)

Add `ca-certificates` to the Dockerfile's apt install:

```Dockerfile
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        ca-certificates \
        texlive-latex-base=2023.20240207-1 && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*
```

This is the minimal change. It does not pin the texlive version differently, does not change the base image, and matches the convention used by the four task Dockerfiles that already install ca-certificates explicitly.

Alternatives considered (and rejected):

- **Switch base image to `python:3.13-slim-bookworm`** — works, but mutates more than needed (different OS, different default packages) and risks breaking the texlive install path the task was tuned for. Not worth the blast radius for a one-package fix.
- **Mount the host's CA bundle into the container at run time via harbor `mounts_json`** — fragile across host OSes (different cert paths on macOS vs. Linux) and leaks host-system trust into the trial, which is the wrong direction for a benchmark.
- **Bake CA certs into the fiz binary (`crypto/x509`'s `SystemRootsPool` override / golang.org/x/crypto pinning)** — far too invasive; would diverge from upstream Go TLS defaults for one bench task.

## 5. Follow-up beads (suggested)

1. **fix(bench/overfull-hbox):** add `ca-certificates` to environment Dockerfile (the change in §4); re-run `smoke-overfull-hbox` against Sonnet 4.6 to confirm reward > 0 and non-zero token usage in `agent/fiz.txt`. — **easy, scoped, ready to file.**
2. **chore(bench):** audit all 89 terminal-bench-2 task Dockerfiles for HTTPS-reachability assumptions. Add a smoke pre-check to the benchmark runner that, before invoking fiz, executes `curl -sS https://openrouter.ai/api/v1/models >/dev/null` (or similar) inside the trial container and fails fast with a clear "missing ca-certificates / network unreachable" message instead of the current zero-token TLS-buried error.
3. **chore(harbor_agent.py):** consider classifying fiz exits whose `error` field matches `tls: failed to verify certificate` as `EnvironmentSetupError` (or a fizeau-specific equivalent) rather than `NonZeroAgentExitCodeError`, so the matrix runner can distinguish "model couldn't solve task" from "container couldn't make any API call." This will keep evidence-grade comparisons honest when a single broken Dockerfile would otherwise silently zero a model's score.
4. **docs(bench/README):** add a short "task Dockerfile checklist" requiring `ca-certificates` for any task whose agent will make HTTPS calls (which is effectively all of them under fiz). Two-line note next to the existing canary-string convention.

## 6. Evidence index

- Failing logs: `benchmark-results/harbor-jobs/smoke-overfull-hbox-20260502T20{3307,3525,3725,4315}Z/*/agent/fiz.txt`, `…T23{0536,0907,1139}Z/*/agent/fiz.txt`, `benchmark-results/iteration-v1/sonnet-runs/.../overfull-hbox__5dJuRue/agent/fiz.txt`
- Passing reference logs (same fiz binary, different task image): `benchmark-results/harbor-jobs/smoke-cobol-modernization-20260502T202221Z/cobol-modernization__numS4Ua/agent/fiz.txt`, `benchmark-results/harbor-jobs/smoke-break-filter-js-from-html-20260502T*/break-filter-js-from-html__*/agent/fiz.txt`
- Dockerfile under investigation: `scripts/benchmark/external/terminal-bench-2/overfull-hbox/environment/Dockerfile`
- Harbor adapter (binary install path, no cert handling): `scripts/benchmark/harbor_agent.py:46–69`
- Reproduction of the bundle-presence delta:
  - `docker run --rm ubuntu:24.04 ls /etc/ssl/certs/ca-certificates.crt` → not found
  - `docker run --rm python:3.13-slim-bookworm ls /etc/ssl/certs/ca-certificates.crt` → present
