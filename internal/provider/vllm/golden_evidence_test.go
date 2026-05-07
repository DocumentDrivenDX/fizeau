package vllm

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/fizeau/internal/provider/testutil"
	"github.com/DocumentDrivenDX/fizeau/internal/provider/utilization"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/dnaeon/go-vcr.v4/pkg/recorder"
)

// TestVLLMGoldenUtilizationEvidenceShape verifies the stable JSON evidence
// shape that vLLM endpoints expose through the utilization probe. It uses the
// pre-recorded cassette so results are deterministic and network-independent.
// The evidence shape is what operators see in route-status --json and
// list-models --json (source, freshness, active/queued counts, cache).
func TestVLLMGoldenUtilizationEvidenceShape(t *testing.T) {
	if testutil.ModeForEnvironment() == recorder.ModeRecordOnly {
		t.Skip("evidence shape test uses cassette replay only")
	}

	rec, err := testutil.NewRecorder(testutil.CassettePath("testdata/cassettes", vllmCassetteName))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, rec.Stop()) })

	probe := NewUtilizationProbe("http://replay.invalid/v1", rec.GetDefaultClient())
	sample := probe.Probe(context.Background())

	// Required evidence fields present in route-status --json output.
	require.Equal(t, utilization.SourceVLLMMetrics, sample.Source, "source must identify the vLLM metrics probe")
	require.Equal(t, utilization.FreshnessFresh, sample.Freshness, "freshness must be fresh from a live cassette")
	require.NotNil(t, sample.ActiveRequests, "active_requests must be present in vLLM evidence")
	require.NotNil(t, sample.QueuedRequests, "queued_requests must be present in vLLM evidence")
	require.NotNil(t, sample.CacheUsage, "cache_usage must be present: vLLM exposes kv_cache_usage_perc")
	require.False(t, sample.ObservedAt.IsZero(), "observed_at must be set")

	// Stable JSON key names: operators rely on these from route-status and list-models.
	type vllmEvidence struct {
		Source         string    `json:"source"`
		Freshness      string    `json:"freshness"`
		ActiveRequests *int      `json:"active_requests"`
		QueuedRequests *int      `json:"queued_requests"`
		CacheUsage     *float64  `json:"cache_usage"`
		ObservedAt     time.Time `json:"observed_at"`
	}
	shape := vllmEvidence{
		Source:         string(sample.Source),
		Freshness:      string(sample.Freshness),
		ActiveRequests: sample.ActiveRequests,
		QueuedRequests: sample.QueuedRequests,
		CacheUsage:     sample.CacheUsage,
		ObservedAt:     sample.ObservedAt,
	}
	data, err := json.Marshal(shape)
	require.NoError(t, err)

	var got map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &got))
	for _, key := range []string{"source", "freshness", "active_requests", "queued_requests", "cache_usage", "observed_at"} {
		assert.Contains(t, got, key, "vLLM evidence shape missing key %q: %s", key, data)
	}

	var source string
	require.NoError(t, json.Unmarshal(got["source"], &source))
	assert.Equal(t, "vllm.metrics", source, "vLLM evidence source must be the documented 'vllm.metrics' value")

	var freshness string
	require.NoError(t, json.Unmarshal(got["freshness"], &freshness))
	assert.Equal(t, "fresh", freshness, "freshness must be 'fresh' from cassette replay")
}
