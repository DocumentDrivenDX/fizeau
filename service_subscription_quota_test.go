package fizeau

import (
	"testing"

	"github.com/easel/fizeau/internal/harnesses"
	claudeharness "github.com/easel/fizeau/internal/harnesses/claude"
)

func TestClaudeQuotaMaxUsedPercentUsesMostConstrainedWindow(t *testing.T) {
	snap := &claudeharness.ClaudeQuotaSnapshot{
		FiveHourRemaining: 95,
		FiveHourLimit:     100,
		WeeklyRemaining:   20,
		WeeklyLimit:       100,
	}
	if got := claudeQuotaMaxUsedPercent(snap); got != 80 {
		t.Fatalf("weekly-constrained quota used percent = %.1f, want 80", got)
	}

	snap = &claudeharness.ClaudeQuotaSnapshot{
		FiveHourRemaining: 5,
		FiveHourLimit:     100,
		WeeklyRemaining:   95,
		WeeklyLimit:       100,
	}
	if got := claudeQuotaMaxUsedPercent(snap); got != 95 {
		t.Fatalf("five-hour-constrained quota used percent = %.1f, want 95", got)
	}
}

func TestQuotaWindowMaxUsedPercentUsesMostConstrainedWindow(t *testing.T) {
	windows := []harnesses.QuotaWindow{
		{Name: "5h", UsedPercent: 10},
		{Name: "7d", UsedPercent: 85},
	}
	if got := maxQuotaWindowUsedPercent(windows); got != 85 {
		t.Fatalf("quota window used percent = %.1f, want 85", got)
	}
}
