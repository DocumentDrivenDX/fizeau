package modelcatalog

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestPolicyDocsAvoidLegacyPolicyNames(t *testing.T) {
	doc := readRoutingPoliciesDoc(t)
	required := []string{
		"--policy",
		"--min-power",
		"--max-power",
		"--model",
		"--provider",
		"--harness",
		"fiz policies",
		"include_by_default",
		"no_remote",
	}
	for _, text := range required {
		if !strings.Contains(doc, text) {
			t.Errorf("docs/routing/policies.md missing %q", text)
		}
	}
	for _, legacy := range []string{"fast", "code-fast", "code-economy", "code-smart", "standard", "local", "offline", "code-high", "code-medium"} {
		token := "`" + legacy + "`"
		if strings.Contains(doc, token) {
			t.Fatalf("docs/routing/policies.md must not mention legacy policy token %q", token)
		}
	}
}

func readRoutingPoliciesDoc(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	data, err := os.ReadFile(filepath.Join(repoRoot, "docs", "routing", "policies.md"))
	if err != nil {
		t.Fatalf("read docs/routing/policies.md: %v", err)
	}
	return string(data)
}
