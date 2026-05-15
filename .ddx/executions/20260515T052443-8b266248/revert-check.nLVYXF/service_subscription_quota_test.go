package fizeau

import (
	"testing"

	"github.com/easel/fizeau/internal/harnesses"
)

func TestQuotaWindowMaxUsedPercentUsesMostConstrainedWindow(t *testing.T) {
	windows := []harnesses.QuotaWindow{
		{Name: "5h", UsedPercent: 10},
		{Name: "7d", UsedPercent: 85},
	}
	if got := maxQuotaWindowUsedPercent(windows); got != 85 {
		t.Fatalf("quota window used percent = %.1f, want 85", got)
	}
}
