package serviceimpl

import (
	"context"

	"github.com/easel/fizeau/internal/harnesses"
)

// SessionSubscriber is the narrow root-facade seam TailSessionLog needs from
// the in-memory session hub.
type SessionSubscriber func(sessionID string) (<-chan harnesses.Event, error)

// TailSessionLog proxies session-hub events to a caller-owned channel that
// closes on upstream completion or context cancellation.
func TailSessionLog(ctx context.Context, sessionID string, subscribe SessionSubscriber) (<-chan harnesses.Event, error) {
	ch, err := subscribe(sessionID)
	if err != nil {
		return nil, err
	}

	proxy := make(chan harnesses.Event, 32)
	go func() {
		defer close(proxy)
		for {
			select {
			case ev, ok := <-ch:
				if !ok {
					return
				}
				select {
				case proxy <- ev:
				case <-ctx.Done():
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()
	return proxy, nil
}
