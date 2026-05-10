# Fizeau homepage demos

The asciicast files embedded on
[`/demos/`](https://easel.github.io/fizeau/demos/) are checked in at
`website/static/demos/*.cast` and regenerated **deterministically** from
canonical session JSONLs in `demos/sessions/`. Regeneration makes **no live
LLM calls** and does not require `asciinema rec`.

## Layout

```
demos/
├── README.md              # this file
├── regen.sh               # entry point — wraps regen.py for all demos
├── regen.py               # JSONL → asciicast v2 renderer (with time compression)
├── capture.sh             # capture sessions against OpenRouter (cloud)
├── capture-docker.sh      # capture sessions in the CPU Docker image (offline)
├── docker/
│   ├── Dockerfile.cpu     # ubuntu + fiz + llama-server + Qwen-Coder-0.5B (~390 MB)
│   ├── Dockerfile.gpu     # CUDA variant; same plumbing, model bind-mounted
│   ├── entrypoint.sh      # boots llama-server in the bg + waits for /health
│   └── fizeau-config.yaml # points fiz at 127.0.0.1:8080 (the local llama-server)
├── record.sh              # legacy live-recording helper (uses LM Studio)
├── scripts/               # legacy live-recording demo scripts
│   ├── demo-read.sh
│   ├── demo-edit.sh
│   └── demo-bash.sh
└── sessions/              # canonical session JSONLs (CHECK THESE IN)
    ├── quickstart.jsonl              # install + download + first query
    ├── quickstart.preface.json       # shell-history frames before $ fiz
    ├── file-read.jsonl
    ├── file-edit.jsonl
    └── bash-explore.jsonl
```

## Capture paths

| Path                    | Backend            | When to use                         |
| ----------------------- | ------------------ | ----------------------------------- |
| `make demos-capture`        | OpenRouter        | Reels that need a strong cloud model |
| `make demos-capture-docker` | local llama-server | Reels the 0.5 B Coder can solve     |

The Docker path is what most reels should use going forward — it is
fully reproducible (no network at run time, model baked in) and the
image works on `linux/amd64` and `linux/arm64`.

## Time-compression banners

The renderer detects slow operations (model downloads, model loads, any
LLM turn slower than `--latency-threshold-ms`, default 8000 ms) and
fast-forwards them in the playback to a configurable `--compressed-s`
(default 2.0 s). A dimmed banner like

    ⏩  Fast-forward: model load (47.2s → 2.0s)

makes the compression visible. The cast title is suffixed with
`[time-compressed]` whenever this happens, and a leading line summarizes
the total wall-clock time hidden.

## Regenerating the casts

```sh
make demos-regen
```

That writes `website/static/demos/{file-read,file-edit,bash-explore}.cast`.
Output is byte-stable across runs (timestamp pinned, no PRNG), so casts
should diff cleanly when intentional content changes.

Override terminal geometry with env vars if needed:

```sh
FIZEAU_DEMO_WIDTH=100 FIZEAU_DEMO_HEIGHT=30 make demos-regen
```

## Adding a new demo

1. **Record a fresh session against any LLM backend** to generate one full
   JSONL trace. The easiest path is the existing `demos/record.sh` helper
   (requires LM Studio or any OpenAI-compatible endpoint), or run `fiz`
   manually and grab the resulting file from `~/.fizeau/sessions/` or
   `.fizeau/sessions/`.
2. **Trim** the JSONL to the canonical events the renderer needs:
   - one `session.start`
   - the final `llm.response` (with `content`, `latency_ms`, and `usage`)
   - one `session.end` (with `tokens` + `status`)

   Other event types (`progress`, `llm.delta`, intermediate `llm.request` /
   `tool.call`) are ignored by `regen.py` and can be dropped to keep the
   file small. The example fixtures in `demos/sessions/` show the minimum
   shape.
3. **Save** the trimmed JSONL as `demos/sessions/<name>.jsonl`.
4. **Wire it up** in `demos/regen.sh` by adding one line:

   ```sh
   regen_one <name> "<prompt to display in the cast header>"
   ```
5. **Re-render**:

   ```sh
   make demos-regen
   asciinema play website/static/demos/<name>.cast   # spot check
   ```
6. **Embed** the cast on the demos page by adding an `<script>` tag in
   `website/content/demos/_index.md` pointing at
   `/fizeau/demos/<name>.cast`.

## Why not `asciinema rec` in CI?

The original recording flow (`demos/record.sh` → `asciinema rec -c`)
depends on a running LM Studio (or other OpenAI-compatible) backend with a
specific model, which is not available in CI and produces non-deterministic
output (timing jitter, model drift). The session-replay path here keeps the
live-recording flow available for authors but lets CI and contributors
regenerate the published casts from the checked-in JSONLs without any of
that infrastructure.
