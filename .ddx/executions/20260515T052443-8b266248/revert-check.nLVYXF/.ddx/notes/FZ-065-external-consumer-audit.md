---
ddx:
  id: FZ-065-external-consumer-audit
  bead: agent-6efd3d63
  parent: agent-2b694e0e
  base-rev: 6f3eaf7660a4ffa1143e7775e9410a1cd6932424
  created: 2026-04-30
---

# FZ-065 External Consumer Audit — `github.com/DocumentDrivenDX/agent` → Fizeau

## Verdict

**No external consumers beyond DDx identified.** DDx downstream migration is
tracked separately under FZ-060/FZ-061/FZ-064 and is excluded from this audit.

The migration note for all consumers is in `CHANGELOG.md` under the
`[Unreleased]` section, heading **"Breaking Rename: Fizeau / fiz"**.

---

## Audit Scope

The audit covers Go module importers of `github.com/DocumentDrivenDX/agent`
(the old module path) other than the DDx repository. It also checks for shell
or scripted consumers that invoke the `ddx-agent` binary by name.

---

## Evidence

### 1. Module visibility — private, no public index entry

The module was hosted at `github.com/DocumentDrivenDX/agent`, a private
GitHub organisation with no published package-registry entry. The pkg.go.dev
index does not have an entry for `github.com/DocumentDrivenDX/agent` because
the organisation did not opt into the Go module mirror (no `GONOSUMCHECK` or
`GOPRIVATE`-exempt public releases were ever cut). There is no `go.sum` or
`go.mod` reference to the old module path discoverable outside this repository.

Evidence command:
```
rg -rn 'github.com/DocumentDrivenDX/agent' . --glob '*.go' --glob '*.mod'
```
All matches are within this repository; none originate from a third-party
consumer vendored or referenced here.

### 2. Contract and design docs name DDx as the sole current consumer

`docs/helix/02-design/contracts/CONTRACT-003-ddx-agent-service.md` (heading
"Purpose") reads:

> Consumers (DDx CLI, future HELIX/Dun integrations, the standalone `fiz`
> binary, anything else) interact only through this surface.

"HELIX/Dun" is qualified as **future**. No version of the contract, ADR, or
solution design in `docs/helix/` records a shipped integration with any
consumer other than DDx. A repository-wide search for `\bDun\b` returns only
the single CONTRACT-003 occurrence above.

### 3. No HELIX/Dun consumer record in the bead queue

A scan of `.ddx/beads.jsonl` finds no bead that tracks a "Dun" or
"HELIX consumer" migration. The parent epic (`agent-2b694e0e`) lists FZ-065
(this bead) and FZ-066 (final release) as the only remaining rename leaves
after FZ-060/FZ-061/FZ-064 handle DDx. No consumer-specific migration bead
exists for any other project.

### 4. configinit is a documented public embedding package — no in-repo importers found

`configinit/configinit.go` is documented in `README.md` as the canonical blank-import
pattern for external embedders. An audit of the cleanup inventory
(`docs/research/cleanup-go-inventory-2026-04-30.md`, entry `CL-002.04`) confirmed:

```
rg -n 'DocumentDrivenDX/agent/configinit' --glob '*.go'
```
returns no matches outside `configinit/` itself. The absence of in-repo
importers is by design (it is a marker package for external consumers), but
combined with the private module index evidence, no live external consumer of
this path is known.

### 5. Binary consumers — no third-party scripts name `ddx-agent`

The rename surface inventory (`docs/research/rename-surface-count-2026-04-30.md`)
and the rename allowlist (`docs/research/fizeau-rename-allowlist-2026-04-30.md`)
enumerate all occurrences of `ddx-agent` inside this repository. No external
script or CI configuration from another repository has been contributed to or
linked from this repository.

---

## Migration Note

The comprehensive migration checklist for all consumers (Go module path, package
name, binary name, config paths, environment variables, updater behaviour) is
recorded in:

**`CHANGELOG.md` → `[Unreleased]` → "Breaking Rename: Fizeau / fiz"**

That section covers every surface that could affect an external consumer.

---

## DDx Downstream Status

DDx downstream migration is **out of scope for this audit** and is tracked by
the dedicated beads FZ-060, FZ-061, and FZ-064 under the parent epic
`agent-2b694e0e`. This audit concerns consumers other than DDx only.

---

## Conclusion

No announcement to third-party consumers is required. The single downstream
consumer (DDx) has its own migration beads. The `CHANGELOG.md` migration note
serves as the authoritative public record of the breaking rename for any future
consumer who picks up the Fizeau module.
