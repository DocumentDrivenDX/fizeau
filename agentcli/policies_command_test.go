package agentcli

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPoliciesCommandJSON(t *testing.T) {
	stdout, stderr, code := captureStdIO(t, func() int {
		return cmdPolicies(t.TempDir(), true)
	})
	if code != 0 {
		t.Fatalf("cmdPolicies exit=%d stderr=%s stdout=%s", code, stderr, stdout)
	}

	var rows []policyCommandRow
	if err := json.Unmarshal([]byte(stdout), &rows); err != nil {
		t.Fatalf("Unmarshal policies JSON: %v\n%s", err, stdout)
	}
	byName := map[string]policyCommandRow{}
	for _, row := range rows {
		byName[row.Name] = row
	}
	for _, name := range []string{"air-gapped", "cheap", "default", "smart"} {
		if _, ok := byName[name]; !ok {
			t.Fatalf("missing policy %q in %#v", name, rows)
		}
	}
	if byName["smart"].MinPower == 0 || byName["smart"].MaxPower == 0 {
		t.Fatalf("smart policy missing power bounds: %#v", byName["smart"])
	}
	if len(byName["air-gapped"].Require) == 0 {
		t.Fatalf("air-gapped policy missing require list: %#v", byName["air-gapped"])
	}
}

func TestPoliciesCommandText(t *testing.T) {
	stdout, stderr, code := captureStdIO(t, func() int {
		return cmdPolicies(t.TempDir(), false)
	})
	if code != 0 {
		t.Fatalf("cmdPolicies exit=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	if !strings.Contains(stdout, "NAME") || !strings.Contains(stdout, "MIN") || !strings.Contains(stdout, "MAX") {
		t.Fatalf("policies text missing header: %s", stdout)
	}
	for _, name := range []string{"air-gapped", "cheap", "default", "smart"} {
		if !strings.Contains(stdout, name) {
			t.Fatalf("policies text missing %q: %s", name, stdout)
		}
	}
}
