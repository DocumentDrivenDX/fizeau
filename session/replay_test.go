package session

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/DocumentDrivenDX/forge"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReplay(t *testing.T) {
	dir := t.TempDir()
	sessionID := "replay-test"

	// Write a test session log
	logger := NewLogger(dir, sessionID)
	logger.Emit(forge.EventSessionStart, SessionStartData{
		Provider:      "openai-compat",
		Model:         "qwen3.5-7b",
		WorkDir:       "/tmp/test",
		MaxIterations: 20,
		Prompt:        "Read main.go",
		SystemPrompt:  "You are a helpful assistant.",
	})
	logger.Emit(forge.EventLLMResponse, LLMResponseData{
		Content:   "",
		ToolCalls: []forge.ToolCall{{ID: "tc1", Name: "read"}},
		Usage:     forge.TokenUsage{Input: 100, Output: 20, Total: 120},
		LatencyMs: 500,
		Model:     "qwen3.5-7b",
	})
	logger.Emit(forge.EventToolCall, ToolCallData{
		Tool:       "read",
		Input:      []byte(`{"path":"main.go"}`),
		Output:     "package main\n\nfunc main() {}\n",
		DurationMs: 1,
	})
	logger.Emit(forge.EventLLMResponse, LLMResponseData{
		Content:   "The package is main.",
		Usage:     forge.TokenUsage{Input: 200, Output: 30, Total: 230},
		LatencyMs: 800,
		Model:     "qwen3.5-7b",
	})
	logger.Emit(forge.EventSessionEnd, SessionEndData{
		Status:     forge.StatusSuccess,
		Output:     "The package is main.",
		Tokens:     forge.TokenUsage{Input: 300, Output: 50, Total: 350},
		CostUSD:    0,
		DurationMs: 1500,
	})
	require.NoError(t, logger.Close())

	// Replay it
	var buf bytes.Buffer
	err := Replay(filepath.Join(dir, sessionID+".jsonl"), &buf)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "Session replay-test")
	assert.Contains(t, output, "qwen3.5-7b")
	assert.Contains(t, output, "[System]")
	assert.Contains(t, output, "You are a helpful assistant.")
	assert.Contains(t, output, "[User]")
	assert.Contains(t, output, "Read main.go")
	assert.Contains(t, output, "> read")
	assert.Contains(t, output, "package main")
	assert.Contains(t, output, "The package is main.")
	assert.Contains(t, output, "End (success)")
	assert.Contains(t, output, "$0 (local)")
}
