package modelregistry

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/easel/fizeau/internal/config"
	"github.com/easel/fizeau/internal/discoverycache"
	"github.com/easel/fizeau/internal/harnesses"
	"github.com/easel/fizeau/internal/modelref"
	openaiadapter "github.com/easel/fizeau/internal/provider/openai"
)

const (
	discoveryTTLHTTPRemote       = 24 * time.Hour
	discoveryTTLHTTPLocal        = time.Hour
	discoveryTTLPTY              = 24 * time.Hour
	discoveryRefreshDeadlineHTTP = 10 * time.Second
	discoveryRefreshDeadlinePTY  = 60 * time.Second
)

type discoveredModel struct {
	ID           string
	Configured   bool
	Via          Source
	DiscoveredAt time.Time
}

type providerDiscoveryResult struct {
	Models  []discoveredModel
	Sources map[string]SourceMeta
}

type discoveryPayload struct {
	CapturedAt      time.Time `json:"captured_at"`
	Models          []string  `json:"models,omitempty"`
	ReasoningLevels []string  `json:"reasoning_levels,omitempty"`
	Source          string    `json:"source,omitempty"`
}

func discoverProvider(ctx context.Context, providerName string, pc config.ProviderConfig, cache *discoverycache.Cache, opts AssembleOptions) providerDiscoveryResult {
	providerType := normalizeProviderType(pc.Type)
	switch providerType {
	case "claude", "codex":
		return discoverHarnessProvider(providerName, providerType, cache, opts)
	case "openai", "openrouter", "vidar-ds4", "sindri-llamacpp", "ds4", "lmstudio", "llama-server", "omlx", "rapid-mlx", "vllm", "ollama", "minimax", "qwen", "zai":
		result := discoverOpenAICompatibleProvider(ctx, providerName, pc, cache, opts)
		if providerType == "ds4" || providerType == "vidar-ds4" {
			result.merge(discoverPropsProvider(providerName, cache))
		}
		return result
	default:
		if strings.TrimSpace(pc.Model) == "" {
			return providerDiscoveryResult{Sources: map[string]SourceMeta{}}
		}
		return providerDiscoveryResult{
			Models: []discoveredModel{{
				ID:           strings.TrimSpace(pc.Model),
				Via:          SourceNativeAPI,
				DiscoveredAt: time.Now().UTC(),
			}},
			Sources: map[string]SourceMeta{
				providerName: {LastRefreshedAt: time.Now().UTC()},
			},
		}
	}
}

func discoverOpenAICompatibleProvider(ctx context.Context, providerName string, pc config.ProviderConfig, cache *discoverycache.Cache, opts AssembleOptions) providerDiscoveryResult {
	src := discoverySource(providerName, discoveryTTLForProvider(pc), discoveryRefreshDeadlineHTTP)
	if cache == nil {
		return providerDiscoveryResult{Sources: map[string]SourceMeta{providerName: {Stale: true, Error: "discovery cache is nil"}}}
	}
	if hasDiscoveryEndpoint(pc) {
		baseURL := strings.TrimRight(firstBaseURL(pc), "/")
		apiKey := pc.APIKey
		refresher := func(refreshCtx context.Context) ([]byte, error) {
			requestCtx := ctx
			if requestCtx == nil {
				requestCtx = refreshCtx
			}
			ids, err := openaiadapter.DiscoverModels(requestCtx, baseURL, apiKey)
			if err != nil {
				return nil, err
			}
			return json.Marshal(discoveryPayload{
				CapturedAt: time.Now().UTC(),
				Models:     ids,
				Source:     "openai-compatible:/v1/models",
			})
		}
		if opts.Refresh == RefreshForce {
			_ = cache.Refresh(src, refresher)
		} else if opts.Refresh != RefreshNone {
			cache.MaybeRefresh(src, refresher)
		}
	}
	result := readDiscoveryCache(cache, src, providerName, SourceNativeAPI)
	if result.Sources == nil {
		result.Sources = map[string]SourceMeta{}
	}
	if len(result.Models) == 0 && strings.TrimSpace(pc.Model) != "" {
		fallback := modelsFromIDs([]string{pc.Model}, SourceNativeAPI, time.Now().UTC(), providerName)
		for i := range fallback.Models {
			fallback.Models[i].Configured = true
		}
		result.Models = fallback.Models
		meta := result.Sources[providerName]
		if meta.LastRefreshedAt.IsZero() {
			meta.LastRefreshedAt = time.Now().UTC()
		}
		result.Sources[providerName] = meta
	}
	return result
}

