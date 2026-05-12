package fizeau

import (
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	agentcore "github.com/easel/fizeau/internal/core"
	"github.com/easel/fizeau/internal/harnesses"
	"github.com/easel/fizeau/internal/session"
)

// serviceSessionLog is the service-owned writer that persists the session
// lifecycle records (session.start + session.end) for one Execute call. It
// is constructed in runExecute after the route is resolved and torn down in
// a defer on the same goroutine. Sub-paths (runNative, runSubprocess, ...)
// emit the terminal event through finalizeAndEmit, which delegates here to
// keep the session log in sync with the public event stream.
type serviceSessionLog struct {
	logger    *session.Logger
	path      string
	sessionID string
	decision  RouteDecision
	override  *overrideContext
	endOnce   sync.Once
	endWrote  atomic.Bool
	closeOnce sync.Once

	progressMu     sync.Mutex
	lastProgressAt time.Time
}

// openSessionLog creates the session-log writer for req and emits the
// session.start record. A nil-or-empty req.SessionLogDir yields a no-op
// writer so callers don't need to branch on whether logging is enabled.
func (s *service) openSessionLog(req ServiceExecuteRequest, decision RouteDecision, sessionID string) *serviceSessionLog {
	if req.SessionLogDir == "" || sessionID == "" {
		return &serviceSessionLog{}
	}
	logger := session.NewLogger(req.SessionLogDir, sessionID)
	sl := &serviceSessionLog{
		logger:    logger,
		path:      filepath.Join(req.SessionLogDir, sessionID+".jsonl"),
		sessionID: sessionID,
		decision:  decision,
	}
	// CONTRACT-003: echo top-level Role + CorrelationID into the
	// session-log header (one line per session). Top-level wins over
	// any caller Metadata entry under the reserved keys.
	headerMeta := metaWithRoleAndCorrelation(req.Metadata, req.Role, req.CorrelationID)
	start := session.SessionStartData{
		Provider:               s.providerTypeLabel(decision.Provider),
		Model:                  decision.Model,
		SelectedProvider:       decision.Provider,
		SelectedEndpoint:       decision.Endpoint,
		SelectedServerInstance: decision.ServerInstance,
		SelectedRoute:          req.SelectedRoute,
		Sticky: session.RoutingStickyState{
			KeyPresent:     req.CorrelationID != "",
			Assignment:     decision.Sticky.Assignment,
			ServerInstance: decision.Sticky.ServerInstance,
			Reason:         decision.Sticky.Reason,
			Bonus:          decision.Sticky.Bonus,
		},
		Utilization: session.RoutingUtilizationState{
			Source:         decision.Utilization.Source,
			Freshness:      decision.Utilization.Freshness,
			ActiveRequests: decision.Utilization.ActiveRequests,
			QueuedRequests: decision.Utilization.QueuedRequests,
			MaxConcurrency: decision.Utilization.MaxConcurrency,
			CachePressure:  decision.Utilization.CachePressure,
			ObservedAt:     decision.Utilization.ObservedAt,
		},
		RequestedHarness: req.Harness,
		ResolvedHarness:  decision.Harness,
		HarnessSource:    harnessSource(req),
		RequestedModel:   req.Model,
		ResolvedModel:    decision.Model,
		Reasoning:        req.Reasoning,
		WorkDir:          req.WorkDir,
		MaxIterations:    req.MaxIterations,
		Prompt:           req.Prompt,
		SystemPrompt:     req.SystemPrompt,
		Metadata:         headerMeta,
	}
	logger.Emit(agentcore.EventSessionStart, start)
	return sl
}

