package fizeau

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	agentcore "github.com/DocumentDrivenDX/fizeau/internal/core"
	"github.com/DocumentDrivenDX/fizeau/internal/harnesses"
)

func TestServiceSessionLogPersistsReasoningStallEvent(t *testing.T) {
	dir := t.TempDir()
	sessionID := "reasoning-stall-session"
	svc := &service{}
	sl := svc.openSessionLog(ServiceExecuteRequest{SessionLogDir: dir}, RouteDecision{}, sessionID)

	payload := map[string]any{
		"code":           agentcore.ReasoningStallCode,
		"model":          "qwen-test",
		"timeout_ms":     50,
		"reasoning_tail": "thinking tail",
		"prompt_id":      "prompt-1",
	}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	sl.writeEvent(agentcore.Event{
		SessionID: sessionID,
		Seq:       1,
		Type:      agentcore.EventReasoningStall,
		Timestamp: time.Now().UTC(),
		Data:      data,
	})
	sl.close()

	body, err := os.ReadFile(filepath.Join(dir, sessionID+".jsonl"))
	if err != nil {
		t.Fatalf("read session log: %v", err)
	}
	text := string(body)
	for _, want := range []string{
		`"type":"reasoning.stall"`,
		`"code":"REASONING_STALL"`,
		`"reasoning_tail":"thinking tail"`,
		`"prompt_id":"prompt-1"`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("session log missing %s:\n%s", want, text)
		}
	}
}

func TestServiceSessionLogPersistsHarnessProvenance(t *testing.T) {
	dir := t.TempDir()
	sessionID := "routing-provenance-session"
	svc := &service{}
	req := ServiceExecuteRequest{
		SessionLogDir: dir,
		Model:         "sonnet",
		Prompt:        "test prompt",
	}
	sl := svc.openSessionLog(req, RouteDecision{
		Harness:  "claude",
		Provider: "claude",
		Model:    "sonnet",
	}, sessionID)
	sl.writeEnd(req, nil, harnesses.FinalData{
		Status: string(agentcore.StatusSuccess),
		RoutingActual: &harnesses.RoutingActual{
			Harness: "claude",
			Model:   "sonnet",
		},
	})
	sl.close()

	body, err := os.ReadFile(filepath.Join(dir, sessionID+".jsonl"))
	if err != nil {
		t.Fatalf("read session log: %v", err)
	}
	text := string(body)
	for _, want := range []string{
		`"type":"session.start"`,
		`"type":"session.end"`,
		`"resolved_harness":"claude"`,
		`"harness_source":"auto_route"`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("session log missing %s:\n%s", want, text)
		}
	}
	if strings.Contains(text, `"requested_harness"`) {
		t.Fatalf("session log unexpectedly recorded requested_harness for auto route:\n%s", text)
	}
}
