# Project rename — Fizeau / fiz

**Status: ADOPTED.** Use these names in code, packaging, and commits effective immediately.

Date: 2026-04-27

## Context

Prior renames (from git history): `luce → lucebox`. Current working name: `agent`.

One of the project's stated goals is supporting **testing**, **optimization**, and **one-shot results** — which is also why it has no TUI. The naming exploration was scoped to metaphors that fit that one-shot framing: scientists known for a single decisive experiment, famous sharpshooters, and golfers known for hole-in-one / albatross shots.

## Proposed names

- **Project / repo / package**: `fizeau`
- **CLI binary**: `fiz`

## Why Fizeau

Hippolyte Fizeau (1849) measured the speed of light using a single rotating toothed wheel and a mirror 8 km away — one precise measurement, one clean answer. The metaphor maps unusually well:

- **One shot**: one rotation of the cog → one measurement
- **Precision over throughput**: not Monte Carlo, not statistical sampling — a single carefully-instrumented observation
- **No interactivity**: the cog spins, the light returns or it doesn't. No TUI in the apparatus.
- **Bonus**: the Fizeau–Foucault apparatus is literally a measurement *harness*

Predates and is more on-brand than Foucault, who is overloaded philosophically.

## Availability snapshot (checked 2026-04-27)

| Slot | Status |
|---|---|
| PyPI `fizeau` | Free (404) |
| npm `fizeau` | Free (404) |
| PyPI `fiz` | Taken but empty/abandoned (no description, no author) |
| npm `fiz` | Taken — old fis3 plugin tool, last release 1.4.5, dormant |
| GitHub `fizeau` repos | Top hit: `averne/Fizeau` (467★, Nintendo Switch color tool). No AI/dev/agent collisions. |
| GitHub `fiz` short name | Some noise: FizzBuzz, `facebook/fizz` (TLS-1.3, different spelling), `fizsh`. No agent/eval tooling. |
| `/usr/bin/fiz*`, `/usr/local/bin/fiz*` | Nothing installed |
| `fizeau.dev` / `fizeau.io` | DNS doesn't resolve (likely registrable) |
| `fiz.dev` | Resolves (likely parked) |

## Other names considered

Scientists: **eratos** (Eratosthenes — clean, good story), **eddington** (1919 eclipse, mild collision with cycling/astronomy), **cavendish** (heavy collision — Cavendish Lab, banana cultivar), **millikan** (workable, less euphonic).

Sharpshooters: **hathcock** (Carlos Hathcock, near-zero collision, slightly grim), **hayha** (Simo Häyhä, darker connotation), **oakley** (killed by sunglasses brand), **deadeye** (generic but clean in AI/dev space).

Golf / hole-in-one: **sarazen** (Gene Sarazen's 1935 "shot heard round the world", uncommon), **albatross** (heavy collision), **ace** (no wiggle room).

Other one-shot motifs: **ricochet**, **potshot**, **breakshot**.

Fizeau won on theme fit + cleanest namespace.

## Open risks before adopting

1. **Pronunciation split** — English speakers will hit "fih-ZOH" vs "FEE-zoh" vs "FYE-zoh". Pick one and document in README.
2. **Dormant `fiz` squatters** — if we ever publish a Python or Node package, distribution name will need to be `fizeau-cli` or similar even if the binary is `fiz` (ripgrep/`rg` pattern).
3. **`averne/Fizeau` GitHub overlap** — will share search results until this repo outranks it. Self-correcting, minor.

## If adopted

Follow the `luce → lucebox` playbook: repo rename, module/import paths, binary name, version bump release commit.

## Tagline candidates

- *"measure once, cleanly"*
- *"one rotation, one number"*
- *"the speed-of-light eval harness"*



## Target Names

| Artifact | Name |
|---|---|
| Project / repo / package | `fizeau` |
| CLI binary | `fiz` |
| Go module path | `fizeau` (e.g. `github.com/<org>/fizeau`) |
| Root package | `fizeau` |

Pronunciation: **FYE-zoh** (rhymes with "size-oh"). Document in README.

## Rename Boundary

The rename is **broad** — every public-facing surface and internal reference is updated:

- **Go module path** — `go.mod` module declaration, all `import` paths
- **Root package** — package name in `main.go` and any top-level package declarations
- **CLI binary** — output binary name, `cmd/` directory structure, `main` package
- **Config directories** — `~/.config/fizeau`, `~/.local/share/fizeau`, any XDG paths
- **Environment variables** — `FIZEAU_*` prefix (e.g. `FIZEAU_HOME`, `FIZEAU_LOG_LEVEL`)
- **Public docs** — README, godoc comments, any markdown under `docs/`
- **DDx dependency** — any reference to the project name in `.ddx/` configuration or execution artifacts
- **Release / updater** — release asset names, self-update endpoints, version tags

## Non-Goals

- **Historical evidence rewrites** — git history, old commit messages, and archived research docs are **not** rewritten. The rename is forward-looking only.
- **Backward-compat aliases** — no `agent` or `lucebox` shim commands, no legacy config migration.
- **Domain registration** — acquiring `fizeau.dev` or `fizeau.io` is out of scope for this rename pass.
- **Package registry squatting** — publishing placeholder packages on PyPI/npm to block `fiz` is deferred.

## Config/Env Policy

- **No compatibility window.** The rename is a hard break. Old config paths (`~/.config/agent`, `~/.config/lucebox`) and old env var prefixes (`AGENT_*`, `LUCEBOX_*`) are **not** read or migrated.
- Users must re-create config at `~/.config/fizeau` and set `FIZEAU_*` env vars.
- If a user has old config present, `fiz` prints a one-line hint pointing to the new path and exits with a non-zero code.

## DDx Dependency

- The `.ddx/` directory and its configuration (bead queue, execution metadata, project config) reference the project name. All such references are updated to `fizeau`/`fiz`.
- DDx execution artifacts (`.ddx/executions/`) are **not** rewritten — they are historical evidence. New executions after the rename will use the new names.
- The `ddx init` command must **not** be re-run; the workspace is already initialized.

## Release/Updater Decision

- **Version bump**: The rename commit is paired with a major version bump (e.g. `v2.0.0` or the next major). This signals the breaking change to downstream consumers.
- **Release assets**: Tarballs and binaries are named `fizeau-<version>-<os>-<arch>.tar.gz` with the binary inside named `fiz`.
- **Self-updater**: If the project has a self-update mechanism, the update endpoint and asset naming are updated to the new names. No backward-compat download path is maintained.
- **Git tags**: A `rename/fizeau` tag is created at the rename commit for traceability. The next release tag follows normal semver.