func discoverPropsProvider(providerName string, cache *discoverycache.Cache) providerDiscoveryResult {
	if cache == nil {
		return providerDiscoveryResult{Sources: map[string]SourceMeta{providerName + ":props": {Stale: true, Error: "discovery cache is nil"}}}
	}
	src := discoverySource(providerName+"-props", discoveryTTLHTTPLocal, discoveryRefreshDeadlineHTTP)
	result := readDiscoveryCache(cache, src, providerName, SourcePropsAPI)
	renamed := make(map[string]SourceMeta, len(result.Sources))
	for _, meta := range result.Sources {
		renamed[providerName+":props"] = meta
	}
	result.Sources = renamed
	return result
}

func discoverHarnessProvider(providerName, providerType string, cache *discoverycache.Cache, opts AssembleOptions) providerDiscoveryResult {
	src := discoverycache.Source{
		Tier:            "discovery",
		Name:            providerName,
		TTL:             discoveryTTLPTY,
		RefreshDeadline: discoveryRefreshDeadlinePTY,
	}
	if cache == nil {
		snapshot, err := harnesses.CachedModelDiscoverySnapshot(providerType, harnesses.EmbeddedDiscoverySource)
		if err != nil {
			return providerDiscoveryResult{Sources: map[string]SourceMeta{providerName: {Stale: true, Error: err.Error()}}}
		}
		return modelsFromIDs(snapshot.Models, SourceHarnessPTY, snapshot.CapturedAt, providerName)
	}
	if opts.Refresh == RefreshForce {
		_ = cache.Refresh(src, func(context.Context) ([]byte, error) {
			snapshot, err := harnesses.CachedModelDiscoverySnapshot(providerType, harnesses.EmbeddedDiscoverySource)
			if err != nil {
				return nil, err
			}
			return json.Marshal(discoveryPayload{
				CapturedAt:      snapshot.CapturedAt,
				Models:          snapshot.Models,
				ReasoningLevels: snapshot.ReasoningLevels,
				Source:          snapshot.Source,
			})
		})
	}
	result := readDiscoveryCache(cache, src, providerName, SourceHarnessPTY)
	if len(result.Models) > 0 {
		return result
	}
	snapshot, err := harnesses.CachedModelDiscoverySnapshot(providerType, harnesses.EmbeddedDiscoverySource)
	if err != nil {
		if result.Sources == nil {
			result.Sources = map[string]SourceMeta{}
		}
		meta := result.Sources[providerName]
		meta.Error = err.Error()
		result.Sources[providerName] = meta
		return result
	}
	return modelsFromIDs(snapshot.Models, SourceHarnessPTY, snapshot.CapturedAt, providerName)
}

func readDiscoveryCache(cache *discoverycache.Cache, src discoverycache.Source, providerName string, via Source) providerDiscoveryResult {
	result := providerDiscoveryResult{Sources: map[string]SourceMeta{}}
	read, err := cache.Read(src)
	meta := SourceMeta{Stale: true}
	if read.Data != nil {
		meta.LastRefreshedAt = time.Now().Add(-read.Age).UTC()
		meta.Stale = read.Stale
	}
	if err != nil {
		meta.Error = err.Error()
		result.Sources[src.Name] = meta
		return result
	}
	result.Sources[src.Name] = meta
	if read.Data == nil {
		return result
	}
	ids, capturedAt, err := parseDiscoveryIDs(read.Data, providerName)
	if err != nil {
		meta.Error = err.Error()
		result.Sources[src.Name] = meta
		return result
	}
	if !capturedAt.IsZero() {
		meta.LastRefreshedAt = capturedAt.UTC()
		result.Sources[src.Name] = meta
	}
	result.Models = modelsFromIDs(ids, via, discoveredAt(capturedAt, meta.LastRefreshedAt), providerName).Models
	return result
}

