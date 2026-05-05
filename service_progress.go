package fizeau

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
	"unicode/utf8"

	agentcore "github.com/DocumentDrivenDX/fizeau/internal/core"
	"github.com/DocumentDrivenDX/fizeau/internal/harnesses"
)

const (
	progressLineLimit            = 80
	progressExceptionalLineLimit = 120
)

type nativeProgressState struct {
	*progressTracker
}

type subprocessProgressState struct {
	*progressTracker
}

type progressTracker struct {
	kind                  string
	toolCalls             map[string]harnesses.ToolCallData
	turnIndex             int
	totalToolCalls        int
	recentTools           []string
	contextMessages       int
	contextTokensEstimate int
	summaryText           string
}

type nativeLLMRequestPayload struct {
	Planning   bool                `json:"planning"`
	AttemptIdx int                 `json:"attempt_index"`
	Messages   []agentcore.Message `json:"messages"`
	Tools      []agentcore.ToolDef `json:"tools"`
}

type nativeLLMResponsePayload struct {
	Planning     bool                 `json:"planning"`
	AttemptIdx   int                  `json:"attempt_index"`
	Content      string               `json:"content"`
	Usage        agentcore.TokenUsage `json:"usage"`
	LatencyMS    int64                `json:"latency_ms"`
	Error        string               `json:"error"`
	Model        string               `json:"model"`
	FinishReason string               `json:"finish_reason"`
}

type nativeToolCallPayload struct {
	Tool       string          `json:"tool"`
	Input      json.RawMessage `json:"input"`
	Output     string          `json:"output"`
	DurationMS int64           `json:"duration_ms"`
	Error      string          `json:"error"`
}

type nativeCompactionPayload struct {
	MessagesBefore int    `json:"messages_before"`
	MessagesAfter  int    `json:"messages_after"`
	TokensBefore   int    `json:"tokens_before"`
	TokensAfter    int    `json:"tokens_after"`
	Summary        string `json:"summary"`
	Warning        string `json:"warning"`
}

func emitProgress(out chan<- ServiceEvent, seq *atomic.Int64, sl *serviceSessionLog, sessionID string, meta map[string]string, payload ServiceProgressData) {
	fillProgressIdentity(&payload, sessionID, meta)
	raw, err := json.Marshal(payload)
	if err != nil {
		return
	}
	eventSeq := seq.Add(1) - 1
	ev := harnesses.Event{
		Type:     harnesses.EventTypeProgress,
		Sequence: eventSeq,
		Time:     time.Now().UTC(),
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
	if payload.TaskID == "" {
		payload.TaskID = progressTaskID(sessionID, meta)
	}
	if payload.Round == 0 {
		payload.Round = payload.TurnIndex
	}
	payload.Message = progressStatusLine(*payload)
}

func progressTaskID(sessionID string, meta map[string]string) string {
	for _, key := range []string{"task_id", "bead_id", "correlation_id"} {
		if meta != nil && strings.TrimSpace(meta[key]) != "" {
			return strings.TrimSpace(meta[key])
		}
	}
	return sessionID
}

func progressStatusLine(payload ServiceProgressData) string {
	msg := strings.TrimSpace(payload.Message)
	prefix := compactProgressIdentity(payload.TaskID, payload.Round)
	if prefix == "" {
		return boundedProgressText(msg, progressMessageLimit(payload))
	}
	if msg == "" {
		return boundedProgressText(prefix, progressMessageLimit(payload))
	}
	return boundedProgressText(prefix+" "+msg, progressMessageLimit(payload))
}

func compactProgressIdentity(taskID string, round int) string {
	taskID = compactProgressTaskID(taskID)
	switch {
	case taskID != "" && round > 0:
		return fmt.Sprintf("%s #%d", taskID, round)
	case taskID != "":
		return taskID
	case round > 0:
		return fmt.Sprintf("#%d", round)
	default:
		return ""
	}
}

func compactProgressTaskID(taskID string) string {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return ""
	}
	return boundedProgressText(taskID, 24)
}

func progressMessageLimit(payload ServiceProgressData) int {
	if payload.Phase == "tool" && payload.Command != "" {
		return progressExceptionalLineLimit
	}
	return progressLineLimit
}

