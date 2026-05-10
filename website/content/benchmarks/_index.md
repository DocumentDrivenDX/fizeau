---
title: Benchmarks
weight: 2
toc: false
---

## Why benchmark

Fizeau exists because we wanted a single agent runtime where the *harness* and the *model* are independently swappable. The benchmarks here exist for the same reason: they **separate harness loss from model loss** so you can answer questions of the form *"is it the loop that's hurting, or the model?"* with evidence.

Each benchmark exercises the agent loop the same way — same prompts, same tools, same compaction policy, same tool-call accounting. We then permute the variables:

- **Same model, different provider/runtime.** The Qwen3.6-27B lanes route the same model through OpenRouter (cloud), vLLM int4 on a local GPU (sindri), and oMLX 8-bit on Apple silicon (vidar). A pass-rate or wall-time delta between these lanes is *provider/runtime loss* — the cost of how the bytes reach the model, not what the model is.
- **Same model, different harness.** The `fiz-harness-*` lanes wrap Claude Code, Codex, Pi, and OpenCode through fiz so the model and the API stay constant while the agent loop changes. A delta here is *harness loss*.
- **Different models, same task.** The leaderboard rows on each report page show how frontier hosted models (Claude Opus 4.6, GPT-5.4, Gemini 3 Pro) score on the same task set. That's the upper bound a small open-weight model is measured against.

Every per-turn timing — first-token latency, decode rate, prefill time — lands on disk in line-delimited JSON. The reports below come from those logs via [`scripts/benchmark/generate-report.py`](https://github.com/easel/fizeau/blob/master/scripts/benchmark/generate-report.py); rerunning the script regenerates every chart and table here.

## Headline numbers

{{< bench-comparison >}}

## What's measured

| signal | source | what it tells you |
|---|---|---|
| pass@k | TB-2.1 verifier | does the agent solve the task, k=5 reps, any-pass |
| TTFT (p50) | per-turn `llm.delta` − `llm.request` | provider prefill + queue latency under realistic context |
| decode tok/s (p50) | per-turn `llm.response.ts` − first delta | steady-state generation rate post-prefill |
| wall (p50) | trial start → trial end | total time the agent took, end-to-end |
| turns (p50) | count of `llm.request` per trial | how much the agent loop iterated |
| cost ($) | provider pricing × tokens | only meaningful for paid lanes |

A turn-by-turn breakdown bucketed by input-token length (prefill scaling) is on each per-benchmark page.

## Available benchmarks

The left navigation links each benchmark we run today; add a new one by editing [`scripts/benchmark/profiles/*.yaml`](https://github.com/easel/fizeau/tree/master/scripts/benchmark/profiles) and rerunning the generator.
