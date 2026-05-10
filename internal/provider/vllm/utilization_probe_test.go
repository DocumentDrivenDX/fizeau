package vllm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/easel/fizeau/internal/provider/testutil"
	"github.com/easel/fizeau/internal/provider/utilization"
	"github.com/stretchr/testify/require"
	"gopkg.in/dnaeon/go-vcr.v4/pkg/recorder"
)

func TestVLLMUtilizationProbe_CassetteReplay(t *testing.T) {
	if testutil.ModeForEnvironment() == recorder.ModeRecordOnly {
		t.Skip("cassette replay coverage is exercised in the existing record-mode test")
	}

	rec, err := testutil.NewRecorder(testutil.CassettePath("testdata/cassettes", vllmCassetteName))
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, rec.Stop())
	})

	probe := NewUtilizationProbe("http://replay.invalid/v1", rec.GetDefaultClient())
	sample := probe.Probe(context.Background())

	require.Equal(t, utilization.SourceVLLMMetrics, sample.Source)
	require.Equal(t, utilization.FreshnessFresh, sample.Freshness)
	require.NotNil(t, sample.ActiveRequests)
	require.NotNil(t, sample.QueuedRequests)
	require.NotNil(t, sample.CacheUsage)
	require.Zero(t, *sample.ActiveRequests)
	require.Zero(t, *sample.QueuedRequests)
	require.NotZero(t, sample.ObservedAt)
}

func TestVLLMUtilizationProbe_FailureReturnsStaleOrUnknown(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/metrics", r.URL.Path)
		hits++
		if hits == 1 {
			_, _ = w.Write([]byte("vllm:num_requests_running 2\nvllm:num_requests_waiting 1\nvllm:kv_cache_usage_perc 0.5\n"))
			return
		}
		http.Error(w, "boom", http.StatusServiceUnavailable)
	}))
	t.Cleanup(srv.Close)

	probe := NewUtilizationProbe(srv.URL+"/v1", srv.Client())

	fresh := probe.Probe(context.Background())
	require.Equal(t, utilization.FreshnessFresh, fresh.Freshness)
	require.NotNil(t, fresh.ActiveRequests)
	require.Equal(t, 2, *fresh.ActiveRequests)

	stale := probe.Probe(context.Background())
	require.Equal(t, utilization.FreshnessStale, stale.Freshness)
	require.NotNil(t, stale.ActiveRequests)
	require.Equal(t, 2, *stale.ActiveRequests)
	require.Equal(t, fresh.ObservedAt, stale.ObservedAt)
}
