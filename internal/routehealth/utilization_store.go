package routehealth

import (
	"sort"
	"strings"
	"sync"

	"github.com/DocumentDrivenDX/fizeau/internal/provider/utilization"
)

// UtilizationKey identifies one provider endpoint/model utilization sample.
type UtilizationKey struct {
	Provider string
	Endpoint string
	Model    string
}

// EndpointLoad is the normalized load signal used by routing.
type EndpointLoad struct {
	LeaseCount           int
	NormalizedLoad       float64
	UtilizationFresh     bool
	UtilizationSaturated bool
}

// UtilizationStore retains the most recent utilization sample for each
// provider endpoint. It is safe for concurrent use.
type UtilizationStore struct {
	mu      sync.RWMutex
	samples map[UtilizationKey]utilization.EndpointUtilization
}

// NewUtilizationStore returns an empty utilization store.
func NewUtilizationStore() *UtilizationStore {
	return &UtilizationStore{
		samples: make(map[UtilizationKey]utilization.EndpointUtilization),
	}
}

// NormalizeUtilizationKey trims whitespace from the utilization dimensions.
func NormalizeUtilizationKey(provider, endpoint, model string) UtilizationKey {
	return UtilizationKey{
		Provider: strings.TrimSpace(provider),
		Endpoint: strings.TrimSpace(endpoint),
		Model:    strings.TrimSpace(model),
	}
}

// Record stores the latest utilization sample for key.
func (s *UtilizationStore) Record(provider, endpoint, model string, sample utilization.EndpointUtilization) {
	if s == nil {
		return
	}
	key := NormalizeUtilizationKey(provider, endpoint, model)
	if key.Provider == "" && key.Endpoint == "" && key.Model == "" {
		return
	}
	if sample.Freshness == "" {
		sample.Freshness = utilization.FreshnessFresh
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.samples == nil {
		s.samples = make(map[UtilizationKey]utilization.EndpointUtilization)
	}
	s.samples[key] = cloneUtilization(sample)
}

// Sample returns the most recent utilization sample for provider/endpoint/model.
// When the endpoint-specific sample is unavailable, provider-wide samples are
// considered as a fallback so callers can still surface coarse utilization
// evidence without guessing at private probe internals.
func (s *UtilizationStore) Sample(provider, endpoint, model string) (utilization.EndpointUtilization, bool) {
	if s == nil {
		return utilization.EndpointUtilization{}, false
	}
	keyProvider := strings.TrimSpace(provider)
	keyEndpoint := strings.TrimSpace(endpoint)
	keyModel := strings.TrimSpace(model)

	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.samples) == 0 {
		return utilization.EndpointUtilization{}, false
	}
	if sample, ok := s.samples[NormalizeUtilizationKey(keyProvider, keyEndpoint, keyModel)]; ok {
		return cloneUtilization(sample), true
	}
	if keyProvider != "" {
		if sample, ok := s.samples[NormalizeUtilizationKey(keyProvider, "", keyModel)]; ok {
			return cloneUtilization(sample), true
		}
	}
	return utilization.EndpointUtilization{}, false
}

// EndpointLoads returns the normalized load per endpoint for provider/model.
// Fresh utilization samples are combined with the supplied lease counts;
// stale or missing samples fall back to lease counts only.
func (s *UtilizationStore) EndpointLoads(provider, model string, leaseCounts map[string]int) map[string]EndpointLoad {
	if s == nil {
		return nil
	}
	keyProvider := strings.TrimSpace(provider)
	keyModel := strings.TrimSpace(model)

	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.samples) == 0 && len(leaseCounts) == 0 {
		return nil
	}

	keys := make(map[string]struct{})
	for endpoint := range leaseCounts {
		keys[endpoint] = struct{}{}
	}
	for key := range s.samples {
		if keyProvider != "" && key.Provider != keyProvider {
			continue
		}
		if keyModel != "" && key.Model != keyModel {
			continue
		}
		if key.Endpoint != "" {
			keys[key.Endpoint] = struct{}{}
		}
	}
	if len(keys) == 0 {
		return nil
	}

	out := make(map[string]EndpointLoad, len(keys))
	for endpoint := range keys {
		leaseCount := leaseCounts[endpoint]
		load := EndpointLoad{
			LeaseCount:       leaseCount,
			NormalizedLoad:   float64(leaseCount),
			UtilizationFresh: false,
		}
		key := NormalizeUtilizationKey(keyProvider, endpoint, keyModel)
		sample, ok := s.samples[key]
		if !ok && keyProvider != "" {
			// Allow provider-wide samples to match when the caller does not
			// have a fully-qualified endpoint name.
			sample, ok = s.samples[NormalizeUtilizationKey(keyProvider, "", keyModel)]
		}
		if !ok || sample.Freshness != utilization.FreshnessFresh {
			out[endpoint] = load
			continue
		}
		normalized, saturated := normalizedLoadFromSample(sample)
		load.NormalizedLoad = float64(leaseCount) + normalized
		load.UtilizationFresh = true
		load.UtilizationSaturated = saturated
		out[endpoint] = load
	}

	return out
}

func normalizedLoadFromSample(sample utilization.EndpointUtilization) (float64, bool) {
	var active int
	var queued int
	if sample.ActiveRequests != nil {
		active = *sample.ActiveRequests
	}
	if sample.QueuedRequests != nil {
		queued = *sample.QueuedRequests
	}
	if sample.MaxConcurrency != nil && *sample.MaxConcurrency > 0 {
		total := active + queued
		pressure := float64(total) / float64(*sample.MaxConcurrency)
		if pressure < 0 {
			pressure = 0
		}
		return pressure, total >= *sample.MaxConcurrency
	}
	if sample.CacheUsage != nil {
		pressure := *sample.CacheUsage
		if pressure < 0 {
			pressure = 0
		}
		return pressure, pressure >= 1
	}
	total := active + queued
	if total < 0 {
		total = 0
	}
	return float64(total), false
}

func cloneUtilization(sample utilization.EndpointUtilization) utilization.EndpointUtilization {
	out := sample
	if sample.ActiveRequests != nil {
		out.ActiveRequests = utilization.Int(*sample.ActiveRequests)
	}
	if sample.QueuedRequests != nil {
		out.QueuedRequests = utilization.Int(*sample.QueuedRequests)
	}
	if sample.CacheUsage != nil {
		out.CacheUsage = utilization.Float64(*sample.CacheUsage)
	}
	if sample.MaxConcurrency != nil {
		out.MaxConcurrency = utilization.Int(*sample.MaxConcurrency)
	}
	return out
}

// EndpointLoadList returns a deterministic endpoint ordering for tests.
func EndpointLoadList(loads map[string]EndpointLoad) []string {
	if len(loads) == 0 {
		return nil
	}
	out := make([]string, 0, len(loads))
	for endpoint := range loads {
		out = append(out, endpoint)
	}
	sort.Strings(out)
	return out
}
