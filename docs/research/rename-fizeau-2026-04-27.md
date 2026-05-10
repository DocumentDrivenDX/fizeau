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

## Rename Mechanism Decision

Use a committed, deterministic script for the module/import-path rewrite. Do
not use `gopls`/`gorename` for the module path because the change is a literal
repository import rewrite, not a single Go symbol rename. Do not leave the
mechanism to local model judgment during FZ-010.

FZ-010 must create and commit the script at
`scripts/rename-fizeau-module.sh` before applying the rewrite. The required
dry-run command is:

```bash
bash scripts/rename-fizeau-module.sh --dry-run
```

The script owns only the FZ-010 surface:

- `go.mod` module path from `github.com/DocumentDrivenDX/agent` to
  `github.com/easel/fizeau`
- Go import paths and active non-historical references that name the old module
  path
- `gofmt`, `go mod tidy`, `go build ./...`, and `go test ./...` verification
  hooks or printed follow-up commands

Root package declarations, exported Go symbol decisions, and `cmd/` directory
moves remain split into their dedicated FZ beads. Those beads may use `git mv`
or focused manual edits where their acceptance criteria say so, but local
models execute the bead-prescribed mechanics rather than choosing a new policy.

## Release/Updater Decision

**Release repository**: The canonical release repository after the rename is
`easel/fizeau`. `DocumentDrivenDX/agent` is treated only as the old
location that GitHub may redirect from; release tooling, installer snippets, and
self-update code must not rely on that redirect as a supported contract.

**Exact choreography**:

1. Land the rename change on `master` while still in the old checkout. The
   change updates module/import paths, binary names, config/env names, docs,
   installer constants, release asset names, and updater constants to
   `fizeau`/`fiz`.
2. Rename the GitHub repository from `DocumentDrivenDX/agent` to
   `easel/fizeau` before publishing any renamed release tag. This
   ensures the tagged Go module path and release URLs are canonical at the
   moment downstream consumers fetch them.
3. Create a traceability tag named `rename/fizeau` at the rename commit.
4. Create the first pre-release tag and GitHub pre-release as `v1.0.0-rc.1`.
   The current binary version is `v0.0.8`, so `v1.0.0` is the next major
   without requiring a Go module `/v2` path.
5. Publish only new-name assets for the pre-release:
   `fizeau-v1.0.0-rc.1-<os>-<arch>.tar.gz` containing a `fiz` binary, plus any
   checksums under the `fizeau` name.
6. Bump DDx downstream to consume
   `github.com/easel/fizeau@v1.0.0-rc.1` and invoke `fiz`. Run the
   DDx integration checks there before cutting a final release.
7. If DDx exposes a rename defect, fix it in this repository and repeat with
   `v1.0.0-rc.2`, `v1.0.0-rc.3`, and so on. DDx must be bumped to the newest
   accepted RC before final.
8. Create the final `v1.0.0` tag and GitHub release from the exact commit
   validated by the accepted RC. Publish the same new-name asset pattern as the
   RC with the final version.
9. Update DDx from the accepted RC to `github.com/easel/fizeau@v1.0.0`
   after the final release is live.

**Bridge artifacts**: **No.** Do not publish `ddx-agent-*` binaries, tarballs,
checksums, aliases, or a final `DocumentDrivenDX/agent` release that points to
the renamed project. Do not add an old `ddx-agent update` bridge that downloads
`fiz` or rewrites user config. Existing `ddx-agent` installs stop at their last
old-name release and users move by installing `fiz` from the renamed repository.

**Rationale**: This keeps the rename consistent with the no compatibility
window policy above: there is one supported repo, one supported binary name, one
config/env namespace, and one update endpoint after the break. The RC gives DDx
a real published artifact to validate before the final release without creating
a parallel old-name distribution channel that would need its own support and
failure handling.
