package serviceimpl

import (
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	agentcore "github.com/easel/fizeau/internal/core"
	"github.com/easel/fizeau/internal/session"
)

// SessionLogOptions carries the API-neutral pieces needed to open one
// historical session log.
type SessionLogOptions struct {
	Dir                 string
	SessionID           string
	Start               session.SessionStartData
	RoutingDecision     any
	RoutingDecisionType agentcore.EventType
}

// SessionLog owns the durable session log writer and progress timing state for
// one Execute call.
type SessionLog struct {
	logger    *session.Logger
	path      string
	endOnce   sync.Once
	endWrote  atomic.Bool
	closeOnce sync.Once

	progressMu     sync.Mutex
	lastProgressAt time.Time
}

// OpenSessionLog opens a historical session log and emits its session.start
// record. Empty dir/session IDs yield a no-op log so callers can stay linear.
func OpenSessionLog(opts SessionLogOptions) *SessionLog {
	if opts.Dir == "" || opts.SessionID == "" {
		return &SessionLog{}
	}
	logger := session.NewLogger(opts.Dir, opts.SessionID)
	sl := &SessionLog{
		logger: logger,
		path:   filepath.Join(opts.Dir, opts.SessionID+".jsonl"),
	}
	logger.Emit(agentcore.EventSessionStart, opts.Start)
	if opts.RoutingDecision != nil {
		eventType := opts.RoutingDecisionType
		if eventType == "" {
			eventType = agentcore.EventType("routing_decision")
		}
		logger.Emit(eventType, opts.RoutingDecision)
	}
	return sl
}

// Enabled reports whether this log has a backing writer.
func (sl *SessionLog) Enabled() bool {
	return sl != nil && sl.logger != nil
}

// Path returns the backing JSONL path, or empty when logging is disabled.
func (sl *SessionLog) Path() string {
	if sl == nil {
		return ""
	}
	return sl.path
}

// WriteEnd records the terminal session.end event. The first call wins.
func (sl *SessionLog) WriteEnd(end session.SessionEndData) {
	if !sl.Enabled() {
		return
	}
	sl.endOnce.Do(func() {
		sl.endWrote.Store(true)
		sl.logger.Emit(agentcore.EventSessionEnd, end)
	})
}

// WriteEvent appends one raw agent/core event, excluding records owned by the
// higher-level session lifecycle helpers.
func (sl *SessionLog) WriteEvent(ev agentcore.Event) {
	if !sl.Enabled() {
		return
	}
	switch ev.Type {
	case agentcore.EventSessionStart, agentcore.EventSessionEnd,
		agentcore.EventOverride, agentcore.EventRejectedOverride:
		return
	}
	sl.logger.Write(ev)
}

// WriteOverrideEvent appends an override-style event to the session log.
func (sl *SessionLog) WriteOverrideEvent(eventType agentcore.EventType, payload any) {
	if !sl.Enabled() {
		return
	}
	sl.logger.Emit(eventType, payload)
}

// Close flushes the underlying log file. Safe to call multiple times.
func (sl *SessionLog) Close() {
	if !sl.Enabled() {
		return
	}
	sl.closeOnce.Do(func() {
		_ = sl.logger.Close()
	})
}

// EndWritten reports whether WriteEnd has already recorded session.end.
func (sl *SessionLog) EndWritten() bool {
	if sl == nil {
		return false
	}
	return sl.endWrote.Load()
}

// ProgressIntervalMS reports elapsed milliseconds since the previous progress
// update and records the current timestamp.
func (sl *SessionLog) ProgressIntervalMS(now time.Time) int64 {
	if sl == nil || now.IsZero() {
		return 0
	}
	sl.progressMu.Lock()
	defer sl.progressMu.Unlock()
	if sl.lastProgressAt.IsZero() {
		sl.lastProgressAt = now
		return 0
	}
	elapsed := now.Sub(sl.lastProgressAt).Milliseconds()
	sl.lastProgressAt = now
	if elapsed <= 0 {
		return 0
	}
	return elapsed
}
