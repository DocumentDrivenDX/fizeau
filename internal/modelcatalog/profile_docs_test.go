package modelcatalog

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRoutingLegacyNamesDocPointsToPowerRouting(t *testing.T) {
	doc := readRoutingProfilesDoc(t)
	required := []string{
		"numeric power",
		"--min-power",
		"--max-power",
		"--model",
		"--provider",
		"--harness",
		"fiz --list-models",
		"compatibility metadata",
		"not the primary routing surface",
	}
	for _, text := range required {
		if !strings.Contains(doc, text) {
			t.Errorf("docs/routing/profiles.md missing %q", text)
		}
	}
	if strings.Contains(doc, "### `") {
		t.Fatal("docs/routing/profiles.md must not enumerate legacy routing names")
	}
}

func readRoutingProfilesDoc(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	data, err := os.ReadFile(filepath.Join(repoRoot, "docs", "routing", "profiles.md"))
	if err != nil {
		t.Fatalf("read docs/routing/profiles.md: %v", err)
	}
	return string(data)
}
