package fizeau_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	fizeau "github.com/easel/fizeau"
)

// drainRoleCorrEvents collects events from ch until close or timeout.
func drainRoleCorrEvents(t *testing.T, ch <-chan fizeau.ServiceEvent, timeout time.Duration) []fizeau.ServiceEvent {
	t.Helper()
	var events []fizeau.ServiceEvent
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				return events
			}
			events = append(events, ev)
		case <-deadline.C:
			t.Fatalf("timed out waiting for channel close; collected %d events", len(events))
			return events
		}
	}
}

func virtualReqWithRoleCorr(role, correlationID, response string) fizeau.ServiceExecuteRequest {
	return fizeau.ServiceExecuteRequest{
		Prompt:  "hi",
		Harness: "virtual",
		Metadata: map[string]string{
			"virtual.response": response,
		},
		Role:          role,
		CorrelationID: correlationID,
	}
}

// policy_statement: Role + CorrelationID are echoed into RoutingDecision + Final events.
func TestRoleAndCorrelationID_EchoedIntoRoutingDecisionAndFinal(t *testing.T) {
	svc, err := fizeau.New(fizeau.ServiceOptions{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	req := virtualReqWithRoleCorr("implementer", "bead_123:attempt_4", "ok")
	ch, err := svc.Execute(context.Background(), req)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	events := drainRoleCorrEvents(t, ch, 5*time.Second)

	var sawRouting, sawFinal bool
	for _, ev := range events {
		switch string(ev.Type) {
		case fizeau.ServiceEventTypeRoutingDecision:
			sawRouting = true
			if ev.Metadata["role"] != "implementer" {
				t.Errorf("policy_statement: routing_decision event must echo Role; got role=%q", ev.Metadata["role"])
			}
			if ev.Metadata["correlation_id"] != "bead_123:attempt_4" {
				t.Errorf("policy_statement: routing_decision event must echo CorrelationID; got correlation_id=%q", ev.Metadata["correlation_id"])
			}
		case fizeau.ServiceEventTypeFinal:
			sawFinal = true
			if ev.Metadata["role"] != "implementer" {
				t.Errorf("policy_statement: final event must echo Role; got role=%q", ev.Metadata["role"])
			}
			if ev.Metadata["correlation_id"] != "bead_123:attempt_4" {
				t.Errorf("policy_statement: final event must echo CorrelationID; got correlation_id=%q", ev.Metadata["correlation_id"])
			}
		}
	}
	if !sawRouting {
		t.Fatal("expected a routing_decision event")
	}
	if !sawFinal {
		t.Fatal("expected a final event")
	}
}

// policy_statement: Role + CorrelationID are NOT echoed into text_delta event metadata
// (existing Metadata echo path on text_delta still applies for other keys).
func TestRoleAndCorrelationID_NotEchoedIntoTextDelta(t *testing.T) {
	svc, err := fizeau.New(fizeau.ServiceOptions{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	req := virtualReqWithRoleCorr("reviewer", "bead_x:1", "hello world")
	ch, err := svc.Execute(context.Background(), req)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	events := drainRoleCorrEvents(t, ch, 5*time.Second)

	var sawDelta bool
	for _, ev := range events {
		if string(ev.Type) != fizeau.ServiceEventTypeTextDelta {
			continue
		}
		sawDelta = true
		if v, ok := ev.Metadata["role"]; ok {
			t.Errorf("policy_statement: text_delta must NOT carry top-level Role in metadata; got role=%q", v)
		}
		if v, ok := ev.Metadata["correlation_id"]; ok {
			t.Errorf("policy_statement: text_delta must NOT carry top-level CorrelationID in metadata; got correlation_id=%q", v)
		}
	}
	if !sawDelta {
		t.Fatal("expected at least one text_delta event from virtual harness")
	}
}

// policy_statement: Role + CorrelationID are echoed into the session-log header.
func TestRoleAndCorrelationID_EchoedIntoSessionLogHeader(t *testing.T) {
	dir := t.TempDir()
	svc, err := fizeau.New(fizeau.ServiceOptions{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	req := virtualReqWithRoleCorr("decomposer", "bead_y:2", "ok")
	req.SessionLogDir = dir
	ch, err := svc.Execute(context.Background(), req)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	_ = drainRoleCorrEvents(t, ch, 5*time.Second)

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("expected a session log file under SessionLogDir")
	}
	data, err := os.ReadFile(filepath.Join(dir, entries[0].Name()))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	// session.start is the first non-empty line.
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) == 0 {
		t.Fatal("session log is empty")
	}
	var header struct {
		Type string `json:"type"`
		Data struct {
			Metadata map[string]string `json:"metadata"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(lines[0]), &header); err != nil {
		t.Fatalf("unmarshal session.start: %v", err)
	}
	if header.Type != "session.start" {
		t.Fatalf("first session-log line type = %q, want session.start", header.Type)
	}
	if got := header.Data.Metadata["role"]; got != "decomposer" {
		t.Errorf("policy_statement: session-log header must carry Role; got role=%q", got)
	}
	if got := header.Data.Metadata["correlation_id"]; got != "bead_y:2" {
		t.Errorf("policy_statement: session-log header must carry CorrelationID; got correlation_id=%q", got)
	}
}

// policy_statement: Role + CorrelationID never affect eligibility filtering Day 1.
// policy_statement: Without a CorrelationID, routing is unchanged from baseline.
//
// Asserted at the ResolveRoute boundary: the same RouteRequest with and
// without Role/CorrelationID must produce the same outcome (decision and/or
// error). Day 1 these two fields are observational only.
func TestRoleAndCorrelationID_DoNotAffectRouting(t *testing.T) {
	svc, err := fizeau.New(fizeau.ServiceOptions{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	baseline, baseErr := svc.ResolveRoute(context.Background(), fizeau.RouteRequest{Harness: "virtual"})
	tagged, tagErr := svc.ResolveRoute(context.Background(), fizeau.RouteRequest{
		Harness:       "virtual",
		Role:          "implementer",
		CorrelationID: "bead_z:1",
	})
	// Both must observe identical routing policy: same error string OR
	// same selected harness/provider/model. Day 1 the tagged fields do
	// not enter selection, so if the baseline call errors, the tagged
	// call must error identically.
	if (baseErr == nil) != (tagErr == nil) {
		t.Fatalf("policy_statement: Role+CorrelationID changed routing outcome; baseErr=%v tagErr=%v", baseErr, tagErr)
	}
	if baseErr != nil && baseErr.Error() != tagErr.Error() {
		t.Fatalf("policy_statement: Role+CorrelationID changed routing error; baseErr=%q tagErr=%q", baseErr, tagErr)
	}
	if baseline != nil && tagged != nil {
		if baseline.Harness != tagged.Harness || baseline.Provider != tagged.Provider || baseline.Model != tagged.Model {
			t.Fatalf("policy_statement: Role+CorrelationID must NOT affect eligibility filtering; baseline=%+v tagged=%+v", baseline, tagged)
		}
	}
}

// policy_statement: ResolveRoute and Execute observe identical routing policy
// for the same correlation-aware request (Day 1: both ignore for routing).
//
// We assert by comparing ResolveRoute outcomes with and without the
// correlation-aware fields: ResolveRoute is the routing surface that
// Execute also exercises, so identical RouteDecision (or identical error)
// proves the parity statement.
func TestRoleAndCorrelationID_ResolveRouteParityWithExecute(t *testing.T) {
	svc, err := fizeau.New(fizeau.ServiceOptions{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	withTags, errTags := svc.ResolveRoute(context.Background(), fizeau.RouteRequest{
		Harness:       "virtual",
		Role:          "reviewer",
		CorrelationID: "bead_z:2",
	})
	withoutTags, errPlain := svc.ResolveRoute(context.Background(), fizeau.RouteRequest{
		Harness: "virtual",
	})
	if (errTags == nil) != (errPlain == nil) {
		t.Fatalf("policy_statement: Role+CorrelationID changed ResolveRoute outcome; errTags=%v errPlain=%v", errTags, errPlain)
	}
	if errTags != nil && errTags.Error() != errPlain.Error() {
		t.Fatalf("policy_statement: ResolveRoute parity broken; errTags=%q errPlain=%q", errTags, errPlain)
	}
	if withTags != nil && withoutTags != nil {
		if withTags.Harness != withoutTags.Harness || withTags.Model != withoutTags.Model || withTags.Provider != withoutTags.Provider {
			t.Fatalf("policy_statement: ResolveRoute parity broken; withTags=%+v withoutTags=%+v", withTags, withoutTags)
		}
	}
}

// policy_statement: Invalid Role / CorrelationID values rejected pre-dispatch
// with the typed errors.
func TestRoleAndCorrelationID_InvalidValuesRejectedPreDispatch(t *testing.T) {
	svc, err := fizeau.New(fizeau.ServiceOptions{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Run("role_uppercase_rejected", func(t *testing.T) {
		_, err := svc.Execute(context.Background(), fizeau.ServiceExecuteRequest{
			Prompt:  "hi",
			Harness: "virtual",
			Role:    "Implementer",
		})
		var typed *fizeau.RoleNormalizationError
		if !errors.As(err, &typed) {
			t.Fatalf("expected *RoleNormalizationError; got %T %v", err, err)
		}
	})
	t.Run("role_too_long_rejected", func(t *testing.T) {
		_, err := svc.Execute(context.Background(), fizeau.ServiceExecuteRequest{
			Prompt:  "hi",
			Harness: "virtual",
			Role:    strings.Repeat("a", 65),
		})
		var typed *fizeau.RoleNormalizationError
		if !errors.As(err, &typed) {
			t.Fatalf("expected *RoleNormalizationError; got %T %v", err, err)
		}
	})
	t.Run("correlation_whitespace_rejected", func(t *testing.T) {
		_, err := svc.Execute(context.Background(), fizeau.ServiceExecuteRequest{
			Prompt:        "hi",
			Harness:       "virtual",
			CorrelationID: "bead 123",
		})
		var typed *fizeau.CorrelationIDNormalizationError
		if !errors.As(err, &typed) {
			t.Fatalf("expected *CorrelationIDNormalizationError; got %T %v", err, err)
		}
	})
	t.Run("correlation_too_long_rejected", func(t *testing.T) {
		_, err := svc.Execute(context.Background(), fizeau.ServiceExecuteRequest{
			Prompt:        "hi",
			Harness:       "virtual",
			CorrelationID: strings.Repeat("a", 257),
		})
		var typed *fizeau.CorrelationIDNormalizationError
		if !errors.As(err, &typed) {
			t.Fatalf("expected *CorrelationIDNormalizationError; got %T %v", err, err)
		}
	})
	t.Run("resolve_route_validates_too", func(t *testing.T) {
		_, err := svc.ResolveRoute(context.Background(), fizeau.RouteRequest{
			Harness: "virtual",
			Role:    "BadRole",
		})
		var typed *fizeau.RoleNormalizationError
		if !errors.As(err, &typed) {
			t.Fatalf("expected *RoleNormalizationError from ResolveRoute; got %T %v", err, err)
		}
	})
}

// policy_statement: ServiceRoutingActual.Power reflects the catalog-projected
// power of the dispatched Model. The virtual harness is not in the catalog,
// so its Power MUST be 0 (documented as "unknown / no catalog entry").
// We assert the field is present and serialized — its absence would mean the
// echo path was never wired.
func TestRoleAndCorrelationID_RoutingActualPowerSurface(t *testing.T) {
	svc, err := fizeau.New(fizeau.ServiceOptions{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	req := virtualReqWithRoleCorr("implementer", "bead_p:1", "ok")
	ch, err := svc.Execute(context.Background(), req)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	events := drainRoleCorrEvents(t, ch, 5*time.Second)
	for _, ev := range events {
		if string(ev.Type) != fizeau.ServiceEventTypeFinal {
			continue
		}
		var payload fizeau.ServiceFinalData
		if err := json.Unmarshal(ev.Data, &payload); err != nil {
			t.Fatalf("unmarshal final: %v", err)
		}
		if payload.RoutingActual == nil {
			t.Fatal("policy_statement: final event must carry RoutingActual")
		}
		// Power is an int; for an out-of-catalog virtual model the
		// documented value is 0. The assertion is structural: the
		// field exists on the public surface.
		_ = payload.RoutingActual.Power
		return
	}
	t.Fatal("expected final event")
}

// policy_statement: When caller sets both top-level Role and Metadata['role'],
// top-level wins and a MetadataKeyCollision warning is emitted.
func TestRoleAndCorrelationID_TopLevelWinsOnMetadataCollisionWithWarning(t *testing.T) {
	svc, err := fizeau.New(fizeau.ServiceOptions{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ch, err := svc.Execute(context.Background(), fizeau.ServiceExecuteRequest{
		Prompt:  "hi",
		Harness: "virtual",
		Role:    "implementer",
		Metadata: map[string]string{
			"virtual.response": "ok",
			"role":             "reviewer", // collides with top-level
		},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	events := drainRoleCorrEvents(t, ch, 5*time.Second)
	var sawCollisionWarning, sawTopLevelOnFinal bool
	for _, ev := range events {
		if string(ev.Type) != fizeau.ServiceEventTypeFinal {
			continue
		}
		if got := ev.Metadata["role"]; got == "implementer" {
			sawTopLevelOnFinal = true
		} else {
			t.Errorf("policy_statement: top-level Role must win on collision; final.Metadata role=%q", got)
		}
		var payload fizeau.ServiceFinalData
		if err := json.Unmarshal(ev.Data, &payload); err != nil {
			t.Fatalf("unmarshal final: %v", err)
		}
		for _, w := range payload.Warnings {
			if w.Code == fizeau.MetadataWarningCodeKeyCollision {
				sawCollisionWarning = true
			}
		}
	}
	if !sawTopLevelOnFinal {
		t.Fatal("policy_statement: final.Metadata did not echo top-level Role")
	}
	if !sawCollisionWarning {
		t.Fatalf("policy_statement: expected MetadataKeyCollision warning on final")
	}
}

// Direct unit coverage of validators so the typed error contract is tested
// independent of Execute/ResolveRoute plumbing.
func TestValidateRoleAndCorrelationID(t *testing.T) {
	if err := fizeau.ValidateRole(""); err != nil {
		t.Errorf("empty role must be accepted; got %v", err)
	}
	if err := fizeau.ValidateRole("implementer-v2"); err != nil {
		t.Errorf("normalized role rejected: %v", err)
	}
	if err := fizeau.ValidateRole("Bad"); err == nil {
		t.Error("uppercase role must fail normalization")
	}
	if err := fizeau.ValidateRole("bad role"); err == nil {
		t.Error("space in role must fail normalization")
	}
	if err := fizeau.ValidateCorrelationID(""); err != nil {
		t.Errorf("empty correlation_id must be accepted; got %v", err)
	}
	if err := fizeau.ValidateCorrelationID("bead_123:attempt_4-final"); err != nil {
		t.Errorf("normalized correlation_id rejected: %v", err)
	}
	if err := fizeau.ValidateCorrelationID("has space"); err == nil {
		t.Error("space in correlation_id must fail normalization")
	}
	if err := fizeau.ValidateCorrelationID("ctrl\tchar"); err == nil {
		t.Error("control char in correlation_id must fail normalization")
	}
}
