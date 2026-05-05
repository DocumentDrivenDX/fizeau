package fizeau

import (
	"time"

	"github.com/DocumentDrivenDX/fizeau/internal/harnesses"
	claudeharness "github.com/DocumentDrivenDX/fizeau/internal/harnesses/claude"
	codexharness "github.com/DocumentDrivenDX/fizeau/internal/harnesses/codex"
	geminiharness "github.com/DocumentDrivenDX/fizeau/internal/harnesses/gemini"
	"github.com/DocumentDrivenDX/fizeau/internal/routing"
)

type subscriptionQuotaView struct {
	OK          bool
	Present     bool
	Fresh       bool
	Windows     []harnesses.QuotaWindow
	PercentUsed int
	Trend       string
	Reason      string
}

func subscriptionQuotaForHarness(name string, now time.Time) (subscriptionQuotaView, bool) {
	switch name {
	case "claude":
		dec := claudeharness.ReadClaudeQuotaRoutingDecision(now, 0)
		view := subscriptionQuotaView{
			OK:      dec.PreferClaude,
			Present: dec.SnapshotPresent,
			Fresh:   dec.Fresh,
			Reason:  dec.Reason,
			Trend:   routing.QuotaTrendUnknown,
		}
		if dec.Snapshot != nil {
			view.Windows = append([]harnesses.QuotaWindow(nil), dec.Snapshot.Windows...)
			view.PercentUsed = int(claudeQuotaMaxUsedPercent(dec.Snapshot))
			view.Trend = quotaTrend(view.PercentUsed, dec.Fresh)
		}
		return view, true
	case "codex":
		dec := codexharness.ReadCodexQuotaRoutingDecision(now, 0)
		view := subscriptionQuotaView{
			OK:      dec.PreferCodex,
			Present: dec.SnapshotPresent,
			Fresh:   dec.Fresh,
			Reason:  dec.Reason,
			Trend:   routing.QuotaTrendUnknown,
		}
		if dec.Snapshot != nil {
			view.Windows = append([]harnesses.QuotaWindow(nil), dec.Snapshot.Windows...)
			view.PercentUsed = int(maxQuotaWindowUsedPercent(dec.Snapshot.Windows))
			view.Trend = quotaTrend(view.PercentUsed, dec.Fresh)
		}
		return view, true
	case "gemini":
		dec := geminiharness.ReadGeminiQuotaRoutingDecision(now, 0)
		view := subscriptionQuotaView{
			OK:      dec.PreferGemini,
			Present: dec.SnapshotPresent,
			Fresh:   dec.Fresh,
			Reason:  dec.Reason,
			Trend:   routing.QuotaTrendUnknown,
		}
		if dec.Snapshot != nil {
			view.Windows = append([]harnesses.QuotaWindow(nil), dec.Snapshot.Windows...)
			view.PercentUsed = int(dec.Snapshot.MaxUsedPercent())
			if len(dec.ExhaustedTiers) > 0 && len(dec.AvailableTiers) == 0 {
				view.Trend = routing.QuotaTrendExhausting
			} else {
				view.Trend = quotaTrend(view.PercentUsed, dec.Fresh)
			}
		}
		return view, true
	default:
		return subscriptionQuotaView{}, false
	}
}

func claudeQuotaMaxUsedPercent(snap *claudeharness.ClaudeQuotaSnapshot) float64 {
	if snap == nil {
		return 0
	}
	maxUsed := 0.0
	if snap.FiveHourLimit > 0 {
		maxUsed = float64(snap.FiveHourLimit-snap.FiveHourRemaining) / float64(snap.FiveHourLimit) * 100
	}
	if snap.WeeklyLimit > 0 {
		weeklyUsed := float64(snap.WeeklyLimit-snap.WeeklyRemaining) / float64(snap.WeeklyLimit) * 100
		if weeklyUsed > maxUsed {
			maxUsed = weeklyUsed
		}
	}
	if windowMax := maxQuotaWindowUsedPercent(snap.Windows); windowMax > maxUsed {
		maxUsed = windowMax
	}
	return maxUsed
}

func maxQuotaWindowUsedPercent(windows []harnesses.QuotaWindow) float64 {
	maxUsed := 0.0
	for _, window := range windows {
		if window.UsedPercent > maxUsed {
			maxUsed = window.UsedPercent
		}
	}
	return maxUsed
}

func quotaTrend(percentUsed int, fresh bool) string {
	switch {
	case percentUsed >= 90:
		return routing.QuotaTrendExhausting
	case percentUsed >= 70:
		return routing.QuotaTrendBurning
	case fresh:
		return routing.QuotaTrendHealthy
	default:
		return routing.QuotaTrendUnknown
	}
}
