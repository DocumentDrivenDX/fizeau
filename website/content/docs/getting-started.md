---
title: Getting Started
weight: 1
---

## Install

```bash
go install github.com/easel/fizeau/cmd/fiz@latest
```

## Quick Start with LM Studio

1. Start [LM Studio](https://lmstudio.ai/) and load a model with tool-calling support (e.g., Qwen 3.5).

2. Run `fiz`:

```bash
fiz -p "Read main.go and tell me the package name"
```

Fizeau connects to LM Studio at `localhost:1234` by default.

## Quick Start with Anthropic

```bash
export FIZEAU_PROVIDER=anthropic
export FIZEAU_API_KEY=sk-ant-...
export FIZEAU_MODEL=claude-sonnet-4-20250514

fiz -p "Read main.go and tell me the package name"
```

## Configuration

Create `.fizeau/config.yaml` in your project:

```yaml
provider: openai-compat
base_url: http://localhost:1234/v1
model: qwen3.5-7b
max_iterations: 20
session_log_dir: .fizeau/sessions
```

Environment variables override the config file:
- `FIZEAU_PROVIDER` — `openai-compat` or `anthropic`
- `FIZEAU_BASE_URL` — provider base URL
- `FIZEAU_API_KEY` — API key
- `FIZEAU_MODEL` — model name

## As a Library

```go
import (
    "context"
    "github.com/easel/fizeau"
    _ "github.com/easel/fizeau/configinit"
)

func main() {
    a, err := fizeau.New(fizeau.ServiceOptions{})
    if err != nil {
        panic(err)
    }
    events, err := a.Execute(context.Background(), fizeau.ServiceExecuteRequest{
        Prompt:   "Read main.go and tell me the package name",
        ModelRef: "cheap",
        WorkDir:  ".",
    })
    if err != nil {
        panic(err)
    }
    for event := range events {
        _ = event
    }
}
```

## Session Replay

Every run is logged. Replay past sessions:

```bash
fiz log                  # list sessions
fiz replay <session-id>  # human-readable replay
```
