//go:build testseam

package fizeau

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRouteProgressData_IncludesEconomicsWhenPresent(t *testing.T) {
	payload := routeProgressData(RouteDecision{
		Harness:  "agent",
		Provider: "alpha",
		Model:    "model-a",
		Power:    7,
		Candidates: []RouteCandidate{{
			Harness:            "agent",
			Provider:           "alpha",
			Model:              "model-a",
			CostUSDPer1kTokens: 0.012,
			CostSource:         "catalog",
			Components: RouteCandidateComponents{
				Power:     7,
				SpeedTPS:  55,
				CostClass: "local",
			},
		}},
	})
	if payload.Phase != "route" || payload.State != "start" {
		t.Fatalf("payload=%#v, want route start", payload)
	}
	if payload.Message == "" {
		t.Fatal("route progress message is empty")
	}
	for _, want := range []string{"agent/alpha/model-a", "power=", "speed=", "cost=", "cost_source="} {
		if !strings.Contains(payload.Message, want) {
			t.Fatalf("route progress message %q missing %q", payload.Message, want)
		}
	}
	if len(payload.Message) > 80 {
		t.Fatalf("route progress message too long: %d", len(payload.Message))
	}
	if payload.SessionSummary != payload.Message {
		t.Fatalf("session summary=%q, want same as message %q", payload.SessionSummary, payload.Message)
	}
}

func TestSummarizeToolTask_TrivialModelSamples(t *testing.T) {
	cases := []struct {
		name       string
		tool       string
		input      string
		wantAction string
		wantTarget string
	}{
		{
			name:       "native read",
			tool:       "read",
			input:      `{"path":"cli/internal/file.go","offset":21,"limit":20}`,
			wantAction: "inspect lines 22-41 in cli/internal/file.go",
			wantTarget: "cli/internal/file.go",
		},
		{
			name:       "native write",
			tool:       "write",
			input:      `{"path":"cli/internal/file.go","content":"package cli"}`,
			wantAction: "write file",
			wantTarget: "cli/internal/file.go",
		},
		{
			name:       "shell sed",
			tool:       "bash",
			input:      `{"command":"sed -n '240,320p' cli/internal/agent/session_log_format.go"}`,
			wantAction: "inspect lines 240,320 in cli/internal/agent/session_log_format.go",
			wantTarget: "cli/internal/agent/session_log_format.go",
		},
		{
			name:       "shell grep",
			tool:       "bash",
			input:      `{"command":"rg -n \"FormatSessionLogLines\" cli/internal/agent/session_log_format_test.go"}`,
			wantAction: `search "FormatSessionLogLines" in cli/internal/agent/session_log_format_test.go`,
			wantTarget: "cli/internal/agent/session_log_format_test.go",
		},
		{
			name:       "shell go test",
			tool:       "bash",
			input:      `{"command":"go test ./internal/agent -run TestFormatSessionLogLines"}`,
			wantAction: "test ./internal/agent -run TestFormatSessionLogLines",
			wantTarget: "./internal/agent -run TestFormatSessionLogLines",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := summarizeToolTask(tc.tool, json.RawMessage(tc.input))
			if got.Action != tc.wantAction {
				t.Fatalf("Action=%q, want %q", got.Action, tc.wantAction)
			}
			if got.Target != tc.wantTarget {
				t.Fatalf("Target=%q, want %q", got.Target, tc.wantTarget)
			}
		})
	}
}