func newNativeProgressState() *nativeProgressState {
	return &nativeProgressState{progressTracker: &progressTracker{kind: "native"}}
}

func newSubprocessProgressState(req ServiceExecuteRequest) *subprocessProgressState {
	return &subprocessProgressState{progressTracker: newHarnessProgressTracker(req)}
}

func newHarnessProgressTracker(req ServiceExecuteRequest) *progressTracker {
	return &progressTracker{
		kind:                  "harness",
		toolCalls:             make(map[string]harnesses.ToolCallData),
		contextMessages:       1,
		contextTokensEstimate: estimateProgressTextTokens(req.Prompt) + estimateProgressTextTokens(req.SystemPrompt),
	}
}

func (p *progressTracker) noteThinkingStart(turnIndex int) ServiceProgressData {
	if turnIndex <= 0 {
		turnIndex = p.turnIndex
	}
	return ServiceProgressData{
		Phase:                 "thinking",
		State:                 "start",
		Message:               shortProgressText("thinking ..."),
		TurnIndex:             turnIndex,
		ContextMessages:       p.contextMessages,
		ContextTokensEstimate: p.contextTokensEstimate,
		SessionSummary:        p.sessionSummary(),
	}
}

func (p *progressTracker) noteThinkingComplete(turnIndex int, durationMS int64, usage *harnesses.FinalUsage) ServiceProgressData {
	if turnIndex <= 0 {
		turnIndex = p.turnIndex
	}
	totalTokens := 0
	if usage != nil {
		totalTokens = derefServiceInt(usage.TotalTokens)
		if totalTokens <= 0 {
			totalTokens = derefServiceInt(usage.OutputTokens)
		}
	}
	msg := "thinking complete"
	if totalTokens > 0 {
		msg = fmt.Sprintf("thought %dtok %s", totalTokens, roundedDuration(durationMS))
	} else if durationMS > 0 {
		msg = fmt.Sprintf("thought %s", roundedDuration(durationMS))
	}
	return ServiceProgressData{
		Phase:                 "thinking",
		State:                 "complete",
		Message:               shortProgressText(msg),
		TurnIndex:             turnIndex,
		DurationMS:            durationMS,
		InputTokens:           finalUsageTokenPtr(usage, func(u *harnesses.FinalUsage) *int { return u.InputTokens }),
		OutputTokens:          finalUsageTokenPtr(usage, func(u *harnesses.FinalUsage) *int { return u.OutputTokens }),
		TotalTokens:           finalUsageTokenPtr(usage, func(u *harnesses.FinalUsage) *int { return u.TotalTokens }),
		ContextMessages:       p.contextMessages,
		ContextTokensEstimate: p.contextTokensEstimate,
		SessionSummary:        p.sessionSummary(),
	}
}

func (p *progressTracker) noteResponseComplete(final harnesses.FinalData) *ServiceProgressData {
	if final.Status != "success" && final.FinalText == "" && final.Usage == nil {
		return nil
	}
	totalTokens := 0
	if final.Usage != nil {
		totalTokens = derefServiceInt(final.Usage.TotalTokens)
		if totalTokens <= 0 {
			totalTokens = derefServiceInt(final.Usage.OutputTokens)
		}
	}
	msg := "done"
	if totalTokens > 0 {
		msg = fmt.Sprintf("done %dtok", totalTokens)
	}
	return &ServiceProgressData{
		Phase:                 "response",
		State:                 "complete",
		Message:               shortProgressText(msg),
		TurnIndex:             p.turnIndex,
		DurationMS:            final.DurationMS,
		InputTokens:           finalUsageTokenPtr(final.Usage, func(u *harnesses.FinalUsage) *int { return u.InputTokens }),
		OutputTokens:          finalUsageTokenPtr(final.Usage, func(u *harnesses.FinalUsage) *int { return u.OutputTokens }),
		TotalTokens:           finalUsageTokenPtr(final.Usage, func(u *harnesses.FinalUsage) *int { return u.TotalTokens }),
		ContextMessages:       p.contextMessages,
		ContextTokensEstimate: p.contextTokensEstimate,
		SessionSummary:        p.sessionSummary(),
	}
}

