package testutil

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/dnaeon/go-vcr.v4/pkg/cassette"
	"gopkg.in/dnaeon/go-vcr.v4/pkg/recorder"
)

func TestVCR_ModeForEnvironment(t *testing.T) {
	t.Setenv(RecordProviderCassettesEnv, "")
	assert.Equal(t, recorder.ModeReplayOnly, ModeForEnvironment())

	t.Setenv(RecordProviderCassettesEnv, "1")
	assert.Equal(t, recorder.ModeRecordOnly, ModeForEnvironment())
}

func TestVCR_ReplayOnlyStableMatcherAndMissingReplayFails(t *testing.T) {
	cassettePath := filepath.Join(t.TempDir(), "provider-http")

	t.Setenv(RecordProviderCassettesEnv, "1")
	recorded, err := NewRecorder(cassettePath)
	require.NoError(t, err)
	require.Equal(t, recorder.ModeRecordOnly, recorded.Mode())

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/models":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"object":"list","data":[{"id":"model-a","object":"model"}]}`)
		case "/metrics":
			w.Header().Set("Content-Type", "text/plain")
			_, _ = io.WriteString(w, "requests_running 1\n")
		case "/slots":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"slots":[{"is_processing":true}],"slot_count":1}`)
		case "/v1/chat/completions":
			raw, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			require.Contains(t, string(raw), `"model":"model-a"`)
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"id":"chatcmpl-1","model":"model-a","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`)
		default:
			t.Fatalf("unexpected path recorded: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	recordClient := recorded.GetDefaultClient()
	doRecordRequest(t, recordClient, http.MethodGet, server.URL+"/v1/models", nil)
	doRecordRequest(t, recordClient, http.MethodGet, server.URL+"/metrics", nil)
	doRecordRequest(t, recordClient, http.MethodGet, server.URL+"/slots", nil)
	doRecordRequest(t, recordClient, http.MethodPost, server.URL+"/v1/chat/completions", bytes.NewBufferString(`{"model":"model-a","messages":[{"role":"user","content":"hello"}]}`))

	require.NoError(t, recorded.Stop())

	t.Setenv(RecordProviderCassettesEnv, "")
	replayed, err := NewRecorder(cassettePath)
	require.NoError(t, err)
	require.Equal(t, recorder.ModeReplayOnly, replayed.Mode())

	replayClient := replayed.GetDefaultClient()
	doReplayRequest(t, replayClient, http.MethodGet, "http://replay.invalid/v1/models", nil, true)
	doReplayRequest(t, replayClient, http.MethodGet, "http://replay.invalid/metrics", nil, true)
	doReplayRequest(t, replayClient, http.MethodGet, "http://replay.invalid/slots", nil, true)
	doReplayRequest(t, replayClient, http.MethodPost, "http://replay.invalid/v1/chat/completions", bytes.NewBufferString(`{"model":"model-a","messages":[{"role":"user","content":"hello"}]}`), true)

	_, err = doReplayRequest(t, replayClient, http.MethodGet, "http://replay.invalid/v1/does-not-exist", nil, false)
	require.Error(t, err)
	require.True(t, errors.Is(err, cassette.ErrInteractionNotFound), "expected missing replay interaction error, got %v", err)
}

func TestVCR_RedactsSecretHeadersBeforeSave(t *testing.T) {
	cassettePath := filepath.Join(t.TempDir(), "secrets")

	t.Setenv(RecordProviderCassettesEnv, "1")
	rec, err := NewRecorder(cassettePath)
	require.NoError(t, err)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	defer server.Close()

	client := rec.GetDefaultClient()
	req, err := http.NewRequest(http.MethodGet, server.URL+"/v1/models", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer secret-token")
	req.Header.Set("X-API-Key", "secret-key")
	req.Header.Set("OpenAI-API-Key", "openai-secret")

	resp, err := client.Do(req)
	require.NoError(t, err)
	_, _ = io.ReadAll(resp.Body)
	_ = resp.Body.Close()

	require.NoError(t, rec.Stop())

	raw, err := os.ReadFile(cassettePath + ".yaml")
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "Authorization")
	assert.NotContains(t, string(raw), "secret-token")
	assert.NotContains(t, string(raw), "X-API-Key")
	assert.NotContains(t, string(raw), "secret-key")
	assert.NotContains(t, string(raw), "OpenAI-API-Key")
	assert.NotContains(t, string(raw), "openai-secret")
}

func doRecordRequest(t *testing.T, client *http.Client, method, url string, body io.Reader) {
	t.Helper()
	req, err := http.NewRequest(method, url, body)
	require.NoError(t, err)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer record-secret")
	resp, err := client.Do(req)
	require.NoError(t, err)
	_, _ = io.ReadAll(resp.Body)
	_ = resp.Body.Close()
}

func doReplayRequest(t *testing.T, client *http.Client, method, url string, body io.Reader, wantOK bool) (*http.Response, error) {
	t.Helper()
	req, err := http.NewRequest(method, url, body)
	require.NoError(t, err)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer replay-secret")
	resp, err := client.Do(req)
	if !wantOK {
		return resp, err
	}
	require.NoError(t, err)
	require.NotNil(t, resp)
	_, _ = io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	return resp, nil
}
