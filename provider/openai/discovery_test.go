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

func TestRankModels(t *testing.T) {
	candidates := []string{"qwen3.5-27b", "llama3.1-8b", "deepseek-r1-distill-qwen-32b"}

	t.Run("no pattern no catalog returns original order", func(t *testing.T) {
		ranked, err := RankModels(candidates, nil, "")
		require.NoError(t, err)
		require.Len(t, ranked, 3)
		assert.Equal(t, "qwen3.5-27b", ranked[0].ID)
		assert.Equal(t, 1, ranked[0].Score)
	})

	t.Run("pattern match raises score", func(t *testing.T) {
		ranked, err := RankModels(candidates, nil, "llama")
		require.NoError(t, err)
		assert.Equal(t, "llama3.1-8b", ranked[0].ID)
		assert.Equal(t, 2, ranked[0].Score)
		assert.True(t, ranked[0].PatternMatch)
	})

	t.Run("case insensitive pattern", func(t *testing.T) {
		ranked, err := RankModels(candidates, nil, "DEEPSEEK")
		require.NoError(t, err)
		assert.Equal(t, "deepseek-r1-distill-qwen-32b", ranked[0].ID)
	})

	t.Run("catalog recognized is highest score", func(t *testing.T) {
		known := map[string]string{"deepseek-r1-distill-qwen-32b": "code-high"}
		ranked, err := RankModels(candidates, known, "")
		require.NoError(t, err)
		assert.Equal(t, "deepseek-r1-distill-qwen-32b", ranked[0].ID)
		assert.Equal(t, 3, ranked[0].Score)
		assert.Equal(t, "code-high", ranked[0].CatalogRef)
	})

	t.Run("catalog beats pattern", func(t *testing.T) {
		known := map[string]string{"llama3.1-8b": "code-economy"}
		ranked, err := RankModels(candidates, known, "qwen")
		require.NoError(t, err)
		assert.Equal(t, "llama3.1-8b", ranked[0].ID)
		assert.Equal(t, 3, ranked[0].Score)
	})

	t.Run("no match falls back to first uncategorized", func(t *testing.T) {
		ranked, err := RankModels(candidates, nil, "gpt-4o")
		require.NoError(t, err)
		assert.Equal(t, "qwen3.5-27b", ranked[0].ID)
	})

	t.Run("empty candidates", func(t *testing.T) {
		ranked, err := RankModels(nil, nil, "qwen")
		require.NoError(t, err)
		assert.Empty(t, ranked)
		assert.Equal(t, "", SelectModel(ranked))
	})

	t.Run("invalid pattern returns error", func(t *testing.T) {
		_, err := RankModels(candidates, nil, "[invalid")
		require.Error(t, err)
	})
}

func TestSelectModel(t *testing.T) {
	t.Run("returns first model ID", func(t *testing.T) {
		ranked := []ScoredModel{{ID: "qwen3.5-27b", Score: 3}, {ID: "llama3.1-8b", Score: 1}}
		assert.Equal(t, "qwen3.5-27b", SelectModel(ranked))
	})

	t.Run("empty list returns empty string", func(t *testing.T) {
		assert.Equal(t, "", SelectModel(nil))
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

	// Full list should be available after resolution.
	discovered := p.DiscoveredModels()
	require.Len(t, discovered, 2)
	assert.Equal(t, "qwen3.5-27b", discovered[0].ID) // pattern-matched, ranked first
	assert.True(t, discovered[0].PatternMatch)
}

func TestProvider_KnownModelsCatalogRank(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"object": "list",
			"data": []map[string]interface{}{
				{"id": "llama3.1-8b"},
				{"id": "gpt-4o"},
				{"id": "qwen3.5-27b"},
			},
		})
	}))
	defer srv.Close()

	// Simulate catalog recognizing gpt-4o.
	known := map[string]string{"gpt-4o": "code-high"}
	p := New(Config{BaseURL: srv.URL + "/v1", KnownModels: known})
	m, err := p.resolveModel(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "gpt-4o", m) // catalog-recognized should be selected

	discovered := p.DiscoveredModels()
	require.Len(t, discovered, 3)
	assert.Equal(t, "gpt-4o", discovered[0].ID)
	assert.Equal(t, "code-high", discovered[0].CatalogRef)
	assert.Equal(t, 3, discovered[0].Score)
}
