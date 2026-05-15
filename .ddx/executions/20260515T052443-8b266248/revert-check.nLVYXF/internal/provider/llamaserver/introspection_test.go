package llamaserver_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/easel/fizeau/internal/provider/llamaserver"
	"github.com/easel/fizeau/internal/provider/openai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLlamaServerIntrospect_PropsFixture parses the recorded /props fixture and
// asserts the derived ProviderIntrospection matches expected llama-server capabilities.
func TestLlamaServerIntrospect_PropsFixture(t *testing.T) {
	fixture, err := os.ReadFile("testdata/props/llama_props.json")
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

	result, err := llamaserver.Introspect(context.Background(), srv.URL+"/v1", "", srv.Client())
	require.NoError(t, err)
	require.NotNil(t, result)

	// AC2 assertions
	assert.Equal(t, string(openai.ThinkingWireFormatQwen), result.EffectiveThinkingFormat,
		"llama-server with enable_thinking in chat_template must use Qwen wire format")
	assert.Equal(t, "deepseek", result.ServerSideReasoningFormat,
		"server-side reasoning_format must reflect the fixture value")
}

// TestLlamaServerIntrospect_ConnectionRefused verifies that an unreachable
// server returns an error (caller falls through to static defaults).
func TestLlamaServerIntrospect_ConnectionRefused(t *testing.T) {
	result, err := llamaserver.Introspect(context.Background(), "http://127.0.0.1:1/v1", "", nil)
	assert.Error(t, err)
	assert.Nil(t, result)
}

// TestLlamaServerIntrospect_ManualSmoke runs against a live llama-server endpoint
// when LLAMA_BASE_URL is set.
// Use: LLAMA_BASE_URL=http://sindri:8020/v1 go test ./internal/provider/llamaserver/... -run TestLlamaServerIntrospect_ManualSmoke -v
func TestLlamaServerIntrospect_ManualSmoke(t *testing.T) {
	baseURL := os.Getenv("LLAMA_BASE_URL")
	if baseURL == "" {
		t.Skip("LLAMA_BASE_URL not set; skipping live introspection smoke test")
	}

	result, err := llamaserver.Introspect(context.Background(), baseURL, "", nil)
	require.NoError(t, err)
	require.NotNil(t, result)

	t.Logf("ThinkingFormat:            %s", result.EffectiveThinkingFormat)
	t.Logf("EffectiveReasoningLevels:  %v", result.EffectiveReasoningLevels)
	t.Logf("AliasMap:                  %v", result.AliasMap)
	t.Logf("SupportedRequestParams:    %v", result.SupportedRequestParams)
	t.Logf("ServerSideReasoningFormat: %s", result.ServerSideReasoningFormat)
}
