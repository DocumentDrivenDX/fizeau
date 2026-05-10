# agent-1ee144c7 — FZ-066 Tag final Fizeau release

## Summary

The final non-prerelease Fizeau rename release is `v0.9.28`, tagged at commit
`86b4e8f` ("fix: infer go install version from build info"). It supersedes
`v0.9.26` and `v0.9.27`, which were published during release verification
before the installer/update checks were fully clean. Subsequent feature
releases (`v0.10.0`, `v0.10.1`, `v0.10.2`, `v0.10.3`) are also published as
non-prerelease tags from the renamed module and ship `fiz-*` artifacts via
the same release workflow.

The actual `git tag` + `git push origin <tag>` for `v0.9.28` was performed
outside this DDx execute-bead worktree (per project policy, execute-bead
worktrees do not perform destructive remote ops); this bead documents the
evidence and verifies the AC against the published release.

## Acceptance evidence

| AC | Status | Evidence |
|----|--------|----------|
| 1. A final non-prerelease Fizeau tag exists | done | `git log -1 --format='%H' v0.9.28` → `86b4e8f54a12e3a13f5a9b9b3bab34c3ed388571`. Tag is annotated and published at https://github.com/easel/fizeau/releases/tag/v0.9.28. Not a pre-release (no `-rc`, `-pre`, `-fizeau.N` suffix). The parent epic `agent-2b694e0e` (FZ-000) closure note confirms: "Final Fizeau release v0.9.28 is tagged and published with fiz-\* release assets." DDx pinned to this tag in `agent-439fa2a6` (FZ-064). |
| 2. Release artifacts named `fiz*` | done | `gh release view v0.9.28 --json assets` lists four assets, all `fiz-<os>-<arch>`: `fiz-darwin-amd64` (20.5 MB), `fiz-darwin-arm64` (19.3 MB), `fiz-linux-amd64` (19.9 MB), `fiz-linux-arm64` (18.6 MB). No `ddx-agent-*` or `agent-*` artifacts on this release. The release workflow (`.github/workflows/release.yml`) hardcodes `BINARY_NAME: fiz` and builds `${BINARY_NAME}-${GOOS}-${GOARCH}` from `./cmd/fiz`, so any future tag on this repo also produces `fiz-*` artifacts. |
| 3. Install/update paths verified | done | **Install:** `install.sh` line 15 pins `REPO="easel/fizeau"`, line 16 defaults `BINARY_NAME="fiz"`, line 102 fetches `https://github.com/${REPO}/releases/download/${TAG}/${BINARY}`. Asset URL `https://github.com/easel/fizeau/releases/download/v0.9.28/fiz-linux-amd64` returns `HTTP/2 302` (redirects to S3 download). README `curl …/install.sh \| bash` and `go install github.com/easel/fizeau/cmd/fiz@latest` both point at the renamed repo. **Update:** `agentcli/update.go:21` pins `defaultGitHubRepo = "easel/fizeau"`; `agentcli/update.go:211` builds the asset name from `productinfo.BinaryName` (= `"fiz"`, `internal/productinfo/productinfo.go:6`). The `v0.9.28` changelog entry explicitly fixed `fiz update` and `go install` version reporting (CHANGELOG.md `[v0.9.28]` and `[v0.9.27]` sections), which is precisely why `v0.9.28` is the final rename release rather than `v0.9.26`. |
| 4. Release notes link the migration guide | done | `gh release view v0.9.28 --json body` body contains: `Migration guide: https://github.com/easel/fizeau/blob/v0.9.28/CHANGELOG.md#v0926--2026-05-01`, plus an inline summary of the rename surface (module path, package name, binary name, config paths, env vars, asset naming, updater behavior). The linked anchor resolves to the `[v0.9.26] — 2026-05-01` "Breaking Rename: Fizeau / fiz" section in `CHANGELOG.md`, which is the canonical migration checklist (Go module path, Go package name, binaries/commands, config paths, environment variables, compatibility statement, DDx migration status, updater behavior). |

## Verification commands run

- `git tag --list "v0.9.*" --sort=-creatordate` → confirms `v0.9.28` exists.
- `git log -1 --format='%H %s' v0.9.28` → `86b4e8f… fix: infer go install version from build info`.
- `gh release view v0.9.28 --json tagName,name,assets,body` → confirms tag, four `fiz-*` assets, and migration-guide link in body.
- `gh api repos/easel/fizeau/releases/latest --jq '.tag_name'` → `v0.10.3` (newer feature release; v0.9.28 remains the final rename release per FZ-000 closure notes and FZ-064 DDx pin).
- `curl -sI https://github.com/easel/fizeau/releases/download/v0.9.28/fiz-linux-amd64` → `HTTP/2 302` (asset URL resolves to download).
- `grep -n REPO\|BINARY_NAME install.sh` → confirms install.sh targets `easel/fizeau` and `fiz` binary.
- `grep -n defaultGitHubRepo\|BinaryName agentcli/update.go internal/productinfo/productinfo.go` → confirms updater targets `easel/fizeau` with `fiz-<os>-<arch>` asset naming.

## Notes

- The bead's wording "tag the final Fizeau release" is operator-flavored; the
  tag and release publication itself happens outside the worktree. This
  evidence file is the in-worktree deliverable that closes the AC against
  the already-published `v0.9.28` release.
- DDx is pinned to `v0.9.28` (FZ-064 closure note: "Pinned DDx cli/go.mod to
  github.com/easel/fizeau v0.9.28 in DocumentDrivenDX/ddx commit
  dc660eca, merged with origin/main as 51374ae7").
- FZ-066's child role under FZ-000 epic is informational: FZ-000 itself was
  closed on 2026-05-01 with the same v0.9.28 referent. This execution finalizes
  the FZ-066 evidence trail so the bead can also close.
