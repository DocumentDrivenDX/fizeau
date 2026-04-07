// Package forge provides an embeddable Go agent runtime — a tool-calling LLM
// loop with file read/write, shell execution, and structured I/O.
package forge

import (
	"context"
	"encoding/json"
	"time"
)

// Status represents the outcome of an agent run.
type Status string

const (
	StatusSuccess        Status = "success"
	StatusIterationLimit Status = "iteration_limit"
	StatusCancelled      Status = "cancelled"
	StatusError          Status = "error"
)

// Role identifies the sender of a message in the conversation.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// TokenUsage tracks input and output token counts.
type TokenUsage struct {
	Input  int `json:"input"`
	Output int `json:"output"`
	Total  int `json:"total"`
}

// Add accumulates token counts from another TokenUsage.
func (u *TokenUsage) Add(other TokenUsage) {
	u.Input += other.Input
	u.Output += other.Output
	u.Total += other.Total
}

// ToolCall represents a tool invocation requested by the model.
type ToolCall struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// Message is a single message in the conversation history.
type Message struct {
	Role       Role       `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

// ToolDef describes a tool for the LLM provider.
type ToolDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"` // JSON Schema
}

// Options configures a single provider Chat call.
type Options struct {
	Model       string   `json:"model,omitempty"`
	Temperature *float64 `json:"temperature,omitempty"`
	MaxTokens   int      `json:"max_tokens,omitempty"`
	Stop        []string `json:"stop,omitempty"`
}

// Response is the result of a single provider Chat call.
type Response struct {
	Content      string     `json:"content"`
	ToolCalls    []ToolCall `json:"tool_calls,omitempty"`
	Usage        TokenUsage `json:"usage"`
	Model        string     `json:"model"`
	FinishReason string     `json:"finish_reason"`
}

// Provider is the interface that LLM backends implement.
// Define it in the consuming package per Go idiom.
type Provider interface {
	Chat(ctx context.Context, messages []Message, tools []ToolDef, opts Options) (Response, error)
}

// Tool is the interface that agent tools implement.
type Tool interface {
	// Name returns the tool's identifier.
	Name() string
	// Description returns a human-readable description for the LLM.
	Description() string
	// Schema returns the JSON Schema for the tool's parameters.
	Schema() json.RawMessage
	// Execute runs the tool with the given parameters and returns the result.
	Execute(ctx context.Context, params json.RawMessage) (string, error)
}

// ToolCallLog records one tool execution during an agent run.
type ToolCallLog struct {
	Tool     string          `json:"tool"`
	Input    json.RawMessage `json:"input"`
	Output   string          `json:"output"`
	Duration time.Duration   `json:"duration_ms"`
	Error    string          `json:"error,omitempty"`
}

// EventType identifies the kind of event emitted during an agent run.
type EventType string

const (
	EventSessionStart EventType = "session.start"
	EventLLMRequest   EventType = "llm.request"
	EventLLMResponse  EventType = "llm.response"
	EventToolCall     EventType = "tool.call"
	EventSessionEnd   EventType = "session.end"
)

// Event is a structured event emitted during an agent run.
type Event struct {
	SessionID string          `json:"session_id"`
	Seq       int             `json:"seq"`
	Type      EventType       `json:"type"`
	Timestamp time.Time       `json:"ts"`
	Data      json.RawMessage `json:"data"`
}

// EventCallback receives events during an agent run. The session logger is
// one implementation; callers can also use it for progress reporting.
type EventCallback func(Event)

// Request configures a single agent run.
type Request struct {
	// Prompt is the user's task description.
	Prompt string

	// SystemPrompt is prepended to the conversation as a system message.
	SystemPrompt string

	// Provider is the configured LLM backend.
	Provider Provider

	// Tools are the tools available to the agent.
	Tools []Tool

	// MaxIterations limits the number of tool-call rounds. Zero means no limit.
	MaxIterations int

	// WorkDir is the working directory for file operations and bash commands.
	WorkDir string

	// Callback receives events in real time. May be nil.
	Callback EventCallback

	// Metadata is correlation data (e.g., bead_id) stored on session events.
	Metadata map[string]string
}

// Result is the outcome of an agent run.
type Result struct {
	// Status indicates whether the run succeeded.
	Status Status `json:"status"`

	// Output is the final text response from the model.
	Output string `json:"output"`

	// ToolCalls logs every tool execution during the run.
	ToolCalls []ToolCallLog `json:"tool_calls,omitempty"`

	// Tokens is the accumulated token usage across all iterations.
	Tokens TokenUsage `json:"tokens"`

	// Duration is the total wall-clock time of the run.
	Duration time.Duration `json:"duration_ms"`

	// CostUSD is the estimated cost. -1 means unknown (model not in pricing table).
	// 0 means free (local model with $0 pricing entry).
	CostUSD float64 `json:"cost_usd"`

	// Model is the model that was used.
	Model string `json:"model"`

	// Error is non-nil when Status is StatusError.
	Error error `json:"-"`

	// SessionID identifies the session log for this run.
	SessionID string `json:"session_id"`
}

// Run executes the agent loop: send prompt, process tool calls, repeat until
// the model produces a final text response or limits are reached.
//
// This is a placeholder that will be implemented in the agent loop bead.
func Run(ctx context.Context, req Request) (Result, error) {
	_ = ctx
	_ = req
	return Result{Status: StatusError}, nil
}
