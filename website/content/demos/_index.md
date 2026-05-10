---
title: Demos
weight: 2
---

Terminal recordings of Fizeau. One reel runs **inside a Docker container**
with `llama-server` + a 0.5 B Qwen coder model — no GPU, no API key, no
internet. The other three were captured against `qwen/qwen3.6-27b` via
OpenRouter, kept here for comparison.

> **About time compression.** Slow operations (model downloads, model
> loads, long LLM responses) are *fast-forwarded* in the playback so the
> reels stay watchable. When this happens a dimmed banner appears, like
> `⏩  Fast-forward: model load (47.2s → 2.0s)`, and the cast title is
> suffixed with `[time-compressed]`. Everything else plays at wall-clock
> speed. The threshold (default: any LLM turn over 8 seconds) is set via
> `FIZEAU_LATENCY_THRESH` in `make demos-regen`.

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

## How these are produced

| Reel            | Capture path                       | Backend           | Model                                  |
| --------------- | ---------------------------------- | ----------------- | -------------------------------------- |
| `quickstart`    | `make demos-capture-docker`        | local llama-server | Qwen2.5-Coder-0.5B-Instruct (Q4_K_M)   |
| `file-read`     | `make demos-capture` (OpenRouter)  | OpenRouter API    | `qwen/qwen3.6-27b`                     |
| `file-edit`     | `make demos-capture` (OpenRouter)  | OpenRouter API    | `qwen/qwen3.6-27b`                     |
| `bash-explore`  | `make demos-capture` (OpenRouter)  | OpenRouter API    | `qwen/qwen3.6-27b`                     |

All four render to asciicast v2 via `make demos-regen` from canonical
session JSONLs in
[`demos/sessions/`](https://github.com/easel/fizeau/tree/main/demos/sessions).
Rendering is deterministic and never makes a live LLM call. The
time-compression banner is implemented in
[`demos/regen.py`](https://github.com/easel/fizeau/blob/main/demos/regen.py).
