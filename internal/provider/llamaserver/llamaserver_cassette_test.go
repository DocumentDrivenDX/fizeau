package llamaserver

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/easel/fizeau/internal/provider/testutil"
	"github.com/stretchr/testify/require"
	"gopkg.in/dnaeon/go-vcr.v4/pkg/recorder"
)

const (
	llamaCassetteName      = "llama_server_utilization"
	llamaRecordImage       = "ghcr.io/ggml-org/llama.cpp:server"
	llamaRecordRepo        = "QuantFactory/tinyllama-15M-alpaca-finetuned-GGUF"
	llamaRecordFile        = "tinyllama-15M-alpaca-finetuned.Q4_K_M.gguf"
	llamaRecordPrompt      = "Reply with one short word."
	llamaBusyPrompt        = "Write a 200-word paragraph about a small robot."
	llamaRequestMaxTokens  = 8
	llamaBusyRequestTokens = 64
)

type llamaMetricsSnapshot struct {
	RequestsProcessing int
	RequestsDeferred   int
	KVCacheUsageRatio  float64
}

type llamaSlotsSnapshot struct {
	SlotCount       int
	ProcessingSlots int
}

type llamaChatMessage struct {
	Content string `json:"content"`
	Role    string `json:"role"`
}

func TestLlamaServerCassetteAndUtilization(t *testing.T) {
	cassettePath := testutil.CassettePath(filepath.Join("testdata", "cassettes"), llamaCassetteName)

	rec, err := testutil.NewRecorder(cassettePath)
	require.NoError(t, err)
	client := rec.GetDefaultClient()
	t.Cleanup(func() {
		require.NoError(t, rec.Stop())
	})

	if testutil.ModeForEnvironment() == recorder.ModeRecordOnly {
		srv := mustStartLlamaServer(t)
		t.Cleanup(srv.Stop)

		recordInteractions(t, client, srv.BaseURL, srv.DirectClient, true)
		return
	}

	recordInteractions(t, client, "http://replay.invalid/v1", http.DefaultClient, false)
}

func TestLlamaServerUtilization_ParseMetricsAndSlots(t *testing.T) {
	metricSamples := []struct {
		name string
		body string
		want llamaMetricsSnapshot
	}{
		{
			name: "metrics snapshot",
			body: strings.Join([]string{
				"# HELP llamacpp:requests_processing Number of requests processing.",
				"llamacpp:requests_processing 1",
				"llamacpp:requests_deferred 2",
				"llamacpp:kv_cache_usage_ratio 0.375",
			}, "\n"),
			want: llamaMetricsSnapshot{
				RequestsProcessing: 1,
				RequestsDeferred:   2,
				KVCacheUsageRatio:  0.375,
			},
		},
	}

	for _, tc := range metricSamples {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseLlamaMetrics(tc.body)
			require.NoError(t, err)
			require.Equal(t, tc.want.RequestsProcessing, got.RequestsProcessing)
			require.Equal(t, tc.want.RequestsDeferred, got.RequestsDeferred)
			require.InDelta(t, tc.want.KVCacheUsageRatio, got.KVCacheUsageRatio, 1e-6)
		})
	}

	slotSamples := []struct {
		name string
		body string
		want llamaSlotsSnapshot
	}{
		{
			name: "array payload",
			body: `[
  {"id":0,"id_task":101,"is_processing":true},
  {"id":1,"id_task":102,"is_processing":false}
]`,
			want: llamaSlotsSnapshot{
				SlotCount:       2,
				ProcessingSlots: 1,
			},
		},
		{
			name: "object payload",
			body: `{"slots":[{"is_processing":true},{"is_processing":true}],"slot_count":2}`,
			want: llamaSlotsSnapshot{
				SlotCount:       2,
				ProcessingSlots: 2,
			},
		},
	}

	for _, tc := range slotSamples {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseLlamaSlots(tc.body)
			require.NoError(t, err)
			require.Equal(t, tc.want.SlotCount, got.SlotCount)
			require.Equal(t, tc.want.ProcessingSlots, got.ProcessingSlots)
		})
	}
}

