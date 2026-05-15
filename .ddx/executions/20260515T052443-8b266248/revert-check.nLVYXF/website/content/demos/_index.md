---
title: Demos
weight: 2
---

Terminal recordings of Fizeau. One reel runs **inside a Docker container**
with `llama-server` + a 0.5 B Qwen coder model — no GPU, no API key, no
internet. Three more were captured against `qwen/qwen3.6-27b` via
OpenRouter, kept here for comparison. The final three reels — `usage`,
`update --check-only`, and a peek at the on-disk JSONL session log —
demonstrate Fizeau-specific affordances (cost attribution, atomic
self-update, and structured observability) and require no LLM at all.

> **About time compression.** Slow operations (model downloads, model
> loads, long LLM responses) are *fast-forwarded* in the playback so the
> reels stay watchable. When this happens a dimmed banner appears, like
> `⏩  Fast-forward: model load (47.2s → 2.0s)`, and the cast title is
> suffixed with `[time-compressed]`. Everything else plays at wall-clock
> speed. The threshold (default: any LLM turn over 8 seconds) is set via
> `FIZEAU_LATENCY_THRESH` in `make demos-regen`.

## Cost cap halts the loop mid-run

`fiz run --cost-cap-usd 0.005 -p '<task>'` walks each iteration's running
cost against the cap and refuses to issue the next `llm.request` once
projected cost would cross the line. Status `budget_halted`, exit code `2`.
Demoed against a tiny scratch repo with a per-file editing task that
naturally takes more than `$0.005` of Qwen3.6-27B time.

<script src="https://asciinema.org/a/demo.js" id="asciicast-cost-cap-halt" async data-src="/fizeau/demos/cost-cap-halt.cast" data-cols="100" data-rows="30"></script>
<noscript>

```
$ fiz --cost-cap-usd 0.005 -p 'add a doc comment to each .go file (one at a time)'

Let me read each file to understand its purpose, then add the doc comments one by one.
[budget_halted] tokens: 4665 in / 613 out
```

</noscript>

> **Origin:** OpenRouter (qwen/qwen3.6-27b). Captured 2026-05-10. Real spend: $0.0035 of $0.005 cap.

## Quickstart — install fiz, run a query, no GPU

The literal end-to-end "getting started" flow: install the binary,
download a 390 MB GGUF model, start `llama-server`, and run your first
prompt. Captured in the CPU-only Docker image (`demos/docker/Dockerfile.cpu`).

<script src="https://asciinema.org/a/demo.js" id="asciicast-quickstart" async data-src="/fizeau/demos/quickstart.cast" data-cols="100" data-rows="30"></script>
<noscript>

```
$ curl -fsSL https://easel.github.io/fizeau/install.sh | bash
  ✓ downloaded fiz v0.14 → /usr/local/bin/fiz

$ wget -q .../qwen2.5-coder-0.5b-instruct-q4_k_m.gguf -O ~/.fizeau/models/tiny.gguf
$ llama-server --model ~/.fizeau/models/tiny.gguf --port 8080 &
  [llama-server] loading weights ...

$ fiz -p 'list go files in .'

⏩  Fast-forward: model download (47.2s → 2.0s)
⏩  Fast-forward: model load (12.4s → 2.0s)
main.go
cmd/server/main.go
internal/api/handler.go
internal/api/middleware.go
internal/db/postgres.go
[success] tokens: 312 in / 58 out
```

</noscript>

