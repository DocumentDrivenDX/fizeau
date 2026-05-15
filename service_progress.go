package fizeau

import (
	"encoding/json"
	"sync/atomic"
	"time"

	agentcore "github.com/easel/fizeau/internal/core"
	"github.com/easel/fizeau/internal/harnesses"
	"github.com/easel/fizeau/internal/transcript"
)

const (
	progressLineLimit            = transcript.DefaultLineLimit
	progressExceptionalLineLimit = transcript.ExceptionalToolLineLimit
)

type toolTaskSummary = transcript.ToolSummary
type toolOutputDetail = transcript.OutputSummary

type nativeLLMRequestPayload = transcript.NativeLLMRequestPayload
type nativeLLMResponsePayload = transcript.NativeLLMResponsePayload
type nativeToolCallPayload = transcript.NativeToolCallPayload
type nativeCompactionPayload = transcript.NativeCompactionPayload

type nativeProgressState struct {
	inner *transcript.NativeProgressState
}

type subprocessProgressState struct {
	inner *transcript.SubprocessProgressState
}

func emitProgress(out chan<- ServiceEvent, seq *atomic.Int64, sl *serviceSessionLog, sessionID string, meta map[string]string, payload ServiceProgressData) {
	now := time.Now().UTC()
	if sl != nil {
		payload.SinceLastMS = sl.progressIntervalMS(now)
	}
	fillProgressIdentity(&payload, sessionID, meta)
	raw, err := json.Marshal(payload)
	if err != nil {
		return
	}
	eventSeq := seq.Add(1) - 1
	ev := harnesses.Event{
		Type:     harnesses.EventTypeProgress,
		Sequence: eventSeq,
		Time:     now,
		Metadata: meta,
		Data:     raw,
	}
	select {
	case out <- ev:
	case <-time.After(time.Second):
	}
	if sl != nil {
		sl.writeEvent(agentcore.Event{
			SessionID: sessionID,
			Seq:       int(eventSeq),
			Type:      agentcore.EventType(ServiceEventTypeProgress),
			Timestamp: ev.Time,
			Data:      raw,
		})
	}
}

func fillProgressIdentity(payload *ServiceProgressData, sessionID string, meta map[string]string) {
	if payload == nil {
		return
	}
	converted := toTranscriptProgress(*payload)
	transcript.FillProgressIdentity(&converted, sessionID, meta, progressMessageLimit(*payload))
	*payload = fromTranscriptProgress(converted)
}

func progressTaskID(sessionID string, meta map[string]string) string {
	return transcript.TaskID(sessionID, meta)
}

func progressStatusLine(payload ServiceProgressData) string {
	return transcript.StatusLine(transcript.StatusLineInput{
		TaskID:      payload.TaskID,
		TurnIndex:   payload.Round,
		Message:     payload.Message,
		SinceLastMS: payload.SinceLastMS,
		Limit:       progressMessageLimit(payload),
	})
}

func compactProgressIdentity(taskID string, round int) string {
	return transcript.CompactIdentity(taskID, round)
}

func compactProgressTaskID(taskID string) string {
	return transcript.CompactTaskID(taskID)
}

func progressMessageLimit(payload ServiceProgressData) int {
	if payload.Phase == "tool" && payload.Command != "" {
		return progressExceptionalLineLimit
	}
	return progressLineLimit
}

func newNativeProgressState() *nativeProgressState {
	return &nativeProgressState{inner: transcript.NewNativeProgressState()}
}

func newSubprocessProgressState(req ServiceExecuteRequest) *subprocessProgressState {
	return &subprocessProgressState{inner: transcript.NewSubprocessProgressState(req.Prompt, req.SystemPrompt)}
}

func (p *nativeProgressState) noteRequest(payload nativeLLMRequestPayload) ServiceProgressData {
	return fromTranscriptProgress(p.inner.NoteRequest(payload))
}

func (p *nativeProgressState) noteResponse(payload nativeLLMResponsePayload) ServiceProgressData {
	return fromTranscriptProgress(p.inner.NoteResponse(payload))
}

func (p *nativeProgressState) noteToolCall(callID string, payload nativeToolCallPayload) (ServiceProgressData, ServiceProgressData) {
	start, complete := p.inner.NoteToolCall(callID, payload)
	return fromTranscriptProgress(start), fromTranscriptProgress(complete)
}

func (p *nativeProgressState) noteCompaction(payload nativeCompactionPayload) (ServiceProgressData, ServiceProgressData) {
	compaction, contextUpdate := p.inner.NoteCompaction(payload)
	return fromTranscriptProgress(compaction), fromTranscriptProgress(contextUpdate)
}

func (p *nativeProgressState) noteFinal(final harnesses.FinalData) *ServiceProgressData {
	return fromTranscriptProgressPtr(p.inner.NoteFinal(final))
}

func (p *subprocessProgressState) noteRequestStart() ServiceProgressData {
	return fromTranscriptProgress(p.inner.NoteRequestStart())
}

func (p *subprocessProgressState) noteEvent(ev harnesses.Event) (ServiceProgressData, bool) {
	payload, ok := p.inner.NoteEvent(ev)
	return fromTranscriptProgress(payload), ok
}

func (p *subprocessProgressState) annotateToolResultDuration(ev harnesses.Event) harnesses.Event {
	return p.inner.AnnotateToolResultDuration(ev)
}

func (p *subprocessProgressState) noteFinal(ev harnesses.Event) (ServiceProgressData, bool) {
	payload, ok := p.inner.NoteFinalEvent(ev)
	return fromTranscriptProgress(payload), ok
}

