package agent_test

import (
	"os"
	"strings"
	"testing"
)

// TestContract003_EmptyFinalTextProseExists asserts that CONTRACT-003 prose
// states that "success + empty final_text" is a valid outcome, so consumers
// know not to retry on empty text alone. The test grep-asserts the prose so
// future edits cannot silently drop the convention without failing CI.
func TestContract003_EmptyFinalTextProseExists(t *testing.T) {
	const path = "docs/helix/02-design/contracts/CONTRACT-003-ddx-agent-service.md"
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	body := string(raw)

	// Required phrases (case-insensitive). The set captures the load-bearing
	// claims: empty final_text is a valid outcome, and consumers must not
	// retry on empty text alone.
	required := []string{
		"empty `final_text`",
		"valid outcome",
		"MUST NOT",
		"retry",
	}
	lower := strings.ToLower(body)
	for _, want := range required {
		if !strings.Contains(lower, strings.ToLower(want)) {
			t.Fatalf("CONTRACT-003 missing prose phrase %q (empty final_text convention)", want)
		}
	}
}
