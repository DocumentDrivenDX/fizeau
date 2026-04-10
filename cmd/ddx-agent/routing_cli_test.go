package main_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/DocumentDrivenDX/agent"
	"github.com/DocumentDrivenDX/agent/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type routingChatRequest struct {
	Model string `json:"model"`
}

type countedOpenAIServer struct {
	server          *httptest.Server
	mu              sync.Mutex
	chatCalls       int
	lastModel       string
	responseStatus  int
	responseModel   string
	responseContent string
}

func newCountedOpenAIServer(t *testing.T, responseStatus int, responseModel, responseContent string) *countedOpenAIServer {
	t.Helper()
	s := &countedOpenAIServer{
		responseStatus:  responseStatus,
		responseModel:   responseModel,
		responseContent: responseContent,
	}
	s.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/models":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":[{"id":"stub-model"}]}`))
		case "/v1/chat/completions":
			s.mu.Lock()
			s.chatCalls++
			s.mu.Unlock()

			defer r.Body.Close()
			var req routingChatRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err == nil {
				s.mu.Lock()
				s.lastModel = req.Model
				s.mu.Unlock()
			}

			if s.responseStatus != http.StatusOK {
				w.WriteHeader(s.responseStatus)
				_, _ = w.Write([]byte(`{"error":{"message":"upstream failed"}}`))
				return
			}

			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"id":"chatcmpl-route",
				"object":"chat.completion",
				"created":1712534400,
				"model":"` + s.responseModel + `",
				"choices":[{"index":0,"message":{"role":"assistant","content":"` + s.responseContent + `"},"finish_reason":"stop"}],
				"usage":{"prompt_tokens":8,"completion_tokens":4,"total_tokens":12}
			}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(s.server.Close)
	return s
}

func (s *countedOpenAIServer) baseURL() string {
	return s.server.URL + "/v1"
}

func (s *countedOpenAIServer) chatCallCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.chatCalls
}

func (s *countedOpenAIServer) requestedModel() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastModel
}

func eventDataByType(t *testing.T, events []agent.Event, eventType agent.EventType) map[string]any {
	t.Helper()
	for _, e := range events {
		if e.Type != eventType {
			continue
		}
		var payload map[string]any
		require.NoError(t, json.Unmarshal(e.Data, &payload))
		return payload
	}
	t.Fatalf("event %s not found", eventType)
	return nil
}

func latestSessionLogPath(t *testing.T, workDir string) string {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(workDir, ".agent", "sessions", "*.jsonl"))
	require.NoError(t, err)
	require.NotEmpty(t, paths, "expected at least one session log")

	latest := paths[0]
	latestInfo, err := os.Stat(latest)
	require.NoError(t, err)
	for _, path := range paths[1:] {
		info, statErr := os.Stat(path)
		require.NoError(t, statErr)
		if info.ModTime().After(latestInfo.ModTime()) {
			latest = path
			latestInfo = info
		}
	}
	return latest
}

func TestCLI_BackendRoutingAttributionFlowsIntoResultAndSession(t *testing.T) {
	exe := buildAgentCLI(t)
	workDir := t.TempDir()
	home := t.TempDir()

	vidar := newCountedOpenAIServer(t, http.StatusOK, "vidar-runtime-model", "vidar ok")
	bragi := newCountedOpenAIServer(t, http.StatusOK, "bragi-runtime-model", "bragi ok")

	writeTempConfig(t, workDir, `
providers:
  vidar:
    type: openai-compat
    base_url: `+vidar.baseURL()+`
    api_key: test
  bragi:
    type: openai-compat
    base_url: `+bragi.baseURL()+`
    api_key: test
backends:
  code-pool:
    model_ref: code-fast
    providers: [vidar, bragi]
    strategy: round-robin