func recordInteractions(t *testing.T, client *http.Client, baseURL string, probeClient *http.Client, recordMode bool) {
	t.Helper()
	rootURL := serverRootURL(baseURL)
	models := fetchModels(t, client, baseURL)
	require.NotEmpty(t, models)
	model := models[0]

	idleMetrics := fetchMetrics(t, client, rootURL)
	require.GreaterOrEqual(t, idleMetrics.RequestsProcessing, 0)
	require.GreaterOrEqual(t, idleMetrics.RequestsDeferred, 0)

	idleSlots := fetchSlots(t, client, rootURL)
	require.GreaterOrEqual(t, idleSlots.SlotCount, 1)
	require.Equal(t, 0, idleSlots.ProcessingSlots)

	minimalChat := chatCompletion(t, client, baseURL, model, llamaRecordPrompt, llamaRequestMaxTokens)
	require.NotEmpty(t, minimalChat)

	if recordMode {
		busyResp := startStreamingChat(t, probeClient, baseURL, model, llamaBusyPrompt, llamaBusyRequestTokens)
		t.Cleanup(func() {
			if busyResp != nil && busyResp.Body != nil {
				_, _ = io.Copy(io.Discard, busyResp.Body)
				_ = busyResp.Body.Close()
			}
		})

		waitForLlamaMetrics(t, probeClient, rootURL, func(s llamaMetricsSnapshot) bool {
			return s.RequestsProcessing > 0
		})
	}

	busyMetrics := fetchMetrics(t, client, rootURL)
	require.GreaterOrEqual(t, busyMetrics.RequestsProcessing, 1)

	busySlots := fetchSlots(t, client, rootURL)
	require.GreaterOrEqual(t, busySlots.ProcessingSlots, 1)
	require.GreaterOrEqual(t, busySlots.SlotCount, busySlots.ProcessingSlots)

	busyUtilization := combineLlamaUtilization(busyMetrics, busySlots)
	require.Greater(t, busyUtilization.KVCacheUsageRatio, 0.0)
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

func fetchMetrics(t *testing.T, client *http.Client, baseURL string) llamaMetricsSnapshot {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, strings.TrimRight(baseURL, "/")+"/metrics", nil)
	require.NoError(t, err)
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.NoError(t, statusOK(resp.StatusCode))

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	snap, err := parseLlamaMetrics(string(body))
	require.NoError(t, err)
	return snap
}

func fetchSlots(t *testing.T, client *http.Client, baseURL string) llamaSlotsSnapshot {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, strings.TrimRight(baseURL, "/")+"/slots", nil)
	require.NoError(t, err)
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.NoError(t, statusOK(resp.StatusCode))

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	snap, err := parseLlamaSlots(string(body))
	require.NoError(t, err)
	return snap
}

func chatCompletion(t *testing.T, client *http.Client, baseURL, model, prompt string, maxTokens int) string {
	t.Helper()
	content, err := chatCompletionRaw(client, baseURL, model, prompt, maxTokens)
	require.NoError(t, err)
	return content
}

func startStreamingChat(t *testing.T, client *http.Client, baseURL, model, prompt string, maxTokens int) *http.Response {
	t.Helper()
	resp, err := startStreamingChatRaw(client, baseURL, model, prompt, maxTokens)
	require.NoError(t, err)
	return resp
}

