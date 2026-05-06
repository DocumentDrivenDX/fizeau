package rapidmlx

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/DocumentDrivenDX/fizeau/internal/provider/utilization"
)

// UtilizationProbe queries Rapid-MLX status endpoints and normalizes them into
// the shared endpoint utilization shape.
type UtilizationProbe struct {
	baseURL string
	client  *http.Client
	cache   utilization.Cache
}

// NewUtilizationProbe creates a probe for an OpenAI-compatible Rapid-MLX base
// URL.
func NewUtilizationProbe(baseURL string, client *http.Client) *UtilizationProbe {
	if client == nil {
		client = http.DefaultClient
	}
	return &UtilizationProbe{
		baseURL: baseURL,
		client:  client,
	}
}

// Probe fetches /v1/status from the server root and returns a normalized
// sample. Failures return stale or unknown utilization instead of surfacing
// endpoint unavailability.
func (p *UtilizationProbe) Probe(ctx context.Context) utilization.EndpointUtilization {
	snapshot, ok := p.probeStatus(ctx)
	if !ok {
		if stale, ok := p.cache.Stale(); ok {
			return stale
		}
		return utilization.Unknown(utilization.SourceRapidMLXStatus)
	}

	return p.cache.Remember(snapshot.normalize())
}

func (p *UtilizationProbe) probeStatus(ctx context.Context) (rapidMLXStatusSnapshot, bool) {
	body, err := p.get(ctx, utilization.ServerRoot(p.baseURL)+"/status")
	if err != nil {
		return rapidMLXStatusSnapshot{}, false
	}

	snapshot, err := parseRapidMLXStatus(body)
	if err != nil {
		return rapidMLXStatusSnapshot{}, false
	}
	return snapshot, true
}

