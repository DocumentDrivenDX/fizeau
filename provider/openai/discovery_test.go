package openai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDiscoverModels(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/models", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"object": "list",
			"data": []map[string]interface{}{
				{"id": "qwen3.5-27b", "object": "model"},
				{"id": "llama3.1-8b", "object": "model"},
			},
		})
	}))
	defer srv.Close()

	models, err := DiscoverModels(context.Background(), srv.URL+"/v1", "")
	require.NoError(t, err)
	assert.Equal(t, []string{"qwen3.5-27b", "llama3.1-8b"}, models)
}

func TestDiscoverModels_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer srv.Close()

	_, err := DiscoverModels(context.Background(), srv.URL+"/v1", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "401")
}

func TestSelectModel(t *testing.T) {
	candidates := []string{"qwen3.5-27b", "llama3.1-8b", "deepseek-r1-distill-qwen-32b"}

	t.Run("no pattern returns first", func(t *testing.T) {
		m, err := SelectModel(candidates, "")
		require.NoError(t, err)
		assert.Equal(t, "qwen3.5-27b", m)
	})

	t.Run("pattern matches specific model", func(t *testing.T) {
		m, err := SelectModel(candidates, "llama")
		require.NoError(t, err)
		assert.Equal(t, "llama3.1-8b", m)
	})

	t.Run("case insensitive", func(t *testing.T) {
		m, err := SelectModel(candidates, "DEEPSEEK")
		require.NoError(t, err)
		assert.Equal(t, "deepseek-r1-distill-qwen-32b", m)
	})

	t.Run("no match falls back to first", func(t *testing.T) {
		m, err := SelectModel(candidates, "gpt-4o")
		require.NoError(t, err)
		assert.Equal(t, "qwen3.5-27b", m)
	})

	t.Run("empty candidates", func(t *testing.T) {
		m, err := SelectModel(nil, "qwen")
		require.NoError(t, err)
		assert.Equal(t, "", m)
	})

	t.Run("invalid pattern returns error", func(t *testing.T) {
		_, err := SelectModel(candidates, "[invalid")
		require.Error(t, err)
	})
}

func TestProvider_LazyModelDiscovery(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" {
			callCount++
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"object": "list",
				"data": []map[string]interface{}{
					{"id": "discovered-model-1", "object": "model"},
				},
			})
			return
		}
		// Reject actual chat requests in this unit test
		http.Error(w, "not a chat test", http.StatusBadRequest)
	}))
	defer srv.Close()

	p := New(Config{BaseURL: srv.URL + "/v1", APIKey: "test"})

	// First resolveModel call triggers discovery.
	m, err := p.resolveModel(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "discovered-model-1", m)
	assert.Equal(t, 1, callCount)

	// Second call should hit the cache — no additional HTTP request.
	m2, err := p.resolveModel(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "discovered-model-1", m2)
	assert.Equal(t, 1, callCount, "discovery endpoint should only be called once")
}

func TestProvider_ModelPatternFilter(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"object": "list",
			"data": []map[string]interface{}{
				{"id": "llama3.1-8b"},
				{"id": "qwen3.5-27b"},
			},
		})
	}))
	defer srv.Close()

	p := New(Config{BaseURL: srv.URL + "/v1", ModelPattern: "qwen"})
	m, err := p.resolveModel(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "qwen3.5-27b", m)
}
