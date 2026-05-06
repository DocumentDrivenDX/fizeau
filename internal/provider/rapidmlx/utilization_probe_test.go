package rapidmlx

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/fizeau/internal/provider/testutil"
	"github.com/DocumentDrivenDX/fizeau/internal/provider/utilization"
	"github.com/stretchr/testify/require"
	"gopkg.in/dnaeon/go-vcr.v4/pkg/recorder"
)

const rapidMLXCassetteName = "rapidmlx_utilization"

func TestRapidMLXUtilizationProbe_ParseStatusAndNormalize(t *testing.T) {
	body := strings.Join([]string{
		`{`,
		`  "num_running": 2,`,
		`  "num_waiting": 3,`,
		`  "total_prompt_tokens": 1234,`,
		`  "total_completion_tokens": 567,`,
		`  "metal": {`,
		`    "active_bytes": 111,`,
		`    "peak_bytes": 222,`,
		`    "cache_bytes": 88`,
		`  },`,
		`  "cache": {`,
		`    "usage": 0.42,`,
		`    "hit_type": "prefix",`,
		`    "cached_tokens": 77,`,
		`    "generated_tokens": 88`,
		`  },`,
		`  "active_requests": [`,
		`    {`,
		`      "phase": "running",`,
		`      "ttft_s": 0.08,`,
		`      "tokens_per_second": 43.6,`,
		`      "cache_hit_type": "prefix",`,
		`      "cached_tokens": 16,`,
		`      "generated_tokens": 8`,
		`    }`,
		`  ]`,
		`}`,
	}, "\n")

	snapshot, err := parseRapidMLXStatus(body)
	require.NoError(t, err)
	require.Equal(t, 2, snapshot.NumRunning)
	require.Equal(t, 3, snapshot.NumWaiting)
	require.Equal(t, 1234, snapshot.TotalPromptTokens)
	require.Equal(t, 567, snapshot.TotalCompletionTokens)
	require.Len(t, snapshot.ActiveRequests, 1)
	require.NotNil(t, snapshot.CacheUsage)
	require.InDelta(t, 0.42, *snapshot.CacheUsage, 1e-9)
	require.NotNil(t, snapshot.MetalActiveMemoryBytes)
	require.NotNil(t, snapshot.MetalPeakMemoryBytes)
	require.NotNil(t, snapshot.MetalCacheMemoryBytes)
	require.Equal(t, int64(111), *snapshot.MetalActiveMemoryBytes)
	require.Equal(t, int64(222), *snapshot.MetalPeakMemoryBytes)
	require.Equal(t, int64(88), *snapshot.MetalCacheMemoryBytes)
	require.NotNil(t, snapshot.ActiveRequestPhase)
	require.Equal(t, "running", *snapshot.ActiveRequestPhase)
	require.NotNil(t, snapshot.TTFTSeconds)
	require.NotNil(t, snapshot.TokensPerSecond)
	require.InDelta(t, 0.08, *snapshot.TTFTSeconds, 1e-9)
	require.InDelta(t, 43.6, *snapshot.TokensPerSecond, 1e-9)

	sample := snapshot.normalize()
	require.Equal(t, utilization.SourceRapidMLXStatus, sample.Source)
	require.Equal(t, utilization.FreshnessUnknown, sample.Freshness)
	require.NotNil(t, sample.ActiveRequests)
	require.NotNil(t, sample.QueuedRequests)
	require.NotNil(t, sample.CacheUsage)
	require.NotNil(t, sample.TotalPromptTokens)
	require.NotNil(t, sample.TotalCompletionTokens)
	require.NotNil(t, sample.CacheHitType)
	require.NotNil(t, sample.CachedTokens)
	require.NotNil(t, sample.GeneratedTokens)
	require.NotNil(t, sample.ActiveRequestPhase)
	require.NotNil(t, sample.TTFTSeconds)
	require.NotNil(t, sample.TokensPerSecond)
	require.NotNil(t, sample.MetalActiveMemoryBytes)
	require.NotNil(t, sample.MetalPeakMemoryBytes)
	require.NotNil(t, sample.MetalCacheMemoryBytes)
	require.Equal(t, 2, *sample.ActiveRequests)
	require.Equal(t, 3, *sample.QueuedRequests)
	require.Equal(t, 1234, *sample.TotalPromptTokens)
	require.Equal(t, 567, *sample.TotalCompletionTokens)
	require.Equal(t, "prefix", *sample.CacheHitType)
	require.Equal(t, 77, *sample.CachedTokens)
	require.Equal(t, 88, *sample.GeneratedTokens)
	require.Equal(t, "running", *sample.ActiveRequestPhase)
	require.InDelta(t, 0.08, *sample.TTFTSeconds, 1e-9)
	require.InDelta(t, 43.6, *sample.TokensPerSecond, 1e-9)
}

