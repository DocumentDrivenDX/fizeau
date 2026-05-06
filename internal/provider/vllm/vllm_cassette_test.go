package vllm

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/fizeau/internal/provider/testutil"
	"github.com/stretchr/testify/require"
	"gopkg.in/dnaeon/go-vcr.v4/pkg/recorder"
)

const (
	vllmCassetteName       = "vllm_utilization"
	vllmRecordImageX86     = "vllm/vllm-openai-cpu:latest-x86_64"
	vllmRecordImageArm     = "vllm/vllm-openai-cpu:latest-arm64"
	vllmRecordModel        = "facebook/opt-125m"
	vllmRecordPrompt       = "Reply with one short word."
	vllmLoadPrompt         = "Write a 200-word paragraph about a small robot."
	vllmRequestModelName   = vllmRecordModel
	vllmRequestMaxTokens   = 8
	vllmLoadMaxTokens      = 96
	vllmRequestTemperature = 0
)

const simpleChatTemplate = `{% for message in messages %}
{% if message['role'] == 'user' %}User: {{ message['content'] }}
{% elif message['role'] == 'assistant' %}Assistant: {{ message['content'] }}
{% endif %}
{% endfor %}Assistant:`

type vllmMetricsSnapshot struct {
	Running      int
	Waiting      int
	CacheUsage   float64
	CacheMetric  string
	ObservedText string
}

func TestVLLMRecordCassetteAndUtilization(t *testing.T) {
	cassettePath := testutil.CassettePath(filepath.Join("testdata", "cassettes"), vllmCassetteName)

	rec, err := testutil.NewRecorder(cassettePath)
	require.NoError(t, err)
	client := rec.GetDefaultClient()
	t.Cleanup(func() {
		require.NoError(t, rec.Stop())
	})

	if testutil.ModeForEnvironment() == recorder.ModeRecordOnly {
		srv := mustStartVLLMServer(t)
		t.Cleanup(srv.Stop)

		recordInteractions(t, client, srv.BaseURL, srv.DirectClient, true)
		return
	}

	recordInteractions(t, client, "http://replay.invalid/v1", http.DefaultClient, false)
}

func TestVLLMUtilization_ParseMetrics(t *testing.T) {
	samples := []struct {
		name string
		body string
		want vllmMetricsSnapshot
	}{
		{
			name: "v1 metrics",
			body: strings.Join([]string{
				"# HELP vllm:num_requests_running Number of requests currently running.",
				"vllm:num_requests_running 1",
				"vllm:num_requests_waiting 2",
				"vllm:kv_cache_usage_perc 0.25",
			}, "\n"),
			want: vllmMetricsSnapshot{Running: 1, Waiting: 2, CacheUsage: 0.25, CacheMetric: "vllm:kv_cache_usage_perc"},
		},
		{
			name: "legacy cache metric",
			body: strings.Join([]string{
				"vllm:num_requests_running 0",
				"vllm:num_requests_waiting 3",
				"vllm:gpu_cache_usage_perc 0.50",
			}, "\n"),
			want: vllmMetricsSnapshot{Running: 0, Waiting: 3, CacheUsage: 0.50, CacheMetric: "vllm:gpu_cache_usage_perc"},
		},
	}

	for _, tc := range samples {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseVLLMMetrics(tc.body)
			require.NoError(t, err)
			require.Equal(t, tc.want.Running, got.Running)
			require.Equal(t, tc.want.Waiting, got.Waiting)
			require.InDelta(t, tc.want.CacheUsage, got.CacheUsage, 1e-6)
			require.Equal(t, tc.want.CacheMetric, got.CacheMetric)
		})
	}
}

