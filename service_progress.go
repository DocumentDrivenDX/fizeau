package fizeau

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	agentcore "github.com/DocumentDrivenDX/fizeau/internal/core"
	"github.com/DocumentDrivenDX/fizeau/internal/harnesses"
	"github.com/DocumentDrivenDX/fizeau/internal/transcript"
)

const (
	progressLineLimit            = transcript.DefaultLineLimit
	progressExceptionalLineLimit = transcript.ExceptionalToolLineLimit
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
	if payload.Source == "" {
		payload.Source = payload.Phase
	}
	if payload.Round == 0 {
		payload.Round = payload.TurnIndex
	}
	payload.Message = progressStatusLine(*payload)
}

func progressTaskID(sessionID string, meta map[string]string) string {
	return transcript.TaskID(sessionID, meta)
}

func progressStatusLine(payload ServiceProgressData) string {
	return transcript.StatusLine(transcript.StatusLineInput{
		TaskID:    payload.TaskID,
		TurnIndex: payload.Round,
		Message:   payload.Message,
		Limit:     progressMessageLimit(payload),
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
	outputTokens := 0
	if usage != nil {
		totalTokens = derefServiceInt(usage.TotalTokens)
		outputTokens = derefServiceInt(usage.OutputTokens)
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
	return ServiceProgressData{
		Phase:                 "thinking",
		State:                 "complete",
		Message:               shortProgressText(msg),
		TurnIndex:             turnIndex,
		DurationMS:            durationMS,
		TokPerSec:             progressTokenThroughput(outputTokens, durationMS),
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
	outputTokens := 0
	if final.Usage != nil {
		totalTokens = derefServiceInt(final.Usage.TotalTokens)
		outputTokens = derefServiceInt(final.Usage.OutputTokens)
		if totalTokens <= 0 {
			totalTokens = outputTokens
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
		TokPerSec:             progressTokenThroughput(outputTokens, final.DurationMS),
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
	task := summarizeToolTask(toolName, input)
	return ServiceProgressData{
		Phase:                 "tool",
		State:                 "start",
		Message:               boundedProgressText(toolStartMessage(toolName, command), progressExceptionalLineLimit),
		TurnIndex:             p.turnIndex,
		ToolName:              toolName,
		Command:               command,
		Action:                task.Action,
		Target:                task.Target,
		ContextMessages:       p.contextMessages,
		ContextTokensEstimate: p.contextTokensEstimate,
		SessionSummary:        p.sessionSummary(),
	}
}

func (p *progressTracker) noteToolComplete(toolName string, input json.RawMessage, output string, durationMS int64, errText string) ServiceProgressData {
	command := summarizeToolInput(toolName, input)
	task := summarizeToolTask(toolName, input)
	outputDetail := summarizeToolOutputDetail(output)
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
	return p.noteToolStart(payload.Tool, payload.Input), p.noteToolComplete(payload.Tool, payload.Input, payload.Output, payload.DurationMS, payload.Error)
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
		return p.noteToolComplete(toolName, call.Input, payload.Output, payload.DurationMS, payload.Error), true
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

type toolTaskSummary struct {
	Action string
	Target string
}

func summarizeToolTask(toolName string, input json.RawMessage) toolTaskSummary {
	toolName = strings.TrimSpace(toolName)
	payload := map[string]any{}
	if len(input) > 0 {
		_ = json.Unmarshal(input, &payload)
	}
	path := summaryString(payload, "path", "file")
	switch toolName {
	case "read":
		if path != "" {
			return toolTaskSummary{Action: "inspect " + lineRangeSummary(payload) + " in " + path, Target: path}
		}
		return toolTaskSummary{Action: "inspect file"}
	case "write":
		return toolTaskSummary{Action: "write file", Target: path}
	case "edit", "anchor_edit":
		return toolTaskSummary{Action: "edit file", Target: path}
	case "patch":
		action := "edit file"
		if op := summaryString(payload, "operation"); op != "" {
			action = op + " file"
		}
		return toolTaskSummary{Action: action, Target: path}
	case "grep":
		pattern := summaryString(payload, "pattern")
		target := firstNonEmpty(summaryString(payload, "dir"), summaryString(payload, "glob"))
		action := "search"
		if pattern != "" {
			action += " " + strconv.Quote(boundedProgressText(pattern, 32))
		}
		if target != "" {
			action += " in " + target
		}
		return toolTaskSummary{Action: action, Target: target}
	case "find":
		pattern := summaryString(payload, "pattern")
		target := summaryString(payload, "dir")
		action := "find files"
		if pattern != "" {
			action += " matching " + strconv.Quote(boundedProgressText(pattern, 32))
		}
		if target != "" {
			action += " in " + target
		}
		return toolTaskSummary{Action: action, Target: target}
	case "ls":
		target := summaryString(payload, "path")
		if target == "" {
			target = "."
		}
		return toolTaskSummary{Action: "list directory " + target, Target: target}
	case "bash":
		return summarizeShellTask(extractBashCommand(payload))
	default:
		if path != "" {
			return toolTaskSummary{Action: toolName, Target: path}
		}
		if toolName != "" {
			return toolTaskSummary{Action: toolName}
		}
		return toolTaskSummary{}
	}
}

func summarizeShellTask(command string) toolTaskSummary {
	command = normalizeShellCommand(command)
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return toolTaskSummary{}
	}
	switch fields[0] {
	case "sed":
		if len(fields) >= 4 && fields[1] == "-n" {
			target := fields[3]
			return toolTaskSummary{Action: "inspect " + shellLineRange(fields[2]) + " in " + target, Target: target}
		}
	case "cat", "head", "tail":
		if len(fields) >= 2 {
			target := fields[len(fields)-1]
			return toolTaskSummary{Action: "inspect " + target, Target: target}
		}
	case "rg", "grep":
		return summarizeSearchCommand(fields)
	case "go":
		if len(fields) >= 2 && fields[1] == "test" {
			target := strings.Join(fields[2:], " ")
			return toolTaskSummary{Action: strings.TrimSpace("test " + target), Target: target}
		}
	case "git":
		return summarizeGitCommand(fields)
	case "apply_patch":
		return toolTaskSummary{Action: "apply patch"}
	}
	return toolTaskSummary{Action: boundedProgressText(command, 96)}
}

func summarizeSearchCommand(fields []string) toolTaskSummary {
	pattern := ""
	target := ""
	for _, field := range fields[1:] {
		if strings.HasPrefix(field, "-") {
			continue
		}
		if pattern == "" {
			pattern = strings.Trim(field, "'\"")
			continue
		}
		target = field
	}
	action := "search"
	if pattern != "" {
		action += " " + strconv.Quote(boundedProgressText(pattern, 32))
	}
	if target != "" {
		action += " in " + target
	}
	return toolTaskSummary{Action: action, Target: target}
}

func summarizeGitCommand(fields []string) toolTaskSummary {
	if len(fields) < 2 {
		return toolTaskSummary{Action: "git"}
	}
	switch fields[1] {
	case "add":
		target := strings.Join(fields[2:], " ")
		if target == "" {
			return toolTaskSummary{Action: "stage changes"}
		}
		return toolTaskSummary{Action: "stage changes", Target: target}
	case "commit":
		return toolTaskSummary{Action: "commit changes"}
	case "diff":
		return toolTaskSummary{Action: "inspect diff"}
	case "status":
		return toolTaskSummary{Action: "inspect git status"}
	case "log":
		return toolTaskSummary{Action: "inspect git log"}
	default:
		return toolTaskSummary{Action: "git " + fields[1]}
	}
}

func normalizeShellCommand(command string) string {
	command = strings.TrimSpace(command)
	for _, prefix := range []string{"/bin/zsh -lc ", "zsh -lc ", "/bin/bash -lc ", "bash -lc "} {
		if !strings.HasPrefix(command, prefix) {
			continue
		}
		inner := strings.TrimSpace(strings.TrimPrefix(command, prefix))
		if unquoted, err := strconv.Unquote(inner); err == nil {
			command = unquoted
		} else {
			command = strings.Trim(inner, `"`)
		}
		break
	}
	for _, sep := range []string{" && ", " || ", " ; "} {
		if idx := strings.Index(command, sep); idx >= 0 {
			command = strings.TrimSpace(command[:idx])
			break
		}
	}
	return strings.Join(strings.Fields(command), " ")
}

func shellLineRange(expr string) string {
	expr = strings.Trim(strings.TrimSpace(expr), "'\"")
	expr = strings.TrimSuffix(expr, "p")
	if expr == "" {
		return "lines"
	}
	return "lines " + expr
}

func lineRangeSummary(payload map[string]any) string {
	offset := summaryInt(payload, "offset")
	limit := summaryInt(payload, "limit")
	if offset <= 0 && limit <= 0 {
		return "file"
	}
	if limit > 0 {
		return fmt.Sprintf("lines %d-%d", offset+1, offset+limit)
	}
	return fmt.Sprintf("from line %d", offset+1)
}

func summaryString(payload map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := payload[key]; ok {
			if s, ok := value.(string); ok && strings.TrimSpace(s) != "" {
				return strings.TrimSpace(s)
			}
		}
	}
	return ""
}

func summaryInt(payload map[string]any, key string) int {
	switch value := payload[key].(type) {
	case float64:
		return int(value)
	case int:
		return value
	default:
		return 0
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

type toolOutputDetail struct {
	Summary string
	Bytes   int
	Lines   int
	Excerpt string
}

func summarizeToolOutput(output string) string {
	return summarizeToolOutputDetail(output).Summary
}

func summarizeToolOutputDetail(output string) toolOutputDetail {
	output = strings.TrimSpace(output)
	if output == "" {
		return toolOutputDetail{}
	}
	lineCount := strings.Count(output, "\n") + 1
	byteCount := len([]byte(output))
	parts := []string{fmt.Sprintf("out=%s", formatByteCount(byteCount))}
	if lineCount == 1 {
		parts = append(parts, "1 line")
	} else {
		parts = append(parts, fmt.Sprintf("%d lines", lineCount))
	}
	excerpt := ""
	if byteCount > 40 {
		for _, line := range strings.Split(output, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			excerpt = boundedProgressText(line, 48)
			parts = append(parts, strconv.Quote(excerpt))
			break
		}
	}
	return toolOutputDetail{
		Summary: boundedProgressText(strings.Join(parts, " "), 96),
		Bytes:   byteCount,
		Lines:   lineCount,
		Excerpt: excerpt,
	}
}

func formatByteCount(n int) string {
	switch {
	case n < 1024:
		return fmt.Sprintf("%dB", n)
	case n < 1024*1024:
		return fmt.Sprintf("%.1fKB", float64(n)/1024)
	default:
		return fmt.Sprintf("%.1fMB", float64(n)/(1024*1024))
	}
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

func progressTokenThroughput(outputTokens int, durationMS int64) *float64 {
	if outputTokens <= 0 || durationMS <= 0 {
		return nil
	}
	v := float64(outputTokens) / (float64(durationMS) / 1000)
	return &v
}

func estimateProgressTextTokens(s string) int {
	return (len(s) + 3) / 4
}

func boundedProgressText(s string, maxRunes int) string {
	return transcript.BoundedText(s, maxRunes)
}
