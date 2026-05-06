package transcript

import "testing"

func TestStatusLineIncludesTaskAndTurn(t *testing.T) {
	got := StatusLine(StatusLineInput{
		TaskID:    "ddx-1234",
		TurnIndex: 22,
		Message:   "add test implementation to cli/internal/file.go",
		Limit:     DefaultLineLimit,
	})
	want := "ddx-1234 #22 add test implementation to cli/internal/file.go"
	if got != want {
		t.Fatalf("StatusLine()=%q, want %q", got, want)
	}
}

func TestStatusLineIncludesElapsedSincePreviousUpdate(t *testing.T) {
	got := StatusLine(StatusLineInput{
		TaskID:      "ddx-1234",
		TurnIndex:   22,
		Message:     "go test ./cmd/bench",
		SinceLastMS: 31624,
		Limit:       DefaultLineLimit,
	})
	want := "ddx-1234 #22 +31.624s go test ./cmd/bench"
	if got != want {
		t.Fatalf("StatusLine()=%q, want %q", got, want)
	}
}

func TestTaskIDPrefersOperatorMetadata(t *testing.T) {
	got := TaskID("session-1", map[string]string{
		"task_id":        "",
		"bead_id":        "ddx-1234",
		"correlation_id": "corr-1",
	})
	if got != "ddx-1234" {
		t.Fatalf("TaskID()=%q, want bead id", got)
	}
}

func TestBoundedText(t *testing.T) {
	got := BoundedText("abcdefghijklmnopqrstuvwxyz", 10)
	if got != "abcdefg..." {
		t.Fatalf("BoundedText()=%q, want ellipsis", got)
	}
}
