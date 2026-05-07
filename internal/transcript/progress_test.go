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
	want := "ddx-1234 #22 +32s go test ./cmd/bench"
	if got != want {
		t.Fatalf("StatusLine()=%q, want %q", got, want)
	}
}

func TestCompactElapsedUsesSecondGranularityUnderEightChars(t *testing.T) {
	cases := []struct {
		name string
		ms   int64
		want string
	}{
		{name: "subsecond rounds up", ms: 250, want: "+1s"},
		{name: "seconds", ms: 31_624, want: "+32s"},
		{name: "minutes", ms: 4_321_000, want: "+1h12m"},
		{name: "double-digit hours", ms: 36_000_000, want: "+10h"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := CompactElapsed(tc.ms)
			if got != tc.want {
				t.Fatalf("CompactElapsed(%d)=%q, want %q", tc.ms, got, tc.want)
			}
			if len(got) >= 8 {
				t.Fatalf("CompactElapsed(%d)=%q has len %d, want <8", tc.ms, got, len(got))
			}
		})
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
