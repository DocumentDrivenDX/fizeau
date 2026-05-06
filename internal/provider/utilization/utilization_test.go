package utilization

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestServerRootStripsTrailingV1(t *testing.T) {
	require.Equal(t, "http://host:8000", ServerRoot("http://host:8000/v1"))
	require.Equal(t, "http://host:8000", ServerRoot("http://host:8000/v1/"))
	require.Equal(t, "http://host:8000/base", ServerRoot("http://host:8000/base/v1"))
}

func TestCacheReturnsFreshThenStale(t *testing.T) {
	var cache Cache
	fresh := cache.Remember(EndpointUtilization{
		ActiveRequests: Int(2),
		QueuedRequests: Int(3),
		CacheUsage:     Float64(0.25),
		MaxConcurrency: Int(4),
		Source:         SourceVLLMMetrics,
	})
	require.Equal(t, FreshnessFresh, fresh.Freshness)
	require.NotZero(t, fresh.ObservedAt)

	stale, ok := cache.Stale()
	require.True(t, ok)
	require.Equal(t, FreshnessStale, stale.Freshness)
	require.Equal(t, fresh.ObservedAt, stale.ObservedAt)
	require.NotNil(t, stale.ActiveRequests)
	require.Equal(t, 2, *stale.ActiveRequests)
}
