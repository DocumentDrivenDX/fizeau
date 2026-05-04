package fizeau

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync/atomic"
	"time"
	"unicode/utf8"

	agentcore "github.com/DocumentDrivenDX/fizeau/internal/core"
	"github.com/DocumentDrivenDX/fizeau/internal/harnesses"
)

type nativeProgressState struct {
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

func (p *nativeProgressState) noteRequest(payload nativeLLMRequestPayload) ServiceProgressData {
	p.turnIndex++
	p.contextMessages = len(payload.Messages)
	p.contextTokensEstimate = estimateMessagesTokens(payload.Messages)
	return ServiceProgressData{
		Phase:                 "thinking",
		State:                 "start",
		Message:               "thinking ...",
		TurnIndex:             p.turnIndex,
		ContextMessages:       p.contextMessages,
		ContextTokensEstimate: p.contextTokensEstimate,
		SessionSummary:        p.sessionSummary(),
	}
}

func (p *nativeProgressState) noteResponse(payload nativeLLMResponsePayload) ServiceProgressData {
	totalTokens := payload.Usage.Total
	if totalTokens <= 0 {
		totalTokens = payload.Usage.Output
	}
	msg := "thinking complete"
	if totalTokens > 0 {
		msg = fmt.Sprintf("thinking complete %d tok in %s", totalTokens, roundedDuration(payload.LatencyMS))
	} else if payload.LatencyMS > 0 {
		msg = fmt.Sprintf("thinking complete in %s", roundedDuration(payload.LatencyMS))
	}
	return ServiceProgressData{
		Phase:                 "thinking",
		State:                 "complete",
		Message:               msg,
		TurnIndex:             p.turnIndex,
		DurationMS:            payload.LatencyMS,
		InputTokens:           intPtrIfPositive(payload.Usage.Input),
		OutputTokens:          intPtrIfPositive(payload.Usage.Output),
		TotalTokens:           intPtrIfPositive(payload.Usage.Total),
		ContextMessages:       p.contextMessages,
		ContextTokensEstimate: p.contextTokensEstimate,
		SessionSummary:        p.sessionSummary(),
	}
}

func (p *nativeProgressState) noteToolCall(payload nativeToolCallPayload) (ServiceProgressData, ServiceProgressData) {
	p.totalToolCalls++
	p.recentTools = appendRecentTool(p.recentTools, payload.Tool)
	command := summarizeToolInput(payload.Tool, payload.Input)
	start := ServiceProgressData{
		Phase:                 "tool",
		State:                 "start",
		Message:               toolStartMessage(payload.Tool, command),
		TurnIndex:             p.turnIndex,
		ToolName:              payload.Tool,
		Command:               command,
		ContextMessages:       p.contextMessages,
		ContextTokensEstimate: p.contextTokensEstimate,
		SessionSummary:        p.sessionSummary(),
	}
	complete := ServiceProgressData{
		Phase:                 "tool",
		State:                 "complete",
		Message:               toolCompleteMessage(payload.Tool, payload.DurationMS, payload.Error),
		TurnIndex:             p.turnIndex,
		ToolName:              payload.Tool,
		Command:               command,
		DurationMS:            payload.DurationMS,
		ContextMessages:       p.contextMessages,
		ContextTokensEstimate: p.contextTokensEstimate,
		SessionSummary:        p.sessionSummary(),
	}
	return start, complete
}

func (p *nativeProgressState) noteCompaction(payload nativeCompactionPayload) (ServiceProgressData, ServiceProgressData) {
	if payload.Summary != "" {
		p.summaryText = boundedProgressText(payload.Summary, 240)
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
		Message:               compactionMessage(payload),
		ContextMessages:       p.contextMessages,
		ContextTokensEstimate: p.contextTokensEstimate,
		SessionSummary:        p.sessionSummary(),
	}
	contextUpdate := ServiceProgressData{
		Phase:                 "context",
		State:                 "update",
		Message:               "context summary updated",
		ContextMessages:       p.contextMessages,
		ContextTokensEstimate: p.contextTokensEstimate,
		SessionSummary:        p.sessionSummary(),
	}
	return compaction, contextUpdate
}

func (p *nativeProgressState) noteFinal(final harnesses.FinalData) *ServiceProgressData {
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
	msg := "sending response"
	if totalTokens > 0 {
		msg = fmt.Sprintf("sending response %d tok", totalTokens)
	}
	return &ServiceProgressData{
		Phase:                 "response",
		State:                 "complete",
		Message:               msg,
		DurationMS:            final.DurationMS,
		InputTokens:           finalUsageTokenPtr(final.Usage, func(u *harnesses.FinalUsage) *int { return u.InputTokens }),
		OutputTokens:          finalUsageTokenPtr(final.Usage, func(u *harnesses.FinalUsage) *int { return u.OutputTokens }),
		TotalTokens:           finalUsageTokenPtr(final.Usage, func(u *harnesses.FinalUsage) *int { return u.TotalTokens }),
		ContextMessages:       p.contextMessages,
		ContextTokensEstimate: p.contextTokensEstimate,
		SessionSummary:        p.sessionSummary(),
	}
}

func (p *nativeProgressState) sessionSummary() string {
	if p.summaryText != "" {
		return p.summaryText
	}
	latest := "none"
	if len(p.recentTools) > 0 {
		latest = strings.Join(p.recentTools, ", ")
	}
	return boundedProgressText(fmt.Sprintf(
		"turns=%d tool_calls=%d latest_tools=%s context_messages=%d context_tokens_estimate=%d",
		p.turnIndex,
		p.totalToolCalls,
		latest,
		p.contextMessages,
		p.contextTokensEstimate,
	), 240)
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
		return fmt.Sprintf("running tool call `%s` ...", command)
	}
	if toolName != "" {
		return fmt.Sprintf("running tool call `%s` ...", toolName)
	}
	return "running tool call ..."
}

func toolCompleteMessage(toolName string, durationMS int64, errMsg string) string {
	if strings.TrimSpace(errMsg) != "" {
		return fmt.Sprintf("tool call `%s` failed in %s", toolName, roundedDuration(durationMS))
	}
	if toolName != "" {
		return fmt.Sprintf("tool call `%s` completed in %s", toolName, roundedDuration(durationMS))
	}
	return fmt.Sprintf("tool call completed in %s", roundedDuration(durationMS))
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
	return string(runes[:maxRunes]) + "...[truncated]"
}
