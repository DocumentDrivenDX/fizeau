//go:build testseam

package fizeau_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	fizeau "github.com/DocumentDrivenDX/fizeau"
)

// extractSessionID reads the session_id field from the routing_decision event.
func extractSessionID(t *testing.T, events []fizeau.ServiceEvent) string {
	t.Helper()
	for _, ev := range events {
		if ev.Type != "routing_decision" {
			continue
		}
		var payload struct {
			SessionID string `json:"session_id"`
		}
		if err := json.Unmarshal(ev.Data, &payload); err != nil {
			t.Fatalf("unmarshal routing_decision: %v", err)
		}
		return payload.SessionID
	}
	t.Fatal("no routing_decision event found; cannot extract session_id")
	return ""
}

// newFakeSvc is a convenience helper that constructs a service with a
// FakeProvider that returns a single static response.
func newFakeSvc(t *testing.T, responses []fizeau.FakeResponse) fizeau.FizeauService {
	t.Helper()
	opts := fizeau.ServiceOptions{}
	opts.FakeProvider = &fizeau.FakeProvider{Static: responses}
	svc, err := fizeau.New(opts)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return svc
}

// stdReq returns a standard single-turn Execute request that uses the
// FakeProvider path (native harness).
func stdReq(prompt string) fizeau.ServiceExecuteRequest {
	return fizeau.ServiceExecuteRequest{
		Prompt:   prompt,
		Harness:  "fiz",
		Provider: "fake",
		Model:    "fake-model",
	}
}

// TestTailSessionLog_streamsActiveSession starts an Execute backed by a
// slow FakeProvider, subscribes to TailSessionLog mid-flight, and asserts
// that events (including the final) arrive on the tail channel.
func TestTailSessionLog_streamsActiveSession(t *testing.T) {
	// Dynamic provider that sleeps briefly so we can call TailSessionLog
	// before the session ends.
	svc := fizeau.ServiceOptions{}
	svc.FakeProvider = &fizeau.FakeProvider{
		Dynamic: func(req fizeau.FakeRequest) (fizeau.FakeResponse, error) {
			time.Sleep(150 * time.Millisecond)
			return fizeau.FakeResponse{Text: "done"}, nil
		},
	}
	s, err := fizeau.New(svc)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	execCh, err := s.Execute(context.Background(), stdReq("hello"))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	// Collect the first event (routing_decision) to get the session ID.
	// Give it a moment to arrive.
	var firstEv fizeau.ServiceEvent
	select {
	case ev := <-execCh:
		firstEv = ev
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for routing_decision")
	}
	if firstEv.Type != "routing_decision" {
		t.Fatalf("first event type: want routing_decision, got %q", firstEv.Type)
	}
	var rdPayload struct {
		SessionID string `json:"session_id"`
	}
	if err := json.Unmarshal(firstEv.Data, &rdPayload); err != nil {
		t.Fatalf("unmarshal routing_decision: %v", err)
	}
	if rdPayload.SessionID == "" {
		t.Fatal("routing_decision missing session_id")
	}
	sessionID := rdPayload.SessionID

	// Subscribe mid-flight.
	tailCh, err := s.TailSessionLog(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("TailSessionLog: %v", err)
	}

	// Drain the tail channel; we expect at least the final event.
	tailEvents := drainEvents(t, tailCh, 5*time.Second)
	if len(tailEvents) == 0 {
		t.Fatal("TailSessionLog: expected at least one event")
	}
	finalEv := findFinal(tailEvents)
	if finalEv == nil {
		t.Fatal("TailSessionLog: expected final event")
	}
	if got := finalStatus(t, finalEv); got != "success" {
		t.Errorf("TailSessionLog final status: want success, got %q (err=%q)", got, finalError(t, finalEv))
	}

	// Also drain the original Execute channel to clean up.
	_ = drainEvents(t, execCh, 5*time.Second)
}

