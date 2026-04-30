# Phase A.1 Live Matrix Preflight — 2026-04-30

This preflight was run in `/home/erik/Projects/agent` for the Phase A.1 live
matrix prerequisite bead.

## Status

| Check | Result |
| --- | --- |
| `OPENAI_API_KEY` is set | FAIL: env var is unset |
| `command -v harbor` | PASS: `/home/erik/.local/bin/harbor` |
| `harbor --version` | PASS: `0.6.2` |
| `command -v pi` | PASS: `/home/linuxbrew/.linuxbrew/bin/pi` |
| `pi --version` | PASS: `0.70.2` |
| `command -v opencode` | PASS: `/home/linuxbrew/.linuxbrew/bin/opencode` |
| `opencode --version` | PASS: `1.14.30` |
| `command -v ddx-agent-bench` | PASS: `/home/erik/.local/bin/ddx-agent-bench` |
| `ddx-agent-bench profiles list --work-dir .` | PASS: includes `gpt-5-3-mini` |

## Notes

- `harbor` was installed with `uv tool install harbor`.
- `opencode` was installed with `npm install -g opencode-ai`.
- `ddx-agent-bench` was built from this repository with
  `go build -o "$HOME/.local/bin/ddx-agent-bench" ./cmd/bench`.
- No secret values were printed or recorded. The live matrix remains blocked
  until `OPENAI_API_KEY` is provided in the intended execution environment.
