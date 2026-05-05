package fizeau_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	fizeau "github.com/DocumentDrivenDX/fizeau"
)

func TestPublicServiceAPISmoke(t *testing.T) {
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(fakeHome, ".config"))

	svc, err := fizeau.New(fizeau.ServiceOptions{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	var _ fizeau.FizeauService = svc

	if _, err := svc.ListHarnesses(context.Background()); err != nil {
		t.Fatalf("ListHarnesses: %v", err)
	}

	req := fizeau.ServiceExecuteRequest{
		Prompt:  "ping",
		Harness: "virtual",
		Model:   "test-model",
	}
	if req.Prompt == "" || req.Harness == "" || req.Model == "" {
		t.Fatalf("unexpected request shape: %+v", req)
	}

	tokPerSec := 42.5
	progress, err := json.Marshal(fizeau.ServiceProgressData{
		Phase:     "llm",
		State:     "complete",
		Source:    "native",
		Message:   "complete",
		TokPerSec: &tokPerSec,
	})
	if err != nil {
		t.Fatalf("marshal progress: %v", err)
	}
	event := fizeau.ServiceEvent{
		Type:     fizeau.ServiceEventTypeProgress,
		Sequence: 1,
		Time:     time.Date(2026, 5, 5, 14, 0, 0, 0, time.UTC),
		Metadata: map[string]string{"session_id": "svc-test"},
		Data:     progress,
	}
	if event.Type != fizeau.ServiceEventTypeProgress || len(event.Data) == 0 {
		t.Fatalf("unexpected event shape: %+v", event)
	}
}
