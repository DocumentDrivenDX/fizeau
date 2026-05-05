//go:build testseam

package fizeau_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	fizeau "github.com/DocumentDrivenDX/fizeau"
	"github.com/DocumentDrivenDX/fizeau/internal/session"
)

type progressTool struct {
	output string
}

func (t *progressTool) Name() string        { return "bash" }
func (t *progressTool) Description() string { return "test progress tool" }
func (t *progressTool) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"command":{"type":"string"}}}`)
}
func (t *progressTool) Execute(ctx context.Context, params json.RawMessage) (string, error) {
	return t.output, nil
}
func (t *progressTool) Parallel() bool { return false }

func progressEvents(events []fizeau.ServiceEvent) []fizeau.ServiceProgressData {
	out := make([]fizeau.ServiceProgressData, 0)
	for _, ev := range events {
		if ev.Type != fizeau.ServiceEventTypeProgress {
			continue
		}
		var payload fizeau.ServiceProgressData
		_ = json.Unmarshal(ev.Data, &payload)
		out = append(out, payload)
	}
	return out
}

func sessionLogHasEventType(t *testing.T, dir, want string) bool {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, "*.jsonl"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(matches) == 0 {
		t.Fatalf("no session log written to %s", dir)
	}
	for _, path := range matches {
		events, err := session.ReadEvents(path)
		if err != nil {
			t.Fatalf("read session log %s: %v", path, err)
		}
		for _, ev := range events {
			if string(ev.Type) == want {
				return true
			}
		}
	}
	return false
}

func TestExecute_NativeEmitsLLMProgress(t *testing.T) {
	fp := &fizeau.FakeProvider{
		Dynamic: func(req fizeau.FakeRequest) (fizeau.FakeResponse, error) {
			time.Sleep(25 * time.Millisecond)
			return fizeau.FakeResponse{
				Text: "done",
				Usage: fizeau.TokenUsage{
					Input:  10,
					Output: 5,
					Total:  15,
				},
			}, nil
		},
	}
	opts := fizeau.ServiceOptions{}
	opts.FakeProvider = fp
	svc, err := fizeau.New(opts)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	dir := t.TempDir()
	ch, err := svc.Execute(context.Background(), fizeau.ServiceExecuteRequest{
		Prompt:        "hi",
		Harness:       "agent",
		Provider:      "fake",
		Model:         "fake-model",
		SessionLogDir: dir,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	events := drainEvents(t, ch, 5*time.Second)
	progress := progressEvents(events)
	if len(progress) < 4 {
		t.Fatalf("expected route + thinking + response progress events, got %v", progress)
	}

	start := progress[0]
	if start.Phase != "route" || start.State != "start" {
		t.Fatalf("route start = %#v", start)
	}
	if !strings.Contains(start.Message, "route") || !strings.Contains(start.Message, "agent") {
		t.Fatalf("route start message = %q", start.Message)
	}
	if len(start.Message) > 80 || len(start.SessionSummary) > 80 {
		t.Fatalf("route start progress exceeded 80 chars: %#v", start)
	}

	thinkingStart := progress[1]
	if thinkingStart.Phase != "thinking" || thinkingStart.State != "start" {
		t.Fatalf("thinking start = %#v", thinkingStart)
	}
	if !strings.Contains(thinkingStart.Message, "thinking") {
		t.Fatalf("thinking start message = %q", thinkingStart.Message)
	}
	if len(thinkingStart.Message) > 80 || len(thinkingStart.SessionSummary) > 80 {
		t.Fatalf("thinking start progress exceeded 80 chars: %#v", thinkingStart)
	}

	complete := progress[2]
	if complete.Phase != "thinking" || complete.State != "complete" {
		t.Fatalf("thinking complete = %#v", complete)
	}
	if complete.DurationMS < 20 {
		t.Fatalf("thinking complete duration = %dms, want >=20ms", complete.DurationMS)
	}
	if complete.TotalTokens == nil || *complete.TotalTokens != 15 {
		t.Fatalf("thinking complete tokens = %#v", complete.TotalTokens)
	}
	if len(complete.Message) > 80 || len(complete.SessionSummary) > 80 {
		t.Fatalf("thinking complete progress exceeded 80 chars: %#v", complete)
	}

	response := progress[3]
	if response.Phase != "response" || response.State != "complete" {
		t.Fatalf("response complete = %#v", response)
	}
	if !strings.Contains(response.Message, "sending response") {
		t.Fatalf("response progress message = %q", response.Message)
	}
	if len(response.Message) > 80 || len(response.SessionSummary) > 80 {
		t.Fatalf("response progress exceeded 80 chars: %#v", response)
	}
	if response.TotalTokens == nil || *response.TotalTokens != 15 {
		t.Fatalf("response progress tokens = %#v", response.TotalTokens)
	}

	for _, p := range progress {
		if len(p.Message) > 80 {
			t.Fatalf("progress message too long: %#v", p)
		}
		if len(p.SessionSummary) > 80 {
			t.Fatalf("progress session summary too long: %#v", p)
		}
	}
	if !sessionLogHasEventType(t, dir, "progress") {
		t.Fatal("session log did not persist a progress event")
	}
}

func TestExecute_EmitsRouteProgressBeforeHarnessWork(t *testing.T) {
	fp := &fizeau.FakeProvider{
		Dynamic: func(req fizeau.FakeRequest) (fizeau.FakeResponse, error) {
			time.Sleep(20 * time.Millisecond)
			return fizeau.FakeResponse{
				Text: "done",
				Usage: fizeau.TokenUsage{
					Input:  7,
					Output: 4,
					Total:  11,
				},
			}, nil
		},
	}
	opts := fizeau.ServiceOptions{}
	opts.FakeProvider = fp
	svc, err := fizeau.New(opts)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	dir := t.TempDir()
	ch, err := svc.Execute(context.Background(), fizeau.ServiceExecuteRequest{
		Prompt:        "route progress",
		Harness:       "agent",
		Provider:      "fake",
		Model:         "gpt-5.4",
		SessionLogDir: dir,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	events := drainEvents(t, ch, 5*time.Second)
	progress := progressEvents(events)
	if len(progress) == 0 {
		t.Fatal("expected progress events")
	}
	if progress[0].Phase != "route" || progress[0].State != "start" {
		t.Fatalf("first progress event = %#v, want route start", progress[0])
	}
	if !strings.Contains(progress[0].Message, "agent/fake/gpt-5.4") {
		t.Fatalf("route progress message = %q", progress[0].Message)
	}
	if len(progress[0].Message) > 80 || len(progress[0].SessionSummary) > 80 {
		t.Fatalf("route progress exceeded 80 chars: %#v", progress[0])
	}
	if len(progress) < 2 || progress[1].Phase != "thinking" || progress[1].State != "start" {
		t.Fatalf("second progress event = %#v, want thinking start", progress[1])
	}
	for _, p := range progress {
		if len(p.Message) > 80 {
			t.Fatalf("progress message too long: %#v", p)
		}
		if len(p.SessionSummary) > 80 {
			t.Fatalf("progress summary too long: %#v", p)
		}
	}
	if !sessionLogHasEventType(t, dir, "progress") {
		t.Fatal("session log did not persist a progress event")
	}
}

func TestProgressMessages_BoundedUnder80Chars(t *testing.T) {
	longOutput := strings.Repeat("workspace-output-", 40)
	fp := &fizeau.FakeProvider{
		Static: []fizeau.FakeResponse{
			{
				ToolCalls: []fizeau.ToolCall{{
					ID:        "tool-1",
					Name:      "bash",
					Arguments: json.RawMessage(`{"command":"echo hi && printf 'this is long but summarized'"}`),
				}},
			},
			{
				Text: "compaction summary sanitized",
				Usage: fizeau.TokenUsage{
					Input:  7,
					Output: 3,
					Total:  10,
				},
			},
			{
				Text: "final answer",
				Usage: fizeau.TokenUsage{
					Input:  4,
					Output: 1,
					Total:  5,
				},
			},
		},
	}
	opts := fizeau.ServiceOptions{}
	opts.FakeProvider = fp
	svc, err := fizeau.New(opts)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	dir := t.TempDir()
	ch, err := svc.Execute(context.Background(), fizeau.ServiceExecuteRequest{
		Prompt:                  "compact me",
		Harness:                 "agent",
		Provider:                "fake",
		Model:                   "fake-model",
		WorkDir:                 t.TempDir(),
		Permissions:             "unrestricted",
		Tools:                   []fizeau.Tool{&progressTool{output: longOutput}},
		CompactionContextWindow: 80,
		CompactionReserveTokens: 0,
		SessionLogDir:           dir,
		MaxIterations:           4,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	events := drainEvents(t, ch, 10*time.Second)
	progress := progressEvents(events)
	if len(progress) == 0 {
		t.Fatal("expected progress events")
	}
	for _, p := range progress {
		if len(p.Message) > 80 {
			t.Fatalf("progress message too long: %#v", p)
		}
		if len(p.SessionSummary) > 80 {
			t.Fatalf("progress session summary too long: %#v", p)
		}
	}
	if !sessionLogHasEventType(t, dir, "progress") {
		t.Fatal("session log did not persist a progress event")
	}
}

func TestSubprocessProgress_ModelTurnsHaveShortSummaries(t *testing.T) {
	binDir := t.TempDir()
	binaryPath := filepath.Join(binDir, "gemini")
	script := `#!/bin/sh
