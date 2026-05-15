package ds4_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/easel/fizeau/internal/provider/ds4"
	"github.com/easel/fizeau/internal/provider/openai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDS4Introspect_PropsFixture parses the recorded /props fixture and
// asserts the derived ProviderIntrospection matches expected ds4 capabilities.
func TestDS4Introspect_PropsFixture(t *testing.T) {
	fixture, err := os.ReadFile("testdata/props/ds4_props.json")
	require.NoError(t, err)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/props" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(fixture)
	}))
	defer srv.Close()

	result, err := ds4.Introspect(context.Background(), srv.URL+"/v1", "", srv.Client())
	require.NoError(t, err)
	require.NotNil(t, result)

	// AC1 assertions
	assert.Equal(t, []string{"high", "max"}, result.EffectiveReasoningLevels,
		"effective levels must exclude alias sources (low, medium, xhigh all → high)")
	assert.Equal(t, map[string]string{"low": "high", "medium": "high", "xhigh": "high"}, result.AliasMap)
	assert.Equal(t, string(openai.ThinkingWireFormatOpenAIEffort), result.EffectiveThinkingFormat)
	assert.Contains(t, result.SupportedRequestParams, "reasoning_effort")
}

// TestDS4Introspect_ConnectionRefused verifies that an unreachable server
// returns an error (caller falls through to static defaults).
func TestDS4Introspect_ConnectionRefused(t *testing.T) {
	result, err := ds4.Introspect(context.Background(), "http://127.0.0.1:1/v1", "", nil)
	assert.Error(t, err)
	assert.Nil(t, result)
}

// TestDS4Introspect_ManualSmoke runs against a live ds4 endpoint when
// DS4_BASE_URL is set. Use: DS4_BASE_URL=http://vidar:1236/v1 go test ./internal/provider/ds4/... -run TestDS4Introspect_ManualSmoke -v
func TestDS4Introspect_ManualSmoke(t *testing.T) {
	baseURL := os.Getenv("DS4_BASE_URL")
	if baseURL == "" {
		t.Skip("DS4_BASE_URL not set; skipping live introspection smoke test")
	}

	result, err := ds4.Introspect(context.Background(), baseURL, "", nil)
	require.NoError(t, err)
	require.NotNil(t, result)

	t.Logf("ThinkingFormat:           %s", result.EffectiveThinkingFormat)
	t.Logf("EffectiveReasoningLevels: %v", result.EffectiveReasoningLevels)
	t.Logf("AliasMap:                 %v", result.AliasMap)
	t.Logf("SupportedRequestParams:   %v", result.SupportedRequestParams)
	t.Logf("ServerSideReasoningFormat:%s", result.ServerSideReasoningFormat)
}
