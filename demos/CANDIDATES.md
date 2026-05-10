---
title: Demo Reel Candidates
status: research
date: 2026-05-10
source: docs/helix/user-story-website-coverage.md (demo-able rows)
length-target: 30 seconds per reel
format: asciinema (.cast), captured via `make demos-capture`
---

# Demo Reel Candidates — Top 8

These are the eight user stories from the coverage map most likely to
make a punchy ~30-second asciinema reel for the website (`/demos/`).
Each is **before/after-able** at the terminal, requires no graphical
output, and captures a *Fizeau-distinct* capability rather than something
every agent CLI does.

The current `/demos/` page has three reels (read-a-file, edit-a-config,
explore-project-structure). Those are file-tool demos and are useful but
not differentiating. The eight below show measurement, routing,
guardrails, and embedding — the pillars of the product.

---

## 1. Cost cap halts the loop mid-run

- **Why this story:** `AC-FEAT-005-07`. Few agent CLIs have a hard
  per-run budget. Showing one halt before the next request is
  visceral and operator-relevant.
- **Prompt:** `fiz run --cost-cap 0.05 -p "rewrite every Go file in this repo to add a doc comment to the package declaration"`
- **What fiz does:** Starts work, executes a few iterations, accumulates
  cost, then halts before the iteration that would push over $0.05.
- **Expected output (last 4 lines):**
  ```
  [llm.request 4] cumulative_cost=$0.043
  [llm.request 5] cumulative_cost=$0.052 → BUDGET_HALTED
  process_outcome=budget_halted
  cost: $0.052 / cap $0.050
  ```
- **Data needed:** A repo dir with ≥30 Go files (Fizeau itself works).
  Use `cheap` policy with a real cloud provider so cost increments
  are visible. Pre-warm credentials.

## 2. Routing decision: same prompt, three policies, three choices

- **Why this story:** `AC-FEAT-004-02`, `AC-FEAT-004-03`. The product's
  measurement-first identity. Showing `--policy cheap → local`,
  `--policy default → mid`, `--policy smart → frontier` in a tight
  three-shot is a textbook differentiator.
- **Prompt:** `for p in cheap default smart; do fiz route-status --policy $p --json | jq '.selected.provider, .selected.model'; done`
- **What fiz does:** Reports the route Fizeau would pick for each
  policy without actually invoking the model. Pure metadata.
- **Expected output:**
  ```
  "lmstudio-local"
  "qwen3.5-7b"
  "openrouter"
  "qwen/qwen3.6-27b"
  "anthropic"
  "claude-opus-4-7"
  ```
- **Data needed:** A `.fizeau/config.yaml` with all three providers
  configured (LM Studio running locally, OpenRouter key, Anthropic
  key). Demo-only stub configs are fine if real keys not available.

## 3. Tool-call loop detector trips and saves the user from a runaway

- **Why this story:** `AC-FEAT-001-09`. The kind of safety story most
  agent demos skip because it requires *intentionally* failing the
  model. High empathy: every operator has watched a runaway loop.
- **Prompt:** `fiz run --max-iter 50 -p "keep calling the ls tool on /tmp until you find a file named impossible.lock — do not stop"`
- **What fiz does:** Issues `ls` 1, 2, 3 — same args, same result,
  same args — then aborts with `ErrToolCallLoop` on call 3.
- **Expected output:**
  ```
  [tool.call 1] ls /tmp → 47 files
  [tool.call 2] ls /tmp → 47 files
  [tool.call 3] ls /tmp → 47 files
  ✗ ErrToolCallLoop: 3 consecutive identical tool calls (ls)
  ```
- **Data needed:** Any model willing to obey the prompt. Local Qwen
  works; cloud Claude works. The reel is the same shape either way.

## 4. JSONL session log → `fiz replay` round trip

- **Why this story:** `AC-FEAT-005-01`, `AC-FEAT-005-02`. The
  observability-first promise made tangible. Show the raw JSONL on
  disk *and* the rendered replay side by side.
- **Prompt (two commands):**
  ```
  fiz run -p "explain what main.go does" --json | tee /tmp/run.json
  fiz replay $(jq -r .session_id /tmp/run.json)
  ```
- **What fiz does:** Runs a small read+explain task, saves the session
  ID, replays it.
- **Expected output:** First half shows JSON `session_id` and lines of
  the JSONL log being appended. Second half shows the human-readable
  replay (USER → ASSISTANT → tool: read → ASSISTANT) with token
  counts and timings inline.
