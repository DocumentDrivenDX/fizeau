# BEAD-HARNESS-IF-00 — Hidden per-harness import grep + inventory reconciliation

Bead: `fizeau-0a9f8d41` (manifest BEAD-HARNESS-IF-00)
Plan: `docs/helix/02-design/plan-2026-05-14-harness-interface-refactor.md`

## Grep result (verbatim, for the PR description)

Command (run from repo root):

```
grep -rEn 'internal/harnesses/(claude|codex|gemini|opencode|pi)' --include='*.go' .
```

Excluded paths (per AC #2): the harness packages themselves
(`./internal/harnesses/{claude,codex,gemini,opencode,pi}/`),
`node_modules/`, `.claude/worktrees/`, `benchmark-results/`, `.ddx/`.

Verbatim filtered output:

```
./harness_golden_integration_test.go:18:	claudeharness "github.com/easel/fizeau/internal/harnesses/claude"
./harness_golden_integration_test.go:19:	codexharness "github.com/easel/fizeau/internal/harnesses/codex"
./internal/discoverycache/atomic_write.go:10:// internal/harnesses/claude/quota_cache.go:WriteClaudeQuota.
./internal/harnesses/runner_info_parity_test.go:8:	claudeharness "github.com/easel/fizeau/internal/harnesses/claude"
./internal/harnesses/runner_info_parity_test.go:9:	codexharness "github.com/easel/fizeau/internal/harnesses/codex"
./internal/harnesses/runner_info_parity_test.go:10:	geminiharness "github.com/easel/fizeau/internal/harnesses/gemini"
./internal/harnesses/runner_info_parity_test.go:11:	opencodeharness "github.com/easel/fizeau/internal/harnesses/opencode"
./internal/harnesses/runner_info_parity_test.go:12:	piharness "github.com/easel/fizeau/internal/harnesses/pi"
./internal/runtimesignals/collect.go:11:	claudecache "github.com/easel/fizeau/internal/harnesses/claude"
./internal/runtimesignals/collect.go:12:	codexcache "github.com/easel/fizeau/internal/harnesses/codex"
./internal/runtimesignals/collect.go:13:	geminicache "github.com/easel/fizeau/internal/harnesses/gemini"
./internal/runtimesignals/collect.go:93://   - "claude"       → existing internal/harnesses/claude/quota_cache.go
./internal/runtimesignals/collect.go:94://   - "codex"        → existing internal/harnesses/codex/quota_cache.go
./internal/runtimesignals/collect.go:95://   - "gemini"       → existing internal/harnesses/gemini/quota_cache.go
./internal/runtimesignals/collect_test.go:15:	claudecache "github.com/easel/fizeau/internal/harnesses/claude"
./internal/serviceimpl/execute_dispatch.go:9:	claudeharness "github.com/easel/fizeau/internal/harnesses/claude"
./internal/serviceimpl/execute_dispatch.go:10:	codexharness "github.com/easel/fizeau/internal/harnesses/codex"
./internal/serviceimpl/execute_dispatch.go:11:	geminiharness "github.com/easel/fizeau/internal/harnesses/gemini"
./internal/serviceimpl/execute_dispatch.go:12:	opencodeharness "github.com/easel/fizeau/internal/harnesses/opencode"
./internal/serviceimpl/execute_dispatch.go:13:	piharness "github.com/easel/fizeau/internal/harnesses/pi"
./service.go:12:	claudeharness "github.com/easel/fizeau/internal/harnesses/claude"
./service.go:13:	codexharness "github.com/easel/fizeau/internal/harnesses/codex"
./service.go:14:	geminiharness "github.com/easel/fizeau/internal/harnesses/gemini"
./service_execute_dispatch_test.go:45:		case `"github.com/easel/fizeau/internal/harnesses/claude"`,
./service_execute_dispatch_test.go:46:			`"github.com/easel/fizeau/internal/harnesses/codex"`,
./service_execute_dispatch_test.go:47:			`"github.com/easel/fizeau/internal/harnesses/gemini"`,
./service_execute_dispatch_test.go:48:			`"github.com/easel/fizeau/internal/harnesses/opencode"`,
./service_execute_dispatch_test.go:49:			`"github.com/easel/fizeau/internal/harnesses/pi"`:
./service_models.go:19:	claudeharness "github.com/easel/fizeau/internal/harnesses/claude"
./service_models.go:20:	codexharness "github.com/easel/fizeau/internal/harnesses/codex"
./service_models.go:21:	geminiharness "github.com/easel/fizeau/internal/harnesses/gemini"
./service_providers.go:22:	claudeharness "github.com/easel/fizeau/internal/harnesses/claude"
./service_providers.go:23:	codexharness "github.com/easel/fizeau/internal/harnesses/codex"
./service_providers_test.go:18:	claudeharness "github.com/easel/fizeau/internal/harnesses/claude"
./service_providers_test.go:19:	codexharness "github.com/easel/fizeau/internal/harnesses/codex"
./service_route_attempts_test.go:12:	codexharness "github.com/easel/fizeau/internal/harnesses/codex"
./service_route_attempts_test.go:13:	geminiharness "github.com/easel/fizeau/internal/harnesses/gemini"
./service_routing_errors_test.go:14:	claudeharness "github.com/easel/fizeau/internal/harnesses/claude"
./service_routing_test.go:18:	claudeharness "github.com/easel/fizeau/internal/harnesses/claude"
./service_status_test.go:13:	claudeharness "github.com/easel/fizeau/internal/harnesses/claude"
./service_status_test.go:14:	codexharness "github.com/easel/fizeau/internal/harnesses/codex"
./service_subscription_quota.go:7:	claudeharness "github.com/easel/fizeau/internal/harnesses/claude"
./service_subscription_quota.go:8:	codexharness "github.com/easel/fizeau/internal/harnesses/codex"
./service_subscription_quota.go:9:	geminiharness "github.com/easel/fizeau/internal/harnesses/gemini"
./service_subscription_quota_test.go:7:	claudeharness "github.com/easel/fizeau/internal/harnesses/claude"
```

## Reconciliation against the plan's inventory tables

Plan tables consulted:

- "Service-side consumers" — `docs/helix/02-design/plan-2026-05-14-harness-interface-refactor.md` lines 99–106.
- "Tests that consume per-harness packages" — same file, lines 110–122.

| File | In plan inventory | Notes |
|------|-------------------|-------|
| `service.go` | Yes (Service-side) | claudeharness/codexharness/geminiharness imports. |
| `service_providers.go` | Yes (Service-side) | claudeharness/codexharness imports. |
| `service_models.go` | Yes (Service-side) | claudeharness/codexharness/geminiharness imports. |
| `service_subscription_quota.go` | Yes (Service-side) | claudeharness/codexharness/geminiharness imports. |
| `internal/serviceimpl/execute_dispatch.go` | Yes (Service-side; **retained** per plan) | dispatcher seam, kept by CONTRACT-004. |
| `internal/runtimesignals/collect.go` | Yes (Service-side) | claudecache/codexcache/geminicache imports + 3 doc-comment refs. |
| `service_status_test.go` | Yes (Tests) | |
| `service_route_attempts_test.go` | Yes (Tests) | |
| `harness_golden_integration_test.go` | Yes (Tests) | |
| `service_subscription_quota_test.go` | Yes (Tests) | |
| `service_routing_errors_test.go` | Yes (Tests) | |
| `service_providers_test.go` | Yes (Tests) | |
| `service_routing_test.go` | Yes (Tests) | |
| `service_execute_dispatch_test.go` | Yes (Tests; "no change") | matches the five package strings literally. |
| `internal/harnesses/runner_info_parity_test.go` | Yes (Tests) | |
| `internal/runtimesignals/collect_test.go` | Yes (Tests) | |
| `internal/discoverycache/atomic_write.go` | **No** | Single doc-comment reference (`// internal/harnesses/claude/quota_cache.go:WriteClaudeQuota.`). No import — comment only. Documents the symmetric writer pattern. After Step 5e renames `WriteClaudeQuota` to a package-private symbol, the comment will need to be re-pointed; recorded here as a documentation touch-up follow-up, not a Service-side consumer. |

### Decision

Every file with an actual import of a per-harness package is already
covered by the plan's Service-side consumers or Tests inventory tables.
The single unlisted reference, in `internal/discoverycache/atomic_write.go:10`,
is a doc comment, not a build dependency, and is recorded above as a
documentation follow-up to thread through Step 12 (or whichever step
unexports `WriteClaudeQuota`). No new file needs to be added to the
plan's inventory tables; the migration scope is unchanged.
