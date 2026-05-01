# FZ-037 External Package Manager Audit

Scope: audit whether this repo owns Homebrew, asdf, scoop, snap, AUR, codesign,
notarization, or similar package/release surfaces. Evidence collected from
worktree at base revision `6f3eaf7660a4ffa1143e7775e9410a1cd6932424`.

## Ecosystem Status Table

| Ecosystem / Surface | Status | Evidence | Follow-up beads |
|---|---|---|---|
| **GitHub Releases** | **present** | `.github/workflows/release.yml` — tag-triggered, builds `fiz` binaries for linux/darwin × amd64/arm64, uploads via `gh release create` | none — binary name already `fiz`, repo already `DocumentDrivenDX/fizeau` |
| **install.sh (bash installer)** | **present** | `install.sh` — downloads latest GitHub Release binary for detected platform | none — `REPO="DocumentDrivenDX/fizeau"` and `BINARY_NAME="${BINARY_NAME:-fiz}"` already use Fizeau names; functional enhancements are in-scope for FZ-033 (`agent-fee0000d`) |
| Homebrew formula / cask | absent | no `Formula/`, `Casks/`, `*.rb` tap files found | — |
| asdf plugin | absent | no `.tool-versions`, no plugin definition | — |
| Scoop manifest | absent | no `bucket/`, no `*.json` manifest | — |
| Snap | absent | no `snapcraft.yaml`, no `snap/` directory | — |
| AUR (PKGBUILD) | absent | no `PKGBUILD` file | — |
| Flatpak | absent | no `*.metainfo.xml`, no Flatpak manifest | — |
| Chocolatey | absent | no `*.nuspec`, no choco packaging | — |
| Debian / RPM packaging | absent | no `debian/` directory, no `.spec` file, no `.deb`/`.rpm` artifacts produced | — |
| macOS codesign / notarization | absent | no entitlements file, no `.p12`/`.pfx` reference, no notarization step in `release.yml`; CGO is disabled so binaries are self-contained and unsigned | — |
| npm / PyPI / other language registries | absent | `package.json` is a local devDependency file (`@redwoodjs/agent-ci` only), not a publish target; no `.npmrc`, no `pyproject.toml`, no registry config | — |
| Docker / container registry | absent | no `Dockerfile`, no `docker-compose.yml` for publishing; Docker usage is limited to local benchmark scripts | — |
| Windows Store / MSIX | N/A | no Windows build target in release matrix | — |

## Notes on present surfaces

### GitHub Releases (`.github/workflows/release.yml`)

Triggered on `v*` tags. Builds four binaries:

```
fiz-linux-amd64
fiz-linux-arm64
fiz-darwin-amd64
fiz-darwin-arm64
```

Version metadata injected via `-ldflags` (`main.Version`, `main.BuildTime`,
`main.GitCommit`). Release notes generated automatically by `gh release create
--generate-notes`. No checksums or SBOMs are currently produced. No codesign
or notarization step.

**Rename status**: fully migrated — binary name `fiz` and repo
`DocumentDrivenDX/fizeau` are already in place; no old-name strings remain.

### install.sh

Curl-pipe installer that detects platform (linux/darwin, amd64/arm64), fetches
the latest GitHub Release tag via the API, downloads the matching binary, and
configures PATH in the user's shell rc file. Usage:

```sh
curl -fsSL https://raw.githubusercontent.com/DocumentDrivenDX/fizeau/master/install.sh | bash
```

**Rename status**: fully migrated — `REPO` and `BINARY_NAME` already use Fizeau
values; no old-name strings remain. Functional improvements (colored output, PATH
config, `--version` verification) are tracked separately in FZ-033
(`agent-fee0000d`).

## Acceptance traceback

Bead `agent-20068588` AC: *"Audit doc lists each ecosystem with status
present/absent/N/A and follow-up bead IDs for any present external surface."*

- Every named ecosystem in the bead description (Homebrew, asdf, scoop, snap,
  AUR, codesign, notarization) appears in the table with an explicit status.
- Additional surfaces (Flatpak, Chocolatey, Debian/RPM, npm, Docker, Windows
  Store) are included for completeness.
- The two present surfaces (GitHub Releases, install.sh) record their current
  rename status and cross-reference existing beads where relevant.
- No follow-up rename beads are required: both present surfaces already use
  correct Fizeau naming.
