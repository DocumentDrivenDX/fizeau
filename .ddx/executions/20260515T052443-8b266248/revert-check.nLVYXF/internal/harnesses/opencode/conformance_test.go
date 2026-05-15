package opencode_test

import (
	"testing"

	"github.com/easel/fizeau/internal/harnesses/harnesstest"
	"github.com/easel/fizeau/internal/harnesses/opencode"
)

func TestOpenCodeRunnerHarnessConformance(t *testing.T) {
	harnesstest.RunHarnessConformance(t, &opencode.Runner{})
}

func TestOpenCodeRunnerModelDiscoveryConformance(t *testing.T) {
	harnesstest.RunModelDiscoveryHarnessConformance(t, &opencode.Runner{})
}
