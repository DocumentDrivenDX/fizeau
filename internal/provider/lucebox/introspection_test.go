package lucebox_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/easel/fizeau/internal/provider/lucebox"
	"github.com/easel/fizeau/internal/provider/openai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLuceboxIntrospect_PropsFixture(t *testing.T) {
	fixture, err := os.ReadFile("testdata/props/lucebox_props.json")
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

	result, err := lucebox.Introspect(context.Background(), srv.URL+"/v1", "", srv.Client())
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Empty(t, result.EffectiveThinkingFormat, "current lucebox props do not advertise a request-side reasoning control")
	assert.Empty(t, result.SupportedRequestParams)
	require.NotNil(t, result.Raw)
	assert.Equal(t, "luce-dflash", result.Raw["server"].(map[string]any)["name"])
}

func TestLuceboxIntrospect_DS4StyleRequestParams(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/props" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"model": {"id": "luce-dflash"},
			"reasoning": {
				"supported_efforts": ["low", "medium", "high", "max"],
				"aliases": {"low": "medium"}
			},
			"api": {"supported_request_parameters": ["enable_thinking", "thinking_budget", "max_tokens"]}
		}`))
	}))
	defer srv.Close()

	result, err := lucebox.Introspect(context.Background(), srv.URL+"/v1", "", srv.Client())
	require.NoError(t, err)

	assert.Equal(t, string(openai.ThinkingWireFormatQwen), result.EffectiveThinkingFormat)
	assert.Equal(t, []string{"medium", "high", "max"}, result.EffectiveReasoningLevels)
	assert.Equal(t, map[string]string{"low": "medium"}, result.AliasMap)
	assert.Contains(t, result.SupportedRequestParams, "enable_thinking")
}

func TestLuceboxIntrospect_ConnectionRefused(t *testing.T) {
	result, err := lucebox.Introspect(context.Background(), "http://127.0.0.1:1/v1", "", nil)
	assert.Error(t, err)
	assert.Nil(t, result)
}

func TestLuceboxIntrospect_ManualSmoke(t *testing.T) {
	baseURL := os.Getenv("LUCEBOX_BASE_URL")
	if baseURL == "" {
		t.Skip("LUCEBOX_BASE_URL not set; skipping live introspection smoke test")
	}

	result, err := lucebox.Introspect(context.Background(), baseURL, "", nil)
	require.NoError(t, err)
	require.NotNil(t, result)

	t.Logf("ThinkingFormat:           %s", result.EffectiveThinkingFormat)
	t.Logf("EffectiveReasoningLevels: %v", result.EffectiveReasoningLevels)
	t.Logf("AliasMap:                 %v", result.AliasMap)
	t.Logf("SupportedRequestParams:   %v", result.SupportedRequestParams)
	t.Logf("Raw keys:                 %v", result.Raw)
}