func chatCompletionRaw(client *http.Client, baseURL, model, prompt string, maxTokens int) (string, error) {
	body := struct {
		MaxTokens   int                `json:"max_tokens"`
		Messages    []llamaChatMessage `json:"messages"`
		Model       string             `json:"model"`
		Temperature int                `json:"temperature"`
	}{
		MaxTokens:   maxTokens,
		Messages:    []llamaChatMessage{{Content: prompt, Role: "user"}},
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

func startStreamingChatRaw(client *http.Client, baseURL, model, prompt string, maxTokens int) (*http.Response, error) {
	body := struct {
		MaxTokens   int                `json:"max_tokens"`
		Messages    []llamaChatMessage `json:"messages"`
		Model       string             `json:"model"`
		Temperature int                `json:"temperature"`
		Stream      bool               `json:"stream"`
	}{
		MaxTokens:   maxTokens,
		Messages:    []llamaChatMessage{{Content: prompt, Role: "user"}},
		Model:       model,
		Temperature: 0,
		Stream:      true,
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

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if err := statusOK(resp.StatusCode); err != nil {
		_ = resp.Body.Close()
		return nil, err
	}
	return resp, nil
}

func waitForLlamaMetrics(t *testing.T, client *http.Client, baseURL string, predicate func(llamaMetricsSnapshot) bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Minute)
	for time.Now().Before(deadline) {
		snap := fetchMetrics(t, client, baseURL)
		if predicate(snap) {
			return
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for llama-server metrics predicate at %s", baseURL)
}

func parseLlamaMetrics(body string) (llamaMetricsSnapshot, error) {
	out := llamaMetricsSnapshot{}
	processing, ok := parsePromMetricValue(body, "llamacpp:requests_processing")
	if !ok {
		return out, fmt.Errorf("parse llamacpp:requests_processing: not found")
	}
	deferred, ok := parsePromMetricValue(body, "llamacpp:requests_deferred")
	if !ok {
		return out, fmt.Errorf("parse llamacpp:requests_deferred: not found")
	}
	cacheUsage, ok := parsePromMetricValue(body, "llamacpp:kv_cache_usage_ratio")
	if ok {
		out.KVCacheUsageRatio = cacheUsage
	}
	out.RequestsProcessing = int(processing)
	out.RequestsDeferred = int(deferred)
	return out, nil
}

func parseLlamaSlots(body string) (llamaSlotsSnapshot, error) {
	out := llamaSlotsSnapshot{}
	trimmed := strings.TrimSpace(body)
	if trimmed == "" {
		return out, fmt.Errorf("parse /slots: empty body")
	}

	var arrayPayload []map[string]any
	if err := json.Unmarshal([]byte(trimmed), &arrayPayload); err == nil {
		out.SlotCount = len(arrayPayload)
		for _, slot := range arrayPayload {
			if processing, _ := slot["is_processing"].(bool); processing {
				out.ProcessingSlots++
			}
		}
		if out.SlotCount == 0 {
			out.SlotCount = out.ProcessingSlots
		}
		return out, nil
	}

	var objectPayload struct {
		Slots     []map[string]any `json:"slots"`
		SlotCount int              `json:"slot_count"`
	}
	if err := json.Unmarshal([]byte(trimmed), &objectPayload); err != nil {
		return out, fmt.Errorf("parse /slots: %w", err)
	}
	out.SlotCount = objectPayload.SlotCount
	if out.SlotCount == 0 {
		out.SlotCount = len(objectPayload.Slots)
	}
	for _, slot := range objectPayload.Slots {
		if processing, _ := slot["is_processing"].(bool); processing {
			out.ProcessingSlots++
		}
	}
	return out, nil
}

func parsePromMetricValue(body, metric string) (float64, bool) {
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || !strings.HasPrefix(line, metric) {
			continue
		}
		rest := strings.TrimSpace(strings.TrimPrefix(line, metric))
		if strings.HasPrefix(rest, "{") {
			if idx := strings.Index(rest, "}"); idx >= 0 {
				rest = strings.TrimSpace(rest[idx+1:])
			}
		}
		fields := strings.Fields(rest)
		if len(fields) == 0 {
			continue
		}
		val, err := strconv.ParseFloat(fields[0], 64)
		if err == nil {
			return val, true
		}
	}
	return 0, false
}

func combineLlamaUtilization(metrics llamaMetricsSnapshot, slots llamaSlotsSnapshot) llamaMetricsSnapshot {
	if metrics.KVCacheUsageRatio > 0 {
		return metrics
	}
	if slots.SlotCount > 0 {
		metrics.KVCacheUsageRatio = float64(slots.ProcessingSlots) / float64(slots.SlotCount)
	}
	return metrics
}

func statusOK(code int) error {
	if code >= 200 && code < 300 {
		return nil
	}
	return fmt.Errorf("HTTP %d", code)
}

type llamaServer struct {
	BaseURL      string
	DirectClient *http.Client
	stop         func() error
}

func (s *llamaServer) Stop() {
	if s.stop != nil {
		_ = s.stop()
	}
}

func mustStartLlamaServer(t *testing.T) *llamaServer {
	t.Helper()

	if os.Getenv(testutil.RecordProviderCassettesEnv) != "1" {
		t.Skip("record-mode bootstrap is disabled outside FIZEAU_RECORD_PROVIDER_CASSETTES=1")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	t.Cleanup(cancel)

	ensureDockerImage(t, ctx, llamaRecordImage)

	port := pickFreePort(t)
	containerName := fmt.Sprintf("fizeau-llama-server-%d", time.Now().UnixNano())
	cacheDir := filepath.Join(t.TempDir(), "hf-cache")
	require.NoError(t, os.MkdirAll(cacheDir, 0o755))

	args := []string{
		"run", "-d", "--rm",
		"--name", containerName,
		"--user", fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid()),
		"-p", fmt.Sprintf("%d:8080", port),
		"-e", "HOME=/tmp",
		"-e", "HF_HOME=/tmp/huggingface",
		"-e", "HF_HUB_DISABLE_TELEMETRY=1",
		"-v", cacheDir + ":/tmp/huggingface",
		llamaRecordImage,
		"--hf-repo", llamaRecordRepo,
		"--hf-file", llamaRecordFile,
		"--chat-template", "chatml",
		"--metrics",
		"--parallel", "1",
		"--host", "0.0.0.0",
		"--port", "8080",
		"-c", "256",
	}
	out, err := exec.CommandContext(ctx, "docker", args...).CombinedOutput()
	require.NoError(t, err, "docker run failed: %s", string(out))
	containerID := strings.TrimSpace(string(out))
	require.NotEmpty(t, containerID)

	baseURL := fmt.Sprintf("http://127.0.0.1:%d/v1", port)
	probeClient := &http.Client{Timeout: 5 * time.Second}
	waitForHTTP(t, probeClient, baseURL+"/models")

	return &llamaServer{
		BaseURL:      baseURL,
		DirectClient: &http.Client{},
		stop: func() error {
			stopCtx, stopCancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer stopCancel()
			_, _ = exec.CommandContext(stopCtx, "docker", "rm", "-f", containerID).CombinedOutput()
			return nil
		},
	}
}

func ensureDockerImage(t *testing.T, ctx context.Context, image string) {
	t.Helper()
	if _, err := exec.CommandContext(ctx, "docker", "image", "inspect", image).CombinedOutput(); err == nil {
		return
	}
	out, err := exec.CommandContext(ctx, "docker", "pull", image).CombinedOutput()
	require.NoError(t, err, "docker pull failed: %s", string(out))
}

func pickFreePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func waitForHTTP(t *testing.T, client *http.Client, endpoint string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Minute)
	for time.Now().Before(deadline) {
		req, err := http.NewRequest(http.MethodGet, endpoint, nil)
		require.NoError(t, err)
		resp, err := client.Do(req)
		if err == nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				return
			}
		}
		time.Sleep(2 * time.Second)
	}
	t.Fatalf("timed out waiting for %s", endpoint)
}

func serverRootURL(baseURL string) string {
	trimmed := strings.TrimRight(baseURL, "/")
	return strings.TrimSuffix(trimmed, "/v1")
}
