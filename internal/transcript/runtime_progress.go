package transcript

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	agentcore "github.com/easel/fizeau/internal/core"
	"github.com/easel/fizeau/internal/harnesses"
)

// ProgressPayload is the API-neutral progress record emitted by internal
// transcript helpers before the root facade projects it onto public types.
type ProgressPayload struct {
	Phase                 string   `json:"phase"`
	State                 string   `json:"state"`
	Source                string   `json:"source,omitempty"`
	TaskID                string   `json:"task_id,omitempty"`
	Round                 int      `json:"round,omitempty"`
	Message               string   `json:"message,omitempty"`
	TurnIndex             int      `json:"turn_index,omitempty"`
	ToolName              string   `json:"tool_name,omitempty"`
	ToolCallID            string   `json:"tool_call_id,omitempty"`
	ToolCallIndex         int      `json:"tool_call_index,omitempty"`
	Command               string   `json:"command,omitempty"`
	Action                string   `json:"action,omitempty"`
	Target                string   `json:"subject,omitempty"`
	OutputSummary         string   `json:"output_summary,omitempty"`
	OutputBytes           int      `json:"output_bytes,omitempty"`
	OutputLines           int      `json:"output_lines,omitempty"`
	OutputExcerpt         string   `json:"output_excerpt,omitempty"`
	DurationMS            int64    `json:"duration_ms,omitempty"`
	SinceLastMS           int64    `json:"since_last_ms,omitempty"`
	TokPerSec             *float64 `json:"tok_per_sec,omitempty"`
	InputTokens           *int     `json:"input_tokens,omitempty"`
	OutputTokens          *int     `json:"output_tokens,omitempty"`
	TotalTokens           *int     `json:"total_tokens,omitempty"`
	ContextMessages       int      `json:"context_messages,omitempty"`
	ContextTokensEstimate int      `json:"context_tokens_estimate,omitempty"`
	SessionSummary        string   `json:"session_summary,omitempty"`
}

type RouteProgressDecision struct {
	Harness    string
	Provider   string
	Model      string
	Power      int
	Candidates []RouteProgressCandidate
}

type RouteProgressCandidate struct {
	Harness            string
	Provider           string
	Model              string
	CostUSDPer1kTokens float64
	CostSource         string
	Components         RouteProgressComponents
}

type RouteProgressComponents struct {
	Power     int
	SpeedTPS  float64
	CostClass string
}

type NativeLLMRequestPayload struct {
	Planning   bool                `json:"planning"`
	AttemptIdx int                 `json:"attempt_index"`
	Messages   []agentcore.Message `json:"messages"`
	Tools      []agentcore.ToolDef `json:"tools"`
}

type NativeLLMResponsePayload struct {
	Planning     bool                 `json:"planning"`
	AttemptIdx   int                  `json:"attempt_index"`
	Content      string               `json:"content"`
	Usage        agentcore.TokenUsage `json:"usage"`
	LatencyMS    int64                `json:"latency_ms"`
	Error        string               `json:"error"`
	Model        string               `json:"model"`
	FinishReason string               `json:"finish_reason"`
}

type NativeToolCallPayload struct {
	Tool       string          `json:"tool"`
	Input      json.RawMessage `json:"input"`
	Output     string          `json:"output"`
	DurationMS int64           `json:"duration_ms"`
	Error      string          `json:"error"`
}

type NativeCompactionPayload struct {
	MessagesBefore int    `json:"messages_before"`
	MessagesAfter  int    `json:"messages_after"`
	TokensBefore   int    `json:"tokens_before"`
	TokensAfter    int    `json:"tokens_after"`
	Summary        string `json:"summary"`
	Warning        string `json:"warning"`
}

type NativeProgressState struct {
	*progressTracker
}

type SubprocessProgressState struct {
	*progressTracker
}