- **Data needed:** Any small Go file. Local model preferred so the
  reel is fast (no cloud RTT).

## 5. Per-turn TTFT measurement, live

- **Why this story:** `AC-FEAT-001-05`. Connects the tagline ("a
  measurement-first agent loop") to a number on the screen. Most
  agent CLIs hide this; Fizeau exposes it.
- **Prompt:** `fiz run --json -p "summarize this README" | jq '.events[] | select(.type=="llm.delta" or .type=="llm.response") | {turn:.turn, ttft_ms:.ttft_ms, decode_tok_s:.decode_tok_s}'`
- **What fiz does:** Runs the prompt with streaming on. The events
  include first-token latency and decode rate per turn.
- **Expected output:** A small table:
  ```
  {turn:1, ttft_ms:412, decode_tok_s:67.4}
  {turn:2, ttft_ms:298, decode_tok_s:71.1}
  ```
- **Data needed:** README.md present. Streaming-capable provider
  (OpenAI-compatible local or remote).

## 6. `fiz usage` report — known vs unknown cost

- **Why this story:** `AC-FEAT-005-03`, `AC-FEAT-005-05`. The cost-attribution
  policy ("never guess") is unique and hard to convey in prose. Showing
  a `fiz usage` table where some rows have `cost = ?` and others have
  `cost = $0.0042` makes the policy click in three seconds.
- **Prompt:** `fiz usage --since 24h --by provider`
- **What fiz does:** Reads recent session logs, groups by provider,
  prints token totals and cost where known, `?` where unknown
  (e.g. local model with no configured price table).
- **Expected output:**
  ```
  provider           input    output  cost
  anthropic         128400     31200  $0.4231
  openrouter         42100      8700  $0.0084
  lmstudio-local    310500     91200  ?
  ```
- **Data needed:** A few hours of mixed-provider session history. Can
  be synthesized for the reel by running 4–5 prompts across providers.

## 7. Embed Fizeau in a 12-line Go program

- **Why this story:** `AC-FEAT-001-01`, `AC-FEAT-006-08`. The
  embed-as-library story is the thesis from `prd.md` and the
  `/docs/embedding/` page. A reel showing `go run` of a tiny program
  proves the in-process pitch.
- **Prompt (split-screen vibe in the asciinema):** type a `main.go`
  using the embedding API, then `go run main.go`.
- **What fiz does:** The Go program calls `fizeau.New(...).Execute(...)`
  and ranges the events channel. The terminal shows `session.start`,
  `llm.request`, `llm.response`, `session.end` events streamed in
  real time.
- **Expected output:** The events channel printed line by line with
  token counts on `llm.response`.
- **Data needed:** Snippet must use `Policy: "cheap"` (NOT `ModelRef`,
  per audit I-03). Local model running on `localhost:1234`.
- **Note:** This reel requires the I-03 fix to land first — otherwise
  the example will not compile.

## 8. `fiz update --check-only` and atomic in-place upgrade

- **Why this story:** `AC-FEAT-007-01`, `AC-FEAT-007-02`. Self-update is
  surprisingly satisfying to watch as a reel because the binary
  literally rewrites itself. Differentiator vs. CLIs that require a
  package manager.
- **Prompt:**
  ```
  fiz version
  fiz update --check-only; echo "exit=$?"
  fiz update
  fiz version
  ```
- **What fiz does:** Reports current version, checks for newer release,
  downloads, atomically replaces, reports new version. The whole loop
  is under 10 seconds on a fast network.
- **Expected output:**
  ```
  fiz v0.14.2
  current: v0.14.2  latest: v0.14.3 → outdated
  exit=1
  Downloading fiz_linux_amd64 …  ✓ verified  ✓ replaced
  fiz v0.14.3
  ```
- **Data needed:** A pre-staged older binary in `~/.local/bin/fiz` and a
  release tag one version ahead. Demo can be filmed against staging
  or against the real release pipeline.

---

## Capture notes

- All eight reels can be captured with the existing `make demos-capture`
  flow (asciinema + `demos/scripts/`); they should land as
  `demos/sessions/*.jsonl` first so they are reproducible from the
  underlying session log, not just a screen recording.
- Reels 1, 3, 5, 6, 8 need real network/credentials; reels 2, 4, 7 can
  run fully offline against LM Studio + a stub config.
- Total runtime budget: 8 × 30s = 4 minutes of asciinema, comfortably
  the size of a single home-page autoplay strip.
