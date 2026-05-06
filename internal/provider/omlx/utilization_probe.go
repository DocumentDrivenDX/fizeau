package omlx

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/DocumentDrivenDX/fizeau/internal/provider/utilization"
)

// UtilizationProbe queries oMLX server-root observability endpoints and
// normalizes them into the shared endpoint utilization shape.
type UtilizationProbe struct {
	baseURL string
	client  *http.Client
	cache   utilization.Cache
}

// NewUtilizationProbe creates a probe for an OpenAI-compatible oMLX base URL.
func NewUtilizationProbe(baseURL string, client *http.Client) *UtilizationProbe {
	if client == nil {
		client = http.DefaultClient
	}
	return &UtilizationProbe{
		baseURL: baseURL,
		client:  client,
	}
}

// Probe fetches /api/status from the server root and returns a normalized
// sample. Failures return stale or unknown utilization instead of surfacing
// endpoint unavailability.
func (p *UtilizationProbe) Probe(ctx context.Context) utilization.EndpointUtilization {
	snapshot, ok := p.probeStatus(ctx)
	if !ok {
		if stale, ok := p.cache.Stale(); ok {
			return stale
		}
		return utilization.Unknown(utilization.SourceOMLXStatus)
	}

	return p.cache.Remember(snapshot.normalize())
}

func (p *UtilizationProbe) probeStatus(ctx context.Context) (omlxStatusSnapshot, bool) {
	reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	body, err := p.get(reqCtx, utilization.ServerRoot(p.baseURL)+"/api/status")
	if err != nil {
		return omlxStatusSnapshot{}, false
	}

	snapshot, err := parseOMLXStatus(body)
	if err != nil {
		return omlxStatusSnapshot{}, false
	}
	return snapshot, true
}

func parseOMLXStatus(body string) (omlxStatusSnapshot, error) {
	var raw any
	dec := json.NewDecoder(strings.NewReader(body))
	dec.UseNumber()
	if err := dec.Decode(&raw); err != nil {
		return omlxStatusSnapshot{}, err
	}

	payload, ok := raw.(map[string]any)
	if !ok {
		return omlxStatusSnapshot{}, errors.New("omlx status payload was not an object")
	}

	snapshot := omlxStatusSnapshot{
		Raw: payload,
	}
	snapshot.TotalRequests = firstInt(payload, "total_requests", "requests_total")
	snapshot.ActiveRequests = firstInt(payload, "active_requests", "requests_active", "num_running", "running")
	snapshot.WaitingRequests = firstInt(payload, "waiting_requests", "requests_waiting", "num_waiting", "waiting")
	snapshot.TotalPromptTokens = firstInt(payload, "total_prompt_tokens", "prompt_tokens", "prompt_tokens_total")
	snapshot.TotalCompletionTokens = firstInt(payload, "total_completion_tokens", "completion_tokens", "completion_tokens_total")
	snapshot.TotalCachedTokens = firstInt(payload, "total_cached_tokens", "cached_tokens", "cache_tokens")
	snapshot.CacheEfficiency = firstFloat(payload, "cache_efficiency", "cache_usage", "cache_usage_ratio")
	snapshot.AvgPrefillTPS = firstFloat(payload, "avg_prefill_tps", "prefill_tps", "avg_prefill_tokens_per_second")
	snapshot.AvgGenerationTPS = firstFloat(payload, "avg_generation_tps", "generation_tps", "avg_generation_tokens_per_second")
	snapshot.ModelMemoryUsedBytes = firstInt64(payload, "model_memory_used_bytes", "model_memory_used", "memory_used_bytes", "memory_used")
	snapshot.ModelMemoryMaxBytes = firstInt64(payload, "model_memory_max_bytes", "model_memory_max", "memory_max_bytes", "memory_max")
	snapshot.UptimeSeconds = firstFloat(payload, "uptime_seconds", "uptime_s", "uptime")
	snapshot.LoadedModels = loadedModelIDs(payload)

	if snapshot.CacheEfficiency == nil {
		if cache, ok := firstMap(payload, "cache", "cache_stats", "cache_state"); ok {
			snapshot.CacheEfficiency = firstFloat(cache, "efficiency", "usage", "ratio", "pressure")
			if snapshot.TotalCachedTokens == 0 {
				snapshot.TotalCachedTokens = firstInt(cache, "cached_tokens", "cache_tokens")
			}
		}
	}

	if snapshot.ModelMemoryUsedBytes == nil || snapshot.ModelMemoryMaxBytes == nil {
		if model, ok := firstMap(payload, "model", "loaded_model", "memory", "memory_stats"); ok {
			if snapshot.ModelMemoryUsedBytes == nil {
				snapshot.ModelMemoryUsedBytes = firstInt64(model, "used_bytes", "used", "memory_used_bytes", "active_bytes")
			}
			if snapshot.ModelMemoryMaxBytes == nil {
				snapshot.ModelMemoryMaxBytes = firstInt64(model, "max_bytes", "max", "memory_max_bytes", "peak_bytes")
			}
		}
	}

	return snapshot, nil
}

