---
title: Agents Comparison
weight: 50
toc: true
---

A side-by-side comparison of Fizeau against every coding-agent harness that
has a row in the Fizeau Terminal-Bench 2.1 leaderboard ([data/aggregates.json](https://github.com/easel/fizeau/blob/master/docs/benchmarks/data/aggregates.json))
plus the canonical open-source CLI agents users typically choose between.

Numbers in the **TB-2.1 best** column are from the snapshot
`data/aggregates.json` regenerated **2026-05-10**. The figure is `tasks_passed /
tasks_attempted` on the 89-task `all` subset, picking the best-scoring
harness×model row for that harness. See [How we measure](#how-we-measure)
below for what that number does and does not mean.

A `?` means the field could not be confirmed from the project's canonical
source. We deliberately do not guess.

## Comparison

| Agent | Source | Language | License | Distribution | Embeddable library | Local-model support | Built-in tools | TB-2.1 best | Best row |
|---|---|---|---|---|---|---|---|---|---|
| **Fizeau (fiz)** | [easel/fizeau](https://github.com/easel/fizeau) | Go | MIT | CLI + Go library | Yes — `fizeau.New(...).Execute(ctx, req)` | Yes — LM Studio, Ollama, vLLM, MLX, any OpenAI-compatible | 9 — read, write, edit, bash, find, grep, ls, patch, task | 52 / 89 (pass@k, fiz-openrouter-qwen3-6-27b)[^fiz] | per_profile |
| Claude Code | [anthropics/claude-code](https://github.com/anthropics/claude-code) | Shell + Python + TypeScript | Proprietary (Anthropic Commercial Terms) | CLI | ? (no documented programmatic API) | ? (not documented in README; community guides exist) | ? (plugins, `/bug`, git workflows; no enumerated list in README) | 46 / 89 | `ClaudeCode__GLM-4.7` |
| OpenAI Codex CLI | [openai/codex](https://github.com/openai/codex) | Rust | Apache-2.0 | CLI (npm, brew, binary) + `/sdk` directory | Partial — SDK directory present, CLI is primary surface | Yes — `--oss` flag plus `OPENAI_API_BASE` to Ollama / LM Studio / MLX[^codex-local] | ? (not enumerated in README) | — (no leaderboard row for Codex CLI itself) | — |
| OpenCode | [sst/opencode](https://github.com/sst/opencode) | TypeScript | MIT | CLI + desktop (beta) | ? (npm package published; library API not documented) | Yes — "not coupled to any provider … local models" per README | ? (not enumerated in README) | 46 / 88 | `OpenCode__Claude-Opus-4.5` |
| Gemini CLI | [google-gemini/gemini-cli](https://github.com/google-gemini/gemini-cli) | TypeScript | Apache-2.0 | CLI (npm, brew, MacPorts, conda) | Yes — `@google/gemini-cli` npm package | No (Google Gemini cloud only) | File ops (read/write/search), shell exec, web fetch, Google Search grounding, MCP server integration | 64 / 89 | `Gemini_CLI__Gemini-3-Flash-Preview` |
| Aider | [Aider-AI/aider](https://github.com/Aider-AI/aider) | Python | Apache-2.0 | CLI (`pip install aider-chat`) | ? (Python package; library API not the documented surface) | Yes — Ollama, LM Studio, any OpenAI-compatible API[^aider-local] | Git auto-commit, lint, test, image/web ingest, voice input, repo map, watch mode | — (no leaderboard row) | — |
| Continue | [continuedev/continue](https://github.com/continuedev/continue) | TypeScript (+ Kotlin / Python / Rust) | Apache-2.0 | VS Code ext, JetBrains plugin, CLI (`cn`) | ? (`@continuedev/cli` published; library API not documented) | Yes — Ollama configurable as provider in `config.yaml` | Context providers: file, code, diff, http, terminal; plus MCP servers | — (no leaderboard row) | — |
| Goose | [block/goose](https://github.com/block/goose) | Rust + TypeScript | Apache-2.0 | Desktop app, CLI, API | Yes — API documented for embedding | Yes — Ollama listed among 15+ providers | 70+ extensions via MCP | — (no leaderboard row) | — |
| Forge (Code-Forge) | [antinomyhq/forge](https://github.com/antinomyhq/forge) | Rust | Apache-2.0 | CLI | ? (CLI-only, no documented library surface) | ? (cloud providers only in README) | 3 agents (forge / sage / muse), plus shell, git, file r/w, 3 skills (create-skill, execute-plan, github-pr-description), MCP | 80 / 89 | `Forge__GPT-5.4` |
| Junie CLI | [JetBrains/junie](https://github.com/JetBrains/junie) | Shell + PowerShell wrappers | JetBrains AI Service Terms (proprietary) | CLI (npm) + IDE | ? (npm package; library API not documented) | ? — BYOK for Anthropic, OpenAI, Google, xAI, OpenRouter, Copilot; local-model support listed as roadmap[^junie-local] | ? (not enumerated; MCP server install supported) | 79 / 89 | `Junie_CLI__Gemini-3-Flash-Preview-Gemini-3.1-Pro-Preview-Claude-Opus-4.6-GPT-5.3-Codex` |
| Pi | [badlogic/pi-mono](https://github.com/badlogic/pi-mono) | TypeScript | MIT | CLI (npm) | Yes — `@earendil-works/pi-agent-core` and `pi-ai` published as separate packages | ? (unified LLM API; local-model specifics not in README) | 4 — read, write, edit, bash | — (no leaderboard row) | — |

[^fiz]: Fizeau's number is **pass@k** on 89 tasks, sourced from
    `per_profile` in `aggregates.json` (`tasks_passed_any` / `tasks_touched`
    for `fiz-openrouter-qwen3-6-27b`). External leaderboard rows are
    `tasks_passed` / `tasks_attempted` from
    `harborframework/terminal-bench-2-leaderboard` on Hugging Face. The
    counting rules differ — see [How we measure](#how-we-measure).

[^codex-local]: Confirmed via [OpenAI Codex docs](https://developers.openai.com/codex/config-advanced)
    and [Ollama integration page](https://ollama.com/blog/codex). Codex CLI
    accepts any OpenAI-compatible base URL, so any local server that exposes
    that surface (Ollama, LM Studio, MLX, vLLM, llama.cpp's server) works.

[^aider-local]: Confirmed via [aider.chat/docs/llms.html](https://aider.chat/docs/llms.html).

[^junie-local]: ["Expanded support for local models" listed on the
    JetBrains roadmap](https://youtrack.jetbrains.com/projects/JUNIE/issues/JUNIE-47);
    not in the shipped 2026-Q1 beta as of this snapshot.

## How we measure

The TB-2.1 column reports the score of each external harness's best
submission to the public Terminal-Bench 2.1 leaderboard
([harborframework/terminal-bench-2-leaderboard](https://huggingface.co/datasets/harborframework/terminal-bench-2-leaderboard)
on Hugging Face). For each `Harness__Model` pair we count
`tasks_passed / tasks_attempted` on the full 89-task set, then keep the
single best pair per harness. Submission dates vary; the snapshot index
date is the date we last refreshed the cache.

Fizeau's row is sourced differently: it is `tasks_passed_any /
tasks_touched` from our local `per_profile` aggregate
(`fiz-openrouter-qwen3-6-27b` is the profile with the most graded reps). That
is **pass@k** with `k=5` reps per task, not the per-rep `pass@1` that the
external leaderboard implicitly reports. The two numbers are not directly
comparable; we report Fizeau's anyway because it is the dominant profile in
the data and a single-number summary is more useful than no summary. For
apples-to-apples comparison see the
[Terminal-Bench 2.1 page](/benchmarks/terminal-bench-2-1/), which exposes
both metrics, the per-task slice, and the cost / wall-time / token
breakdown.

External rows count one pass per task per submission. Reps are not exposed
uniformly across submissions, so we cannot reconstruct per-rep `pass@1` for
the external column without cooperation from each submitter.

## Where we don't compare

Some agents users ask about are excluded from the table, deliberately:

- **Cursor** — IDE-only product. There is no CLI or programmatic API for an
  external benchmark to drive, so it cannot be measured under the harness
  model TB-2.1 uses. It is outside the comparable surface.
- **GitHub Copilot Chat** — same reason as Cursor.
- **Crux**, **Capy**, **Mux**, **Judy**, **MAYA**, **OB-1**, **OpenSage**,
  **Terminus2**, **Terminus-KIRA**, **Ante**, **CodeBrain-1**,
  **Deep-Agents**, **Droid (Factory)**, **IndusAGICodingAgent**,
  **Simple-Codex**, **pilot-real**, **cchuter**, **dakou**, **grok-cli**,
  **BashAgent**, **terminus-2** — present in the leaderboard data but
  without a publicly canonical source repository we could verify in this
  pass. They are tracked in `external_per_subset`; their facts are
  unverified, so they are intentionally not in the table rather than filled
  with `?` rows. Submit a correction (see below) and we will add them.
- **Continue, Aider, Goose, Pi** — included in the table for build-vs-buy
  reference, even though they have not submitted to Terminal-Bench 2.1.
  Their TB-2.1 column reads `—`.

## How to add a row

To correct a fact or add a missing harness:

1. Open a PR against `website/content/docs/comparison/_index.md` on
   [easel/fizeau](https://github.com/easel/fizeau).
2. Cite the canonical source for each fact in the row (project README,
   docs page, license file). Inline footnotes are the right shape.
3. If the harness has a Terminal-Bench 2.1 leaderboard submission we
   missed, link the Hugging Face submission ID. The next refresh of
   `data/aggregates.json` (via `scripts/benchmark/generate-report.py
   --refresh-leaderboard`) will pick it up automatically.
4. Do not add qualitative ranking ("best", "easiest", "fastest"). The
   table is a fact sheet; ranking belongs in the
   [benchmarks](/benchmarks/) pages where the per-task data backs it.