func parseRapidMLXStatus(body string) (rapidMLXStatusSnapshot, error) {
	var raw any
	dec := json.NewDecoder(strings.NewReader(body))
	dec.UseNumber()
	if err := dec.Decode(&raw); err != nil {
		return rapidMLXStatusSnapshot{}, err
	}

	payload, ok := raw.(map[string]any)
	if !ok {
		return rapidMLXStatusSnapshot{}, errors.New("rapid-mlx status payload was not an object")
	}

	snapshot := rapidMLXStatusSnapshot{
		Raw: payload,
	}
	snapshot.NumRunning = firstInt(payload, "num_running", "running", "requests_running")
	snapshot.NumWaiting = firstInt(payload, "num_waiting", "waiting", "requests_waiting")
	snapshot.TotalPromptTokens = firstInt(payload, "total_prompt_tokens", "prompt_tokens", "prompt_tokens_total")
	snapshot.TotalCompletionTokens = firstInt(payload, "total_completion_tokens", "completion_tokens", "generated_tokens_total")
	snapshot.CacheUsage = firstFloat(payload, "cache_usage", "cache_pressure", "cache_usage_ratio")
	snapshot.CacheHitType = firstString(payload, "cache_hit_type", "cache_hit")
	snapshot.CachedTokens = firstIntPtr(payload, "cached_tokens", "cache_tokens")
	snapshot.GeneratedTokens = firstIntPtr(payload, "generated_tokens")
	snapshot.MetalActiveMemoryBytes = firstInt64(payload, "metal_active_memory_bytes", "metal_active_bytes", "active_memory_bytes")
	snapshot.MetalPeakMemoryBytes = firstInt64(payload, "metal_peak_memory_bytes", "metal_peak_bytes", "peak_memory_bytes")
	snapshot.MetalCacheMemoryBytes = firstInt64(payload, "metal_cache_memory_bytes", "metal_cache_bytes", "cache_memory_bytes")

	if metal, ok := firstMap(payload, "metal", "metal_memory", "metal_stats"); ok {
		if snapshot.MetalActiveMemoryBytes == nil {
			snapshot.MetalActiveMemoryBytes = firstInt64(metal, "active_bytes", "active_memory_bytes")
		}
		if snapshot.MetalPeakMemoryBytes == nil {
			snapshot.MetalPeakMemoryBytes = firstInt64(metal, "peak_bytes", "peak_memory_bytes")
		}
		if snapshot.MetalCacheMemoryBytes == nil {
			snapshot.MetalCacheMemoryBytes = firstInt64(metal, "cache_bytes", "cache_memory_bytes")
		}
	}

	if cache, ok := firstMap(payload, "cache", "cache_stats", "cache_state"); ok {
		if snapshot.CacheUsage == nil {
			snapshot.CacheUsage = firstFloat(cache, "usage", "pressure", "ratio")
		}
		if snapshot.CacheHitType == nil {
			snapshot.CacheHitType = firstString(cache, "hit_type", "cache_hit_type")
		}
		if snapshot.CachedTokens == nil {
			snapshot.CachedTokens = firstIntPtr(cache, "cached_tokens", "cache_tokens")
		}
		if snapshot.GeneratedTokens == nil {
			snapshot.GeneratedTokens = firstIntPtr(cache, "generated_tokens")
		}
	}

	if active, ok := firstSlice(payload, "active_requests", "requests", "active"); ok {
		snapshot.ActiveRequests = make([]rapidMLXRequestSnapshot, 0, len(active))
		for _, entry := range active {
			req, ok := entry.(map[string]any)
			if !ok {
				continue
			}
			item := rapidMLXRequestSnapshot{
				Phase:           firstString(req, "phase", "state"),
				CacheHitType:    firstString(req, "cache_hit_type", "cache_hit"),
				CachedTokens:    firstIntPtr(req, "cached_tokens", "cache_tokens"),
				GeneratedTokens: firstIntPtr(req, "generated_tokens"),
				TTFTSeconds:     firstFloat(req, "ttft_s", "ttft_seconds"),
				TokensPerSecond: firstFloat(req, "tokens_per_second", "tokens_per_sec"),
			}
			snapshot.ActiveRequests = append(snapshot.ActiveRequests, item)
		}
		if len(snapshot.ActiveRequests) > 0 {
			first := snapshot.ActiveRequests[0]
			snapshot.ActiveRequestPhase = first.Phase
			if snapshot.CacheHitType == nil {
				snapshot.CacheHitType = first.CacheHitType
			}
			if snapshot.CachedTokens == nil {
				snapshot.CachedTokens = first.CachedTokens
			}
			if snapshot.GeneratedTokens == nil {
				snapshot.GeneratedTokens = first.GeneratedTokens
			}
			if snapshot.TTFTSeconds == nil {
				snapshot.TTFTSeconds = first.TTFTSeconds
			}
			if snapshot.TokensPerSecond == nil {
				snapshot.TokensPerSecond = first.TokensPerSecond
			}
		}
	}

	if snapshot.CacheUsage == nil {
		switch {
		case snapshot.MetalActiveMemoryBytes != nil && snapshot.MetalPeakMemoryBytes != nil && *snapshot.MetalPeakMemoryBytes > 0:
			usage := float64(*snapshot.MetalActiveMemoryBytes) / float64(*snapshot.MetalPeakMemoryBytes)
			snapshot.CacheUsage = utilization.Float64(usage)
		case snapshot.MetalCacheMemoryBytes != nil && snapshot.MetalPeakMemoryBytes != nil && *snapshot.MetalPeakMemoryBytes > 0:
			usage := float64(*snapshot.MetalCacheMemoryBytes) / float64(*snapshot.MetalPeakMemoryBytes)
			snapshot.CacheUsage = utilization.Float64(usage)
		}
	}

	if snapshot.NumRunning < 0 || snapshot.NumWaiting < 0 {
		return rapidMLXStatusSnapshot{}, fmt.Errorf("invalid negative running/waiting counts")
	}

	return snapshot, nil
}

type rapidMLXStatusSnapshot struct {
	NumRunning             int
	NumWaiting             int
	TotalPromptTokens      int
	TotalCompletionTokens  int
	CacheUsage             *float64
	CacheHitType           *string
	CachedTokens           *int
	GeneratedTokens        *int
	ActiveRequestPhase     *string
	TTFTSeconds            *float64
	TokensPerSecond        *float64
	MetalActiveMemoryBytes *int64
	MetalPeakMemoryBytes   *int64
	MetalCacheMemoryBytes  *int64
	ActiveRequests         []rapidMLXRequestSnapshot
	Raw                    map[string]any
}

