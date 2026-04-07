package session

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/DocumentDrivenDX/forge"
)

// Replay reads a session log and renders a human-readable conversation.
func Replay(path string, w io.Writer) error {
	events, err := ReadEvents(path)
	if err != nil {
		return fmt.Errorf("replay: %w", err)
	}

	for _, e := range events {
		switch e.Type {
		case forge.EventSessionStart:
			var data SessionStartData
			if err := json.Unmarshal(e.Data, &data); err != nil {
				continue
			}
			fmt.Fprintf(w, "=== Session %s ===\n", e.SessionID)
			fmt.Fprintf(w, "Time: %s\n", e.Timestamp.Format("2006-01-02 15:04:05 UTC"))
			fmt.Fprintf(w, "Provider: %s | Model: %s\n", data.Provider, data.Model)
			fmt.Fprintf(w, "Max iterations: %d | Work dir: %s\n", data.MaxIterations, data.WorkDir)
			if data.SystemPrompt != "" {
				fmt.Fprintf(w, "\n[System]\n%s\n", data.SystemPrompt)
			}
			fmt.Fprintf(w, "\n[User]\n%s\n", data.Prompt)

		case forge.EventLLMResponse:
			var data LLMResponseData
			if err := json.Unmarshal(e.Data, &data); err != nil {
				continue
			}
			fmt.Fprintf(w, "\n[Assistant] (%dms, %d in / %d out tokens",
				data.LatencyMs, data.Usage.Input, data.Usage.Output)
			if data.CostUSD > 0 {
				fmt.Fprintf(w, ", $%.4f", data.CostUSD)
			}
			fmt.Fprintf(w, ")\n")
			if data.Content != "" {
				fmt.Fprintf(w, "%s\n", data.Content)
			}
			if len(data.ToolCalls) > 0 {
				fmt.Fprintf(w, "[%d tool call(s)]\n", len(data.ToolCalls))
			}

		case forge.EventToolCall:
			var data ToolCallData
			if err := json.Unmarshal(e.Data, &data); err != nil {
				continue
			}
			fmt.Fprintf(w, "\n  > %s (%dms)\n", data.Tool, data.DurationMs)
			fmt.Fprintf(w, "    Input:  %s\n", compactJSON(data.Input))
			output := data.Output
			if len(output) > 200 {
				output = output[:200] + "...[truncated]"
			}
			fmt.Fprintf(w, "    Output: %s\n", strings.ReplaceAll(output, "\n", "\n            "))
			if data.Error != "" {
				fmt.Fprintf(w, "    Error:  %s\n", data.Error)
			}

		case forge.EventSessionEnd:
			var data SessionEndData
			if err := json.Unmarshal(e.Data, &data); err != nil {
				continue
			}
			fmt.Fprintf(w, "\n=== End (%s) ===\n", data.Status)
			fmt.Fprintf(w, "Duration: %dms | Tokens: %d in / %d out",
				data.DurationMs, data.Tokens.Input, data.Tokens.Output)
			if data.CostUSD > 0 {
				fmt.Fprintf(w, " | Cost: $%.4f", data.CostUSD)
			} else if data.CostUSD == 0 {
				fmt.Fprintf(w, " | Cost: $0 (local)")
			} else {
				fmt.Fprintf(w, " | Cost: unknown")
			}
			fmt.Fprintln(w)
			if data.Error != "" {
				fmt.Fprintf(w, "Error: %s\n", data.Error)
			}
		}
	}
	return nil
}

func compactJSON(raw json.RawMessage) string {
	var v any
	if json.Unmarshal(raw, &v) != nil {
		return string(raw)
	}
	data, _ := json.Marshal(v)
	return string(data)
}
