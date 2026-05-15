package fizeau

import (
	"context"
	"time"

	"github.com/easel/fizeau/internal/harnesses"
	"github.com/easel/fizeau/internal/harnesses/builtin"
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
	qh, ok := builtin.New(name).(harnesses.QuotaHarness)
	if !ok {
		return subscriptionQuotaView{}, false
	}
	status, err := qh.QuotaStatus(context.Background(), now)
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
	if !present {
		return view, true
	}
	view.Windows = append([]harnesses.QuotaWindow(nil), status.Windows...)
	view.PercentUsed = int(maxQuotaWindowUsedPercent(status.Windows))
	if name == "gemini" {
		exhausted, available := 0, 0
		for _, w := range status.Windows {
			switch w.State {
			case "blocked":
				exhausted++
			case "ok":
				available++
			}
		}
		if exhausted > 0 && available == 0 {
			view.Trend = routing.QuotaTrendExhausting
			return view, true
		}
	}
	view.Trend = quotaTrend(view.PercentUsed, status.Fresh)
	return view, true
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