func (p *progressTracker) noteToolStart(toolName string, input json.RawMessage) ServiceProgressData {
	p.totalToolCalls++
	p.recentTools = appendRecentTool(p.recentTools, toolName)
	command := summarizeToolInput(toolName, input)
	return ServiceProgressData{
		Phase:                 "tool",
		State:                 "start",
		Message:               boundedProgressText(toolStartMessage(toolName, command), progressExceptionalLineLimit),
		TurnIndex:             p.turnIndex,
		ToolName:              toolName,
		Command:               command,
		ContextMessages:       p.contextMessages,
		ContextTokensEstimate: p.contextTokensEstimate,
		SessionSummary:        p.sessionSummary(),
	}
}

func (p *progressTracker) noteToolComplete(toolName string, input json.RawMessage, durationMS int64, errText string) ServiceProgressData {
	command := summarizeToolInput(toolName, input)
	details := toolName
	if command != "" {
		details = command
	}
	if durationMS <= 0 {
		durationMS = 1
	}
	return ServiceProgressData{
		Phase:                 "tool",
		State:                 "complete",
		Message:               boundedProgressText(toolCompleteMessage(details, durationMS, errText), progressExceptionalLineLimit),
		TurnIndex:             p.turnIndex,
		ToolName:              toolName,
		Command:               command,
		DurationMS:            durationMS,
		ContextMessages:       p.contextMessages,
		ContextTokensEstimate: p.contextTokensEstimate,
		SessionSummary:        p.sessionSummary(),
	}
}

func (p *nativeProgressState) noteRequest(payload nativeLLMRequestPayload) ServiceProgressData {
	p.turnIndex++
	p.contextMessages = len(payload.Messages)
	p.contextTokensEstimate = estimateMessagesTokens(payload.Messages)
	return p.noteThinkingStart(p.turnIndex)
}

func (p *nativeProgressState) noteResponse(payload nativeLLMResponsePayload) ServiceProgressData {
	usage := &harnesses.FinalUsage{
		InputTokens:  intPtrIfPositive(payload.Usage.Input),
		OutputTokens: intPtrIfPositive(payload.Usage.Output),
		TotalTokens:  intPtrIfPositive(payload.Usage.Total),
	}
	return p.noteThinkingComplete(p.turnIndex, payload.LatencyMS, usage)
}

func (p *nativeProgressState) noteToolCall(payload nativeToolCallPayload) (ServiceProgressData, ServiceProgressData) {
	return p.noteToolStart(payload.Tool, payload.Input), p.noteToolComplete(payload.Tool, payload.Input, payload.DurationMS, payload.Error)
}

func (p *nativeProgressState) noteCompaction(payload nativeCompactionPayload) (ServiceProgressData, ServiceProgressData) {
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
	compaction := ServiceProgressData{
		Phase:                 "compaction",
		State:                 "complete",
		Message:               shortProgressText(compactionMessage(payload)),
		ContextMessages:       p.contextMessages,
		ContextTokensEstimate: p.contextTokensEstimate,
		SessionSummary:        p.sessionSummary(),
	}
	contextUpdate := ServiceProgressData{
		Phase:                 "context",
		State:                 "update",
		Message:               shortProgressText("context summary updated"),
		ContextMessages:       p.contextMessages,
		ContextTokensEstimate: p.contextTokensEstimate,
		SessionSummary:        p.sessionSummary(),
	}
	return compaction, contextUpdate
}

func (p *nativeProgressState) noteFinal(final harnesses.FinalData) *ServiceProgressData {
	return p.noteResponseComplete(final)
}

func (p *subprocessProgressState) noteRequestStart() ServiceProgressData {
	p.turnIndex = 1
	return p.noteThinkingStart(1)
}

func (p *subprocessProgressState) noteThinkingComplete(final harnesses.FinalData) ServiceProgressData {
	return p.progressTracker.noteThinkingComplete(1, final.DurationMS, final.Usage)
}