type rapidMLXRequestSnapshot struct {
	Phase           *string
	CacheHitType    *string
	CachedTokens    *int
	GeneratedTokens *int
	TTFTSeconds     *float64
	TokensPerSecond *float64
}

func (s rapidMLXStatusSnapshot) normalize() utilization.EndpointUtilization {
	out := utilization.EndpointUtilization{
		ActiveRequests:         utilization.Int(s.NumRunning),
		QueuedRequests:         utilization.Int(s.NumWaiting),
		Source:                 utilization.SourceRapidMLXStatus,
		Freshness:              utilization.FreshnessUnknown,
		TotalPromptTokens:      utilization.Int(s.TotalPromptTokens),
		TotalCompletionTokens:  utilization.Int(s.TotalCompletionTokens),
		CacheHitType:           s.CacheHitType,
		CachedTokens:           s.CachedTokens,
		GeneratedTokens:        s.GeneratedTokens,
		ActiveRequestPhase:     s.ActiveRequestPhase,
		TTFTSeconds:            s.TTFTSeconds,
		TokensPerSecond:        s.TokensPerSecond,
		MetalActiveMemoryBytes: s.MetalActiveMemoryBytes,
		MetalPeakMemoryBytes:   s.MetalPeakMemoryBytes,
		MetalCacheMemoryBytes:  s.MetalCacheMemoryBytes,
	}
	if s.CacheUsage != nil {
		out.CacheUsage = utilization.Float64(*s.CacheUsage)
	}
	if out.ActiveRequestPhase == nil && len(s.ActiveRequests) > 0 {
		out.ActiveRequestPhase = s.ActiveRequests[0].Phase
	}
	return out
}

func firstInt(payload map[string]any, keys ...string) int {
	if v := firstNumber(payload, keys...); v != nil {
		return int(*v)
	}
	return 0
}

func firstIntPtr(payload map[string]any, keys ...string) *int {
	for _, key := range keys {
		raw, ok := payload[key]
		if !ok || raw == nil {
			continue
		}
		if v := firstNumber(payload, key); v != nil {
			val := int(*v)
			return &val
		}
	}
	return nil
}

func firstInt64(payload map[string]any, keys ...string) *int64 {
	if v := firstNumber(payload, keys...); v != nil {
		val := int64(*v)
		return &val
	}
	return nil
}

func firstFloat(payload map[string]any, keys ...string) *float64 {
	if v := firstNumber(payload, keys...); v != nil {
		val := *v
		return &val
	}
	return nil
}

func firstString(payload map[string]any, keys ...string) *string {
	for _, key := range keys {
		raw, ok := payload[key]
		if !ok || raw == nil {
			continue
		}
		switch v := raw.(type) {
		case string:
			s := strings.TrimSpace(v)
			if s != "" {
				return &s
			}
		case json.Number:
			s := strings.TrimSpace(v.String())
			if s != "" {
				return &s
			}
		default:
			s := strings.TrimSpace(fmt.Sprint(v))
			if s != "" && s != "<nil>" {
				return &s
			}
		}
	}
	return nil
}

func firstMap(payload map[string]any, keys ...string) (map[string]any, bool) {
	for _, key := range keys {
		raw, ok := payload[key]
		if !ok || raw == nil {
			continue
		}
		if out, ok := raw.(map[string]any); ok {
			return out, true
		}
	}
	return nil, false
}

func firstSlice(payload map[string]any, keys ...string) ([]any, bool) {
	for _, key := range keys {
		raw, ok := payload[key]
		if !ok || raw == nil {
			continue
		}
		if out, ok := raw.([]any); ok {
			return out, true
		}
	}
	return nil, false
}

func firstNumber(payload map[string]any, keys ...string) *float64 {
	for _, key := range keys {
		raw, ok := payload[key]
		if !ok || raw == nil {
			continue
		}
		switch v := raw.(type) {
		case json.Number:
			if f, err := v.Float64(); err == nil {
				return &f
			}
		case float64:
			f := v
			return &f
		case float32:
			f := float64(v)
			return &f
		case int:
			f := float64(v)
			return &f
		case int64:
			f := float64(v)
			return &f
		case int32:
			f := float64(v)
			return &f
		case uint:
			f := float64(v)
			return &f
		case uint64:
			f := float64(v)
			return &f
		case string:
			if f, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
				return &f
			}
		}
	}
	return nil
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
