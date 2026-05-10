//go:build integration

package openai_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/easel/fizeau/internal/provider/lmstudio"
	"github.com/easel/fizeau/internal/provider/omlx"
	"github.com/easel/fizeau/internal/provider/openai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func getAndDecode(ctx context.Context, timeout time.Duration, endpoint string, out any) error {
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func lmStudioURLForDiscovery(t *testing.T) string {
	t.Helper()
	if url := os.Getenv("LMSTUDIO_URL"); url != "" {
		if providerReachable(t, url) {
			return url
		}
		t.Skipf("LM Studio at %q is unreachable", url)
	}

	for _, host := range []string{"vidar:1234", "bragi:1234"} {
		url := fmt.Sprintf("http://%s/v1", host)
		if providerReachable(t, url) {
			return url
		}
	}

	t.Skip("No LM Studio instance found for discovery tests (set LMSTUDIO_URL)")
	return ""
}

func omlxURL(t *testing.T) string {
	t.Helper()
	if url := os.Getenv("OMLX_URL"); url != "" {
		if providerReachable(t, url) {
			return url
		}
		t.Skipf("oMLX at %q is unreachable", url)
	}

	for _, host := range []string{"vidar:1235", "localhost:1235"} {
		url := fmt.Sprintf("http://%s/v1", host)
		if providerReachable(t, url) {
			return url
		}
	}

	t.Skip("No oMLX instance found for discovery tests (set OMLX_URL)")
	return ""
}

func providerReachable(t *testing.T, baseURL string) bool {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := openai.DiscoverModels(ctx, baseURL, "")
	return err == nil
}

func firstDiscoveredModel(t *testing.T, baseURL string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ids, err := openai.DiscoverModels(ctx, baseURL, "")
	require.NoError(t, err)
	require.NotEmpty(t, ids)
	return ids[0]
}

func TestIntegration_LookupModelLimits_LMStudio(t *testing.T) {
	baseURL := lmStudioURLForDiscovery(t)
	model := firstDiscoveredModel(t, baseURL)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	limits := lmstudio.LookupModelLimits(ctx, baseURL, model)
	require.Greater(t, limits.ContextLength, 0)

	root := strings.TrimSuffix(strings.TrimRight(baseURL, "/"), "/v1")
	var info struct {
		LoadedContextLength int `json:"loaded_context_length"`
		MaxContextLength    int `json:"max_context_length"`
	}
	err := getAndDecode(ctx, 5*time.Second, root+"/api/v0/models/"+url.PathEscape(model), &info)
	require.NoError(t, err)
	require.Greater(t, info.MaxContextLength, 0)
	require.Greater(t, info.LoadedContextLength, 0)
	assert.Equal(t, info.LoadedContextLength, limits.ContextLength)
}

func TestIntegration_LookupModelLimits_Omlx(t *testing.T) {
	baseURL := omlxURL(t)
	model := firstDiscoveredModel(t, baseURL)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	limits := omlx.LookupModelLimits(ctx, baseURL, model)
	require.Greater(t, limits.ContextLength, 0)
	require.Greater(t, limits.MaxCompletionTokens, 0)

	var status struct {
		Models []struct {
			ID               string `json:"id"`
			MaxContextWindow int    `json:"max_context_window"`
			MaxTokens        int    `json:"max_tokens"`
		} `json:"models"`
	}
	err := getAndDecode(ctx, 5*time.Second, strings.TrimRight(baseURL, "/")+"/models/status", &status)
	require.NoError(t, err)

	for _, entry := range status.Models {
		if strings.EqualFold(entry.ID, model) {
			assert.Equal(t, entry.MaxContextWindow, limits.ContextLength)
			assert.Equal(t, entry.MaxTokens, limits.MaxCompletionTokens)
			return
		}
	}
	t.Fatalf("model %q not found in oMLX status payload", model)
}

func TestIntegration_DiscoverModels_LMStudio(t *testing.T) {
	baseURL := lmStudioURLForDiscovery(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ids, err := openai.DiscoverModels(ctx, baseURL, "")
	require.NoError(t, err)
	assert.NotEmpty(t, ids)
}

func TestIntegration_DiscoverModels_Omlx(t *testing.T) {
	baseURL := omlxURL(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ids, err := openai.DiscoverModels(ctx, baseURL, "")
	require.NoError(t, err)
	assert.NotEmpty(t, ids)
}
