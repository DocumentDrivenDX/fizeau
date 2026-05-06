package utilization

import (
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Source identifies the probe path that produced a utilization sample.
type Source string

const (
	SourceUnknown        Source = "unknown"
	SourceOMLXStatus     Source = "omlx.status"
	SourceVLLMMetrics    Source = "vllm.metrics"
	SourceLlamaMetrics   Source = "llama-server.metrics"
	SourceLlamaSlots     Source = "llama-server.slots"
	SourceRapidMLXStatus Source = "rapid-mlx.status"
)

// Freshness describes whether a sample was observed just now, reused after a
// failed probe, or has no known prior observation.
type Freshness string

const (
	FreshnessFresh   Freshness = "fresh"
	FreshnessStale   Freshness = "stale"
	FreshnessUnknown Freshness = "unknown"
)

// EndpointUtilization is the normalized utilization shape shared by local
// provider probes.
type EndpointUtilization struct {
	ActiveRequests         *int
	QueuedRequests         *int
	CacheUsage             *float64
	MaxConcurrency         *int
	TotalPromptTokens      *int
	TotalCompletionTokens  *int
	CacheHitType           *string
	CachedTokens           *int
	GeneratedTokens        *int
	ActiveRequestPhase     *string
	TTFTSeconds            *float64
	TokensPerSecond        *float64
	MetalActiveMemoryBytes *int64
	MetalPeakMemoryBytes   *int64
	MetalCacheMemoryBytes  *int64
	Source                 Source
	Freshness              Freshness
	ObservedAt             time.Time
}

// Cache preserves the most recent successful sample so probe failures can
// return stale utilization instead of surfacing hard endpoint unavailability.
type Cache struct {
	mu   sync.Mutex
	last *EndpointUtilization
}

// Remember stores a fresh sample and returns a normalized copy with fresh
// freshness and an observed timestamp.
func (c *Cache) Remember(sample EndpointUtilization) EndpointUtilization {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now().UTC()
	sample.Freshness = FreshnessFresh
	sample.ObservedAt = now
	stored := clone(sample)
	c.last = &stored
	return stored
}

// Stale returns the last successful sample marked stale. The boolean reports
// whether a previous sample existed.
func (c *Cache) Stale() (EndpointUtilization, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.last == nil {
		return EndpointUtilization{}, false
	}
	stale := clone(*c.last)
	stale.Freshness = FreshnessStale
	return stale, true
}

// Unknown returns a sample with unknown freshness and no numeric values.
func Unknown(source Source) EndpointUtilization {
	return EndpointUtilization{
		Source:    source,
		Freshness: FreshnessUnknown,
	}
}

// Int returns a pointer to v.
func Int(v int) *int {
	return &v
}

// Float64 returns a pointer to v.
func Float64(v float64) *float64 {
	return &v
}

// Int64 returns a pointer to v.
func Int64(v int64) *int64 {
	return &v
}

// String returns a pointer to v.
func String(v string) *string {
	return &v
}

// ServerRoot strips a trailing /v1 path component from an OpenAI-compatible
// base URL while preserving the scheme, host, and any prefix path.
func ServerRoot(baseURL string) string {
	trimmed := strings.TrimSpace(strings.TrimRight(baseURL, "/"))
	if trimmed == "" {
		return ""
	}

	parsed, err := url.Parse(trimmed)
	if err != nil {
		return strings.TrimSuffix(trimmed, "/v1")
	}

	parsed.Fragment = ""
	parsed.RawQuery = ""
	path := strings.TrimRight(parsed.Path, "/")
	if strings.HasSuffix(path, "/v1") {
		path = strings.TrimSuffix(path, "/v1")
	}
	parsed.Path = path
	return strings.TrimRight(parsed.String(), "/")
}

// ParsePrometheusMetricValue returns the first numeric value for metric from
// a Prometheus-style plaintext metrics body.
func ParsePrometheusMetricValue(body, metric string) (float64, bool) {
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || !strings.HasPrefix(line, metric) {
			continue
		}
		rest := strings.TrimSpace(strings.TrimPrefix(line, metric))
		if strings.HasPrefix(rest, "{") {
			if idx := strings.Index(rest, "}"); idx >= 0 {
				rest = strings.TrimSpace(rest[idx+1:])
			}
		}
		fields := strings.Fields(rest)
		if len(fields) == 0 {
			continue
		}
		val, err := strconv.ParseFloat(fields[0], 64)
		if err == nil {
			return val, true
		}
	}
	return 0, false
}

func clone(sample EndpointUtilization) EndpointUtilization {
	out := sample
	if sample.ActiveRequests != nil {
		out.ActiveRequests = Int(*sample.ActiveRequests)
	}
	if sample.QueuedRequests != nil {
		out.QueuedRequests = Int(*sample.QueuedRequests)
	}
	if sample.CacheUsage != nil {
		out.CacheUsage = Float64(*sample.CacheUsage)
	}
	if sample.MaxConcurrency != nil {
		out.MaxConcurrency = Int(*sample.MaxConcurrency)
	}
	if sample.TotalPromptTokens != nil {
		out.TotalPromptTokens = Int(*sample.TotalPromptTokens)
	}
	if sample.TotalCompletionTokens != nil {
		out.TotalCompletionTokens = Int(*sample.TotalCompletionTokens)
	}
	if sample.CacheHitType != nil {
		out.CacheHitType = String(*sample.CacheHitType)
	}
	if sample.CachedTokens != nil {
		out.CachedTokens = Int(*sample.CachedTokens)
	}
	if sample.GeneratedTokens != nil {
		out.GeneratedTokens = Int(*sample.GeneratedTokens)
	}
	if sample.ActiveRequestPhase != nil {
		out.ActiveRequestPhase = String(*sample.ActiveRequestPhase)
	}
	if sample.TTFTSeconds != nil {
		out.TTFTSeconds = Float64(*sample.TTFTSeconds)
	}
	if sample.TokensPerSecond != nil {
		out.TokensPerSecond = Float64(*sample.TokensPerSecond)
	}
	if sample.MetalActiveMemoryBytes != nil {
		out.MetalActiveMemoryBytes = Int64(*sample.MetalActiveMemoryBytes)
	}
	if sample.MetalPeakMemoryBytes != nil {
		out.MetalPeakMemoryBytes = Int64(*sample.MetalPeakMemoryBytes)
	}
	if sample.MetalCacheMemoryBytes != nil {
		out.MetalCacheMemoryBytes = Int64(*sample.MetalCacheMemoryBytes)
	}
	return out
}
