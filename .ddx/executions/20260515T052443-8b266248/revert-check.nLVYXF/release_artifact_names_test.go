package fizeau_test

import (
	"os"
	"strings"
	"testing"
)

// TestReleaseWorkflowArtifactNamesFiz asserts that the release workflow sets
// BINARY_NAME to "fiz", which drives all upload-artifact and download-artifact
// names. This is a static guard: the test fails if someone renames the binary
// back to a legacy name without updating the release workflow.
func TestReleaseWorkflowArtifactNamesFiz(t *testing.T) {
	const path = ".github/workflows/release.yml"
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	body := string(raw)

	if !strings.Contains(body, "BINARY_NAME: fiz") {
		t.Fatalf("%s: expected BINARY_NAME to be set to \"fiz\" for artifact naming; got none", path)
	}
}

// TestReleaseWorkflowNoLegacyArtifactLiterals asserts that no literal
// "ddx-agent-" string appears as a hard-coded artifact name in the release
// workflow. Artifact names must use the BINARY_NAME variable so they stay
// consistent with the canonical binary name.
func TestReleaseWorkflowNoLegacyArtifactLiterals(t *testing.T) {
	const path = ".github/workflows/release.yml"
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	body := string(raw)

	if strings.Contains(body, "ddx-agent-") {
		t.Fatalf("%s: found legacy artifact name literal \"ddx-agent-\"; use BINARY_NAME variable instead", path)
	}
}
