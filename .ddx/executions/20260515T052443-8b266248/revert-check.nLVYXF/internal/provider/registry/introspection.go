package registry

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
)

// ProviderIntrospection is the structured output of a live provider
// introspection call. Fields are zero-valued when the endpoint doesn't
// supply the corresponding information.
type ProviderIntrospection struct {
	// EffectiveThinkingFormat overrides the static ThinkingFormat default.
	// Holds an openai.ThinkingWireFormat value (string alias); consumers cast
	// to openai.ThinkingWireFormat before use.
	EffectiveThinkingFormat string
	// EffectiveReasoningLevels holds the distinct effort tiers after
	// de-aliasing (e.g. ["high", "max"] for ds4 which aliases low/medium/xhigh→high).
	EffectiveReasoningLevels []string
	// AliasMap maps non-canonical effort names to their canonical targets.
	// Example: {"low": "high", "medium": "high", "xhigh": "high"}.
	AliasMap map[string]string
	// SupportedRequestParams lists the request-body parameters the server
	// accepts (from api.supported_request_parameters on ds4).
	SupportedRequestParams []string
	// ServerSideReasoningFormat is the server's current reasoning_format
	// default (e.g. "deepseek", "auto"). Only set by llama-server.
	ServerSideReasoningFormat string
	// Raw is the full unmarshaled /props payload for audit and debug.
	Raw map[string]any
}

// IntrospectionFn is the signature of a provider introspection adapter.
// The adapter fetches the provider's introspection endpoint and returns
// structured ProviderIntrospection. On failure it returns an error and
// the caller falls through to static defaults without failing construction.
type IntrospectionFn func(ctx context.Context, baseURL, model string, client *http.Client) (*ProviderIntrospection, error)

type introspectionKey struct {
	providerType string
	baseURL      string
	model        string
}

var (
	introspectionAdaptersMu sync.RWMutex
	introspectionAdapters   = map[string]IntrospectionFn{}

	introspectionCacheMu sync.Mutex
	introspectionCache   = map[introspectionKey]*ProviderIntrospection{}
)

// RegisterIntrospectionAdapter registers an introspection adapter for a
// provider type. Called automatically by Register when Descriptor.IntrospectionFn
// is non-nil. Can also be called directly (e.g. from tests).
func RegisterIntrospectionAdapter(providerType string, fn IntrospectionFn) {
	introspectionAdaptersMu.Lock()
	defer introspectionAdaptersMu.Unlock()
	introspectionAdapters[providerType] = fn
}

// IntrospectProvider runs the registered introspection adapter for providerType,
// caching results keyed by (providerType, baseURL, model) for the process
// lifetime. Returns (nil, false) when no adapter is registered or the adapter
// returns an error; a structured warning is logged on error.
//
// The cache mutex is held while the adapter fn executes, preventing concurrent
// duplicate calls for the same key during parallel provider construction.
func IntrospectProvider(ctx context.Context, providerType, baseURL, model string) (*ProviderIntrospection, bool) {
	introspectionAdaptersMu.RLock()
	fn, ok := introspectionAdapters[providerType]
	introspectionAdaptersMu.RUnlock()
	if !ok {
		return nil, false
	}

	key := introspectionKey{providerType, baseURL, model}

	introspectionCacheMu.Lock()
	defer introspectionCacheMu.Unlock()

	if cached, found := introspectionCache[key]; found {
		return cached, true
	}

	result, err := fn(ctx, baseURL, model, http.DefaultClient)
	if err != nil {
		slog.Warn("provider introspection failed; using static defaults",
			"provider_type", providerType,
			"base_url", baseURL,
			"model", model,
			"error", err)
		return nil, false
	}

	introspectionCache[key] = result
	return result, true
}
