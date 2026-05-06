package omlx

import (
	"bytes"
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

const omlxCassetteName = "omlx_utilization"

func TestOMLXUtilizationProbe_ParseStatusAndNormalize(t *testing.T) {
	body := strings.Join([]string{
		`{`,
		`  "total_requests": 9,`,
		`  "active_requests": 2,`,
		`  "waiting_requests": 3,`,
		`  "total_prompt_tokens": 1234,`,
		`  "total_completion_tokens": 567,`,
		`  "total_cached_tokens": 77,`,
		`  "cache_efficiency": 0.42,`,
		`  "avg_prefill_tps": 588.337,`,
		`  "avg_generation_tps": 43.627,`,
		`  "loaded_models": [`,
		`    "Qwen3.6-27B-MLX-8bit",`,
		`    {"id":"Qwen3.5-27B-4bit"}`,
		`  ],`,
		`  "model_memory_used_bytes": 111,`,
		`  "model_memory_max_bytes": 222,`,
		`  "uptime_seconds": 12345.5`,
		`}`,
	}, "\n")

	snapshot, err := parseOMLXStatus(body)
	require.NoError(t, err)
	require.Equal(t, 9, snapshot.TotalRequests)
	require.Equal(t, 2, snapshot.ActiveRequests)
	require.Equal(t, 3, snapshot.WaitingRequests)
	require.Equal(t, 1234, snapshot.TotalPromptTokens)
	require.Equal(t, 567, snapshot.TotalCompletionTokens)
	require.Equal(t, 77, snapshot.TotalCachedTokens)
	require.NotNil(t, snapshot.CacheEfficiency)
	require.NotNil(t, snapshot.AvgPrefillTPS)
	require.NotNil(t, snapshot.AvgGenerationTPS)
	require.NotNil(t, snapshot.ModelMemoryUsedBytes)
	require.NotNil(t, snapshot.ModelMemoryMaxBytes)
	require.NotNil(t, snapshot.UptimeSeconds)
	require.Equal(t, []string{"Qwen3.6-27B-MLX-8bit", "Qwen3.5-27B-4bit"}, snapshot.LoadedModels)
	require.InDelta(t, 0.42, *snapshot.CacheEfficiency, 1e-9)
	require.InDelta(t, 588.337, *snapshot.AvgPrefillTPS, 1e-9)
	require.InDelta(t, 43.627, *snapshot.AvgGenerationTPS, 1e-9)
	require.Equal(t, int64(111), *snapshot.ModelMemoryUsedBytes)
	require.Equal(t, int64(222), *snapshot.ModelMemoryMaxBytes)
	require.InDelta(t, 12345.5, *snapshot.UptimeSeconds, 1e-9)

	sample := snapshot.normalize()
	require.Equal(t, utilization.SourceOMLXStatus, sample.Source)
	require.Equal(t, utilization.FreshnessUnknown, sample.Freshness)
	require.NotNil(t, sample.ActiveRequests)
	require.NotNil(t, sample.QueuedRequests)
	require.NotNil(t, sample.CacheUsage)
	require.NotNil(t, sample.CachedTokens)
	require.NotNil(t, sample.TotalPromptTokens)
	require.NotNil(t, sample.TotalCompletionTokens)
	require.NotNil(t, sample.MetalActiveMemoryBytes)
	require.NotNil(t, sample.MetalPeakMemoryBytes)
	require.Equal(t, 2, *sample.ActiveRequests)
	require.Equal(t, 3, *sample.QueuedRequests)
	require.Equal(t, 77, *sample.CachedTokens)
	require.Equal(t, 1234, *sample.TotalPromptTokens)
	require.Equal(t, 567, *sample.TotalCompletionTokens)
	require.Equal(t, int64(111), *sample.MetalActiveMemoryBytes)
	require.Equal(t, int64(222), *sample.MetalPeakMemoryBytes)
	require.InDelta(t, 0.42, *sample.CacheUsage, 1e-9)
	require.InDelta(t, 43.627, *sample.TokensPerSecond, 1e-9)
}

func TestOMLXUtilizationProbe_CassetteReplay(t *testing.T) {
	if testutil.ModeForEnvironment() == recorder.ModeRecordOnly {
		t.Skip("record mode coverage is exercised in TestOMLXRecordCassetteAndUtilization")
	}

	rec, err := testutil.NewRecorder(testutil.CassettePath("testdata/cassettes", omlxCassetteName))
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, rec.Stop())
	})

	probe := NewUtilizationProbe("http://replay.invalid/v1", rec.GetDefaultClient())
	sample := probe.Probe(context.Background())

	require.Equal(t, utilization.SourceOMLXStatus, sample.Source)
	require.Equal(t, utilization.FreshnessFresh, sample.Freshness)
	require.NotNil(t, sample.ActiveRequests)
	require.NotNil(t, sample.QueuedRequests)
	require.NotNil(t, sample.CacheUsage)
	require.NotNil(t, sample.TotalPromptTokens)
	require.NotNil(t, sample.TotalCompletionTokens)
	require.NotNil(t, sample.CachedTokens)
	require.NotNil(t, sample.MetalActiveMemoryBytes)
	require.NotNil(t, sample.MetalPeakMemoryBytes)
	require.NotZero(t, sample.ObservedAt)
	require.Equal(t, 0, *sample.ActiveRequests)
	require.Equal(t, 0, *sample.QueuedRequests)
}