func (p *subprocessProgressState) noteEvent(ev harnesses.Event) (ServiceProgressData, bool) {
	switch ev.Type {
	case harnesses.EventTypeToolCall:
		var payload harnesses.ToolCallData
		if err := json.Unmarshal(ev.Data, &payload); err != nil {
			return ServiceProgressData{}, false
		}
		if payload.ID != "" {
			p.toolCalls[payload.ID] = payload
		}
		return p.noteToolStart(payload.Name, payload.Input), true
	case harnesses.EventTypeToolResult:
		var payload harnesses.ToolResultData
		if err := json.Unmarshal(ev.Data, &payload); err != nil {
			return ServiceProgressData{}, false
		}
		call := p.toolCalls[payload.ID]
		toolName := call.Name
		if toolName == "" {
			toolName = payload.ID
		}
		return p.noteToolComplete(toolName, call.Input, payload.DurationMS, payload.Error), true
	}
	return ServiceProgressData{}, false
}

func (p *subprocessProgressState) noteFinal(ev harnesses.Event) (ServiceProgressData, bool) {
	var final harnesses.FinalData
	if err := json.Unmarshal(ev.Data, &final); err != nil {
		return ServiceProgressData{}, false
	}
	payload := p.noteResponseComplete(final)
	if payload == nil {
		return ServiceProgressData{}, false
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

func routeProgressData(decision RouteDecision) ServiceProgressData {
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
	return ServiceProgressData{
		Phase:          "route",
		State:          "start",
		Message:        line,
		SessionSummary: line,
	}
}

func selectedRouteCandidate(decision RouteDecision) *RouteCandidate {
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

func shortProgressText(s string) string {
	return boundedProgressText(s, progressLineLimit)
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

func summarizeToolInput(toolName string, input json.RawMessage) string {
	if len(input) == 0 {
		return ""
	}
	if toolName == "bash" {
		var payload any
		if err := json.Unmarshal(input, &payload); err == nil {
			if command := extractBashCommand(payload); command != "" {
				return boundedProgressText(command, 120)
			}
		}
	}
	return boundedProgressText(summarizeJSONValue(input), 120)
}

func summarizeJSONValue(raw json.RawMessage) string {
	var payload any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return strings.TrimSpace(string(raw))
	}
	return summarizeAnyValue(payload)
}

func summarizeAnyValue(v any) string {
	switch x := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		if len(keys) > 3 {
			keys = keys[:3]
		}
		parts := make([]string, 0, len(keys))
		for _, key := range keys {
			parts = append(parts, fmt.Sprintf("%s=%s", key, summarizeValueForKey(key, x[key])))
		}
		if len(x) > len(keys) {
			parts = append(parts, "...")
		}
		return "{" + strings.Join(parts, ", ") + "}"
	case []any:
		return fmt.Sprintf("[%d item(s)]", len(x))
	case string:
		return fmt.Sprintf("%q", boundedProgressText(x, 32))
	case float64, bool, nil:
		return fmt.Sprint(x)
	default:
		return fmt.Sprintf("%T", x)
	}
}

func summarizeValueForKey(key string, v any) string {
	if isSensitiveSummaryKey(key) {
		return "[redacted]"
	}
	switch x := v.(type) {
	case string:
		return fmt.Sprintf("%q", boundedProgressText(x, 32))
	case map[string]any, []any:
		return summarizeAnyValue(x)
	case float64, bool, nil:
		return fmt.Sprint(x)
	default:
		return fmt.Sprintf("%T", x)
	}
}

func isSensitiveSummaryKey(key string) bool {
	key = strings.ToLower(key)
	switch {
	case strings.Contains(key, "secret"):
		return true
	case strings.Contains(key, "token"):
		return true
	case strings.Contains(key, "password"):
		return true
	case strings.Contains(key, "passwd"):
		return true
	case strings.Contains(key, "api_key"):
		return true
	case strings.Contains(key, "apikey"):
		return true
	case strings.Contains(key, "key"):
		return true
	case strings.Contains(key, "auth"):
		return true
	default:
		return false
	}
}

func compactionMessage(payload nativeCompactionPayload) string {
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

func derefServiceInt(v *int) int {
	if v == nil {
		return 0
	}
	return *v
}

func estimateProgressTextTokens(s string) int {
	return (len(s) + 3) / 4
}

func boundedProgressText(s string, maxRunes int) string {
	s = strings.TrimSpace(s)
	if maxRunes <= 0 || s == "" {
		return ""
	}
	if utf8.RuneCountInString(s) <= maxRunes {
		return s
	}
	runes := []rune(s)
	if maxRunes <= 3 {
		return string(runes[:maxRunes])
	}
	return string(runes[:maxRunes-3]) + "..."
}