func recordInteractions(t *testing.T, client *http.Client, baseURL string, probeClient *http.Client, recordMode bool) {
	t.Helper()
	rootURL := serverRootURL(baseURL)
	models := fetchModels(t, client, baseURL)
	require.Contains(t, models, vllmRequestModelName)

	idleMetrics := fetchMetrics(t, client, rootURL)
	require.Equal(t, 0, idleMetrics.Running)
	require.Equal(t, 0, idleMetrics.Waiting)
	require.GreaterOrEqual(t, idleMetrics.CacheUsage, 0.0)
	require.NotEmpty(t, idleMetrics.CacheMetric)

	minimalChat := chatCompletion(t, client, baseURL, vllmRecordPrompt, vllmRequestMaxTokens)
	require.NotEmpty(t, minimalChat)

	afterChatMetrics := fetchMetrics(t, client, rootURL)
	require.GreaterOrEqual(t, afterChatMetrics.Running, 0)
	require.GreaterOrEqual(t, afterChatMetrics.Waiting, 0)
	require.NotEmpty(t, afterChatMetrics.CacheMetric)

	if recordMode {
		loadResp, err := startStreamingLoadRequest(probeClient, strings.TrimRight(baseURL, "/"), vllmLoadPrompt, vllmLoadMaxTokens)
		require.NoError(t, err)

		waitForMetric(t, probeClient, rootURL, func(s vllmMetricsSnapshot) bool {
			return s.Running > 0
		})

		loadMetrics := fetchMetrics(t, client, rootURL)
		require.GreaterOrEqual(t, loadMetrics.Running, 1, "load metrics must capture an in-flight request")
		require.GreaterOrEqual(t, loadMetrics.Waiting, 0)
		require.NotEmpty(t, loadMetrics.CacheMetric)

		waitForMetric(t, probeClient, rootURL, func(s vllmMetricsSnapshot) bool {
			return s.Running == 0
		})

		_, _ = io.Copy(io.Discard, loadResp.Body)
		require.NoError(t, loadResp.Body.Close())
		return
	}

	loadMetrics := fetchMetrics(t, client, rootURL)
	require.GreaterOrEqual(t, loadMetrics.Running, 0)
	require.GreaterOrEqual(t, loadMetrics.Waiting, 0)
	require.NotEmpty(t, loadMetrics.CacheMetric)
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
		models = append(models, entry.ID)
	}
	return models
}

func fetchMetrics(t *testing.T, client *http.Client, baseURL string) vllmMetricsSnapshot {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, strings.TrimRight(baseURL, "/")+"/metrics", nil)
	require.NoError(t, err)
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.NoError(t, statusOK(resp.StatusCode))

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	snap, err := parseVLLMMetrics(string(body))
	require.NoError(t, err)
	snap.ObservedText = string(body)
	return snap
}

func chatCompletion(t *testing.T, client *http.Client, baseURL, prompt string, maxTokens int) string {
	t.Helper()
	content, err := chatCompletionRaw(client, baseURL, prompt, maxTokens)
	require.NoError(t, err)
	return content
}

