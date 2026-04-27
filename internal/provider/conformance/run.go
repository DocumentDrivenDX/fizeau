package conformance

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	agent "github.com/DocumentDrivenDX/agent/internal/core"
)

// Factory builds a fresh provider subject for one conformance scenario.
type Factory func(t *testing.T) Subject

// Subject is the provider plus any protocol-specific discovery hooks needed to
// exercise health and model-discovery capabilities.
type Subject struct {
	Provider agent.Provider

	HealthCheck func(context.Context) error
	ListModels  func(context.Context) ([]string, error)
}

// Capabilities declares the scenarios that apply to a provider. Add
// fields as the shared catalog grows rather than baking provider names into Run.
type Capabilities struct {
	Name              string
	ExpectedModels    []string
	SupportsStreaming bool
	SupportsThinking  bool
	SupportsToolCalls bool
	MaxTokensSlack    int

	ChatContains      string
	StreamContains    string
	ReasoningContains string

	// ChatMaxTokens overrides the default chat/stream max-tokens budget
	// (8 tokens). Thinking-mode providers like luce burn output budget on
	// reasoning_content before the visible content; without headroom they
	// return empty content and the chat assertions fail. 0 = use default.
	ChatMaxTokens int
	// StreamMaxTokensCheck overrides the per-test cap for the
	// "streaming max_tokens honored" subtest. The check still asserts the
	// returned word count is bounded by this value + MaxTokensSlack. 0 =
	// use the existing default of 3.
	StreamMaxTokensCheck int
	// ScenarioTimeout overrides the per-subtest wall-clock budget
	// (default 5 seconds). Local thinking-capable providers can need
	// substantially more — set generously for those. 0 = use default.
	ScenarioTimeout time.Duration
}

// chatMaxTokens returns the configured chat budget or the default.
func (c Capabilities) chatMaxTokens() int {
	if c.ChatMaxTokens > 0 {
		return c.ChatMaxTokens
	}
	return 8
}

// streamMaxTokensCheck returns the configured stream cap or the default.
func (c Capabilities) streamMaxTokensCheck() int {
	if c.StreamMaxTokensCheck > 0 {
		return c.StreamMaxTokensCheck
	}
	return 3
}

// scenarioTimeout returns the configured timeout or the default 5s.
func (c Capabilities) scenarioTimeout() time.Duration {
	if c.ScenarioTimeout > 0 {
		return c.ScenarioTimeout
	}
	return 5 * time.Second
}

