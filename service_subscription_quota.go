package fizeau

import (
	"time"

	"github.com/easel/fizeau/internal/harnesses"
	quotaimpl "github.com/easel/fizeau/internal/quota"
)

type subscriptionQuotaView = quotaimpl.SubscriptionView

func subscriptionQuotaForHarness(name string, now time.Time) (subscriptionQuotaView, bool) {
	return quotaimpl.SubscriptionForHarness(name, now)
}

func maxQuotaWindowUsedPercent(windows []harnesses.QuotaWindow) float64 {
	return quotaimpl.MaxQuotaWindowUsedPercent(windows)
}

func quotaTrend(percentUsed int, fresh bool) string {
	return quotaimpl.TrendFromUsage(percentUsed, fresh)
}
