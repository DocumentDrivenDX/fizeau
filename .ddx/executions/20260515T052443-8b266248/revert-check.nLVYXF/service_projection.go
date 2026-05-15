package fizeau

import (
	"fmt"

	"github.com/easel/fizeau/internal/harnesses"
)

// projectQuotaStatus projects a CONTRACT-004 harnesses.QuotaStatus onto the
// public CONTRACT-003 QuotaState surface per the projection table in
// docs/helix/02-design/contracts/CONTRACT-004-harness-implementation.md.
//
// The helper covers the success path only: it converts a successfully
// returned QuotaStatus value. Call failure (a non-nil error from the
// harness method) is the caller's responsibility — they own LastError
// population because only they know which probe failed. State-driven
// absence such as State=QuotaUnavailable is reported via Status, not
// LastError, exactly as the projection table specifies.
//
// QuotaStatus.RoutingPreference is intentionally not projected: it is an
// internal routing signal that never reaches the public surface (see
// CONTRACT-004 §"Projection to CONTRACT-003" and invariant #4).
//
// No service code calls this helper yet; the per-harness migrations in
// BEAD-HARNESS-IF-05B+ will wire it in.
func projectQuotaStatus(status harnesses.QuotaStatus) *QuotaState {
	state := &QuotaState{
		Windows:    status.Windows,
		CapturedAt: status.CapturedAt,
		Fresh:      status.Fresh,
		Source:     status.Source,
		Status:     string(status.State),
	}
	if status.Reason != "" {
		if state.Status == "" {
			state.Status = status.Reason
		} else {
			state.Status = fmt.Sprintf("%s (%s)", state.Status, status.Reason)
		}
	}
	return state
}

// projectAccountSnapshot projects a CONTRACT-004 harnesses.AccountSnapshot
// onto the public CONTRACT-003 AccountStatus surface. Every public field
// maps 1:1 from the snapshot, including the unknown-state convention
// (Authenticated=false && Unauthenticated=false) and the free-form Detail
// string. No service code calls this helper yet.
func projectAccountSnapshot(snapshot harnesses.AccountSnapshot) *AccountStatus {
	return &AccountStatus{
		Authenticated:   snapshot.Authenticated,
		Unauthenticated: snapshot.Unauthenticated,
		Email:           snapshot.Email,
		PlanType:        snapshot.PlanType,
		OrgName:         snapshot.OrgName,
		Source:          snapshot.Source,
		CapturedAt:      snapshot.CapturedAt,
		Fresh:           snapshot.Fresh,
		Detail:          snapshot.Detail,
	}
}
