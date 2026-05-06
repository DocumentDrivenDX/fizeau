package transcript

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	DefaultLineLimit         = 80
	ExceptionalToolLineLimit = 120
)

type ProgressType string

const (
	ProgressTypeRoute        ProgressType = "route"
	ProgressTypeThinking     ProgressType = "thinking"
	ProgressTypeTool         ProgressType = "tool"
	ProgressTypeResponse     ProgressType = "response"
	ProgressTypeContext      ProgressType = "context"
	ProgressTypeCompaction   ProgressType = "compaction"
	ProgressTypeToolStart    ProgressType = "tool.start"
	ProgressTypeToolComplete ProgressType = "tool.complete"
	ProgressTypeLLMRequest   ProgressType = "llm.request"
	ProgressTypeLLMResponse  ProgressType = "llm.response"
)

type Callback func(Event)

type Event struct {
	Type      ProgressType `json:"type"`
	Source    string       `json:"source,omitempty"`
	TaskID    string       `json:"task_id,omitempty"`
	TurnIndex int          `json:"turn_index,omitempty"`
	Phase     string       `json:"phase,omitempty"`
	Status    string       `json:"status,omitempty"`
	Message   string       `json:"message,omitempty"`
	Action    string       `json:"action,omitempty"`
	Target    string       `json:"target,omitempty"`
	Tool      Tool         `json:"tool,omitempty"`
	LLM       LLM          `json:"llm,omitempty"`
	Timing    Timing       `json:"timing,omitempty"`
	Usage     Usage        `json:"usage,omitempty"`
	Output    Output       `json:"output,omitempty"`
}

type Tool struct {
	Name     string         `json:"name,omitempty"`
	CallID   string         `json:"call_id,omitempty"`
	Input    map[string]any `json:"input,omitempty"`
	ExitCode *int           `json:"exit_code,omitempty"`
	Error    string         `json:"error,omitempty"`
}

type LLM struct {
	Provider string `json:"provider,omitempty"`
	Model    string `json:"model,omitempty"`
}

type Timing struct {
	DurationMS int64   `json:"duration_ms,omitempty"`
	TokPerSec  float64 `json:"tok_per_sec,omitempty"`
}

type Usage struct {
	InputTokens        int `json:"input_tokens,omitempty"`
	OutputTokens       int `json:"output_tokens,omitempty"`
	CachedInputTokens  int `json:"cached_input_tokens,omitempty"`
	RetriedInputTokens int `json:"retried_input_tokens,omitempty"`
	TotalTokens        int `json:"total_tokens,omitempty"`
}

type Output struct {
	Bytes   int    `json:"bytes,omitempty"`
	Lines   int    `json:"lines,omitempty"`
	Excerpt string `json:"excerpt,omitempty"`
}

type StatusLineInput struct {
	TaskID      string
	TurnIndex   int
	Message     string
	SinceLastMS int64
	Limit       int
}

func StatusLine(in StatusLineInput) string {
	limit := in.Limit
	if limit <= 0 {
		limit = DefaultLineLimit
	}
	msg := strings.TrimSpace(in.Message)
	prefix := CompactIdentity(in.TaskID, in.TurnIndex)
	age := CompactElapsed(in.SinceLastMS)
	switch {
	case prefix == "" && age == "":
		return BoundedText(msg, limit)
	case prefix == "":
		return BoundedText(strings.TrimSpace(age+" "+msg), limit)
	case msg == "" && age == "":
		return BoundedText(prefix, limit)
	case msg == "":
		return BoundedText(prefix+" "+age, limit)
	case age == "":
		return BoundedText(prefix+" "+msg, limit)
	default:
		return BoundedText(prefix+" "+age+" "+msg, limit)
	}
}

func CompactElapsed(ms int64) string {
	if ms <= 0 {
		return ""
	}
	return "+" + (time.Duration(ms) * time.Millisecond).String()
}

func CompactIdentity(taskID string, turnIndex int) string {
	taskID = CompactTaskID(taskID)
	switch {
	case taskID != "" && turnIndex > 0:
		return fmt.Sprintf("%s #%d", taskID, turnIndex)
	case taskID != "":
		return taskID
	case turnIndex > 0:
		return fmt.Sprintf("#%d", turnIndex)
	default:
		return ""
	}
}

func CompactTaskID(taskID string) string {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return ""
	}
	return BoundedText(taskID, 24)
}

func TaskID(sessionID string, meta map[string]string) string {
	for _, key := range []string{"task_id", "bead_id", "correlation_id"} {
		if meta != nil && strings.TrimSpace(meta[key]) != "" {
			return strings.TrimSpace(meta[key])
		}
	}
	return sessionID
}

func BoundedText(s string, maxRunes int) string {
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
