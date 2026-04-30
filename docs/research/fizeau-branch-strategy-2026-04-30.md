---
ddx:
  id: fizeau-branch-strategy-2026-04-30
  bead: agent-ec6e3bf3
  parent: agent-2b694e0e
  base-rev: cd0b14984c175f4a84e3ca3afaa7d9e800445667
  created: 2026-04-30
---

# FZ-008 Fizeau rename branch and PR strategy

## Decision

Land the Fizeau rename as **sequential main commits** following the DDx bead
dependency graph. Do not use a long-lived integration branch and do not use a
stacked PR chain for the rename leaves.

Rationale:

- The rename has many independent leaves, but the tracker already records the
  safe order through explicit bead dependencies.
- DDx execute-bead branches carry audit commits and must not be squashed,
  rebased, filtered, or amended after they become part of the execution trail.
- A long-lived integration branch would hide breakage until the end and make
  conflicting mechanical rewrites harder to attribute.
- Stacked PRs would force frequent restacking across large string, path, and
  import rewrites, which conflicts with the no-rebase audit policy.
- Sequential main commits keep every failure attributable to one bead or one
  dependency cluster and give maintainers concrete rollback checkpoints.

If repository policy requires GitHub pull requests, use one PR per bead or per
tightly coupled dependency cluster, always targeting `main`. Merge only after
that PR's predecessor beads have merged and its local/CI checks are green. Use
normal merge or fast-forward merge only; do not squash or rebase these PRs.

## Merge Order

Use this order for rename work:

1. **Decision and guardrail setup.** Merge FZ-004, FZ-005, FZ-006, FZ-008,
   FZ-002, and FZ-003 before runtime rename leaves. FZ-003 installs the
   rename-noise checker in report-only mode, so old names are visible without
   blocking the intentionally mixed transition state.
2. **Core runtime identity.** Merge the module/import rewrite, root package
   rename, CLI directory rename, product identity constants, and config-path
   helper chain in dependency order: FZ-010, FZ-011, FZ-012*, FZ-013,
   FZ-020, FZ-021*, FZ-022, and FZ-030.
3. **Runtime external surfaces.** Merge env-var, fixtures, embedded asset,
   user-facing error/log, Makefile, release workflow, installer, updater,
   benchmark command, workflow metadata, repo metadata, telemetry, completion,
   model-catalog docs, and bridge-artifact beads only after their explicit
   predecessors are on main.
4. **Public documentation and downstream coordination.** Merge README,
   website, install docs, routing docs, demo, active-governance, migration note,
   external-consumer, and DDx downstream beads after the runtime surfaces they
   describe have landed. DDx import work must wait for the Fizeau pre-release
   choreography owned by the release/updater beads.
5. **Final stale-name enforcement.** Merge FZ-070 only after all rename leaves
   it depends on have landed and the report-only checker has no unallowlisted
   findings in the active scope. FZ-071 follows FZ-070 as the final cleanup
   pass for remaining allowlisted or historical stale names; it must not require
   weakening the active CI gate.

## FZ-070 CI Gate Staging

FZ-070 is not an early safety gate. It is the final enforcement gate.

- Before runtime leaves: FZ-003 runs the checker in report-only mode. It may
  report old module paths, `package agent`, `ddx-agent`, `DDX Agent`,
  `.agent`, `AGENT_*`, and `DDX_AGENT_*` while the rename is intentionally
  incomplete.
- During sequential rename commits: local tests and existing CI protect each
  bead. The rename-noise checker remains non-blocking so main can pass through
  valid intermediate states.
- At FZ-070: promote the checker to failing CI only when all FZ-070 dependency
  leaves have merged and the allowlist represents only historical evidence or
  intentionally retained external-standard names.
- After FZ-070: any new unallowlisted old-name surface is a CI failure. Fix it
  with a narrow follow-up commit. FZ-071 may clean remaining allowlisted
  historical noise, but it should not be needed to make FZ-070 pass.

## Rollback Checkpoints

Use these checkpoints instead of trying to partially unwind the whole rename:

1. **Pre-rename snapshot.** FZ-006 creates the checkpoint before any runtime
   rewrite. If the rename is abandoned before public release, revert sequential
   rename commits back to this snapshot in reverse merge order.
2. **Report-only checker checkpoint.** After FZ-003, checker defects are
   rollback-local: revert or fix the report-only checker without touching
   runtime rename commits.
3. **Core runtime checkpoint.** After the module, package, CLI, productinfo,
   and config helper chain is green, treat it as an atomic runtime checkpoint.
   If a later runtime surface breaks, revert the later bead or cluster first;
   revert the core checkpoint only if the module/package/CLI identity itself is
   invalid.
4. **External surface checkpoint.** After installer, updater, release assets,
   env vars, telemetry, and workflow metadata are green, rollback is by
   contiguous cluster. Revert installer/updater/release changes together if
   artifact discovery breaks; revert env/config changes together if runtime
   startup breaks.
5. **Pre-release and DDx checkpoint.** Once a Fizeau pre-release tag is used by
   DDx downstream work, do not move or rewrite that tag. Roll forward with a
   corrective pre-release and update DDx again, or revert the DDx bump while
   leaving the published tag as historical evidence.
6. **FZ-070 enforcement checkpoint.** If the promoted CI gate fails on main,
   first revert FZ-070 to report-only or fix the allowlist/checker rule. Do not
   revert completed rename leaves unless the checker exposes a real defect in a
   specific leaf; in that case revert or repair that leaf in dependency order.
