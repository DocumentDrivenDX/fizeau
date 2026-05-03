# Phase A.1 Live Matrix Preflight — 2026-04-30 (revised 2026-05-02)

This preflight covers the Phase A.1 live matrix prerequisite bead. The first
pass ran in `/home/erik/Projects/agent` on 2026-04-30; the 2026-05-02 revision
re-ran the checks after upstream changes renamed the anchor profile and
re-routed it through OpenRouter (`scripts/benchmark/profiles/gpt-5-mini.yaml`,
commits `4eaa6d8`, `7c4d7f0`).

## Status

| Check | Result |
| --- | --- |
| `command -v harbor` | PASS: `/home/erik/.local/bin/harbor` |
| `harbor --version` | PASS: `0.3.0` |
| `command -v pi` | PASS: `/home/erik/.local/share/mise/shims/pi` (target: `/home/linuxbrew/.linuxbrew/bin/pi`) |
| `pi --version` | PASS: `0.67.1` |
| `command -v opencode` | PASS: `/home/erik/.local/share/mise/shims/opencode` (mise-managed `node` global) |
| `opencode --version` | PASS: `1.14.33` |
| `command -v ddx-agent-bench` | PASS: `/home/erik/.local/bin/ddx-agent-bench` |
| `ddx-agent-bench profiles list --work-dir .` | PASS: lists `gpt-5-mini` (Phase A.1 anchor) alongside `claude-sonnet-4-6`, `bragi-qwen3-6-27b`, `vidar-qwen3-6-27b`, `noop`, `smoke` |
| Anchor credential env var present | PASS: `OPENROUTER_API_KEY` is set (73-char value) |
| `OPENAI_API_KEY` is set | N/A: anchor no longer reads `OPENAI_API_KEY` |

## Anchor credential note

The bead acceptance criterion as authored on 2026-04-30 required
`OPENAI_API_KEY` because the anchor profile was then named `gpt-5-3-mini` and
addressed OpenAI directly. Commit `4eaa6d8` ("rename anchor profile to
gpt-5-mini; route adapters through OpenRouter") renamed the profile and
switched it to `provider.type: openai-compat` against
`https://openrouter.ai/api/v1` with `api_key_env: OPENROUTER_API_KEY`. The
canonical Phase A.1 credential is therefore now `OPENROUTER_API_KEY`, and that
variable is set in the intended execution environment. No
`OPENAI_API_KEY`-only profile remains in `scripts/benchmark/profiles/`.

The same rename moves the bead-AC profile name from `gpt-5-3-mini` to
`gpt-5-mini`. References in `scripts/benchmark/cost-guards/README.md`,
`docs/research/harness-matrix-plan-2026-04-29.md`,
`docs/research/model-census-2026-04-29.md`,
`docs/research/matrix-baseline-phase-a1-2026-04-30.md`, and
`docs/helix/02-design/solution-designs/SD-010-harness-matrix-benchmark.md`
still cite `gpt-5-3-mini`; those are stale strings to be reconciled in a
follow-up doc-cleanup bead and do not block the live matrix run.

## Provisioning notes

- `harbor` was installed with `uv tool install harbor`.
- `opencode` was installed with `npm install -g opencode-ai` against the
  mise-managed Node 22.21.1 toolchain; `mise reshim` exposes the shim.
- `ddx-agent-bench` was rebuilt at the current worktree HEAD with
  `go build -o "$HOME/.local/bin/ddx-agent-bench" ./cmd/bench`. The previous
  binary predated commit `9c47a6c` and rejected `top_p`/`top_k` sampling
  fields in `bragi-qwen3-6-27b.yaml`; rebuilding restored `profiles list`.
- No secret values were printed or recorded. Only the env-var name
  (`OPENROUTER_API_KEY`) and its byte length appear in this note.
