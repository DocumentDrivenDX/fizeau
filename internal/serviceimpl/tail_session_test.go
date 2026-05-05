package serviceimpl

import (
	"context"
	"errors"
	"testing"

	"github.com/DocumentDrivenDX/fizeau/internal/harnesses"
)

func TestTailSessionLogProxiesEvents(t *testing.T) {
	upstream := make(chan harnesses.Event, 1)
	upstream <- harnesses.Event{Type: harnesses.EventTypeFinal, Sequence: 1}
	close(upstream)

	out, err := TailSessionLog(context.Background(), "svc-1", func(sessionID string) (<-chan harnesses.Event, error) {
		if sessionID != "svc-1" {
			t.Fatalf("sessionID = %q, want svc-1", sessionID)
		}
		return upstream, nil
	})
	if err != nil {
		t.Fatalf("TailSessionLog: %v", err)
	}
	ev, ok := <-out
	if !ok {
		t.Fatal("output channel closed before event")
	}
	if ev.Type != harnesses.EventTypeFinal || ev.Sequence != 1 {
		t.Fatalf("event = %+v", ev)
	}
	if _, ok := <-out; ok {
		t.Fatal("output channel did not close after upstream close")
	}
}

func TestTailSessionLogReturnsSubscribeError(t *testing.T) {
	want := errors.New("unknown session")
	_, err := TailSessionLog(context.Background(), "missing", func(string) (<-chan harnesses.Event, error) {
		return nil, want
	})
	if !errors.Is(err, want) {
		t.Fatalf("err = %v, want %v", err, want)
	}
}

func TestTailSessionLogClosesOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	upstream := make(chan harnesses.Event)
	out, err := TailSessionLog(ctx, "svc-1", func(string) (<-chan harnesses.Event, error) {
		return upstream, nil
	})
	if err != nil {
		t.Fatalf("TailSessionLog: %v", err)
	}
	cancel()
	if _, ok := <-out; ok {
		t.Fatal("output channel remained open after context cancel")
	}
}