default_backend: code-pool
`)

	type routingResult struct {
		Status           string `json:"status"`
		SessionID        string `json:"session_id"`
		Model            string `json:"model"`
		SelectedProvider string `json:"selected_provider"`
		SelectedRoute    string `json:"selected_route"`
		ResolvedModelRef string `json:"resolved_model_ref"`
		ResolvedModel    string `json:"resolved_model"`
	}

	first := runBuiltCLI(t, exe, workDir, testEnvWithHome(home, nil), "--json", "--work-dir", workDir, "-p", "first request")
	require.Equal(t, 0, first.exitCode, "stderr=%s", first.stderr)
	var firstResult routingResult
	require.NoError(t, json.Unmarshal([]byte(first.stdout), &firstResult), "stdout=%s", first.stdout)
	assert.Equal(t, "success", firstResult.Status)
	assert.Equal(t, "code-pool", firstResult.SelectedRoute)
	assert.Equal(t, "vidar", firstResult.SelectedProvider)
	assert.Equal(t, "qwen3-coder-next", firstResult.ResolvedModelRef)
	assert.Equal(t, "qwen/qwen3-coder-next", firstResult.ResolvedModel)
	assert.Equal(t, "qwen/qwen3-coder-next", firstResult.Model)
	assert.Equal(t, "qwen/qwen3-coder-next", vidar.requestedModel())

	firstSessionPath := latestSessionLogPath(t, workDir)
	firstEvents, err := session.ReadEvents(firstSessionPath)
	require.NoError(t, err)
	firstStart := eventDataByType(t, firstEvents, agent.EventSessionStart)
	assert.Equal(t, "vidar", firstStart["selected_provider"])
	assert.Equal(t, "code-pool", firstStart["selected_route"])
	assert.Equal(t, "qwen3-coder-next", firstStart["resolved_model_ref"])
	assert.Equal(t, "qwen/qwen3-coder-next", firstStart["resolved_model"])
	firstEnd := eventDataByType(t, firstEvents, agent.EventSessionEnd)
	assert.Equal(t, "vidar", firstEnd["selected_provider"])
	assert.Equal(t, "code-pool", firstEnd["selected_route"])
	assert.Equal(t, "qwen3-coder-next", firstEnd["resolved_model_ref"])
	assert.Equal(t, "qwen/qwen3-coder-next", firstEnd["resolved_model"])

	second := runBuiltCLI(t, exe, workDir, testEnvWithHome(home, nil), "--json", "--work-dir", workDir, "-p", "second request")
	require.Equal(t, 0, second.exitCode, "stderr=%s", second.stderr)
	var secondResult routingResult
	require.NoError(t, json.Unmarshal([]byte(second.stdout), &secondResult), "stdout=%s", second.stdout)
	assert.Equal(t, "success", secondResult.Status)
	assert.Equal(t, "code-pool", secondResult.SelectedRoute)
	assert.Equal(t, "bragi", secondResult.SelectedProvider)
	assert.Equal(t, "qwen3-coder-next", secondResult.ResolvedModelRef)
	assert.Equal(t, "qwen/qwen3-coder-next", secondResult.ResolvedModel)
	assert.Equal(t, "qwen/qwen3-coder-next", secondResult.Model)
	assert.Equal(t, "qwen/qwen3-coder-next", bragi.requestedModel())

	secondSessionPath := latestSessionLogPath(t, workDir)
	secondEvents, err := session.ReadEvents(secondSessionPath)
	require.NoError(t, err)
	secondStart := eventDataByType(t, secondEvents, agent.EventSessionStart)
	assert.Equal(t, "bragi", secondStart["selected_provider"])
	assert.Equal(t, "code-pool", secondStart["selected_route"])
	assert.Equal(t, "qwen3-coder-next", secondStart["resolved_model_ref"])
	assert.Equal(t, "qwen/qwen3-coder-next", secondStart["resolved_model"])
	secondEnd := eventDataByType(t, secondEvents, agent.EventSessionEnd)
	assert.Equal(t, "bragi", secondEnd["selected_provider"])
	assert.Equal(t, "code-pool", secondEnd["selected_route"])
	assert.Equal(t, "qwen3-coder-next", secondEnd["resolved_model_ref"])
	assert.Equal(t, "qwen/qwen3-coder-next", secondEnd["resolved_model"])

	assert.Equal(t, 1, vidar.chatCallCount())
	assert.Equal(t, 1, bragi.chatCallCount())
}

func TestCLI_BackendPoolFailureDoesNotFailoverToAnotherProvider(t *testing.T) {
	exe := buildAgentCLI(t)
	workDir := t.TempDir()
	home := t.TempDir()

	dead := newCountedOpenAIServer(t, http.StatusServiceUnavailable, "", "")
	healthy := newCountedOpenAIServer(t, http.StatusOK, "healthy-runtime-model", "healthy ok")

	writeTempConfig(t, workDir, `
providers:
  dead:
    type: openai-compat
    base_url: `+dead.baseURL()+`
    api_key: test
    model: local-dead
  healthy:
    type: openai-compat
    base_url: `+healthy.baseURL()+`
    api_key: test
    model: local-healthy
backends:
  local-pool:
    providers: [dead, healthy]
    strategy: first-available
default_backend: local-pool
`)

	res := runBuiltCLI(t, exe, workDir, testEnvWithHome(home, nil), "--work-dir", workDir, "-p", "fail request")
	require.Equal(t, 1, res.exitCode, "stdout=%s stderr=%s", res.stdout, res.stderr)
	assert.Contains(t, res.stderr, "agent: provider error")
	assert.Equal(t, 0, healthy.chatCallCount(), "phase 1 / 2A should not fail over to secondary provider")
	assert.Equal(t, 3, dead.chatCallCount(), "runtime should retry selected provider only")
}
