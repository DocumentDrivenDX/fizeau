package fizeau

import (
	"context"

	"github.com/easel/fizeau/internal/serviceimpl"
)

func newSessionHub() *serviceimpl.SessionHub {
	return serviceimpl.NewSessionHub()
}

// TailSessionLog streams events from an in-progress or completed session by
// ID. Multiple concurrent callers on the same sessionID each receive the full
// remaining event stream. Callers attached after completion receive the stored
// final event then see the channel close. Returns an error for unknown IDs.
func (s *service) TailSessionLog(ctx context.Context, sessionID string) (<-chan ServiceEvent, error) {
	return serviceimpl.TailSessionLog(ctx, sessionID, s.hub.Subscribe)
}