// TestTailSessionLog_replaysCompletedSession finishes an Execute first, then
// calls TailSessionLog. The returned channel must yield the final event and
// then close.
func TestTailSessionLog_replaysCompletedSession(t *testing.T) {
	svc := newFakeSvc(t, []fizeau.FakeResponse{{Text: "hello"}})

	execCh, err := svc.Execute(context.Background(), stdReq("go"))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	// Drain all events from Execute so the session completes.
	execEvents := drainEvents(t, execCh, 5*time.Second)
	sessionID := extractSessionID(t, execEvents)
	if sessionID == "" {
		t.Fatal("no session_id in routing_decision")
	}

	// Verify the session finished with success.
	if got := finalStatus(t, findFinal(execEvents)); got != "success" {
		t.Errorf("Execute: want success, got %q", got)
	}

	// Now subscribe AFTER the session is complete.
	tailCh, err := svc.TailSessionLog(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("TailSessionLog (completed session): %v", err)
	}
	tailEvents := drainEvents(t, tailCh, 3*time.Second)
	if len(tailEvents) == 0 {
		t.Fatal("TailSessionLog replay: expected at least one event (the final)")
	}
	finalEv := findFinal(tailEvents)
	if finalEv == nil {
		t.Fatal("TailSessionLog replay: expected final event")
	}
	if got := finalStatus(t, finalEv); got != "success" {
		t.Errorf("TailSessionLog replay final status: want success, got %q", got)
	}
}

// TestTailSessionLog_multipleSubscribers subscribes two concurrent
// TailSessionLog callers to the same sessionID and asserts both receive the
// final event.
func TestTailSessionLog_multipleSubscribers(t *testing.T) {
	svcOpts := fizeau.ServiceOptions{}
	svcOpts.FakeProvider = &fizeau.FakeProvider{
		Dynamic: func(req fizeau.FakeRequest) (fizeau.FakeResponse, error) {
			time.Sleep(100 * time.Millisecond)
			return fizeau.FakeResponse{Text: "multi"}, nil
		},
	}
	s, err := fizeau.New(svcOpts)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	execCh, err := s.Execute(context.Background(), stdReq("multi"))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	// Read the routing_decision to get session_id.
	var rdEv fizeau.ServiceEvent
	select {
	case ev := <-execCh:
		rdEv = ev
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for routing_decision")
	}
	var rdPayload struct {
		SessionID string `json:"session_id"`
	}
	_ = json.Unmarshal(rdEv.Data, &rdPayload)
	sessionID := rdPayload.SessionID
	if sessionID == "" {
		t.Fatal("no session_id")
	}

	// Subscribe two tail channels concurrently.
	tail1, err := s.TailSessionLog(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("TailSessionLog sub1: %v", err)
	}
	tail2, err := s.TailSessionLog(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("TailSessionLog sub2: %v", err)
	}

	var wg sync.WaitGroup
	type result struct {
		events []fizeau.ServiceEvent
		label  string
	}
	results := make([]result, 2)

	for i, ch := range []<-chan fizeau.ServiceEvent{tail1, tail2} {
		wg.Add(1)
		go func(idx int, c <-chan fizeau.ServiceEvent, label string) {
			defer wg.Done()
			results[idx] = result{
				events: drainEvents(t, c, 5*time.Second),
				label:  label,
			}
		}(i, ch, []string{"sub1", "sub2"}[i])
	}

	// Drain the original exec channel.
	_ = drainEvents(t, execCh, 5*time.Second)
	wg.Wait()

	for _, r := range results {
		finalEv := findFinal(r.events)
		if finalEv == nil {
			t.Errorf("%s: no final event", r.label)
			continue
		}
		if got := finalStatus(t, finalEv); got != "success" {
			t.Errorf("%s: final status want success, got %q", r.label, got)
		}
	}
}

// TestTailSessionLog_unknownSessionReturnsError verifies that calling
// TailSessionLog with a sessionID that was never registered returns an error.
func TestTailSessionLog_unknownSessionReturnsError(t *testing.T) {
	svc, err := fizeau.New(fizeau.ServiceOptions{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	_, err = svc.TailSessionLog(context.Background(), "no-such-session-xyz")
	if err == nil {
		t.Fatal("expected error for unknown session, got nil")
	}
}