> Origin: **Docker / local llama-server**. Model:
> [Qwen2.5-Coder-0.5B-Instruct (Q4_K_M)](https://huggingface.co/Qwen/Qwen2.5-Coder-0.5B-Instruct-GGUF)
> — 390 MB on disk, ~900 MB RSS at first boot. Runs on a 2-core / 2 GB
> CI runner. See [`demos/docker/`](https://github.com/easel/fizeau/tree/main/demos/docker)
> for the Dockerfile.

## Read a file and explain it

Model reads `main.go` using the read tool and describes the program.

<script src="https://asciinema.org/a/demo.js" id="asciicast-read" async data-src="/fizeau/demos/file-read.cast" data-cols="100" data-rows="30"></script>
<noscript>

```
$ fiz -p 'Read main.go and explain what this program does'

This Go program starts an HTTP server on port 8080. It responds with
"Hello from Fizeau!" to any request made to the root path (`/`).
[success] tokens: 4625 in / 138 out
```

</noscript>

> Origin: **OpenRouter** / `qwen/qwen3.6-27b`.

## Edit a config file

Model reads a config, edits the port number, and verifies the change.

<script src="https://asciinema.org/a/demo.js" id="asciicast-edit" async data-src="/fizeau/demos/file-edit.cast" data-cols="100" data-rows="30"></script>
<noscript>

```
$ fiz -p 'Read config.yaml, change the server port from 8080 to 9090, then verify'

The server port has been changed from 8080 to 9090 and verified in the file.
[success] tokens: 9562 in / 245 out
```

</noscript>

> Origin: **OpenRouter** / `qwen/qwen3.6-27b`.

## Explore project structure

Model uses bash to find all Go files and summarizes the package layout.

<script src="https://asciinema.org/a/demo.js" id="asciicast-bash" async data-src="/fizeau/demos/bash-explore.cast" data-cols="100" data-rows="30"></script>
<noscript>

```
$ fiz -p 'List all Go files and summarize the package structure'

Here's the package structure:

project/
├── main.go                  (root package — entry point)
├── cmd/
│   └── server/main.go       (cmd/server — server binary)
└── internal/
    ├── api/handler.go       (internal/api — HTTP handlers)
    ├── api/middleware.go    (internal/api — middleware)
    └── db/postgres.go       (internal/db — Postgres database layer)

5 Go files across 4 packages: a root main, a cmd/server binary,
and two internal packages (api and db) under internal/.
[success] tokens: 4670 in / 238 out
```

</noscript>

> Origin: **OpenRouter** / `qwen/qwen3.6-27b`.

## Cost attribution — known vs unknown

`fiz usage` rolls up every session JSONL in your history and prints
per-(provider, model) totals. Where the catalog has a price for the
model the `COST` column is exact; where it doesn't (a self-hosted
`vllm` deployment, a model with no published rate) the column reads
`unknown` rather than guessing. Operators can see at a glance which
slice of their spend Fizeau can attribute and which it cannot — the
"never guess" policy from the cost-attribution spec made tangible.

<script src="https://asciinema.org/a/demo.js" id="asciicast-usage" async data-src="/fizeau/demos/fiz-usage.cast" data-cols="170" data-rows="18"></script>
<noscript>

```
$ fiz usage --since 30d
Window: 2026-04-10 .. 2026-05-10
PROVIDER     MODEL                       SESSIONS  INPUT  OUTPUT  COST
openrouter   anthropic/claude-sonnet-4.6        2  73048    2501  $0.2567
openrouter   openai/gpt-5.3-mini                3      0       0  unknown
vllm         qwen3.6-27b-autoround              1   5204      17  unknown
```

</noscript>

> Origin: **local** (no LLM call). Reads existing `~/.fizeau/sessions/*.jsonl`.

## Self-update check

`fiz` ships as a single static binary and updates itself in place; the
`--check-only` flag does the version comparison without downloading or
swapping anything. Exit code 1 means "outdated", 0 means "current". A
shell script can wrap this for a daily cron, or you can drop the flag
to perform the actual atomic in-place upgrade.

<script src="https://asciinema.org/a/demo.js" id="asciicast-update" async data-src="/fizeau/demos/fiz-update-check.cast" data-cols="100" data-rows="18"></script>
<noscript>

```
$ fiz version
fiz v0.10.16 (commit f7bbeb1c, built 2026-05-08T19:07:05Z)
  Update available: v0.12.1

$ fiz update --check-only; echo "exit=$?"
Current: v0.10.16
Latest:  v0.12.1

Update available. Run 'fiz update' to upgrade.
exit=1
```

</noscript>

> Origin: **local** (single GET to GitHub releases API, no LLM call).

## Structured session log on disk

Every fiz invocation appends a line-delimited JSON event log to
`~/.fizeau/sessions/<session-id>.jsonl`. The file is the source of
truth for `fiz replay`, `fiz usage`, and downstream observability.
A short `jq` projection over the first three events shows the
per-turn token counts, latency, and model identifier — every figure
on the website's benchmark pages comes from rolling up these files.

<script src="https://asciinema.org/a/demo.js" id="asciicast-jsonl" async data-src="/fizeau/demos/fiz-jsonl.cast" data-cols="170" data-rows="14"></script>
<noscript>

```
$ jq -c '{ts, type, model: .data.model, tokens: (.data.usage // .data.tokens),
          cost_usd: .data.cost_usd, latency_ms: .data.latency_ms}' \
     .fizeau/sessions/svc-*.jsonl | head -3
{"ts":"...","type":"session.start","model":"qwen/qwen3.6-27b","tokens":null,...}
{"ts":"...","type":"llm.response","model":"qwen/qwen3.6-27b","tokens":{"input":2249,"output":53,"total":2302},"cost_usd":0.00088928,"latency_ms":1671}
{"ts":"...","type":"llm.response","model":"qwen/qwen3.6-27b","tokens":{"input":2376,"output":85,"total":2461},"cost_usd":0.00103232,"latency_ms":5577}
```

</noscript>

> Origin: **local** (reads `demos/sessions/file-read.jsonl`, the same
> JSONL that powers the *Read a file and explain it* reel above).

## How these are produced

| Reel                | Capture path                              | Backend            | Model                                |
| ------------------- | ----------------------------------------- | ------------------ | ------------------------------------ |
| `quickstart`        | `make demos-capture-docker`               | local llama-server | Qwen2.5-Coder-0.5B-Instruct (Q4_K_M) |
| `file-read`         | `make demos-capture` (OpenRouter)         | OpenRouter API     | `qwen/qwen3.6-27b`                   |
| `file-edit`         | `make demos-capture` (OpenRouter)         | OpenRouter API     | `qwen/qwen3.6-27b`                   |
| `bash-explore`      | `make demos-capture` (OpenRouter)         | OpenRouter API     | `qwen/qwen3.6-27b`                   |
| `fiz-usage`         | `./demos/capture-subcommands.sh`          | local              | n/a (reads existing session logs)    |
| `fiz-update-check`  | `./demos/capture-subcommands.sh`          | local              | n/a (single GitHub releases GET)     |
| `fiz-jsonl`         | `./demos/capture-subcommands.sh`          | local              | n/a (reads `demos/sessions/`)        |

The first four reels render to asciicast v2 via `make demos-regen` from
canonical session JSONLs in
[`demos/sessions/`](https://github.com/easel/fizeau/tree/main/demos/sessions).
Rendering is deterministic and never makes a live LLM call. The
time-compression banner is implemented in
[`demos/regen.py`](https://github.com/easel/fizeau/blob/main/demos/regen.py).

The three subcommand reels (`fiz-usage`, `fiz-update-check`, `fiz-jsonl`)
have no agent loop, so they bypass `regen.py` and emit asciicast v2
directly via
[`demos/scripts/build-subcommand-cast.py`](https://github.com/easel/fizeau/blob/main/demos/scripts/build-subcommand-cast.py).
Each step's stdout is captured verbatim from a real `fiz` invocation —
no fabrication — and replayed with realistic typing/pause delays.