type progressTracker struct {
	kind                  string
	toolCalls             map[string]harnesses.ToolCallData
	toolCallIndexes       map[string]int
	toolCallStarts        map[string]time.Time
	turnIndex             int
	totalToolCalls        int
	recentTools           []string
	contextMessages       int
	contextTokensEstimate int
	summaryText           string
}

func FillProgressIdentity(payload *ProgressPayload, sessionID string, meta map[string]string, lineLimit int) {
	if payload == nil {
		return
	}
	if payload.TaskID == "" {
		payload.TaskID = TaskID(sessionID, meta)
	}
	if payload.Source == "" {
		payload.Source = payload.Phase
	}
	if payload.Round == 0 {
		payload.Round = payload.TurnIndex
	}
	payload.Message = StatusLine(StatusLineInput{
		TaskID:      payload.TaskID,
		TurnIndex:   payload.Round,
		Message:     payload.Message,
		SinceLastMS: payload.SinceLastMS,
		Limit:       lineLimit,
	})
}

func NewNativeProgressState() *NativeProgressState {
	return &NativeProgressState{progressTracker: &progressTracker{
		kind:            "native",
		toolCallIndexes: make(map[string]int),
	}}
}

func NewSubprocessProgressState(prompt, systemPrompt string) *SubprocessProgressState {
	return &SubprocessProgressState{progressTracker: &progressTracker{
		kind:                  "harness",
		toolCalls:             make(map[string]harnesses.ToolCallData),
		toolCallIndexes:       make(map[string]int),
		toolCallStarts:        make(map[string]time.Time),
		contextMessages:       1,
		contextTokensEstimate: estimateProgressTextTokens(prompt) + estimateProgressTextTokens(systemPrompt),
	}}
}

func (p *progressTracker) noteThinkingStart(turnIndex int) ProgressPayload {
	if turnIndex <= 0 {
		turnIndex = p.turnIndex
	}
	return ProgressPayload{
		Phase:                 "thinking",
		State:                 "start",
		Message:               shortProgressText("thinking ..."),
		TurnIndex:             turnIndex,
		ContextMessages:       p.contextMessages,
		ContextTokensEstimate: p.contextTokensEstimate,
		SessionSummary:        p.sessionSummary(),
	}
}

func (p *progressTracker) noteThinkingComplete(turnIndex int, durationMS int64, usage *harnesses.FinalUsage) ProgressPayload {
	if turnIndex <= 0 {
		turnIndex = p.turnIndex
	}
	totalTokens := 0
	outputTokens := 0
	if usage != nil {
		totalTokens = derefInt(usage.TotalTokens)
		outputTokens = derefInt(usage.OutputTokens)
		if totalTokens <= 0 {
			totalTokens = outputTokens
		}
	}
	msg := "thinking complete"
	if totalTokens > 0 {
		msg = fmt.Sprintf("thought %dtok %s", totalTokens, roundedDuration(durationMS))
	} else if durationMS > 0 {
		msg = fmt.Sprintf("thought %s", roundedDuration(durationMS))
	}
	return ProgressPayload{
		Phase:                 "thinking",
		State:                 "complete",
		Message:               shortProgressText(msg),
		TurnIndex:             turnIndex,
		DurationMS:            durationMS,
		TokPerSec:             TokenThroughput(outputTokens, durationMS),
		InputTokens:           finalUsageTokenPtr(usage, func(u *harnesses.FinalUsage) *int { return u.InputTokens }),
		OutputTokens:          finalUsageTokenPtr(usage, func(u *harnesses.FinalUsage) *int { return u.OutputTokens }),
		TotalTokens:           finalUsageTokenPtr(usage, func(u *harnesses.FinalUsage) *int { return u.TotalTokens }),
		ContextMessages:       p.contextMessages,
		ContextTokensEstimate: p.contextTokensEstimate,
		SessionSummary:        p.sessionSummary(),
	}
}