type omlxStatusSnapshot struct {
	TotalRequests         int
	ActiveRequests        int
	WaitingRequests       int
	TotalPromptTokens     int
	TotalCompletionTokens int
	TotalCachedTokens     int
	CacheEfficiency       *float64
	AvgPrefillTPS         *float64
	AvgGenerationTPS      *float64
	LoadedModels          []string
	ModelMemoryUsedBytes  *int64
	ModelMemoryMaxBytes   *int64
	UptimeSeconds         *float64
	Raw                   map[string]any
}

func (s omlxStatusSnapshot) normalize() utilization.EndpointUtilization {
	out := utilization.EndpointUtilization{
		ActiveRequests:         utilization.Int(s.ActiveRequests),
		QueuedRequests:         utilization.Int(s.WaitingRequests),
		TotalPromptTokens:      utilization.Int(s.TotalPromptTokens),
		TotalCompletionTokens:  utilization.Int(s.TotalCompletionTokens),
		CachedTokens:           utilization.Int(s.TotalCachedTokens),
		Source:                 utilization.SourceOMLXStatus,
		Freshness:              utilization.FreshnessUnknown,
		MetalActiveMemoryBytes: s.ModelMemoryUsedBytes,
		MetalPeakMemoryBytes:   s.ModelMemoryMaxBytes,
	}
	if s.CacheEfficiency != nil {
		out.CacheUsage = utilization.Float64(*s.CacheEfficiency)
	}
	if s.AvgGenerationTPS != nil {
		out.TokensPerSecond = utilization.Float64(*s.AvgGenerationTPS)
	}
	return out
}

func firstInt(payload map[string]any, keys ...string) int {
	if v, ok := firstNumber(payload, keys...); ok {
		return int(v)
	}
	return 0
}

func firstInt64(payload map[string]any, keys ...string) *int64 {
	for _, key := range keys {
		raw, ok := payload[key]
		if !ok || raw == nil {
			continue
		}
		if v, ok := int64Value(raw); ok {
			return utilization.Int64(v)
		}
	}
	return nil
}

func firstFloat(payload map[string]any, keys ...string) *float64 {
	if v, ok := firstNumber(payload, keys...); ok {
		return utilization.Float64(v)
	}
	return nil
}

func firstNumber(payload map[string]any, keys ...string) (float64, bool) {
	for _, key := range keys {
		raw, ok := payload[key]
		if !ok || raw == nil {
			continue
		}
		if v, ok := numberValue(raw); ok {
			return v, true
		}
	}
	return 0, false
}

func firstMap(payload map[string]any, keys ...string) (map[string]any, bool) {
	for _, key := range keys {
		raw, ok := payload[key]
		if !ok || raw == nil {
			continue
		}
		if m, ok := raw.(map[string]any); ok {
			return m, true
		}
	}
	return nil, false
}

func loadedModelIDs(payload map[string]any) []string {
	for _, key := range []string{"loaded_models", "loaded_model_ids", "models"} {
		raw, ok := payload[key]
		if !ok || raw == nil {
			continue
		}
		ids := collectModelIDs(raw)
		if len(ids) > 0 {
			return ids
		}
	}
	return nil
}

func collectModelIDs(raw any) []string {
	switch v := raw.(type) {
	case []any:
		ids := make([]string, 0, len(v))
		for _, item := range v {
			if id := modelIDFromValue(item); id != "" {
				ids = append(ids, id)
			}
		}
		return ids
	case map[string]any:
		if id := modelIDFromValue(v); id != "" {
			return []string{id}
		}
	}
	return nil
}

func modelIDFromValue(raw any) string {
	switch v := raw.(type) {
	case string:
		return strings.TrimSpace(v)
	case map[string]any:
		for _, key := range []string{"id", "name", "model", "model_id"} {
			if value, ok := v[key].(string); ok && strings.TrimSpace(value) != "" {
				return strings.TrimSpace(value)
			}
		}
	}
	return ""
}

func numberValue(raw any) (float64, bool) {
	switch v := raw.(type) {
	case json.Number:
		f, err := v.Float64()
		return f, err == nil
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case int32:
		return float64(v), true
	case uint:
		return float64(v), true
	case uint64:
		return float64(v), true
	case uint32:
		return float64(v), true
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		return f, err == nil
	default:
		return 0, false
	}
}

func int64Value(raw any) (int64, bool) {
	switch v := raw.(type) {
	case json.Number:
		i, err := v.Int64()
		return i, err == nil
	case float64:
		return int64(v), true
	case float32:
		return int64(v), true
	case int:
		return int64(v), true
	case int64:
		return v, true
	case int32:
		return int64(v), true
	case uint:
		return int64(v), true
	case uint64:
		return int64(v), true
	case uint32:
		return int64(v), true
	case string:
		i, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		return i, err == nil
	default:
		return 0, false
	}
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
