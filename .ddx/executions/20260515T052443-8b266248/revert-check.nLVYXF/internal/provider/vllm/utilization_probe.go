package vllm

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/easel/fizeau/internal/provider/utilization"
)

// UtilizationProbe queries vLLM server-root observability endpoints and
// normalizes them into the shared endpoint utilization shape.
type UtilizationProbe struct {
	baseURL string
	client  *http.Client
	cache   utilization.Cache
}

// NewUtilizationProbe creates a probe for an OpenAI-compatible vLLM base URL.
func NewUtilizationProbe(baseURL string, client *http.Client) *UtilizationProbe {
	if client == nil {
		client = http.DefaultClient
	}
	return &UtilizationProbe{
		baseURL: baseURL,
		client:  client,
	}
}

// Probe fetches /metrics from the server root and returns a normalized sample.
// Failures return stale or unknown utilization instead of surfacing endpoint
// unavailability.
func (p *UtilizationProbe) Probe(ctx context.Context) utilization.EndpointUtilization {
	body, err := p.get(ctx, utilization.ServerRoot(p.baseURL)+"/metrics")
	if err != nil {
		if stale, ok := p.cache.Stale(); ok {
			return stale
		}
		return utilization.Unknown(utilization.SourceVLLMMetrics)
	}

	running, ok := utilization.ParsePrometheusMetricValue(body, "vllm:num_requests_running")
	if !ok {
		if stale, ok := p.cache.Stale(); ok {
			return stale
		}
		return utilization.Unknown(utilization.SourceVLLMMetrics)
	}
	waiting, ok := utilization.ParsePrometheusMetricValue(body, "vllm:num_requests_waiting")
	if !ok {
		if stale, ok := p.cache.Stale(); ok {
			return stale
		}
		return utilization.Unknown(utilization.SourceVLLMMetrics)
	}

	sample := utilization.EndpointUtilization{
		ActiveRequests: utilization.Int(int(running)),
		QueuedRequests: utilization.Int(int(waiting)),
		Source:         utilization.SourceVLLMMetrics,
	}
	if cacheUsage, ok := utilization.ParsePrometheusMetricValue(body, "vllm:kv_cache_usage_perc"); ok {
		sample.CacheUsage = utilization.Float64(cacheUsage)
	} else if cacheUsage, ok := utilization.ParsePrometheusMetricValue(body, "vllm:gpu_cache_usage_perc"); ok {
		sample.CacheUsage = utilization.Float64(cacheUsage)
	}
	return p.cache.Remember(sample)
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
