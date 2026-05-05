package serviceimpl

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestRunVirtualInlineResponseBuildsFinalData(t *testing.T) {
	result := RunVirtual(context.Background(), ExecuteRunnerRequest{
		Prompt: "hello",
		Metadata: map[string]string{
			"virtual.response":      "virtual ok",
			"virtual.input_tokens":  "3",
			"virtual.output_tokens": "4",
			"virtual.total_tokens":  "7",
			"virtual.model":         "virtual-model",
		},
		Decision: ExecuteRunnerDecision{
			Harness:  "virtual",
			Provider: "virtual",
			Model:    "fallback-model",
		},
		Started: time.Now(),
	})

	if result.Final.Status != "success" {
		t.Fatalf("status = %q, want success", result.Final.Status)
	}
	if !result.EmitText || result.Text != "virtual ok" || result.Final.FinalText != "virtual ok" {
		t.Fatalf("text result = (%v, %q, %q), want emitted virtual ok", result.EmitText, result.Text, result.Final.FinalText)
	}
	if result.Final.RoutingActual == nil || result.Final.RoutingActual.Model != "virtual-model" {
		t.Fatalf("routing actual = %#v, want virtual-model", result.Final.RoutingActual)
	}
	if result.Final.Usage == nil || result.Final.Usage.TotalTokens == nil || *result.Final.Usage.TotalTokens != 7 {
		t.Fatalf("usage = %#v, want total tokens 7", result.Final.Usage)
	}
}

func TestRunScriptHandlesExitCodeAndStderr(t *testing.T) {
	result := RunScript(context.Background(), ExecuteRunnerRequest{
		Metadata: map[string]string{
			"script.stdout":    "partial output",
			"script.stderr":    "script failed",
			"script.exit_code": "2",
		},
		Decision: ExecuteRunnerDecision{Harness: "script", Model: "script"},
		Started:  time.Now(),
	})

	if result.Final.Status != "failed" {
		t.Fatalf("status = %q, want failed", result.Final.Status)
	}
	if result.Final.ExitCode != 2 || result.Final.Error != "script failed" {
		t.Fatalf("final = %#v, want exit code 2 with stderr", result.Final)
	}
	if !result.EmitText || result.Text != "partial output" {
		t.Fatalf("text result = (%v, %q), want emitted partial output", result.EmitText, result.Text)
	}
}

func TestRunScriptDelayCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result := RunScript(ctx, ExecuteRunnerRequest{
		Metadata: map[string]string{
			"script.delay_ms": "1000",
			"script.stdout":   "late",
		},
		Decision: ExecuteRunnerDecision{Harness: "script", Model: "script"},
		Started:  time.Now(),
	})

	if result.Final.Status != "cancelled" {
		t.Fatalf("status = %q, want cancelled", result.Final.Status)
	}
	if !strings.Contains(result.Final.Error, "context canceled") {
		t.Fatalf("error = %q, want context canceled", result.Final.Error)
	}
	if result.EmitText {
		t.Fatal("cancelled script emitted text")
	}
}
