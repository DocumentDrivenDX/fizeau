# agent-3bb96bf5 — CONTRACT-003 Role + CorrelationID + Power

## Acceptance evidence

| AC | Status | Evidence |
|----|--------|----------|
| 1. CONTRACT-003 amended | done | `docs/helix/02-design/contracts/CONTRACT-003-fizeau-service.md` — ExecuteRequest/RouteRequest amendments at the type definitions; precedence-section amendment after auto-selection inputs paragraph; ServiceRoutingActual `Power int` added; nine new bullets in the **Routing Policy Test Contract** section |
| 2. Go types updated | done | `service.go` ServiceExecuteRequest (Role/CorrelationID), RouteRequest (Role/CorrelationID), RouteDecision (Power); `service_events.go` ServiceRoutingActual.Power; `internal/harnesses/types.go` RoutingActual.Power |
| 3. Normalization validators + typed errors | done | `role_correlation.go` — `ValidateRole`, `ValidateCorrelationID`, `*RoleNormalizationError`, `*CorrelationIDNormalizationError`; called from `service_execute.go` Execute and `service_routing.go` ResolveRoute |
| 4. Echo paths | done | `service_execute.go` runExecute routing_decision emit (`metaWithRoleAndCorrelation`); `finalizeAndEmit` echoes onto final event Metadata + emits MetadataKeyCollision warning + stamps RoutingActual.Power; `service_session_log.go` openSessionLog echoes Role/CorrelationID into the session.start header |
| 5. Routing Policy Test Contract tests | done | `service_role_correlation_test.go` — one test per minimum statement (echo to routing_decision/final, NOT to text_delta, session-log header, no routing impact, ResolveRoute parity, typed-error rejection, RoutingActual.Power surface, top-level wins on collision + warning) plus direct unit coverage of validators |
| 6. Tagged release published (v0.10.0) | post-merge | CHANGELOG entry under `[v0.10.0] — 2026-05-02` documents the additive surface change. The actual `git tag v0.10.0` + push happens in the merge / release pipeline, not inside this execution worktree (DDx execute-bead worktrees are not allowed to perform destructive remote ops). |
| 7. DDx repo can import the tagged release | post-merge | Once v0.10.0 is published, downstream DDx pulls in the new `ServiceExecuteRequest.Role / CorrelationID` and `ServiceRoutingActual.Power` fields. The contract change is strictly additive and back-compat: the new fields default to their zero values, existing callers compile unchanged. Verifiable via `go get github.com/easel/fizeau@v0.10.0` after publish. |

## Notes

- Test gating: existing pre-existing `-tags testseam` build failures (`TokenUsage redeclared`, `unknown field FakeProvider`) reproduce on the unmodified `HEAD` commit — confirmed via `git stash` reversal — and are out of scope for this bead.
- `Power` projection uses `catalogPowerForModel(serviceRoutingCatalog(), model)`. For models not in the catalog (including the test `virtual` harness) this returns 0, matching the documented "unknown / exact-pin-only / no catalog entry" semantics.
- `MetadataKeyCollision` is surfaced as `ServiceFinalWarning.Code = "metadata_key_collision"` on the final event rather than introducing a new event type; this keeps the contract churn minimal while satisfying the AC ("warning event emitted").
