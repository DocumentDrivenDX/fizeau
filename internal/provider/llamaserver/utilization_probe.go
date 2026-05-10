package llamaserver

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/easel/fizeau/internal/provider/utilization"
)

// UtilizationProbe queries llama-server observability endpoints and
// normalizes them into the shared endpoint utilization shape.
type UtilizationProbe struct {
	baseURL string
	client  *http.Client
	cache   utilization.Cache
}

// NewUtilizationProbe creates a probe for an OpenAI-compatible llama-server
// base URL.
func NewUtilizationProbe(baseURL string, client *http.Client) *UtilizationProbe {
	if client == nil {
		client = http.DefaultClient
	}
	return &UtilizationProbe{
		baseURL: baseURL,
		client:  client,
	}
}

// Probe first tries /metrics on the server root and falls back to /slots when
// metrics are unavailable.
func (p *UtilizationProbe) Probe(ctx context.Context) utilization.EndpointUtilization {
	if sample, ok := p.probeMetrics(ctx); ok {
		return p.cache.Remember(sample)
	}
	if sample, ok := p.probeSlots(ctx); ok {
		return p.cache.Remember(sample)
	}
	if stale, ok := p.cache.Stale(); ok {
		return stale
	}
	return utilization.Unknown(utilization.SourceLlamaMetrics)
}

func (p *UtilizationProbe) probeMetrics(ctx context.Context) (utilization.EndpointUtilization, bool) {
	body, err := p.get(ctx, utilization.ServerRoot(p.baseURL)+"/metrics")
	if err != nil {
		return utilization.EndpointUtilization{}, false
	}

	processing, ok := utilization.ParsePrometheusMetricValue(body, "llamacpp:requests_processing")
	if !ok {
		return utilization.EndpointUtilization{}, false
	}
	deferred, ok := utilization.ParsePrometheusMetricValue(body, "llamacpp:requests_deferred")
	if !ok {
		return utilization.EndpointUtilization{}, false
	}

	sample := utilization.EndpointUtilization{
		ActiveRequests: utilization.Int(int(processing)),
		QueuedRequests: utilization.Int(int(deferred)),
		Source:         utilization.SourceLlamaMetrics,
	}
	if cacheUsage, ok := utilization.ParsePrometheusMetricValue(body, "llamacpp:kv_cache_usage_ratio"); ok {
		sample.CacheUsage = utilization.Float64(cacheUsage)
	}
	return sample, true
}

func (p *UtilizationProbe) probeSlots(ctx context.Context) (utilization.EndpointUtilization, bool) {
	body, err := p.get(ctx, utilization.ServerRoot(p.baseURL)+"/slots")
	if err != nil {
		return utilization.EndpointUtilization{}, false
	}

	var arrayPayload []map[string]any
	slots := 0
	processing := 0
	if err := json.Unmarshal([]byte(strings.TrimSpace(body)), &arrayPayload); err == nil {
		slots = len(arrayPayload)
		for _, slot := range arrayPayload {
			if isProcessing(slot) {
				processing++
			}
		}
		return p.slotSample(processing, slots), true
	}

	var objectPayload struct {
		Slots     []map[string]any `json:"slots"`
		SlotCount int              `json:"slot_count"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(body)), &objectPayload); err != nil {
		return utilization.EndpointUtilization{}, false
	}
	slots = objectPayload.SlotCount
	if slots == 0 {
		slots = len(objectPayload.Slots)
	}
	for _, slot := range objectPayload.Slots {
		if isProcessing(slot) {
			processing++
		}
	}
	return p.slotSample(processing, slots), true
}

func (p *UtilizationProbe) slotSample(processing, slots int) utilization.EndpointUtilization {
	sample := utilization.EndpointUtilization{
		ActiveRequests: utilization.Int(processing),
		Source:         utilization.SourceLlamaSlots,
	}
	if slots > 0 {
		sample.MaxConcurrency = utilization.Int(slots)
		occupancy := float64(processing) / float64(slots)
		sample.CacheUsage = utilization.Float64(occupancy)
	}
	return sample
}

func isProcessing(slot map[string]any) bool {
	processing, _ := slot["is_processing"].(bool)
	return processing
}

func (p *UtilizationProbe) get(ctx context.Context, endpoint string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}
