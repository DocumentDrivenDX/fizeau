---
title: Demos
weight: 2
---

Terminal recordings showing Fizeau in action with [OpenRouter](https://openrouter.ai/) and `qwen/qwen3.6-27b`.

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

All demos run against [qwen/qwen3.6-27b](https://openrouter.ai/qwen/qwen3.6-27b) via OpenRouter.
Captured with `make demos-capture` and rendered with `make demos-regen` from
canonical session JSONLs in [`demos/sessions/`](https://github.com/easel/fizeau/tree/main/demos/sessions).