cat <<'EOF'
{"type":"init","model":"gemini-test-model"}
{"type":"message","role":"assistant","content":"subprocess hello","delta":true}
{"type":"result","status":"success","stats":{"total_tokens":9,"input_tokens":4,"output_tokens":5}}
EOF
`
	if err := os.WriteFile(binaryPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake gemini: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	svc, err := fizeau.New(fizeau.ServiceOptions{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	dir := t.TempDir()
	ch, err := svc.Execute(context.Background(), fizeau.ServiceExecuteRequest{
		Prompt:        "subprocess turn",
		Harness:       "gemini",
		Model:         "gemini-2.5-flash",
		SessionLogDir: dir,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	events := drainEvents(t, ch, 5*time.Second)
	progress := progressEvents(events)
	if len(progress) < 4 {
		t.Fatalf("expected route + subprocess thinking + response progress, got %#v", progress)
	}
	if progress[0].Phase != "route" || progress[0].State != "start" {
		t.Fatalf("first progress = %#v, want route start", progress[0])
	}
	if progress[1].Phase != "thinking" || progress[1].State != "start" {
		t.Fatalf("second progress = %#v, want thinking start", progress[1])
	}
	var sawThinkingComplete, sawResponseComplete bool
	for _, p := range progress {
		if len(p.Message) > 80 {
			t.Fatalf("progress message too long: %#v", p)
		}
		if len(p.SessionSummary) > 80 {
			t.Fatalf("progress session summary too long: %#v", p)
		}
		switch {
		case p.Phase == "thinking" && p.State == "complete":
			sawThinkingComplete = true
			if !strings.Contains(p.Message, "thinking complete") {
				t.Fatalf("thinking complete message = %q", p.Message)
			}
		case p.Phase == "response" && p.State == "complete":
			sawResponseComplete = true
			if !strings.Contains(p.Message, "sending response") {
				t.Fatalf("response complete message = %q", p.Message)
			}
		}
	}
	if !sawThinkingComplete || !sawResponseComplete {
		t.Fatalf("missing subprocess thinking/response progress: %#v", progress)
	}
	if !sessionLogHasEventType(t, dir, "progress") {
		t.Fatal("session log did not persist a progress event")
	}
}

func TestExecute_NativeEmitsToolProgress(t *testing.T) {
	fp := &fizeau.FakeProvider{
		Static: []fizeau.FakeResponse{
			{
				ToolCalls: []fizeau.ToolCall{{
					ID:        "tool-1",
					Name:      "bash",
					Arguments: json.RawMessage(`{"command":"echo hi"}`),
				}},
			},
			{
				Text: "done",
				Usage: fizeau.TokenUsage{
					Input:  3,
					Output: 2,
					Total:  5,
				},
			},
		},
	}
	opts := fizeau.ServiceOptions{}
	opts.FakeProvider = fp
	svc, err := fizeau.New(opts)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	dir := t.TempDir()
	ch, err := svc.Execute(context.Background(), fizeau.ServiceExecuteRequest{
		Prompt:        "run tool",
		Harness:       "agent",
		Provider:      "fake",
		Model:         "fake-model",
		WorkDir:       t.TempDir(),
		Permissions:   "unrestricted",
		Tools:         []fizeau.Tool{&progressTool{output: "tool output"}},
		SessionLogDir: dir,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	events := drainEvents(t, ch, 5*time.Second)
	progress := progressEvents(events)
	var sawToolStart, sawToolComplete bool
	for _, p := range progress {
		if p.Phase != "tool" {
			continue
		}
		switch p.State {
		case "start":
			sawToolStart = true
			if p.ToolName != "bash" {
				t.Fatalf("tool start ToolName = %q", p.ToolName)
			}
			if !strings.Contains(p.Command, "echo hi") {
				t.Fatalf("tool start command = %q", p.Command)
			}
			if p.SessionSummary == "" {
				t.Fatal("tool start session summary is empty")
			}
		case "complete":
			sawToolComplete = true
			if p.ToolName != "bash" {
				t.Fatalf("tool complete ToolName = %q", p.ToolName)
			}
			if p.DurationMS <= 0 {
				t.Fatalf("tool complete duration = %dms, want > 0", p.DurationMS)
			}
		}
	}
	if !sawToolStart || !sawToolComplete {
		t.Fatalf("missing tool progress events: %#v", progress)
	}
	if !sessionLogHasEventType(t, dir, "progress") {
		t.Fatal("session log did not persist a progress event")
	}
}

func TestExecute_NativeEmitsContextProgressOnCompaction(t *testing.T) {
	longOutput := strings.Repeat("workspace-output-", 40)
	fp := &fizeau.FakeProvider{
		Static: []fizeau.FakeResponse{
			{
				ToolCalls: []fizeau.ToolCall{{
					ID:        "tool-1",
					Name:      "bash",
					Arguments: json.RawMessage(`{"command":"echo hi"}`),
				}},
			},
			{
				Text: "compaction summary sanitized",
				Usage: fizeau.TokenUsage{
					Input:  7,
					Output: 3,
					Total:  10,
				},
			},
			{
				Text: "final answer",
				Usage: fizeau.TokenUsage{
					Input:  4,
					Output: 1,
					Total:  5,
				},
			},
		},
	}
	opts := fizeau.ServiceOptions{}
	opts.FakeProvider = fp
	svc, err := fizeau.New(opts)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	dir := t.TempDir()
	ch, err := svc.Execute(context.Background(), fizeau.ServiceExecuteRequest{
		Prompt:                  "compact me",
		Harness:                 "agent",
		Provider:                "fake",
		Model:                   "fake-model",
		WorkDir:                 t.TempDir(),
		Permissions:             "unrestricted",
		Tools:                   []fizeau.Tool{&progressTool{output: longOutput}},
		CompactionContextWindow: 80,
		CompactionReserveTokens: 0,
		SessionLogDir:           dir,
		MaxIterations:           4,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	events := drainEvents(t, ch, 10*time.Second)
	progress := progressEvents(events)
	var compaction, contextUpdate *fizeau.ServiceProgressData
	for i := range progress {
		if progress[i].Phase == "compaction" && progress[i].State == "complete" {
			compaction = &progress[i]
		}
		if progress[i].Phase == "context" && progress[i].State == "update" {
			contextUpdate = &progress[i]
		}
	}
	if compaction == nil || contextUpdate == nil {
		t.Fatalf("missing compaction/context progress events: %#v", progress)
	}
	if !strings.Contains(compaction.Message, "compacted") || !strings.Contains(compaction.Message, "freed") {
		t.Fatalf("compaction progress message = %q", compaction.Message)
	}
	if compaction.ContextMessages <= 0 || compaction.ContextTokensEstimate <= 0 {
		t.Fatalf("compaction context counts = %#v", compaction)
	}
	if contextUpdate.SessionSummary == "" {
		t.Fatal("context update session summary is empty")
	}
	if len(contextUpdate.SessionSummary) > 80 {
		t.Fatalf("context update session summary too long: %d chars", len(contextUpdate.SessionSummary))
	}
	if !sessionLogHasEventType(t, dir, "progress") {
		t.Fatal("session log did not persist a progress event")
	}
}

func TestProgressEvent_RedactsUnboundedPayloads(t *testing.T) {
	prompt := strings.Repeat("PROMPT-SECRET-", 16)
	toolOutput := strings.Repeat("TOOL-SECRET-", 32)
	fp := &fizeau.FakeProvider{
		Static: []fizeau.FakeResponse{
			{
				ToolCalls: []fizeau.ToolCall{{
					ID:        "tool-1",
					Name:      "bash",
					Arguments: json.RawMessage(`{"command":"echo hello"}`),
				}},
			},
			{
				Text: "done",
				Usage: fizeau.TokenUsage{
					Input:  2,
					Output: 1,
					Total:  3,
				},
			},
		},
	}
	opts := fizeau.ServiceOptions{}
	opts.FakeProvider = fp
	svc, err := fizeau.New(opts)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	dir := t.TempDir()
	ch, err := svc.Execute(context.Background(), fizeau.ServiceExecuteRequest{
		Prompt:        prompt,
		Harness:       "agent",
		Provider:      "fake",
		Model:         "fake-model",
		WorkDir:       t.TempDir(),
		Permissions:   "unrestricted",
		Tools:         []fizeau.Tool{&progressTool{output: toolOutput}},
		SessionLogDir: dir,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	events := drainEvents(t, ch, 5*time.Second)
	progress := progressEvents(events)
	if len(progress) == 0 {
		t.Fatal("expected at least one progress event")
	}
	for _, p := range progress {
		raw, err := json.Marshal(p)
		if err != nil {
			t.Fatalf("marshal progress: %v", err)
		}
		if strings.Contains(string(raw), prompt) {
			t.Fatalf("progress event leaked full prompt: %s", raw)
		}
		if strings.Contains(string(raw), toolOutput) {
			t.Fatalf("progress event leaked full tool output: %s", raw)
		}
		if len(p.SessionSummary) > 80 {
			t.Fatalf("session summary too long: %d chars", len(p.SessionSummary))
		}
		if len(p.Command) > 120 {
			t.Fatalf("command summary too long: %d chars", len(p.Command))
		}
	}
	if !sessionLogHasEventType(t, dir, "progress") {
		t.Fatal("session log did not persist a progress event")
	}
}
