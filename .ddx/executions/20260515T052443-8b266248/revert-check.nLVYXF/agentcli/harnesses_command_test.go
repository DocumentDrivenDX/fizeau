package agentcli

import (
	"encoding/json"
	"testing"
)

func TestHarnessesCommandJSON(t *testing.T) {
	stdout, stderr, code := captureStdIO(t, func() int {
		return cmdHarnesses(t.TempDir(), true)
	})
	if code != 0 {
		t.Fatalf("cmdHarnesses exit=%d stderr=%s stdout=%s", code, stderr, stdout)
	}

	var rows []map[string]any
	if err := json.Unmarshal([]byte(stdout), &rows); err != nil {
		t.Fatalf("Unmarshal harnesses JSON: %v\n%s", err, stdout)
	}
	if len(rows) == 0 {
		t.Fatalf("harnesses JSON was empty: %s", stdout)
	}
	for _, row := range rows {
		if _, ok := row["name"].(string); !ok {
			t.Fatalf("harness row missing name: %#v", row)
		}
		if _, ok := row["billing"].(string); !ok {
			t.Fatalf("harness row missing billing: %#v", row)
		}
	}
}