// writeEnd records the terminal session.end event. It is idempotent: the
// first call wins. Callers should invoke this whenever a harnesses.FinalData
// is produced — finalizeAndEmit threads it together with the public-stream
// emit so the two views stay consistent.
func (sl *serviceSessionLog) writeEnd(req ServiceExecuteRequest, meta map[string]string, final harnesses.FinalData) {
	if sl == nil || sl.logger == nil {
		return
	}
	sl.endOnce.Do(func() {
		sl.endWrote.Store(true)
		end := session.SessionEndData{
			Status:           harnessStatusToCoreStatus(final.Status),
			ProcessOutcome:   processOutcomeForFinal(final.Status),
			Output:           final.FinalText,
			Tokens:           finalUsageToCoreTokens(final.Usage),
			DurationMs:       final.DurationMS,
			SelectedRoute:    req.SelectedRoute,
			SelectedEndpoint: sl.decision.Endpoint,
			Sticky: session.RoutingStickyState{
				KeyPresent:     req.CorrelationID != "",
				Assignment:     sl.decision.Sticky.Assignment,
				ServerInstance: sl.decision.Sticky.ServerInstance,
				Reason:         sl.decision.Sticky.Reason,
				Bonus:          sl.decision.Sticky.Bonus,
			},
			Utilization: session.RoutingUtilizationState{
				Source:         sl.decision.Utilization.Source,
				Freshness:      sl.decision.Utilization.Freshness,
				ActiveRequests: sl.decision.Utilization.ActiveRequests,
				QueuedRequests: sl.decision.Utilization.QueuedRequests,
				MaxConcurrency: sl.decision.Utilization.MaxConcurrency,
				CachePressure:  sl.decision.Utilization.CachePressure,
				ObservedAt:     sl.decision.Utilization.ObservedAt,
			},
			RequestedHarness: req.Harness,
			ResolvedHarness:  "",
			HarnessSource:    harnessSource(req),
			RequestedModel:   req.Model,
			Reasoning:        req.Reasoning,
			Metadata:         meta,
			Error:            final.Error,
		}
		if final.CostUSD > 0 {
			cost := final.CostUSD
			end.CostUSD = &cost
		}
		if req.CostCapUSD > 0 {
			cap := req.CostCapUSD
			end.CostCapUSD = &cap
		}
		if final.RoutingActual != nil {
			end.ResolvedHarness = final.RoutingActual.Harness
			end.Model = final.RoutingActual.Model
			end.SelectedProvider = final.RoutingActual.Provider
			if final.RoutingActual.ServerInstance != "" {
				end.SelectedServerInstance = final.RoutingActual.ServerInstance
			}
			end.ResolvedModel = final.RoutingActual.Model
			end.AttemptedProviders = append([]string(nil), final.RoutingActual.FallbackChainFired...)
			if len(end.AttemptedProviders) > 1 {
				end.FailoverCount = len(end.AttemptedProviders) - 1
			}
		}
		if final.Reasoning != nil {
			end.ResolvedReasoning = agentcore.Reasoning(final.Reasoning.ResolvedReasoning)
			end.ReasoningSource = final.Reasoning.Source
		}
		if end.SelectedServerInstance == "" {
			end.SelectedServerInstance = sl.decision.ServerInstance
		}
		sl.logger.Emit(agentcore.EventSessionEnd, end)
	})
}

// writeEvent persists a raw agent event to the session log. Used by the
// native loop callback to capture llm.request / llm.response / tool.call /
// compaction.* events so kept-sandbox bundles preserve a complete trace
// for benchmark reruns and post-mortem debugging. session.start and
// session.end are skipped because writeStart/writeEnd own those records and
// enrich them with service-side routing fields the loop does not see.
func (sl *serviceSessionLog) writeEvent(ev agentcore.Event) {
	if sl == nil || sl.logger == nil {
		return
	}
	switch ev.Type {
	case agentcore.EventSessionStart, agentcore.EventSessionEnd,
		agentcore.EventOverride, agentcore.EventRejectedOverride:
		return
	}
	sl.logger.Write(ev)
}

// writeOverrideEvent persists an override or rejected_override payload to
// the session log so windowed reporting (UsageReport, ADR-006 §5) can
// recompute routing-quality across restarts and beyond the in-memory
// ring's bounded retention. eventType is one of ServiceEventTypeOverride
// / ServiceEventTypeRejectedOverride.
func (sl *serviceSessionLog) writeOverrideEvent(eventType string, payload ServiceOverrideData) {
	if sl == nil || sl.logger == nil {
		return
	}
	payload.SessionID = sl.sessionID
	sl.logger.Emit(agentcore.EventType(eventType), payload)
}