func TestRapidMLXUtilizationProbe_FailureReturnsStaleOrUnknown(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/status":
			hits++
			if hits == 1 {
				_, _ = w.Write([]byte(`{"num_running":1,"num_waiting":0,"cache":{"usage":0.5}}`))
				return
			}
			http.Error(w, "boom", http.StatusServiceUnavailable)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	probe := NewUtilizationProbe(srv.URL+"/v1", srv.Client())
	fresh := probe.Probe(context.Background())
	require.Equal(t, utilization.FreshnessFresh, fresh.Freshness)
	require.NotNil(t, fresh.ActiveRequests)
	require.Equal(t, 1, *fresh.ActiveRequests)

	stale := probe.Probe(context.Background())
	require.Equal(t, utilization.FreshnessStale, stale.Freshness)
	require.NotNil(t, stale.ActiveRequests)
	require.Equal(t, 1, *stale.ActiveRequests)
	require.Equal(t, fresh.ObservedAt, stale.ObservedAt)
}

func TestRapidMLXUtilizationProbe_UnknownOnInitialFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, strings.TrimPrefix(r.URL.Path, "/"), http.StatusServiceUnavailable)
	}))
	t.Cleanup(srv.Close)

	probe := NewUtilizationProbe(srv.URL+"/v1", srv.Client())
	sample := probe.Probe(context.Background())

	require.Equal(t, utilization.FreshnessUnknown, sample.Freshness)
	require.Equal(t, utilization.SourceRapidMLXStatus, sample.Source)
	require.Nil(t, sample.ActiveRequests)
	require.Nil(t, sample.QueuedRequests)
	require.Nil(t, sample.CacheUsage)
	require.Nil(t, sample.TotalPromptTokens)
	require.Nil(t, sample.TotalCompletionTokens)
}

func TestRapidMLXRecordCassetteAndUtilization(t *testing.T) {
	cassettePath := testutil.CassettePath(filepath.Join("testdata", "cassettes"), rapidMLXCassetteName)

	rec, err := testutil.NewRecorder(cassettePath)
	require.NoError(t, err)
	client := rec.GetDefaultClient()
	t.Cleanup(func() {
		require.NoError(t, rec.Stop())
	})

	if testutil.ModeForEnvironment() == recorder.ModeRecordOnly {
		baseURL := os.Getenv("RAPID_MLX_RECORD_BASE_URL")
		if baseURL == "" {
			t.Skip("RAPID_MLX_RECORD_BASE_URL is required in record mode")
		}
		recordInteractions(t, client, baseURL, true)
		return
	}

	recordInteractions(t, client, "http://replay.invalid/v1", false)
}

func recordInteractions(t *testing.T, client *http.Client, baseURL string, recordMode bool) {
	t.Helper()
	rootURL := utilization.ServerRoot(baseURL)

	models := fetchModels(t, client, baseURL)
	require.NotEmpty(t, models)
	model := models[0]

	idleStatus := fetchStatus(t, client, rootURL)
	require.Equal(t, 0, idleStatus.NumRunning)
	require.Equal(t, 0, idleStatus.NumWaiting)
	require.NotNil(t, idleStatus.CacheUsage)
	require.NotNil(t, idleStatus.MetalActiveMemoryBytes)
	require.NotNil(t, idleStatus.MetalPeakMemoryBytes)

	minimalChat := chatCompletion(t, client, baseURL, model, "Reply with one short word.", 8)
	require.NotEmpty(t, minimalChat)

	afterChatStatus := fetchStatus(t, client, rootURL)
	require.Equal(t, 0, afterChatStatus.NumRunning)
	require.GreaterOrEqual(t, afterChatStatus.TotalPromptTokens, idleStatus.TotalPromptTokens)
	require.GreaterOrEqual(t, afterChatStatus.TotalCompletionTokens, idleStatus.TotalCompletionTokens)

	loadResp, err := startStreamingChat(client, baseURL, model, "Write a 200-word paragraph about a small robot.", 64)
	require.NoError(t, err)
	if recordMode {
		t.Cleanup(func() {
			if loadResp != nil && loadResp.Body != nil {
				_, _ = io.Copy(io.Discard, loadResp.Body)
				_ = loadResp.Body.Close()
			}
		})
	}

	loadStatus := waitForStatus(t, client, rootURL, func(s rapidMLXStatusSnapshot) bool {
		return s.NumRunning > 0
	})

	require.GreaterOrEqual(t, loadStatus.NumRunning, 1)
	require.NotNil(t, loadStatus.TTFTSeconds)
	require.NotNil(t, loadStatus.TokensPerSecond)
	require.NotNil(t, loadStatus.ActiveRequestPhase)
	require.NotNil(t, loadStatus.CacheHitType)

	if loadResp != nil && loadResp.Body != nil {
		_, _ = io.Copy(io.Discard, loadResp.Body)
		_ = loadResp.Body.Close()
	}
}

