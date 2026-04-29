---
ddx:
  id: oss-harness-install-2026-04-29
  created: 2026-04-29
  parent_plan: harness-matrix-plan-2026-04-29
  parent_bead: agent-37275fa1
status: DRAFT v1
exit_criterion: Step 8 adapters (NEW15, NEW16) land or harness is dropped with rationale below.
---

# OSS Harness Install Spike — pi, opencode

Per Step 7 of `harness-matrix-plan-2026-04-29.md`, this spike captures the
in-container install method, non-interactive flags, profile mapping,
custom-base-url support, license, and known limitations for the **initial
tranche** of OSS harnesses (`pi`, `opencode`). `forge`, `codex`, and
`claude-code` are explicitly **out of scope** for this spike — they live in
the follow-up epic per the plan's codex-v6 cut.

**Drop rule (D1 / plan §Architectural decisions):** if a harness can't be
installed in-container under Harbor it is dropped, *not* run host-side. Both
harnesses below pass that gate.

## In-container `--help` verification (this spike's AC gate)

Both harnesses were verified inside `node:22-slim` (the same Debian/Node
runtime Harbor's `BaseInstalledAgent` containers use) on 2026-04-29:

| Harness  | Pinned version | Install command                                                       | `--help` exit | Notes                                       |
|----------|----------------|-----------------------------------------------------------------------|---------------|---------------------------------------------|
| pi       | 0.67.1         | `npm install -g @mariozechner/pi-coding-agent@0.67.1`                 | 0             | Pure npm install; no apt deps.              |
| opencode | 1.3.17         | `curl -fsSL https://opencode.ai/install \| VERSION=1.3.17 bash`       | 0             | Needs `curl` + `unzip` from apt; PATH=`/root/.opencode/bin`. |

The `node:22-slim` image is the lightest reproducible base that matches the
Harbor agent container's Node major. The same commands work on the heavier
`python:3.12-bookworm` Harbor uses for ddx-agent today (verified by inspection
of `scripts/benchmark/run_benchmark.sh` — both base images ship `apt`, both
install Node ≥ 20 in the existing flow).

The transcripts that produced `0.67.1` / `1.3.17` versions and the literal
`--help` output are reproducible by re-running the two `docker run` commands
above; we do not check transcripts in.

## pi (`@mariozechner/pi-coding-agent`)

**Status: KEPT — proceed to Step 8 adapter (NEW15).**

| Field                    | Value                                                                                                  |
|--------------------------|--------------------------------------------------------------------------------------------------------|
| Upstream repo            | `https://github.com/badlogic/pi-mono` (pkg `packages/coding-agent`)                                    |
| npm package              | `@mariozechner/pi-coding-agent`                                                                        |
| Pinned version           | `0.67.1` (matches host CLI we already ship harness adapter against in `internal/harnesses/pi/`)        |
| Install method           | `npm install -g @mariozechner/pi-coding-agent@0.67.1`                                                  |
| sha256 / lock            | Pin via `package-lock.json` checked into `scripts/benchmark/harness_adapters/pi/` once NEW15 lands. npm registry resolves to a single immutable tarball per version, so version pin is sufficient for reproducibility. |
| Binary on PATH           | `pi` (resolved via `npm` `bin` entry → `dist/cli.js`)                                                  |
| License                  | **MIT** (`license` field in `package.json`)                                                            |
| Benchmarking permission  | MIT permits use, modification, distribution, and benchmarking with no further author approval needed. |
| Memo-publish permission  | MIT — public memos OK; cite repo + commit/version, no logos.                                           |
| Default model            | `google/gemini-2.5-flash` (changeable via `--model` / `--provider`)                                    |

### Non-interactive invocation

`pi -p` (`--print`) is the documented non-interactive mode: process prompt
and exit, no TTY. Combined with `--mode json` the harness emits one
JSON-line-per-event on stdout, which `internal/harnesses/pi/runner.go`
already parses (see `runStreaming` at `internal/harnesses/pi/runner.go:171`).

For Harbor adapter use:

```sh
pi --mode json --print --no-session \
   --provider <provider> --model <model> \
   [--thinking <level>] \
   "<prompt>"               # prompt as last positional arg
# or
pi --mode json --print --no-session ... < /dev/null   # if prompt comes via stdin
```

- **No-TTY safe:** `--print` is the explicit non-interactive switch; pi does
  not call `isatty()` to gate behavior in this mode.
- **Stdin handling:** the adapter should attach `/dev/null` to stdin unless
  it intentionally pipes the prompt. Pi's interactive mode reads stdin; in
  `--print` mode it does not, so `/dev/null` is safe and recommended.
- **Exit-on-done:** `--print` exits 0 on success, non-zero on auth /
  provider failure. `--no-session` prevents writing session files into the
  task workdir, which would otherwise leak across reps.

### Profile mapping (Step 2 schema → pi flags)

| Profile field                         | pi mechanism                                                  |
|---------------------------------------|---------------------------------------------------------------|
| `provider.type` / `provider.model`    | `--provider <name> --model <id>` (or `--model provider/id`)   |
| `provider.base_url`                   | **See "custom base URL" below — env-var, not CLI flag.**      |
| `provider.api_key_env`                | adapter exports the named env var into the child process     |
| `sampling.temperature`                | not exposed as a flag; pi defers to provider default. Document as a known limitation. |
| `sampling.reasoning`                  | `--thinking <off\|minimal\|low\|medium\|high\|xhigh>`         |
| `limits.max_output_tokens`            | not exposed as a flag; provider default applies.              |

### Custom base URL / OpenAI-compat / OpenRouter routing

Pi's `--provider` selects from a built-in registry; arbitrary base URLs are
**not** wired up as a first-class CLI flag. However, the registry includes
an `openai-compat` provider that accepts:

```
PI_OPENAI_COMPAT_BASE_URL=https://openrouter.ai/api/v1
PI_OPENAI_COMPAT_API_KEY=$OPENROUTER_API_KEY
```

(verified against `picompat/auth.go` and `picompat/settings.go` in this
repo, which mirror pi's settings shape). For OpenRouter routing we therefore
set `--provider openai-compat --model <openrouter-model-id>` and inject the
two env vars from the profile. Direct providers (anthropic, openai, google)
use their own env vars (`ANTHROPIC_API_KEY`, `OPENAI_API_KEY`,
`GOOGLE_API_KEY` / `GEMINI_API_KEY`).

The adapter normalises this so the profile YAML stays portable: profiles
declare `provider.type: openai-compat` + `base_url` + `api_key_env`, and the
adapter sets `PI_OPENAI_COMPAT_*` accordingly.

### Known limitations

- **Sampling temperature not pluggable** via CLI; if Phase A.2 needs it,
  file a feature request upstream or write a `--system-prompt` shim.
- **`max_output_tokens` not pluggable** via CLI; same caveat.
- **Default model is `google/gemini-2.5-flash`** — the adapter MUST set
  `--provider` + `--model` explicitly so a missing profile field doesn't
  silently fall through to gemini.
- **Extension auto-discovery** — pi loads extensions from `~/.pi` and project
  paths by default. The adapter must pass `--no-extensions --no-skills
  --no-prompt-templates --no-themes` to keep cells reproducible across
  developer machines and the matrix container.
- **Sessions on disk** — without `--no-session`, pi writes session JSONL
  into the cwd. This will trip TB-2 verifiers that snapshot the workdir.
  Adapter must pass `--no-session` (or `--session-dir <ephemeral>` if we
  later need session continuity for multi-turn tasks).

## opencode (`sst/opencode`)

**Status: KEPT — proceed to Step 8 adapter (NEW16).**

| Field                    | Value                                                                                                  |
|--------------------------|--------------------------------------------------------------------------------------------------------|
| Upstream repo            | `https://github.com/sst/opencode`                                                                      |
| Install URL              | `https://opencode.ai/install` (Bash installer; downloads pinned tarball + verifies)                    |
| Pinned version           | `1.3.17` (matches `internal/harnesses/opencode/runner.go` BaseArgs assumptions)                        |
| Install method           | `curl -fsSL https://opencode.ai/install \| VERSION=1.3.17 bash`                                        |
| sha256 / lock            | Installer fetches a versioned binary from `github.com/sst/opencode/releases/download/v<VERSION>/`. The adapter Dockerfile must `curl -fsSL` the release archive directly and verify against the upstream-published `*.sha256` (released next to each tag) rather than piping `install` through `bash`. The piped form is OK for this spike; for NEW16 the adapter pins the archive URL + sha. |
| Binary on PATH           | `/root/.opencode/bin/opencode` (installer puts it there; adapter prepends to PATH)                     |
| License                  | **MIT** (per `LICENSE` in upstream repo, root)                                                         |
| Benchmarking permission  | MIT permits use, modification, distribution, and benchmarking with no author approval needed.          |
| Memo-publish permission  | MIT — public memos OK; cite repo + version.                                                            |
| Default model            | none enforced; profile must set `-m provider/model`. (`internal/harnesses/opencode/runner.go:54` defaults `opencode/gpt-5.4` for *our* harness wrapper, not opencode itself.) |

### Non-interactive invocation

`opencode run` is the documented non-interactive subcommand. Combined with
`--format json`, it emits one JSON event per line and exits when the agent
turns conclude. This is what `internal/harnesses/opencode/runner.go:170`
already drives.

```sh
opencode run --format json \
   --dir <task-workdir> \
   -m <provider>/<model> \
   [--variant <reasoning>] \
   --                 \
   "<prompt>"
```

- **No-TTY safe:** `run` does not assume a TTY; `--format json` disables
  the spinner / TUI rendering paths.
- **Stdin handling:** opencode `run` reads its prompt from positional args,
  *not* stdin. The adapter MUST attach `/dev/null` to stdin to prevent the
  process from blocking if Harbor's runner happens to leave a pipe open.
- **Exit-on-done:** `run` exits when the model emits its final assistant
  turn. Non-zero exit on auth / provider / tool failures.
- **`opencode run` auto-approves all tool permissions** (verified in the
  comment at `internal/harnesses/opencode/runner.go:177`); no extra
  permission flags are needed for the matrix.

### Profile mapping (Step 2 schema → opencode flags)

| Profile field                         | opencode mechanism                                            |
|---------------------------------------|---------------------------------------------------------------|
| `provider.type` / `provider.model`    | `-m <provider>/<model>` (single combined flag)                |
| `provider.base_url`                   | `~/.config/opencode/opencode.json` `provider.<id>.options.baseURL` (JSON config; no CLI flag). The adapter writes this file on first run from the profile. |
| `provider.api_key_env`                | adapter exports the named env var (e.g. `OPENROUTER_API_KEY`); opencode reads provider creds from env per its provider plugin |
| `sampling.temperature`                | not exposed as CLI; configurable via `opencode.json` `provider.<id>.options.temperature`. |
| `sampling.reasoning`                  | `--variant <high\|max\|minimal\|...>` (provider-specific)     |
| `limits.max_output_tokens`            | `opencode.json` `provider.<id>.options.maxTokens`             |

The adapter's `apply_profile()` writes a minimal `opencode.json` per run
into a per-cell ephemeral `OPENCODE_CONFIG_DIR=<cell-tmp>` so cells don't
contend on `~/.config/opencode/opencode.json`.

### Custom base URL / OpenAI-compat / OpenRouter routing

Opencode has first-class OpenAI-compat support via its provider plugin
system. For OpenRouter:

```jsonc
// $OPENCODE_CONFIG_DIR/opencode.json
{
  "provider": {
    "openrouter": {
      "npm": "@openrouter/ai-sdk-provider",
      "options": { "apiKey": "{env:OPENROUTER_API_KEY}" },
      "models": { "qwen/qwen3.6-plus": {} }
    }
  }
}
```

then `opencode run -m openrouter/qwen/qwen3.6-plus ...`. The adapter
generates this JSON from the profile YAML; `provider.type: openai-compat` +
`base_url` + `api_key_env` map to a generic `openai-compat` provider entry.

### Known limitations

- **Profile cleanup:** opencode writes session DBs into
  `$OPENCODE_DATA_DIR` (defaults to `~/.local/share/opencode`). Adapter
  must override `OPENCODE_DATA_DIR=<cell-tmp>` and clean it between reps —
  otherwise rep-2 starts with rep-1's session cache and the matrix is
  contaminated.
- **Auto-update:** stock `opencode` checks for updates on launch over
  HTTPS; this is undesirable in a benchmark cell. Adapter sets
  `OPENCODE_DISABLE_AUTOUPDATE=1` (and `--pure` to skip external plugin
  loading) for hermeticity.
- **Plugin loading:** the `--pure` flag bypasses external plugin
  discovery; the adapter passes it on every cell to keep behaviour
  identical across hosts.
- **mDNS / port-binding:** `opencode run` defaults `--hostname 127.0.0.1`
  and a random port; in-container this is fine, but the adapter explicitly
  sets `--port 0 --hostname 127.0.0.1 --mdns false` to avoid surprising
  network exposure inside Harbor's container.
- **No CLI for max_output_tokens / temperature** — must go through the
  generated `opencode.json`. Document this in the profile loader so
  fields aren't silently dropped.

## Dropped harnesses (this spike's tranche)

None. Both `pi` and `opencode` install in-container, run `--help` cleanly,
ship under MIT, and have profile-mappable provider routing.

## Out of scope (deferred to follow-up epic)

Per the plan's codex-v6 cut, the following are explicitly **not part of this
spike** and have no entry above:

- **forge** — install-method spike deferred; if the follow-up spike confirms
  in-container installability, an adapter is filed then. Otherwise dropped
  per D1.
- **codex** — frontier reference, ToS / fair-use review must precede any
  public memo. Adapter implementation is filed in the follow-up epic.
- **claude-code** — same as codex. Internal memos only until ToS reviewed.

The follow-up epic re-runs the same `--help`-in-container gate against each
of these before adapter beads are claimed.

## Acceptance evidence (cross-ref to bead AC)

| AC item                                                                                              | Evidence                                                                                                              |
|------------------------------------------------------------------------------------------------------|-----------------------------------------------------------------------------------------------------------------------|
| Doc lands at `docs/research/oss-harness-install-2026-XX-XX.md`                                       | this file                                                                                                             |
| For each kept harness, in-container `--help` runs successfully under Harbor                          | §"In-container `--help` verification" — both pi and opencode exited 0 inside `node:22-slim` on 2026-04-29              |
| License + benchmarking permission recorded                                                           | §pi license: MIT; §opencode license: MIT; both permit benchmarking + memo publication with attribution                |
| Dropped harnesses get explicit rationale                                                             | §"Dropped harnesses": none in this tranche; forge / codex / claude-code listed under "Out of scope" with reason       |
| Per-harness install method (binary URL + sha256 OR pinned pip/npm)                                   | pi: `npm install -g @mariozechner/pi-coding-agent@0.67.1`; opencode: `VERSION=1.3.17` installer (sha pin lifted to NEW16 adapter Dockerfile) |
| Per-harness non-interactive flags                                                                    | pi: `--print --mode json --no-session --no-extensions --no-skills --no-prompt-templates --no-themes`; opencode: `run --format json --pure --mdns false` + stdin=/dev/null |
| Per-harness profile mapping                                                                          | mapping tables in §pi and §opencode                                                                                   |
| Per-harness custom-base-url support                                                                  | pi: `PI_OPENAI_COMPAT_BASE_URL` env; opencode: `opencode.json` provider entry with `baseURL`                          |
| Per-harness known limitations                                                                        | §pi and §opencode each end with a "Known limitations" subsection                                                       |
