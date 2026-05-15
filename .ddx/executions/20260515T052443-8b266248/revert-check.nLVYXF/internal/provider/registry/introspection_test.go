package registry_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/easel/fizeau/internal/provider/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIntrospectProvider_AdapterFailure verifies that a failing introspection
// adapter returns (nil, false) without panicking or failing provider construction.
func TestIntrospectProvider_AdapterFailure(t *testing.T) {
	typ := "test-introspect-fail-" + t.Name()
	registry.RegisterIntrospectionAdapter(typ, func(_ context.Context, _, _ string, _ *http.Client) (*registry.ProviderIntrospection, error) {
		return nil, errors.New("simulated introspection failure")
	})

	result, ok := registry.IntrospectProvider(context.Background(), typ, "http://localhost:9999/v1", "test-model")
	assert.False(t, ok, "failed adapter must return ok=false")
	assert.Nil(t, result, "failed adapter must return nil result")
}

// TestIntrospectProvider_CacheHit verifies that two IntrospectProvider calls
// with the same (providerType, baseURL, model) trigger exactly one adapter call.
func TestIntrospectProvider_CacheHit(t *testing.T) {
	typ := "test-introspect-cache-" + t.Name()
	callCount := 0

	registry.RegisterIntrospectionAdapter(typ, func(_ context.Context, _, _ string, _ *http.Client) (*registry.ProviderIntrospection, error) {
		callCount++
		return &registry.ProviderIntrospection{
			EffectiveThinkingFormat: "test-format",
		}, nil
	})

	r1, ok1 := registry.IntrospectProvider(context.Background(), typ, "http://localhost:8000/v1", "test-model")
	require.True(t, ok1)
	require.NotNil(t, r1)

	r2, ok2 := registry.IntrospectProvider(context.Background(), typ, "http://localhost:8000/v1", "test-model")
	require.True(t, ok2)
	require.NotNil(t, r2)

	assert.Equal(t, 1, callCount, "adapter must be called exactly once for same (base_url, model)")
	assert.Equal(t, r1, r2, "both calls must return the same cached result")
}

// TestIntrospectProvider_UnregisteredType verifies that an unknown provider
// type returns (nil, false) without error.
func TestIntrospectProvider_UnregisteredType(t *testing.T) {
	result, ok := registry.IntrospectProvider(context.Background(), "unknown-provider-type-xyz", "http://localhost:8000/v1", "")
	assert.False(t, ok)
	assert.Nil(t, result)
}

// TestIntrospectProvider_DifferentKeysMissCache verifies that different
// (baseURL, model) pairs each trigger their own adapter call.
func TestIntrospectProvider_DifferentKeysMissCache(t *testing.T) {
	typ := "test-introspect-diffkeys-" + t.Name()
	callCount := 0

	registry.RegisterIntrospectionAdapter(typ, func(_ context.Context, _, _ string, _ *http.Client) (*registry.ProviderIntrospection, error) {
		callCount++
		return &registry.ProviderIntrospection{EffectiveThinkingFormat: "fmt"}, nil
	})

	_, _ = registry.IntrospectProvider(context.Background(), typ, "http://host-a:8000/v1", "model-a")
	_, _ = registry.IntrospectProvider(context.Background(), typ, "http://host-b:8000/v1", "model-b")

	assert.Equal(t, 2, callCount, "different (baseURL, model) pairs must each invoke the adapter")
}