func fetchModels(t *testing.T, client *http.Client, baseURL string) []string {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, strings.TrimRight(baseURL, "/")+"/models", nil)
	require.NoError(t, err)
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.NoError(t, statusOK(resp.StatusCode))

	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&payload))
	models := make([]string, 0, len(payload.Data))
	for _, entry := range payload.Data {
		if entry.ID != "" {
			models = append(models, entry.ID)
		}
	}
	return models
}

func fetchStatus(t *testing.T, client *http.Client, baseURL string) rapidMLXStatusSnapshot {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, strings.TrimRight(baseURL, "/")+"/status", nil)
	require.NoError(t, err)
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.NoError(t, statusOK(resp.StatusCode))

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	snapshot, err := parseRapidMLXStatus(string(body))
	require.NoError(t, err)
	return snapshot
}

func chatCompletion(t *testing.T, client *http.Client, baseURL, model, prompt string, maxTokens int) string {
	t.Helper()
	content, err := chatCompletionRaw(client, baseURL, model, prompt, maxTokens)
	require.NoError(t, err)
	return content
}

func chatCompletionRaw(client *http.Client, baseURL, model, prompt string, maxTokens int) (string, error) {
	body := struct {
		MaxTokens   int                 `json:"max_tokens"`
		Messages    []map[string]string `json:"messages"`
		Model       string              `json:"model"`
		Temperature int                 `json:"temperature"`
	}{
		MaxTokens:   maxTokens,
		Messages:    []map[string]string{{"role": "user", "content": prompt}},
		Model:       model,
		Temperature: 0,
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest(http.MethodPost, strings.TrimRight(baseURL, "/")+"/chat/completions", strings.NewReader(string(raw)))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if err := statusOK(resp.StatusCode); err != nil {
		return "", err
	}

	var payload struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", err
	}
	if len(payload.Choices) == 0 {
		return "", fmt.Errorf("chat completion returned no choices")
	}
	return payload.Choices[0].Message.Content, nil
}

func startStreamingChat(client *http.Client, baseURL, model, prompt string, maxTokens int) (*http.Response, error) {
	body := struct {
		MaxTokens   int                 `json:"max_tokens"`
		Messages    []map[string]string `json:"messages"`
		Model       string              `json:"model"`
		Stream      bool                `json:"stream"`
		Temperature int                 `json:"temperature"`
	}{
		MaxTokens:   maxTokens,
		Messages:    []map[string]string{{"role": "user", "content": prompt}},
		Model:       model,
		Stream:      true,
		Temperature: 0,
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(http.MethodPost, strings.TrimRight(baseURL, "/")+"/chat/completions", strings.NewReader(string(raw)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	return client.Do(req)
}

func waitForStatus(t *testing.T, client *http.Client, baseURL string, predicate func(rapidMLXStatusSnapshot) bool) rapidMLXStatusSnapshot {
	t.Helper()
	deadline := time.Now().Add(2 * time.Minute)
	for time.Now().Before(deadline) {
		snapshot := fetchStatus(t, client, baseURL)
		if predicate(snapshot) {
			return snapshot
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for rapid-mlx status predicate at %s", baseURL)
	return rapidMLXStatusSnapshot{}
}

func statusOK(code int) error {
	if code < 200 || code >= 300 {
		return fmt.Errorf("HTTP %d", code)
	}
	return nil
}