func chatCompletionRaw(client *http.Client, baseURL, prompt string, maxTokens int) (string, error) {
	body := map[string]any{
		"model":       vllmRequestModelName,
		"messages":    []map[string]string{{"role": "user", "content": prompt}},
		"max_tokens":  maxTokens,
		"temperature": vllmRequestTemperature,
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

func startStreamingLoadRequest(client *http.Client, baseURL, prompt string, maxTokens int) (*http.Response, error) {
	body := map[string]any{
		"model":       vllmRequestModelName,
		"messages":    []map[string]string{{"role": "user", "content": prompt}},
		"max_tokens":  maxTokens,
		"temperature": vllmRequestTemperature,
		"stream":      true,
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

func waitForMetric(t *testing.T, client *http.Client, baseURL string, predicate func(vllmMetricsSnapshot) bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Minute)
	for time.Now().Before(deadline) {
		snap := fetchMetrics(t, client, baseURL)
		if predicate(snap) {
			return
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for vllm metrics predicate at %s", baseURL)
}

func parseVLLMMetrics(body string) (vllmMetricsSnapshot, error) {
	out := vllmMetricsSnapshot{}
	running, ok := parsePromMetricValue(body, "vllm:num_requests_running")
	if !ok {
		return out, fmt.Errorf("parse vllm:num_requests_running: not found")
	}
	waiting, ok := parsePromMetricValue(body, "vllm:num_requests_waiting")
	if !ok {
		return out, fmt.Errorf("parse vllm:num_requests_waiting: not found")
	}
	cacheMetric := "vllm:kv_cache_usage_perc"
	cacheUsage, ok := parsePromMetricValue(body, cacheMetric)
	if !ok {
		cacheMetric = "vllm:gpu_cache_usage_perc"
		cacheUsage, ok = parsePromMetricValue(body, cacheMetric)
	}
	if !ok {
		return out, fmt.Errorf("parse vllm cache metric: kv_cache_usage_perc or gpu_cache_usage_perc not found")
	}
	out.Running = int(running)
	out.Waiting = int(waiting)
	out.CacheUsage = cacheUsage
	out.CacheMetric = cacheMetric
	return out, nil
}

func parsePromMetricValue(body, metric string) (float64, bool) {
	scanner := bufio.NewScanner(strings.NewReader(body))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
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

func statusOK(code int) error {
	if code >= 200 && code < 300 {
		return nil
	}
	return fmt.Errorf("HTTP %d", code)
}

type vllmServer struct {
	BaseURL      string
	DirectClient *http.Client
	stop         func() error
}

func (s *vllmServer) Stop() {
	if s.stop != nil {
		_ = s.stop()
	}
}

func mustStartVLLMServer(t *testing.T) *vllmServer {
	t.Helper()

	if os.Getenv(testutil.RecordProviderCassettesEnv) != "1" {
		t.Skip("record-mode bootstrap is disabled outside FIZEAU_RECORD_PROVIDER_CASSETTES=1")
	}
	if runtime.GOOS != "linux" {
		t.Skip("vllm record bootstrap currently expects docker on linux")
	}

	image := vllmRecordImageX86
	switch runtime.GOARCH {
	case "amd64":
		image = vllmRecordImageX86
	case "arm64":
		image = vllmRecordImageArm
	default:
		t.Skipf("no vllm CPU image mapping for GOARCH=%s", runtime.GOARCH)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	t.Cleanup(cancel)

	ensureDockerImage(t, ctx, image)

	port := pickFreePort(t)
	containerName := fmt.Sprintf("fizeau-vllm-%d", time.Now().UnixNano())
	templateDir := filepath.Join(t.TempDir(), "templates")
	require.NoError(t, os.MkdirAll(templateDir, 0o755))
	templatePath := filepath.Join(templateDir, "simple-chat.jinja")
	require.NoError(t, os.WriteFile(templatePath, []byte(simpleChatTemplate), 0o600))
	args := []string{
		"run", "-d", "--rm",
		"--name", containerName,
		"--network", "host",
		"--shm-size=4g",
		"--cap-add", "SYS_NICE",
		"--security-opt", "seccomp=unconfined",
		"-e", "HF_HOME=/tmp/huggingface",
		"-e", "HF_HUB_DISABLE_TELEMETRY=1",
		"-e", "VLLM_CPU_KVCACHE_SPACE=4",
		"-e", "VLLM_CPU_NUM_OF_RESERVED_CPU=1",
		"-v", templateDir + ":/templates",
		image,
		vllmRecordModel,
		"--dtype=bfloat16",
		"--max-model-len=128",
		"--max-num-seqs=1",
		"--max-num-batched-tokens=128",
		"--host=127.0.0.1",
		"--port", strconv.Itoa(port),
		"--chat-template", "/templates/simple-chat.jinja",
	}
	out, err := exec.CommandContext(ctx, "docker", args...).CombinedOutput()
	require.NoError(t, err, "docker run failed: %s", string(out))
	containerID := strings.TrimSpace(string(out))
	require.NotEmpty(t, containerID)

	baseURL := fmt.Sprintf("http://127.0.0.1:%d/v1", port)
	probeClient := &http.Client{Timeout: 5 * time.Second}
	waitForHTTP(t, probeClient, baseURL+"/models")

	return &vllmServer{
		BaseURL:      baseURL,
		DirectClient: probeClient,
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