func (p *progressTracker) noteResponseComplete(final harnesses.FinalData) *ProgressPayload {
	if final.Status != "success" && final.FinalText == "" && final.Usage == nil {
		return nil
	}
	totalTokens := 0
	outputTokens := 0
	if final.Usage != nil {
		totalTokens = derefInt(final.Usage.TotalTokens)
		outputTokens = derefInt(final.Usage.OutputTokens)
		if totalTokens <= 0 {
			totalTokens = outputTokens
		}
	}
	msg := "done"
	if totalTokens > 0 {
		msg = fmt.Sprintf("done %dtok", totalTokens)
	}
	return &ProgressPayload{
		Phase:                 "response",
		State:                 "complete",
		Message:               shortProgressText(msg),
		TurnIndex:             p.turnIndex,
		DurationMS:            final.DurationMS,
		TokPerSec:             TokenThroughput(outputTokens, final.DurationMS),
		InputTokens:           finalUsageTokenPtr(final.Usage, func(u *harnesses.FinalUsage) *int { return u.InputTokens }),
		OutputTokens:          finalUsageTokenPtr(final.Usage, func(u *harnesses.FinalUsage) *int { return u.OutputTokens }),
		TotalTokens:           finalUsageTokenPtr(final.Usage, func(u *harnesses.FinalUsage) *int { return u.TotalTokens }),
		ContextMessages:       p.contextMessages,
		ContextTokensEstimate: p.contextTokensEstimate,
		SessionSummary:        p.sessionSummary(),
	}
}

func (p *progressTracker) noteToolStart(toolName, callID string, input json.RawMessage) ProgressPayload {
	p.totalToolCalls++
	toolCallIndex := p.totalToolCalls
	if callID != "" && p.toolCallIndexes != nil {
		p.toolCallIndexes[callID] = toolCallIndex
	}
	p.recentTools = appendRecentTool(p.recentTools, toolName)
	command := SummarizeToolInput(toolName, input)
	task := SummarizeToolCall(toolName, input)
	return ProgressPayload{
		Phase:                 "tool",
		State:                 "start",
		Message:               BoundedText(toolStartMessage(toolName, command), ExceptionalToolLineLimit),
		TurnIndex:             p.turnIndex,
		ToolName:              toolName,
		ToolCallID:            callID,
		ToolCallIndex:         toolCallIndex,
		Command:               command,
		Action:                task.Action,
		Target:                task.Target,
		ContextMessages:       p.contextMessages,
		ContextTokensEstimate: p.contextTokensEstimate,
		SessionSummary:        p.sessionSummary(),
	}
}

func (p *progressTracker) noteToolStartTime(callID string, at time.Time) {
	if callID == "" || at.IsZero() || p.toolCallStarts == nil {
		return
	}
	p.toolCallStarts[callID] = at
}

func (p *progressTracker) toolElapsedMS(callID string, completedAt time.Time) int64 {
	if callID == "" || completedAt.IsZero() || p.toolCallStarts == nil {
		return 0
	}
	startedAt, ok := p.toolCallStarts[callID]
	if !ok || startedAt.IsZero() || completedAt.Before(startedAt) {
		return 0
	}
	return completedAt.Sub(startedAt).Milliseconds()
}

func (p *progressTracker) forgetToolStartTime(callID string) {
	if callID != "" && p.toolCallStarts != nil {
		delete(p.toolCallStarts, callID)
	}
}

func (p *progressTracker) noteToolComplete(toolName, callID string, input json.RawMessage, output string, durationMS int64, errText string) ProgressPayload {
	command := SummarizeToolInput(toolName, input)
	task := SummarizeToolCall(toolName, input)
	outputDetail := SummarizeOutput(output)
	toolCallIndex := 0
	if callID != "" && p.toolCallIndexes != nil {
		toolCallIndex = p.toolCallIndexes[callID]
	}
	details := toolName
	if command != "" {
		details = command
	}
	if durationMS <= 0 {
		durationMS = 0
	}
	return ProgressPayload{
		Phase:                 "tool",
		State:                 "complete",
		Message:               BoundedText(toolCompleteMessage(details, durationMS, errText), ExceptionalToolLineLimit),
		TurnIndex:             p.turnIndex,
		ToolName:              toolName,
		ToolCallID:            callID,
		ToolCallIndex:         toolCallIndex,
		Command:               command,
		Action:                task.Action,
		Target:                task.Target,
		OutputSummary:         outputDetail.Summary,
		OutputBytes:           outputDetail.Bytes,
		OutputLines:           outputDetail.Lines,
		OutputExcerpt:         outputDetail.Excerpt,
		DurationMS:            durationMS,
		ContextMessages:       p.contextMessages,
		ContextTokensEstimate: p.contextTokensEstimate,
		SessionSummary:        p.sessionSummary(),
	}
}

