package fizeau

import (
	"context"
	"time"

	"github.com/easel/fizeau/internal/harnesses"
	claudeharness "github.com/easel/fizeau/internal/harnesses/claude"
	codexharness "github.com/easel/fizeau/internal/harnesses/codex"
	geminiharness "github.com/easel/fizeau/internal/harnesses/gemini"
	"github.com/easel/fizeau/internal/routing"
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
		status, err := (&claudeharness.Runner{}).QuotaStatus(context.Background(), now)
		if err != nil {
			return subscriptionQuotaView{}, false
		}
		present := status.State != harnesses.QuotaUnavailable
		view := subscriptionQuotaView{
			OK:      status.RoutingPreference == harnesses.RoutingPreferenceAvailable,
			Present: present,
			Fresh:   status.Fresh,
			Reason:  status.Reason,
			Trend:   routing.QuotaTrendUnknown,
		}
		if present {
			view.Windows = append([]harnesses.QuotaWindow(nil), status.Windows...)
			view.PercentUsed = int(maxQuotaWindowUsedPercent(status.Windows))
			view.Trend = quotaTrend(view.PercentUsed, status.Fresh)
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