// Run executes the shared provider conformance catalog.
func Run(t *testing.T, factory Factory, caps Capabilities) {
	t.Helper()
	if caps.Name == "" {
		t.Fatal("conformance: Capabilities.Name is required")
	}
	if caps.ChatContains == "" {
		caps.ChatContains = "pong"
	}
	if caps.StreamContains == "" {
		caps.StreamContains = "stream-pong"
	}
	if caps.ReasoningContains == "" {
		caps.ReasoningContains = "reasoning"
	}
	if caps.MaxTokensSlack == 0 {
		caps.MaxTokensSlack = 1
	}

	t.Run("health check", func(t *testing.T) {
		subject := newSubject(t, factory)
		if subject.HealthCheck == nil {
			t.Fatalf("%s: health check hook is required", caps.Name)
		}
		ctx, cancel := scenarioContext(caps)
		defer cancel()
		if err := subject.HealthCheck(ctx); err != nil {
			t.Fatalf("%s: health check failed: %v", caps.Name, err)
		}
	})

	t.Run("model discovery", func(t *testing.T) {
		subject := newSubject(t, factory)
		if len(caps.ExpectedModels) == 0 {
			t.Fatalf("%s: ExpectedModels is required for model discovery", caps.Name)
		}
		if subject.ListModels == nil {
			t.Fatalf("%s: model discovery hook is required", caps.Name)
		}
		ctx, cancel := scenarioContext(caps)
		defer cancel()
		models, err := subject.ListModels(ctx)
		if err != nil {
			t.Fatalf("%s: list models failed: %v", caps.Name, err)
		}
		for _, want := range caps.ExpectedModels {
			if !contains(models, want) {
				t.Fatalf("%s: discovered models %v, want %q", caps.Name, models, want)
			}
		}
	})

	t.Run("non-streaming chat", func(t *testing.T) {
		subject := newSubject(t, factory)
		ctx, cancel := scenarioContext(caps)
		defer cancel()
		resp, err := subject.Provider.Chat(ctx, []agent.Message{
			{Role: agent.RoleUser, Content: "conformance: reply with pong"},
		}, nil, agent.Options{MaxTokens: caps.chatMaxTokens()})
		if err != nil {
			t.Fatalf("%s: Chat failed: %v", caps.Name, err)
		}
		if !strings.Contains(resp.Content, caps.ChatContains) {
			t.Fatalf("%s: Chat content %q, want substring %q", caps.Name, resp.Content, caps.ChatContains)
		}
		if resp.Model == "" {
			t.Fatalf("%s: Chat response model is empty", caps.Name)
		}
	})

	if !caps.SupportsStreaming {
		return
	}

	t.Run("streaming chat", func(t *testing.T) {
		subject := newSubject(t, factory)
		streamer := requireStreamer(t, caps.Name, subject.Provider)
		ctx, cancel := scenarioContext(caps)
		defer cancel()
		result := collectStream(t, caps.Name, streamer, ctx, []agent.Message{
			{Role: agent.RoleUser, Content: "conformance: stream-pong"},
		}, nil, agent.Options{MaxTokens: caps.chatMaxTokens()})
		if !result.done {
			t.Fatalf("%s: stream did not emit Done", caps.Name)
		}
		if !strings.Contains(result.content, caps.StreamContains) {
			t.Fatalf("%s: stream content %q, want substring %q", caps.Name, result.content, caps.StreamContains)
		}
	})

	t.Run("streaming max_tokens honored", func(t *testing.T) {
		subject := newSubject(t, factory)
		streamer := requireStreamer(t, caps.Name, subject.Provider)
		ctx, cancel := scenarioContext(caps)
		defer cancel()
		maxTokens := caps.streamMaxTokensCheck()
		result := collectStream(t, caps.Name, streamer, ctx, []agent.Message{
			{Role: agent.RoleUser, Content: "conformance: max tokens"},
		}, nil, agent.Options{MaxTokens: maxTokens})
		words := len(strings.Fields(result.content))
		if words == 0 {
			t.Fatalf("%s: max_tokens stream returned empty content", caps.Name)
		}
		if words > maxTokens+caps.MaxTokensSlack {
			t.Fatalf("%s: stream returned %d words, want <= %d", caps.Name, words, maxTokens+caps.MaxTokensSlack)
		}
	})

	if caps.SupportsThinking {
		t.Run("thinking reasoning", func(t *testing.T) {
			subject := newSubject(t, factory)
			streamer := requireStreamer(t, caps.Name, subject.Provider)
			ctx, cancel := scenarioContext(caps)
			defer cancel()
			result := collectStream(t, caps.Name, streamer, ctx, []agent.Message{
				{Role: agent.RoleUser, Content: "conformance: reason briefly then answer"},
			}, nil, agent.Options{MaxTokens: caps.chatMaxTokens(), Reasoning: agent.ReasoningTokens(32)})
			if !strings.Contains(result.reasoning, caps.ReasoningContains) {
				t.Fatalf("%s: reasoning content %q, want substring %q", caps.Name, result.reasoning, caps.ReasoningContains)
			}
		})
	}

	if caps.SupportsToolCalls {
		t.Run("tool call streaming", func(t *testing.T) {
			subject := newSubject(t, factory)
			streamer := requireStreamer(t, caps.Name, subject.Provider)
			ctx, cancel := scenarioContext(caps)
			defer cancel()
			result := collectStream(t, caps.Name, streamer, ctx, []agent.Message{
				{Role: agent.RoleUser, Content: "conformance: call the inspect tool"},
			}, []agent.ToolDef{inspectTool()}, agent.Options{MaxTokens: caps.chatMaxTokens()})
			if result.toolID == "" {
				t.Fatalf("%s: stream did not emit a tool call id", caps.Name)
			}
			if result.toolName != "inspect" {
				t.Fatalf("%s: stream tool name %q, want inspect", caps.Name, result.toolName)
			}
			if !strings.Contains(result.toolArgs, "target") {
				t.Fatalf("%s: stream tool args %q, want target field", caps.Name, result.toolArgs)
			}
		})
	}
}

type streamResult struct {
	content   string
	reasoning string
	toolID    string
	toolName  string
	toolArgs  string
	done      bool
}

func newSubject(t *testing.T, factory Factory) Subject {
	t.Helper()
	subject := factory(t)
	if subject.Provider == nil {
		t.Fatal("conformance: factory returned nil Provider")
	}
	return subject
}

func requireStreamer(t *testing.T, name string, provider agent.Provider) agent.StreamingProvider {
	t.Helper()
	streamer, ok := provider.(agent.StreamingProvider)
	if !ok {
		t.Fatalf("%s: provider does not implement agent.StreamingProvider", name)
	}
	return streamer
}

func collectStream(t *testing.T, name string, streamer agent.StreamingProvider, ctx context.Context, messages []agent.Message, tools []agent.ToolDef, opts agent.Options) streamResult {
	t.Helper()
	ch, err := streamer.ChatStream(ctx, messages, tools, opts)
	if err != nil {
		t.Fatalf("%s: ChatStream setup failed: %v", name, err)
	}
	var result streamResult
	for delta := range ch {
		if delta.Err != nil {
			t.Fatalf("%s: stream error: %v", name, delta.Err)
		}
		result.content += delta.Content
		result.reasoning += delta.ReasoningContent
		if delta.ToolCallID != "" {
			result.toolID = delta.ToolCallID
		}
		if delta.ToolCallName != "" {
			result.toolName = delta.ToolCallName
		}
		result.toolArgs += delta.ToolCallArgs
		result.done = result.done || delta.Done
	}
	return result
}

func inspectTool() agent.ToolDef {
	return agent.ToolDef{
		Name:        "inspect",
		Description: "Inspect a named test target.",
		Parameters:  json.RawMessage(`{"type":"object","properties":{"target":{"type":"string"}},"required":["target"]}`),
	}
}

func scenarioContext(caps Capabilities) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), caps.scenarioTimeout())
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func HTTPStatusError(status int) error {
	if status >= 200 && status < 300 {
		return nil
	}
	return fmt.Errorf("HTTP %d", status)
}