func (p *subprocessProgressState) noteResponseComplete(final harnesses.FinalData) *ServiceProgressData {
	return fromTranscriptProgressPtr(p.inner.NoteResponseComplete(final))
}

func routeProgressData(decision RouteDecision) ServiceProgressData {
	return fromTranscriptProgress(transcript.RouteProgressData(toTranscriptRouteDecision(decision)))
}

func summarizeToolInput(toolName string, input json.RawMessage) string {
	return transcript.SummarizeToolInput(toolName, input)
}

func summarizeToolTask(toolName string, input json.RawMessage) toolTaskSummary {
	return transcript.SummarizeToolCall(toolName, input)
}

func summarizeShellTask(command string) toolTaskSummary {
	return transcript.SummarizeShellCommand(command)
}

func normalizeShellCommand(command string) string {
	return transcript.NormalizeShellCommand(command)
}

func summarizeToolOutput(output string) string {
	return transcript.SummarizeOutput(output).Summary
}

func summarizeToolOutputDetail(output string) toolOutputDetail {
	return transcript.SummarizeOutput(output)
}

func formatByteCount(n int) string {
	return transcript.FormatByteCount(n)
}

func summarizeJSONValue(raw json.RawMessage) string {
	return transcript.SummarizeJSONValue(raw)
}

func isSensitiveSummaryKey(key string) bool {
	return transcript.IsSensitiveSummaryKey(key)
}

func progressTokenThroughput(outputTokens int, durationMS int64) *float64 {
	return transcript.TokenThroughput(outputTokens, durationMS)
}

func boundedProgressText(s string, maxRunes int) string {
	return transcript.BoundedText(s, maxRunes)
}

func shortProgressText(s string) string {
	return boundedProgressText(s, progressLineLimit)
}

func toTranscriptProgress(payload ServiceProgressData) transcript.ProgressPayload {
	return transcript.ProgressPayload{
		Phase:                 payload.Phase,
		State:                 payload.State,
		Source:                payload.Source,
		TaskID:                payload.TaskID,
		Round:                 payload.Round,
		Message:               payload.Message,
		TurnIndex:             payload.TurnIndex,
		ToolName:              payload.ToolName,
		ToolCallID:            payload.ToolCallID,
		ToolCallIndex:         payload.ToolCallIndex,
		Command:               payload.Command,
		Action:                payload.Action,
		Target:                payload.Target,
		OutputSummary:         payload.OutputSummary,
		OutputBytes:           payload.OutputBytes,
		OutputLines:           payload.OutputLines,
		OutputExcerpt:         payload.OutputExcerpt,
		DurationMS:            payload.DurationMS,
		SinceLastMS:           payload.SinceLastMS,
		TokPerSec:             payload.TokPerSec,
		InputTokens:           payload.InputTokens,
		OutputTokens:          payload.OutputTokens,
		TotalTokens:           payload.TotalTokens,
		ContextMessages:       payload.ContextMessages,
		ContextTokensEstimate: payload.ContextTokensEstimate,
		SessionSummary:        payload.SessionSummary,
	}
}

func fromTranscriptProgress(payload transcript.ProgressPayload) ServiceProgressData {
	return ServiceProgressData{
		Phase:                 payload.Phase,
		State:                 payload.State,
		Source:                payload.Source,
		TaskID:                payload.TaskID,
		Round:                 payload.Round,
		Message:               payload.Message,
		TurnIndex:             payload.TurnIndex,
		ToolName:              payload.ToolName,
		ToolCallID:            payload.ToolCallID,
		ToolCallIndex:         payload.ToolCallIndex,
		Command:               payload.Command,
		Action:                payload.Action,
		Target:                payload.Target,
		OutputSummary:         payload.OutputSummary,
		OutputBytes:           payload.OutputBytes,
		OutputLines:           payload.OutputLines,
		OutputExcerpt:         payload.OutputExcerpt,
		DurationMS:            payload.DurationMS,
		SinceLastMS:           payload.SinceLastMS,
		TokPerSec:             payload.TokPerSec,
		InputTokens:           payload.InputTokens,
		OutputTokens:          payload.OutputTokens,
		TotalTokens:           payload.TotalTokens,
		ContextMessages:       payload.ContextMessages,
		ContextTokensEstimate: payload.ContextTokensEstimate,
		SessionSummary:        payload.SessionSummary,
	}
}

func fromTranscriptProgressPtr(payload *transcript.ProgressPayload) *ServiceProgressData {
	if payload == nil {
		return nil
	}
	converted := fromTranscriptProgress(*payload)
	return &converted
}

func toTranscriptRouteDecision(decision RouteDecision) transcript.RouteProgressDecision {
	out := transcript.RouteProgressDecision{
		Harness:  decision.Harness,
		Provider: decision.Provider,
		Model:    decision.Model,
		Power:    decision.Power,
	}
	if len(decision.Candidates) == 0 {
		return out
	}
	out.Candidates = make([]transcript.RouteProgressCandidate, len(decision.Candidates))
	for i, candidate := range decision.Candidates {
		out.Candidates[i] = transcript.RouteProgressCandidate{
			Harness:            candidate.Harness,
			Provider:           candidate.Provider,
			Model:              candidate.Model,
			CostUSDPer1kTokens: candidate.CostUSDPer1kTokens,
			CostSource:         candidate.CostSource,
			Components: transcript.RouteProgressComponents{
				Power:     candidate.Components.Power,
				SpeedTPS:  candidate.Components.SpeedTPS,
				CostClass: candidate.Components.CostClass,
			},
		}
	}
	return out
}