func (p *NativeProgressState) NoteRequest(payload NativeLLMRequestPayload) ProgressPayload {
	p.turnIndex++
	p.contextMessages = len(payload.Messages)
	p.contextTokensEstimate = estimateMessagesTokens(payload.Messages)
	return p.noteThinkingStart(p.turnIndex)
}

func (p *NativeProgressState) NoteResponse(payload NativeLLMResponsePayload) ProgressPayload {
	usage := &harnesses.FinalUsage{
		InputTokens:  intPtrIfPositive(payload.Usage.Input),
		OutputTokens: intPtrIfPositive(payload.Usage.Output),
		TotalTokens:  intPtrIfPositive(payload.Usage.Total),
	}
	return p.noteThinkingComplete(p.turnIndex, payload.LatencyMS, usage)
}

func (p *NativeProgressState) NoteToolCall(callID string, payload NativeToolCallPayload) (ProgressPayload, ProgressPayload) {
	return p.noteToolStart(payload.Tool, callID, payload.Input), p.noteToolComplete(payload.Tool, callID, payload.Input, payload.Output, payload.DurationMS, payload.Error)
}

func (p *NativeProgressState) NoteCompaction(payload NativeCompactionPayload) (ProgressPayload, ProgressPayload) {
	if payload.Summary != "" {
		p.summaryText = shortProgressText(payload.Summary)
	}
	if payload.MessagesAfter > 0 {
		p.contextMessages = payload.MessagesAfter
	} else if payload.MessagesBefore > 0 {
		p.contextMessages = payload.MessagesBefore
	}
	if payload.TokensAfter > 0 {
		p.contextTokensEstimate = payload.TokensAfter
	} else if payload.TokensBefore > 0 {
		p.contextTokensEstimate = payload.TokensBefore
	}
	compaction := ProgressPayload{
		Phase:                 "compaction",
		State:                 "complete",
		Message:               shortProgressText(compactionMessage(payload)),
		ContextMessages:       p.contextMessages,
		ContextTokensEstimate: p.contextTokensEstimate,
		SessionSummary:        p.sessionSummary(),
	}
	contextUpdate := ProgressPayload{
		Phase:                 "context",
		State:                 "update",
		Message:               shortProgressText("context summary updated"),
		ContextMessages:       p.contextMessages,
		ContextTokensEstimate: p.contextTokensEstimate,
		SessionSummary:        p.sessionSummary(),
	}
	return compaction, contextUpdate
}

func (p *NativeProgressState) NoteFinal(final harnesses.FinalData) *ProgressPayload {
	return p.noteResponseComplete(final)
}

func (p *SubprocessProgressState) NoteRequestStart() ProgressPayload {
	p.turnIndex = 1
	return p.noteThinkingStart(1)
}

func (p *SubprocessProgressState) NoteResponseComplete(final harnesses.FinalData) *ProgressPayload {
	return p.noteResponseComplete(final)
}

