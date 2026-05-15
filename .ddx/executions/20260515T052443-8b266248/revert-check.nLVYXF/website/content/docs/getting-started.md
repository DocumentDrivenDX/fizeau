---
title: Getting Started
weight: 1
---

A five-minute tour: install `fiz`, point it at a provider, ask it a
question. Then see how `fiz` auto-detects agent CLIs you already have
installed (`claude`, `codex`, `pi`, `opencode`) and routes through
them as harnesses.

## 1. Install

```bash
go install github.com/easel/fizeau/cmd/fiz@latest
```

Confirm:

```bash
fiz version
```

## 2. Smallest run

Set an API key for any OpenAI-compatible endpoint, then run a one-shot
prompt. The defaults assume a local LM Studio at `http://localhost:1234`,
so the only required variable for cloud use is the key:

```bash
export FIZEAU_API_KEY=sk-...
fiz -p "what is the capital of France?"
```

That's it. `fiz` resolves a model, dispatches the request, streams the
answer to stdout, and writes a session log under `.fizeau/sessions/`.

For a local model with [LM Studio](https://lmstudio.ai/), no key is
needed — start LM Studio with a tool-capable model loaded, then run:

```bash
fiz -p "Read main.go and tell me the package name"
```

## 3. What's installed?

`fiz` probes your `$PATH` at startup and discovers which agent CLI
wrappers are available. List the result with:

```bash
fiz harnesses
```

Sample output on a machine with the Claude and Codex CLIs installed:

```
NAME      TYPE        BILLING       STATUS       MODEL                ROUTE
codex     subprocess  subscription  available    gpt-5.4              yes
claude    subprocess  subscription  available    claude-sonnet-4-6    yes
opencode  subprocess  subscription  unavailable  opencode/gpt-5.4     no
fiz       native      per_token     available    -                    yes
pi        subprocess  subscription  unavailable  gemini-2.5-flash     no
gemini    subprocess  subscription  unavailable  gemini-2.5-flash     no
```

* **STATUS** is `available` when the binary resolves on `$PATH`,
  `unavailable` when it doesn't. Embedded harnesses (`fiz`) and
  HTTP-only providers (`openrouter`, `lmstudio`, …) are always shown
  as available because they have no CLI to probe for.
* **ROUTE** is `yes` when the harness is eligible for automatic
  routing under the default policy.

Pass `--json` for the same data in machine-readable form:

```bash
fiz harnesses --json
```

No configuration is required to enable detection — installing `claude`
or `codex` is enough. There is no harness path to set in
`.fizeau/config.yaml`; if you uninstall a wrapper, its row flips back
to `unavailable` on the next run.

## 4. Wrapping `claude`, `codex`, `pi`, `opencode`

When `fiz` finds one of these CLIs on your `$PATH`, it becomes a
routing target. The default preference order is:

```
codex → claude → opencode → fiz → pi → gemini
```

If both `codex` and `claude` are installed, `fiz` prefers `codex` for
unpinned requests. To force a specific harness for one run:

```bash
fiz --harness claude -p "summarize git log -n 5"
```

To force a specific concrete model (which implies its harness):

```bash
fiz --model claude-sonnet-4-6 -p "..."
```

Each wrapped harness contributes its native subscription quota to the
routing decision: `fiz` reads `claude --usage` / `codex --status` /
similar so a quota-exhausted harness is skipped. See the
[routing documentation](routing/) for the full decision flow.

If a CLI is installed but not authenticated, `fiz` surfaces the
provider's own auth error in the session log — log in with the wrapped
CLI directly (`claude login`, `codex login`, …) and re-run.

## 5. As a Go library

`fiz` is a thin CLI over the `github.com/easel/fizeau` package. The
same routing, harness-detection, and execution machinery runs
in-process:

```go
package main

import (
    "context"
    "fmt"

    "github.com/easel/fizeau"
    _ "github.com/easel/fizeau/configinit"
)

func main() {
    fmt.Println("installed harnesses:", fizeau.AvailableHarnesses())

    svc, err := fizeau.New(fizeau.ServiceOptions{})
    if err != nil {
        panic(err)
    }
    events, err := svc.Execute(context.Background(), fizeau.ServiceExecuteRequest{
        Prompt:  "what is the capital of France?",
        Policy:  "cheap",
        WorkDir: ".",
    })
    if err != nil {
        panic(err)
    }
    for ev := range events {
        _ = ev // stream events: text deltas, tool calls, final summary
    }
}
```

`fizeau.AvailableHarnesses()` returns the same list `fiz harnesses`
prints — an `exec.LookPath` probe with no subprocess spawn, safe to
call at process startup.

For richer per-harness detail (quota, auth, capability matrix), call
`svc.ListHarnesses(ctx)` instead.

## Where to next

* **[CLI reference](cli/)** — every `fiz` subcommand and flag.
* **[Embedding](embedding/)** — the full Go API surface.
* **[Tools](tools/)** — built-in tool catalog the agent loop can call.
* **[Routing](routing/)** — how `fiz` chooses a harness and model.
* **[Observability](observability/)** — session logs, replay, and
  usage reports.

## Session replay

Every run writes a session log. Inspect or replay past sessions with:

```bash
fiz log                  # list sessions
fiz replay <session-id>  # human-readable replay
fiz usage                # token + cost rollup
```
