package pi_test

import (
	"testing"

	"github.com/easel/fizeau/internal/harnesses/harnesstest"
	"github.com/easel/fizeau/internal/harnesses/pi"
)

func TestPiRunnerHarnessConformance(t *testing.T) {
	harnesstest.RunHarnessConformance(t, &pi.Runner{})
}

func TestPiRunnerModelDiscoveryConformance(t *testing.T) {
	harnesstest.RunModelDiscoveryHarnessConformance(t, &pi.Runner{})
}