func TestOMLXUtilizationProbe_FailureReturnsStaleOrUnknown(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/status":
			hits++
			if hits == 1 {
				_, _ = w.Write([]byte(`{"total_requests":1,"active_requests":1,"waiting_requests":0,"cache_efficiency":0.5,"model_memory_used_bytes":111,"model_memory_max_bytes":222}`))
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

func TestOMLXUtilizationProbe_UnknownOnInitialFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, strings.TrimPrefix(r.URL.Path, "/"), http.StatusServiceUnavailable)
	}))
	t.Cleanup(srv.Close)

	probe := NewUtilizationProbe(srv.URL+"/v1", srv.Client())
	sample := probe.Probe(context.Background())

	require.Equal(t, utilization.FreshnessUnknown, sample.Freshness)
	require.Equal(t, utilization.SourceOMLXStatus, sample.Source)
	require.Nil(t, sample.ActiveRequests)
	require.Nil(t, sample.QueuedRequests)
	require.Nil(t, sample.CacheUsage)
	require.Nil(t, sample.CachedTokens)
	require.Nil(t, sample.MetalActiveMemoryBytes)
	require.Nil(t, sample.MetalPeakMemoryBytes)
}

func TestOMLXRecordCassetteAndUtilization(t *testing.T) {
	cassettePath := testutil.CassettePath(filepath.Join("testdata", "cassettes"), omlxCassetteName)

	rec, err := testutil.NewRecorder(cassettePath)
	require.NoError(t, err)
	client := rec.GetDefaultClient()
	t.Cleanup(func() {
		require.NoError(t, rec.Stop())
	})

	if testutil.ModeForEnvironment() == recorder.ModeRecordOnly {
		baseURL := liveOMLXBaseURL(t)
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
	require.Equal(t, 0, idleStatus.ActiveRequests)
	require.Equal(t, 0, idleStatus.WaitingRequests)
	require.NotNil(t, idleStatus.CacheEfficiency)
	require.NotNil(t, idleStatus.ModelMemoryUsedBytes)
	require.NotNil(t, idleStatus.ModelMemoryMaxBytes)
	require.NotEmpty(t, idleStatus.LoadedModels)

	minimalChat := chatCompletion(t, client, baseURL, model, "Reply with one short word.", 8, false)
	require.NotEmpty(t, minimalChat)

	afterChatStatus := fetchStatus(t, client, rootURL)
	require.Equal(t, 0, afterChatStatus.ActiveRequests)
	require.GreaterOrEqual(t, afterChatStatus.TotalPromptTokens, idleStatus.TotalPromptTokens)
	require.GreaterOrEqual(t, afterChatStatus.TotalCompletionTokens, idleStatus.TotalCompletionTokens)

	loadResp, err := startStreamingChat(client, baseURL, model, "Write a 200-word paragraph about a small robot.", 64)
	require.NoError(t, err)
	require.NoError(t, statusOK(loadResp.StatusCode))
	if recordMode {
		t.Cleanup(func() {
			if loadResp != nil && loadResp.Body != nil {
				_, _ = io.Copy(io.Discard, loadResp.Body)
				_ = loadResp.Body.Close()
			}
		})
	}

	loadStatus := waitForStatus(t, client, rootURL, func(s omlxStatusSnapshot) bool {
		return s.ActiveRequests > 0
	})

	require.GreaterOrEqual(t, loadStatus.ActiveRequests, 1)
	require.GreaterOrEqual(t, loadStatus.WaitingRequests, 0)
	require.NotNil(t, loadStatus.AvgPrefillTPS)
	require.NotNil(t, loadStatus.AvgGenerationTPS)
	require.NotEmpty(t, loadStatus.LoadedModels)

	if loadResp != nil && loadResp.Body != nil {
		_, _ = io.Copy(io.Discard, loadResp.Body)
		_ = loadResp.Body.Close()
	}
}

func liveOMLXBaseURL(t *testing.T) string {
	t.Helper()
	if url := strings.TrimSpace(os.Getenv("OMLX_URL")); url != "" {
		if providerReachable(t, url) {
			return url
		}
		t.Skipf("oMLX at %q is unreachable", url)
	}

	for _, candidate := range []string{"http://vidar:1235/v1", "http://localhost:1235/v1"} {
		if providerReachable(t, candidate) {
			return candidate
		}
	}

	t.Skip("No oMLX instance found for record mode (set OMLX_URL or run Vidar on port 1235)")
	return ""
}

func providerReachable(t *testing.T, baseURL string) bool {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(baseURL, "/")+"/models", nil)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return false
	}

	req, err = http.NewRequestWithContext(ctx, http.MethodGet, utilization.ServerRoot(baseURL)+"/api/status", nil)
	require.NoError(t, err)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode >= 200 && resp.StatusCode < 300
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

