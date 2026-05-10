# agent-744cd55e — fizeau v0.10.0 release evidence

## AC mapping

| AC | Status | Evidence |
|----|--------|----------|
| 1. Release tag pushed | done | Annotated tag `v0.10.0` created on commit `404684a` (origin/master HEAD covering 8c0cca7 types/echo/tests + 404684a spec finalization) and pushed to `origin`. Release workflow run 25259853114 completed `success`; release published at https://github.com/easel/fizeau/releases/tag/v0.10.0 with darwin/linux × amd64/arm64 binaries. |
| 2. Release notes mention new fields + reserved-key extensions + Power return field | done | Release body edited via `gh release edit` to include CONTRACT-003 highlights: `Role` / `CorrelationID` on `ServiceExecuteRequest` and `RouteRequest` (with normalization + typed errors); `Power` on `ServiceRoutingActual` and `RouteDecision`; reserved cross-tool metadata keys extended with `role` and `correlation_id` plus `metadata_key_collision` warning behavior; echo paths into `routing_decision` / `final` / session-log header but not `text_delta`. CHANGELOG.md `[v0.10.0] — 2026-05-02` covers the same surface in-repo. |
| 3. DDx repo notified | done | DDx bead `ddx-8f1ac866` ("fizeau: import agent-3bb96bf5 release once tagged") reopened and annotated via `ddx bead update --status open --notes` pointing at the `v0.10.0` tag and release URL, with a per-field summary so the DDx work-loop can resume the cli/go.mod bump from v0.9.29 → v0.10.0. The DDx tracker change is committed locally on DDx `main` (commit `bcdd4c83`), not pushed — the user controls publication. |

## Notes

- The tag points at `404684a` (spec finalization) which is origin/master HEAD for the fizeau repo. Local checkpoint commits inside this DDx execute-bead worktree (e.g. `c2dd61a`) are intentionally NOT in the tagged history — they are worktree bookkeeping, not contract surface.
- `go build ./...` and `go test ./...` were green prior to tag creation; release workflow build also succeeded.
- Per project policy, no `go.mod replace` is used for the DDx import; the DDx-side bump consumes the published `v0.10.0` tag directly. That bump itself is the scope of `ddx-8f1ac866`, not this bead.