// close flushes the underlying file. Safe to call multiple times.
func (sl *serviceSessionLog) close() {
	if sl == nil || sl.logger == nil {
		return
	}
	sl.closeOnce.Do(func() {
		_ = sl.logger.Close()
	})
}

// endWritten reports whether writeEnd has already recorded a terminal event.
func (sl *serviceSessionLog) endWritten() bool {
	if sl == nil {
		return false
	}
	return sl.endWrote.Load()
}

// persistRejectedOverride writes a session.start + rejected_override pair
// to a freshly-opened session log for sessionID. Used by Execute's
// pre-dispatch rejection branch, which never reaches runExecute and so
// never goes through the normal openSessionLog path. Without this,
// rejected_override events are invisible to UsageReport (which scans
// session logs over a --since window).
//
// The session.start is required by ScanRoutingQuality, which only counts
// log files that include one — and we want this rejection counted in
// TotalRequests so AutoAcceptanceRate reflects reality.
func (s *service) persistRejectedOverride(req ServiceExecuteRequest, sessionID string, payload ServiceOverrideData) {
	if req.SessionLogDir == "" || sessionID == "" {
		return
	}
	sl := s.openSessionLog(req, RouteDecision{}, sessionID)
	if sl == nil || sl.logger == nil {
		return
	}
	defer sl.close()
	sl.writeOverrideEvent(ServiceEventTypeRejectedOverride, payload)
}

// providerTypeLabel maps a configured provider name ("local") to its provider
// type ("lmstudio") when available. Returns the input unchanged if no
// ServiceConfig is attached or the name is not configured. Callers use this
// to populate session-log Provider fields, which historically carry the
// provider *type* rather than the configured name.
func (s *service) providerTypeLabel(name string) string {
	if s == nil || s.opts.ServiceConfig == nil || name == "" {
		return name
	}
	entry, ok := s.opts.ServiceConfig.Provider(name)
	if !ok || entry.Type == "" {
		return name
	}
	return entry.Type
}

// harnessStatusToCoreStatus maps a public harnesses.FinalData.Status string
// to an internal agentcore.Status. Unknown / error-y statuses collapse to
// StatusError so session.end always carries a well-defined status.
func harnessStatusToCoreStatus(status string) agentcore.Status {
	switch status {
	case "success":
		return agentcore.StatusSuccess
	case "iteration_limit":
		return agentcore.StatusIterationLimit
	case "cancelled":
		return agentcore.StatusCancelled
	case string(agentcore.StatusBudgetHalted):
		return agentcore.StatusBudgetHalted
	default:
		return agentcore.StatusError
	}
}

// processOutcomeForFinal returns the FEAT-005 §27 process_outcome label for
// a session.end record. Today only "budget_halted" is mapped explicitly;
// other statuses leave process_outcome empty so existing semantics ("status"
// alone) are preserved.
func processOutcomeForFinal(status string) string {
	if status == string(agentcore.StatusBudgetHalted) {
		return "budget_halted"
	}
	return ""
}

// finalUsageToCoreTokens converts the public FinalUsage pointer form into
// the internal TokenUsage struct used by session.end. Nil usage yields a
// zero-value struct.
func finalUsageToCoreTokens(usage *harnesses.FinalUsage) agentcore.TokenUsage {
	if usage == nil {
		return agentcore.TokenUsage{}
	}
	return agentcore.TokenUsage{
		Input:      derefHarnessInt(usage.InputTokens),
		Output:     derefHarnessInt(usage.OutputTokens),
		CacheRead:  derefHarnessInt(usage.CacheReadTokens),
		CacheWrite: derefHarnessInt(usage.CacheWriteTokens),
		Total:      derefHarnessInt(usage.TotalTokens),
	}
}

func derefHarnessInt(v *int) int {
	if v == nil {
		return 0
	}
	return *v
}
