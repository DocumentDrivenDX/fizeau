package anthropic_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestArchitectureDocCachingSection asserts the architecture.md doc has a
// Caching section with the four required bullet points. Failing this test
// means the design doc went out of sync with the implementation.
func TestArchitectureDocCachingSection(t *testing.T) {
	_, here, _, ok := runtime.Caller(0)
	require.True(t, ok, "could not resolve test file path")
	root := filepath.Clean(filepath.Join(filepath.Dir(here), "..", "..", ".."))
	docPath := filepath.Join(root, "docs", "helix", "02-design", "architecture.md")

	raw, err := os.ReadFile(docPath)
	require.NoError(t, err, "architecture.md not found at %s", docPath)
	doc := string(raw)

	assert.Contains(t, doc, "## Caching", "architecture.md must have a '## Caching' section header")

	requiredBullets := []string{
		"Prefix order invariant",
		"Two-marker placement",
		"Compaction caveat",
		"Tool-mutation caveat",
	}
	for _, b := range requiredBullets {
		assert.True(t, strings.Contains(doc, b),
			"architecture.md Caching section must mention %q", b)
	}
}
