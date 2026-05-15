package openrouter

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	agent "github.com/easel/fizeau/internal/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLookupModelLimits(t *testing.T) {
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		if r.URL.Path != "/v1/models" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{
					"id":             "openai/gpt-4o-mini",
					"context_length": 128_000,
					"top_provider": map[string]any{
						"max_completion_tokens": 16_384,
					},
				},
			},
		})
	}))
	defer srv.Close()

	got := LookupModelLimits(context.Background(), srv.URL+"/v1", "sk-test", nil, "openai/gpt-4o-mini")
	assert.Equal(t, "Bearer sk-test", auth)
	assert.Equal(t, 128_000, got.ContextLength)
	assert.Equal(t, 16_384, got.MaxCompletionTokens)
}

func TestUsageCostAttribution(t *testing.T) {
	got, ok := UsageCostAttribution(`{"prompt_tokens":12,"completion_tokens":5,"total_tokens":17,"cost":0.00321}`)
	require.True(t, ok)
	require.NotNil(t, got.Amount)
	assert.Equal(t, agent.CostSourceGatewayReported, got.Source)
	assert.Equal(t, "USD", got.Currency)
	assert.Equal(t, "openrouter/usage.cost", got.PricingRef)
	assert.InDelta(t, 0.00321, *got.Amount, 1e-12)
}

func TestProtocolCapabilities(t *testing.T) {
	p := New(Config{BaseURL: "https://openrouter.ai/api/v1"})
	assert.True(t, p.SupportsTools())
	assert.True(t, p.SupportsStream())
	assert.True(t, p.SupportsStructuredOutput())
	assert.True(t, p.SupportsThinking())
}
