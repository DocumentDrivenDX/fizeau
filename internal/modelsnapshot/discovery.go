package modelsnapshot

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

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
	Provider        string
	ProviderType    string
	Harness         string
	ID              string
	Configured      bool
	EndpointName    string
	EndpointBaseURL string
	ServerInstance  string
	Via             Source
	DiscoveredAt    time.Time
}

type modelDiscoveryEndpoint struct {
	Name           string
	BaseURL        string
	ServerInstance string
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

func discoverProvider(ctx context.Context, providerName string, pc ProviderConfig, cache *discoverycache.Cache, opts AssembleOptions) providerDiscoveryResult {
	providerType := normalizeProviderType(pc.Type)
	switch providerType {
	case "claude", "codex":
		return discoverHarnessProvider(providerName, providerType, cache, opts)
	case "openai", "openrouter", "vidar-ds4", "sindri-llamacpp", "ds4", "lmstudio", "llama-server", "omlx", "rapid-mlx", "vllm", "ollama", "minimax", "qwen", "zai":
		result := discoverOpenAICompatibleProvider(ctx, providerName, pc, cache, opts)
		if providerType == "ds4" || providerType == "vidar-ds4" {
			result.merge(discoverPropsProvider(providerName, pc, cache))
		}
		return result
	default:
		if strings.TrimSpace(pc.Model) == "" {
			return providerDiscoveryResult{Sources: map[string]SourceMeta{}}
		}
		return providerDiscoveryResult{
			Models: []discoveredModel{{
				Provider:        providerName,
				ProviderType:    providerType,
				ID:              strings.TrimSpace(pc.Model),
				EndpointName:    endpointNameForConfig(providerName, pc.BaseURL, pc.Endpoints),
				EndpointBaseURL: firstBaseURL(pc),
				ServerInstance:  firstServerInstance(pc),
				Via:             SourceNativeAPI,
				DiscoveredAt:    time.Now().UTC(),
			}},
			Sources: map[string]SourceMeta{
				providerName: {LastRefreshedAt: time.Now().UTC()},
			},
		}
	}
}

func discoverOpenAICompatibleProvider(ctx context.Context, providerName string, pc ProviderConfig, cache *discoverycache.Cache, opts AssembleOptions) providerDiscoveryResult {
	if cache == nil {
		return providerDiscoveryResult{Sources: map[string]SourceMeta{providerName: {Stale: true, Error: "discovery cache is nil"}}}
	}
	endpoints := discoveryEndpoints(providerName, pc)
	if len(endpoints) == 0 {
		endpoints = []modelDiscoveryEndpoint{{
			Name:           endpointNameForConfig(providerName, pc.BaseURL, nil),
			BaseURL:        firstBaseURL(pc),
			ServerInstance: firstServerInstance(pc),
		}}
	}
	result := providerDiscoveryResult{Sources: map[string]SourceMeta{}}
	for _, endpoint := range endpoints {
		endpointResult := discoverOpenAICompatibleEndpoint(ctx, providerName, providerTypeFromProviderConfig(pc), endpoint, pc.APIKey, discoveryTTLForProvider(pc), cache, opts)
		result.merge(endpointResult)
	}
	if len(result.Models) == 0 && strings.TrimSpace(pc.Model) != "" {
		fallback := modelsFromIDs([]string{pc.Model}, SourceNativeAPI, time.Now().UTC(), discoveryIdentity{
			Provider:        providerName,
			ProviderType:    providerTypeFromProviderConfig(pc),
			EndpointName:    endpointNameForConfig(providerName, pc.BaseURL, pc.Endpoints),
			EndpointBaseURL: firstBaseURL(pc),
			ServerInstance:  firstServerInstance(pc),
		})
		for i := range fallback.Models {
			fallback.Models[i].Configured = true
		}
		result.merge(fallback)
		meta := result.Sources[providerName]
		if meta.LastRefreshedAt.IsZero() {
			meta.LastRefreshedAt = time.Now().UTC()
		}
		result.Sources[providerName] = meta
	}
	return result
}

func discoverOpenAICompatibleEndpoint(ctx context.Context, providerName, providerType string, endpoint modelDiscoveryEndpoint, apiKey string, ttl time.Duration, cache *discoverycache.Cache, opts AssembleOptions) providerDiscoveryResult {
	src := discoverySource(endpointSourceName(providerName, endpoint.Name, endpoint.BaseURL, endpoint.ServerInstance), ttl, discoveryRefreshDeadlineHTTP)
	refresher := func(refreshCtx context.Context) ([]byte, error) {
		requestCtx := ctx
		if requestCtx == nil {
			requestCtx = refreshCtx
		}
		ids, err := openaiadapter.DiscoverModels(requestCtx, strings.TrimRight(endpoint.BaseURL, "/"), apiKey)
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
	return readDiscoveryCache(cache, src, providerName, SourceNativeAPI, discoveryIdentity{
		Provider:        providerName,
		ProviderType:    providerType,
		EndpointName:    endpoint.Name,
		EndpointBaseURL: endpoint.BaseURL,
		ServerInstance:  endpoint.ServerInstance,
	})
}

func discoverPropsProvider(providerName string, pc ProviderConfig, cache *discoverycache.Cache) providerDiscoveryResult {
	if cache == nil {
		return providerDiscoveryResult{Sources: map[string]SourceMeta{providerName + ":props": {Stale: true, Error: "discovery cache is nil"}}}
	}
	src := discoverySource(providerName+"-props", discoveryTTLHTTPLocal, discoveryRefreshDeadlineHTTP)
	result := readDiscoveryCache(cache, src, providerName, SourcePropsAPI, discoveryIdentity{
		Provider:        providerName,
		ProviderType:    providerTypeFromProviderConfig(pc),
		EndpointName:    endpointNameForConfig(providerName, pc.BaseURL, pc.Endpoints),
		EndpointBaseURL: firstBaseURL(pc),
		ServerInstance:  firstServerInstance(pc),
	})
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
		return modelsFromIDs(snapshot.Models, SourceHarnessPTY, snapshot.CapturedAt, discoveryIdentity{
			Provider:       providerName,
			ProviderType:   providerType,
			Harness:        providerType,
			EndpointName:   providerName,
			ServerInstance: providerName,
		})
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
	result := readDiscoveryCache(cache, src, providerName, SourceHarnessPTY, discoveryIdentity{
		Provider:       providerName,
		ProviderType:   providerType,
		Harness:        providerType,
		EndpointName:   providerName,
		ServerInstance: providerName,
	})
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
	return modelsFromIDs(snapshot.Models, SourceHarnessPTY, snapshot.CapturedAt, discoveryIdentity{
		Provider:       providerName,
		ProviderType:   providerType,
		Harness:        providerType,
		EndpointName:   providerName,
		ServerInstance: providerName,
	})
}

func readDiscoveryCache(cache *discoverycache.Cache, src discoverycache.Source, providerName string, via Source, identity discoveryIdentity) providerDiscoveryResult {
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
	result.Models = modelsFromIDs(ids, via, discoveredAt(capturedAt, meta.LastRefreshedAt), identity).Models
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

type discoveryIdentity struct {
	Provider        string
	ProviderType    string
	Harness         string
	EndpointName    string
	EndpointBaseURL string
	ServerInstance  string
}

func modelsFromIDs(ids []string, via Source, at time.Time, identity discoveryIdentity) providerDiscoveryResult {
	at = discoveredAt(at, time.Now().UTC())
	out := make([]discoveredModel, 0, len(ids))
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, discoveredModel{
			Provider:        identity.Provider,
			ProviderType:    identity.ProviderType,
			Harness:         identity.Harness,
			ID:              id,
			EndpointName:    identity.EndpointName,
			EndpointBaseURL: identity.EndpointBaseURL,
			ServerInstance:  identity.ServerInstance,
			Via:             via,
			DiscoveredAt:    at,
		})
	}
	sort.Slice(out, func(i, j int) bool { return discoveredModelSortKey(out[i]) < discoveredModelSortKey(out[j]) })
	return providerDiscoveryResult{
		Models:  out,
		Sources: map[string]SourceMeta{identity.Provider: {LastRefreshedAt: at}},
	}
}

func (r *providerDiscoveryResult) merge(other providerDiscoveryResult) {
	if r.Sources == nil {
		r.Sources = map[string]SourceMeta{}
	}
	for k, v := range other.Sources {
		r.Sources[k] = v
	}
	seenFull := make(map[string]bool, len(r.Models)+len(other.Models))
	seenGeneric := make(map[string]bool, len(r.Models)+len(other.Models))
	for _, model := range r.Models {
		seenFull[model.identityKey()] = true
		seenGeneric[model.providerModelKey()] = true
	}
	for _, model := range other.Models {
		if model.hasEndpointIdentity() {
			if seenFull[model.identityKey()] {
				continue
			}
			r.Models = removeGenericModel(r.Models, model.Provider, model.ID)
			seenFull[model.identityKey()] = true
			seenGeneric[model.providerModelKey()] = true
			r.Models = append(r.Models, model)
			continue
		}
		if seenGeneric[model.providerModelKey()] {
			continue
		}
		seenFull[model.identityKey()] = true
		seenGeneric[model.providerModelKey()] = true
		r.Models = append(r.Models, model)
	}
	sort.Slice(r.Models, func(i, j int) bool { return discoveredModelSortKey(r.Models[i]) < discoveredModelSortKey(r.Models[j]) })
}

func discoverySource(name string, ttl, deadline time.Duration) discoverycache.Source {
	return discoverycache.Source{Tier: "discovery", Name: name, TTL: ttl, RefreshDeadline: deadline}
}

func discoveryTTLForProvider(pc ProviderConfig) time.Duration {
	switch normalizeProviderType(pc.Type) {
	case "ds4", "vidar-ds4", "sindri-llamacpp", "lmstudio", "llama-server", "omlx", "rapid-mlx", "vllm", "ollama":
		return discoveryTTLHTTPLocal
	default:
		return discoveryTTLHTTPRemote
	}
}

func firstBaseURL(pc ProviderConfig) string {
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

func firstServerInstance(pc ProviderConfig) string {
	if strings.TrimSpace(pc.ServerInstance) != "" {
		return strings.TrimSpace(pc.ServerInstance)
	}
	for _, ep := range pc.Endpoints {
		if strings.TrimSpace(ep.ServerInstance) != "" {
			return strings.TrimSpace(ep.ServerInstance)
		}
	}
	return ""
}

func providerTypeFromProviderConfig(pc ProviderConfig) string {
	return normalizeProviderType(pc.Type)
}

func discoveryEndpoints(providerName string, pc ProviderConfig) []modelDiscoveryEndpoint {
	if len(pc.Endpoints) > 0 {
		out := make([]modelDiscoveryEndpoint, 0, len(pc.Endpoints))
		for i, ep := range pc.Endpoints {
			if strings.TrimSpace(ep.BaseURL) == "" {
				continue
			}
			name := strings.TrimSpace(ep.Name)
			if name == "" {
				name = fmt.Sprintf("%s-%d", providerName, i+1)
			}
			out = append(out, modelDiscoveryEndpoint{
				Name:           name,
				BaseURL:        strings.TrimSpace(ep.BaseURL),
				ServerInstance: strings.TrimSpace(ep.ServerInstance),
			})
		}
		return out
	}
	if strings.TrimSpace(pc.BaseURL) == "" {
		return nil
	}
	name := strings.TrimSpace(providerName)
	if name == "" {
		name = endpointNameForConfig(providerName, pc.BaseURL, nil)
	}
	return []modelDiscoveryEndpoint{{
		Name:           name,
		BaseURL:        strings.TrimSpace(pc.BaseURL),
		ServerInstance: firstServerInstance(pc),
	}}
}

func endpointSourceName(providerName, endpointName, baseURL, serverInstance string) string {
	name := strings.TrimSpace(providerName)
	trimmedEndpoint := strings.TrimSpace(endpointName)
	if trimmedEndpoint == "" || trimmedEndpoint == "default" || trimmedEndpoint == name {
		return sanitizeDiscoveryName(name)
	}
	switch {
	case trimmedEndpoint != "":
		name = name + "-" + trimmedEndpoint
	case strings.TrimSpace(serverInstance) != "":
		name = name + "-" + strings.TrimSpace(serverInstance)
	case strings.TrimSpace(baseURL) != "":
		name = name + "-" + strings.TrimSpace(baseURL)
	}
	return sanitizeDiscoveryName(name)
}

func endpointNameForConfig(providerName, baseURL string, endpoints []ProviderEndpoint) string {
	if len(endpoints) > 0 {
		for _, ep := range endpoints {
			if trimmed := strings.TrimSpace(ep.Name); trimmed != "" {
				return trimmed
			}
		}
	}
	if trimmed := strings.TrimSpace(providerName); trimmed != "" {
		return trimmed
	}
	if trimmed := strings.TrimSpace(baseURL); trimmed != "" {
		return trimmed
	}
	return "default"
}

func sanitizeDiscoveryName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return "discovery"
	}
	var b strings.Builder
	b.Grow(len(name))
	lastDash := false
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
			lastDash = false
		case r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "discovery"
	}
	return out
}

func discoveredModelSortKey(model discoveredModel) string {
	return strings.Join([]string{
		model.Provider,
		model.ProviderType,
		model.Harness,
		model.EndpointName,
		model.EndpointBaseURL,
		model.ServerInstance,
		model.ID,
	}, "\x00")
}

func (m discoveredModel) providerModelKey() string {
	return m.Provider + "\x00" + m.ID
}

func (m discoveredModel) hasEndpointIdentity() bool {
	return strings.TrimSpace(m.Harness) != "" || strings.TrimSpace(m.EndpointName) != "" || strings.TrimSpace(m.EndpointBaseURL) != "" || strings.TrimSpace(m.ServerInstance) != ""
}

func (m discoveredModel) identityKey() string {
	return strings.Join([]string{
		m.Provider,
		m.ProviderType,
		m.Harness,
		m.ID,
		m.EndpointName,
		m.EndpointBaseURL,
		m.ServerInstance,
	}, "\x00")
}

func removeGenericModel(models []discoveredModel, provider, id string) []discoveredModel {
	out := models[:0]
	for _, model := range models {
		if model.Provider == provider && model.ID == id && !model.hasEndpointIdentity() {
			continue
		}
		out = append(out, model)
	}
	return out
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