func (p *SubprocessProgressState) NoteEvent(ev harnesses.Event) (ProgressPayload, bool) {
	switch ev.Type {
	case harnesses.EventTypeToolCall:
		var payload harnesses.ToolCallData
		if err := json.Unmarshal(ev.Data, &payload); err != nil {
			return ProgressPayload{}, false
		}
		if payload.ID != "" {
			p.toolCalls[payload.ID] = payload
			p.noteToolStartTime(payload.ID, ev.Time)
		}
		_ = p.noteToolStart(payload.Name, payload.ID, payload.Input)
		return ProgressPayload{}, false
	case harnesses.EventTypeToolResult:
		var payload harnesses.ToolResultData
		if err := json.Unmarshal(ev.Data, &payload); err != nil {
			return ProgressPayload{}, false
		}
		if payload.DurationMS <= 0 {
			payload.DurationMS = p.toolElapsedMS(payload.ID, ev.Time)
		}
		p.forgetToolStartTime(payload.ID)
		call := p.toolCalls[payload.ID]
		toolName := call.Name
		if toolName == "" {
			toolName = payload.ID
		}
		return p.noteToolComplete(toolName, payload.ID, call.Input, payload.Output, payload.DurationMS, payload.Error), true
	}
	return ProgressPayload{}, false
}

func (p *SubprocessProgressState) AnnotateToolResultDuration(ev harnesses.Event) harnesses.Event {
	if ev.Type != harnesses.EventTypeToolResult {
		return ev
	}
	var payload harnesses.ToolResultData
	if err := json.Unmarshal(ev.Data, &payload); err != nil {
		return ev
	}
	if payload.DurationMS > 0 {
		return ev
	}
	payload.DurationMS = p.toolElapsedMS(payload.ID, ev.Time)
	if payload.DurationMS <= 0 {
		return ev
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return ev
	}
	ev.Data = raw
	return ev
}

func (p *SubprocessProgressState) NoteFinalEvent(ev harnesses.Event) (ProgressPayload, bool) {
	var final harnesses.FinalData
	if err := json.Unmarshal(ev.Data, &final); err != nil {
		return ProgressPayload{}, false
	}
	payload := p.noteResponseComplete(final)
	if payload == nil {
		return ProgressPayload{}, false
	}
	return *payload, true
}

func (p *progressTracker) sessionSummary() string {
	if p.summaryText != "" {
		return p.summaryText
	}
	if p.kind != "native" {
		return shortProgressText(fmt.Sprintf(
			"subprocess tool_calls=%d context_messages=%d context_tokens_estimate=%d",
			p.totalToolCalls,
			p.contextMessages,
			p.contextTokensEstimate,
		))
	}
	latest := "none"
	if len(p.recentTools) > 0 {
		latest = strings.Join(p.recentTools, ", ")
	}
	return shortProgressText(fmt.Sprintf(
		"turns=%d tool_calls=%d latest_tools=%s context_messages=%d context_tokens_estimate=%d",
		p.turnIndex,
		p.totalToolCalls,
		latest,
		p.contextMessages,
		p.contextTokensEstimate,
	))
}

func RouteProgressData(decision RouteProgressDecision) ProgressPayload {
	candidate := selectedRouteCandidate(decision)
	label := joinProgressParts(decision.Harness, decision.Provider, decision.Model)
	if label == "" {
		label = joinProgressParts(decision.Provider, decision.Model)
	}
	parts := []string{"route"}
	if label != "" {
		parts = append(parts, label)
	}
	power := decision.Power
	if power <= 0 && candidate != nil {
		power = candidate.Components.Power
	}
	if power > 0 {
		parts = append(parts, fmt.Sprintf("power=%d", power))
	}
	if candidate != nil {
		if speed := candidate.Components.SpeedTPS; speed > 0 {
			parts = append(parts, "speed="+formatProgressFloat(speed))
		}
		if cost := candidate.CostUSDPer1kTokens; cost > 0 {
			parts = append(parts, "cost="+formatProgressFloat(cost))
		}
		if source := strings.TrimSpace(candidate.CostSource); source != "" {
			parts = append(parts, "cost_source="+source)
		}
	}
	line := shortProgressText(strings.Join(compactProgressParts(parts), " "))
	return ProgressPayload{
		Phase:          "route",
		State:          "start",
		Message:        line,
		SessionSummary: line,
	}
}

