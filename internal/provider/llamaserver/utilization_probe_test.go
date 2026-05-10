package llamaserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/easel/fizeau/internal/provider/testutil"
	"github.com/easel/fizeau/internal/provider/utilization"
	"github.com/stretchr/testify/require"
	"gopkg.in/dnaeon/go-vcr.v4/pkg/recorder"
)

func TestLlamaServerUtilizationProbe_CassetteReplay(t *testing.T) {
	if testutil.ModeForEnvironment() == recorder.ModeRecordOnly {
		t.Skip("cassette replay coverage is exercised in the existing record-mode test")
	}

	rec, err := testutil.NewRecorder(testutil.CassettePath("testdata/cassettes", llamaCassetteName))
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, rec.Stop())
	})

	probe := NewUtilizationProbe("http://replay.invalid/v1", rec.GetDefaultClient())
	sample := probe.Probe(context.Background())

	require.Equal(t, utilization.SourceLlamaMetrics, sample.Source)
	require.Equal(t, utilization.FreshnessFresh, sample.Freshness)
	require.NotNil(t, sample.ActiveRequests)
	require.NotNil(t, sample.QueuedRequests)
	require.Zero(t, *sample.ActiveRequests)
	require.Zero(t, *sample.QueuedRequests)
	require.Nil(t, sample.CacheUsage)
	require.NotZero(t, sample.ObservedAt)
}

func TestLlamaServerUtilizationProbe_FallsBackToSlots(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/metrics":
			http.Error(w, "metrics disabled", http.StatusNotFound)
		case "/slots":
			_, _ = w.Write([]byte(`{"slots":[{"is_processing":true},{"is_processing":false},{"is_processing":true}],"slot_count":3}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	probe := NewUtilizationProbe(srv.URL+"/v1", srv.Client())
	sample := probe.Probe(context.Background())

	require.Equal(t, utilization.SourceLlamaSlots, sample.Source)
	require.Equal(t, utilization.FreshnessFresh, sample.Freshness)
	require.NotNil(t, sample.ActiveRequests)
	require.NotNil(t, sample.MaxConcurrency)
	require.NotNil(t, sample.CacheUsage)
	require.Equal(t, 2, *sample.ActiveRequests)
	require.Equal(t, 3, *sample.MaxConcurrency)
	require.InDelta(t, 2.0/3.0, *sample.CacheUsage, 1e-9)
	require.Nil(t, sample.QueuedRequests)
}

func TestLlamaServerUtilizationProbe_UnknownOnInitialFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, strings.TrimPrefix(r.URL.Path, "/"), http.StatusServiceUnavailable)
	}))
	t.Cleanup(srv.Close)

	probe := NewUtilizationProbe(srv.URL+"/v1", srv.Client())
	sample := probe.Probe(context.Background())

	require.Equal(t, utilization.FreshnessUnknown, sample.Freshness)
	require.Equal(t, utilization.SourceLlamaMetrics, sample.Source)
	require.Nil(t, sample.ActiveRequests)
	require.Nil(t, sample.QueuedRequests)
	require.Nil(t, sample.CacheUsage)
	require.Nil(t, sample.MaxConcurrency)
}