func fetchStatus(t *testing.T, client *http.Client, baseURL string) omlxStatusSnapshot {
	t.Helper()

	req, err := http.NewRequest(http.MethodGet, strings.TrimRight(baseURL, "/")+"/api/status", nil)
	require.NoError(t, err)
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.NoError(t, statusOK(resp.StatusCode))

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	snapshot, err := parseOMLXStatus(string(body))
	require.NoError(t, err)
	return snapshot
}

func chatCompletion(t *testing.T, client *http.Client, baseURL, model, prompt string, maxTokens int, stream bool) string {
	t.Helper()
	content, err := chatCompletionRaw(client, baseURL, model, prompt, maxTokens, stream)
	require.NoError(t, err)
	return content
}

func chatCompletionRaw(client *http.Client, baseURL, model, prompt string, maxTokens int, stream bool) (string, error) {
	body := struct {
		MaxTokens    int                    `json:"max_tokens"`
		Messages     []map[string]string    `json:"messages"`
		Model        string                 `json:"model"`
		Stream       bool                   `json:"stream,omitempty"`
		StreamOption *streamOptionsEnvelope `json:"stream_options,omitempty"`
		Temperature  int                    `json:"temperature"`
	}{
		MaxTokens:   maxTokens,
		Messages:    []map[string]string{{"role": "user", "content": prompt}},
		Model:       model,
		Stream:      stream,
		Temperature: 0,
	}
	if stream {
		body.StreamOption = &streamOptionsEnvelope{IncludeUsage: true}
	}

	raw, err := json.Marshal(body)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest(http.MethodPost, strings.TrimRight(baseURL, "/")+"/chat/completions", bytes.NewReader(raw))
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

	if stream {
		_, err := io.Copy(io.Discard, resp.Body)
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
		MaxTokens    int                    `json:"max_tokens"`
		Messages     []map[string]string    `json:"messages"`
		Model        string                 `json:"model"`
		Stream       bool                   `json:"stream,omitempty"`
		StreamOption *streamOptionsEnvelope `json:"stream_options,omitempty"`
		Temperature  int                    `json:"temperature"`
	}{
		MaxTokens:    maxTokens,
		Messages:     []map[string]string{{"role": "user", "content": prompt}},
		Model:        model,
		Stream:       true,
		StreamOption: &streamOptionsEnvelope{IncludeUsage: true},
		Temperature: 0,
	}

	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(http.MethodPost, strings.TrimRight(baseURL, "/")+"/chat/completions", bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	return client.Do(req)
}

type streamOptionsEnvelope struct {
	IncludeUsage bool `json:"include_usage"`
}

func waitForStatus(t *testing.T, client *http.Client, baseURL string, predicate func(omlxStatusSnapshot) bool) omlxStatusSnapshot {
	t.Helper()

	deadline := time.Now().Add(20 * time.Second)
	var last omlxStatusSnapshot
	for time.Now().Before(deadline) {
		last = fetchStatus(t, client, baseURL)
		if predicate(last) {
			return last
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for oMLX status predicate; last snapshot: %+v", last)
	return last
}

func statusOK(code int) error {
	if code < 200 || code >= 300 {
		return fmt.Errorf("unexpected status code %d", code)
	}
	return nil
}