func parseDiscoveryIDs(data []byte, providerName string) ([]string, time.Time, error) {
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return nil, time.Time{}, nil
	}
	var list []string
	if err := json.Unmarshal(data, &list); err == nil {
		return uniqueSortedStrings(list), time.Time{}, nil
	}
	var payload discoveryPayload
	if err := json.Unmarshal(data, &payload); err == nil && (len(payload.Models) > 0 || !payload.CapturedAt.IsZero()) {
		return uniqueSortedStrings(payload.Models), payload.CapturedAt, nil
	}
	var byRef map[string]json.RawMessage
	if err := json.Unmarshal(data, &byRef); err != nil {
		return nil, time.Time{}, fmt.Errorf("decode discovery cache: %w", err)
	}
	ids := make([]string, 0, len(byRef))
	var capturedAt time.Time
	for key, raw := range byRef {
		includeValueFields := true
		if ref, err := modelref.Parse(key); err == nil {
			includeValueFields = ref.Provider == providerName
			if ref.Provider == providerName {
				ids = append(ids, ref.ID)
			}
		}
		if !includeValueFields {
			continue
		}
		var item struct {
			ID         string    `json:"id"`
			ModelID    string    `json:"model_id"`
			CapturedAt time.Time `json:"captured_at"`
		}
		if err := json.Unmarshal(raw, &item); err == nil {
			if item.ID != "" {
				ids = append(ids, item.ID)
			}
			if item.ModelID != "" {
				ids = append(ids, item.ModelID)
			}
			if capturedAt.IsZero() && !item.CapturedAt.IsZero() {
				capturedAt = item.CapturedAt
			}
		}
	}
	return uniqueSortedStrings(ids), capturedAt, nil
}

func modelsFromIDs(ids []string, via Source, at time.Time, providerName string) providerDiscoveryResult {
	at = discoveredAt(at, time.Now().UTC())
	out := make([]discoveredModel, 0, len(ids))
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, discoveredModel{ID: id, Via: via, DiscoveredAt: at})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return providerDiscoveryResult{
		Models:  out,
		Sources: map[string]SourceMeta{providerName: {LastRefreshedAt: at}},
	}
}

func (r *providerDiscoveryResult) merge(other providerDiscoveryResult) {
	if r.Sources == nil {
		r.Sources = map[string]SourceMeta{}
	}
	for k, v := range other.Sources {
		r.Sources[k] = v
	}
	seen := make(map[string]bool, len(r.Models)+len(other.Models))
	for _, model := range r.Models {
		seen[model.ID] = true
	}
	for _, model := range other.Models {
		if seen[model.ID] {
			continue
		}
		seen[model.ID] = true
		r.Models = append(r.Models, model)
	}
	sort.Slice(r.Models, func(i, j int) bool { return r.Models[i].ID < r.Models[j].ID })
}

func discoverySource(name string, ttl, deadline time.Duration) discoverycache.Source {
	return discoverycache.Source{Tier: "discovery", Name: name, TTL: ttl, RefreshDeadline: deadline}
}

func discoveryTTLForProvider(pc config.ProviderConfig) time.Duration {
	switch normalizeProviderType(pc.Type) {
	case "ds4", "vidar-ds4", "sindri-llamacpp", "lmstudio", "llama-server", "omlx", "rapid-mlx", "vllm", "ollama":
		return discoveryTTLHTTPLocal
	default:
		return discoveryTTLHTTPRemote
	}
}

func hasDiscoveryEndpoint(pc config.ProviderConfig) bool {
	return firstBaseURL(pc) != ""
}

func firstBaseURL(pc config.ProviderConfig) string {
	if strings.TrimSpace(pc.BaseURL) != "" {
		return strings.TrimSpace(pc.BaseURL)
	}
	for _, ep := range pc.Endpoints {
		if strings.TrimSpace(ep.BaseURL) != "" {
			return strings.TrimSpace(ep.BaseURL)
		}
	}
	return ""
}

func discoveredAt(primary, fallback time.Time) time.Time {
	if !primary.IsZero() {
		return primary.UTC()
	}
	if !fallback.IsZero() {
		return fallback.UTC()
	}
	return time.Now().UTC()
}

func uniqueSortedStrings(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func normalizeProviderType(providerType string) string {
	return strings.ToLower(strings.TrimSpace(providerType))
}