func selectedRouteCandidate(decision RouteProgressDecision) *RouteProgressCandidate {
	if len(decision.Candidates) == 0 {
		return nil
	}
	for i := range decision.Candidates {
		c := &decision.Candidates[i]
		if c.Harness == decision.Harness && c.Provider == decision.Provider && c.Model == decision.Model {
			return c
		}
	}
	for i := range decision.Candidates {
		c := &decision.Candidates[i]
		if c.Provider == decision.Provider && c.Model == decision.Model {
			return c
		}
	}
	if len(decision.Candidates) == 1 {
		return &decision.Candidates[0]
	}
	return nil
}

func SummarizeToolInput(toolName string, input json.RawMessage) string {
	if len(input) == 0 {
		return ""
	}
	if toolName == "bash" {
		var payload any
		if err := json.Unmarshal(input, &payload); err == nil {
			if command := ExtractBashCommand(payload); command != "" {
				return BoundedText(command, ExceptionalToolLineLimit)
			}
		}
	}
	return BoundedText(SummarizeJSONValue(input), ExceptionalToolLineLimit)
}

func shortProgressText(s string) string {
	return BoundedText(s, DefaultLineLimit)
}

func appendRecentTool(tools []string, name string) []string {
	if name == "" {
		return tools
	}
	tools = append(tools, name)
	if len(tools) > 3 {
		tools = tools[len(tools)-3:]
	}
	return tools
}

func estimateMessagesTokens(messages []agentcore.Message) int {
	total := 0
	for _, msg := range messages {
		total += estimateProgressTextTokens(string(msg.Role))
		total += estimateProgressTextTokens(msg.Content)
		if msg.ToolCallID != "" {
			total += estimateProgressTextTokens(msg.ToolCallID)
		}
		for _, tc := range msg.ToolCalls {
			total += estimateProgressTextTokens(tc.Name)
			total += estimateProgressTextTokens(string(tc.Arguments))
		}
	}
	return total
}

func joinProgressParts(parts ...string) string {
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if strings.TrimSpace(part) != "" {
			out = append(out, strings.TrimSpace(part))
		}
	}
	return strings.Join(out, "/")
}

func compactProgressParts(parts []string) []string {
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if strings.TrimSpace(part) != "" {
			out = append(out, strings.TrimSpace(part))
		}
	}
	return out
}

func formatProgressFloat(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

func compactionMessage(payload NativeCompactionPayload) string {
	freed := payload.TokensBefore - payload.TokensAfter
	if freed < 0 {
		freed = 0
	}
	return fmt.Sprintf("compacted %d -> %d messages, freed %d tokens", payload.MessagesBefore, payload.MessagesAfter, freed)
}

func toolStartMessage(toolName, command string) string {
	if command != "" {
		return fmt.Sprintf("tool `%s` start", command)
	}
	if toolName != "" {
		return fmt.Sprintf("tool `%s` start", toolName)
	}
	return "tool start"
}

func toolCompleteMessage(details string, durationMS int64, errMsg string) string {
	if strings.TrimSpace(details) == "" {
		details = "tool call"
	}
	if strings.TrimSpace(errMsg) != "" {
		return fmt.Sprintf("tool `%s` failed %s", details, roundedDuration(durationMS))
	}
	return fmt.Sprintf("tool `%s` done %s", details, roundedDuration(durationMS))
}

func roundedDuration(ms int64) string {
	if ms <= 0 {
		return "0s"
	}
	return (time.Duration(ms) * time.Millisecond).String()
}

func intPtrIfPositive(v int) *int {
	if v <= 0 {
		return nil
	}
	return &v
}

func finalUsageTokenPtr(usage *harnesses.FinalUsage, pick func(*harnesses.FinalUsage) *int) *int {
	if usage == nil {
		return nil
	}
	return pick(usage)
}

func derefInt(v *int) int {
	if v == nil {
		return 0
	}
	return *v
}

func estimateProgressTextTokens(s string) int {
	return (len(s) + 3) / 4
}
