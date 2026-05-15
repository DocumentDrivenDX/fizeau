package openai

import (
	"context"
	"errors"
	"fmt"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestReachabilityError_ErrorsIs(t *testing.T) {
	err := newReachabilityError("http://host:1234/v1", "probe_models", 502, errors.New("bad gateway"))
	assert.True(t, errors.Is(err, ErrEndpointUnreachable),
		"errors.Is must surface the sentinel regardless of underlying cause")
}

func TestReachabilityError_Unwrap(t *testing.T) {
	cause := errors.New("dial tcp: connection refused")
	err := newReachabilityError("http://host:1234/v1", "probe_models", 0, cause)
	assert.Same(t, cause, errors.Unwrap(err), "Unwrap must return the original cause")
}

func TestReachabilityError_Message(t *testing.T) {
	t.Run("with status code", func(t *testing.T) {
		err := newReachabilityError("http://host:1234/v1", "chat_completions", 502, errors.New("bad gateway"))
		msg := err.Error()
		assert.Contains(t, msg, "http://host:1234/v1")
		assert.Contains(t, msg, "chat_completions")
		assert.Contains(t, msg, "HTTP 502")
		assert.Contains(t, msg, "bad gateway")
	})
	t.Run("without status code", func(t *testing.T) {
		err := newReachabilityError("http://host:1234/v1", "probe_models", 0, errors.New("dial failed"))
		assert.NotContains(t, err.Error(), "HTTP 0")
	})
}

func TestClassifyHTTPStatus(t *testing.T) {
	assert.Nil(t, ClassifyHTTPStatus("url", "op", 200, ""), "2xx is not unreachable")
	assert.Nil(t, ClassifyHTTPStatus("url", "op", 404, ""), "4xx is not unreachable (handled by ShouldFailover)")
	assert.Nil(t, ClassifyHTTPStatus("url", "op", 429, ""), "429 is client-bound, not unreachable")

	err := ClassifyHTTPStatus("url", "op", 502, "Bad Gateway")
	assert.True(t, errors.Is(err, ErrEndpointUnreachable), "5xx wraps as ReachabilityError")

	err = ClassifyHTTPStatus("url", "op", 503, "Service Unavailable")
	assert.True(t, errors.Is(err, ErrEndpointUnreachable))
}

// testNetErr is a minimal net.Error used by the net.Error classification path.
type testNetErr struct{ msg string }

func (e testNetErr) Error() string   { return e.msg }
func (e testNetErr) Timeout() bool   { return true }
func (e testNetErr) Temporary() bool { return true }

// assert testNetErr implements net.Error at compile time.
var _ net.Error = testNetErr{}

func TestClassifyNetwork(t *testing.T) {
	cases := []struct {
		name     string
		err      error
		wantWrap bool // true → expect ReachabilityError wrap
	}{
		{"nil passes through", nil, false},
		{"context canceled bubbles", context.Canceled, false},
		{"deadline exceeded bubbles", context.DeadlineExceeded, false},
		{"net.Error wraps", testNetErr{msg: "dial timeout"}, true},
		{"connection refused string-match wraps", errors.New("dial tcp 127.0.0.1:1234: connection refused"), true},
		{"TLS handshake error wraps", errors.New("tls: handshake failure"), true},
		{"broken pipe wraps", errors.New("write: broken pipe"), true},
		{"unrelated error bubbles", errors.New("some random app error"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ClassifyNetwork("http://host/v1", "chat_completions", tc.err)
			if tc.err == nil {
				assert.Nil(t, got)
				return
			}
			if tc.wantWrap {
				assert.True(t, errors.Is(got, ErrEndpointUnreachable),
					"expected ReachabilityError wrap for %q", tc.err.Error())
			} else {
				assert.False(t, errors.Is(got, ErrEndpointUnreachable),
					"expected %q to bubble without wrapping", tc.err.Error())
			}
		})
	}
}

func TestIsModelNotFound(t *testing.T) {
	cases := []struct {
		body string
		want bool
	}{
		{`{"error":{"code":"model_not_found","message":"Model 'foo' not found"}}`, true},
		{`{"error":{"code":"model_not_supported"}}`, true},
		{`{"error":{"code":"invalid_model"}}`, true},
		{`{"error":{"code":"not_found"}}`, true},
		{`{"type":"not_found_error","message":"Model 'qwen3.5-27b' not found"}`, true}, // mlx shape
		{`<html>404 Not Found</html>`, false},
		{`{"error":"generic"}`, false},
		{``, false},
	}
	for _, tc := range cases {
		t.Run(tc.body, func(t *testing.T) {
			assert.Equal(t, tc.want, IsModelNotFound(tc.body))
		})
	}
}

func TestShouldFailover(t *testing.T) {
	reach := newReachabilityError("url", "chat", 502, fmt.Errorf("bad gateway"))
	badRequest := &HTTPStatusError{Endpoint: "url", Operation: "chat", StatusCode: 400, Body: "validation failed"}
	unauthorized := &HTTPStatusError{Endpoint: "url", Operation: "chat", StatusCode: 401, Body: "bad key"}
	forbidden := &HTTPStatusError{Endpoint: "url", Operation: "chat", StatusCode: 403, Body: "forbidden"}
	notFoundModel := &HTTPStatusError{Endpoint: "url", Operation: "chat", StatusCode: 404, Body: `{"error":{"code":"model_not_found"}}`}
	notFoundGeneric := &HTTPStatusError{Endpoint: "url", Operation: "chat", StatusCode: 404, Body: "not found"}
	rateLimited := &HTTPStatusError{Endpoint: "url", Operation: "chat", StatusCode: 429, Body: "rate limit"}
	serverError := &HTTPStatusError{Endpoint: "url", Operation: "chat", StatusCode: 503, Body: "svc unavailable"}

	cases := []struct {
		name   string
		err    error
		pinned bool
		want   bool
	}{
		{"nil err, unpinned", nil, false, false},
		{"nil err, pinned", nil, true, false},
		{"reachability, unpinned → failover", reach, false, true},
		{"reachability, pinned → bubble", reach, true, false},
		{"400 bad request, unpinned → bubble", badRequest, false, false},
		{"401 auth, unpinned → failover", unauthorized, false, true},
		{"401 auth, pinned → bubble", unauthorized, true, false},
		{"403 auth → failover", forbidden, false, true},
		{"404 model-not-found → failover", notFoundModel, false, true},
		{"404 generic → bubble", notFoundGeneric, false, false},
		{"429 rate limit → failover", rateLimited, false, true},
		{"503 server error → failover", serverError, false, true},
		{"context canceled → bubble", context.Canceled, false, false},
		{"deadline exceeded → bubble", context.DeadlineExceeded, false, false},
		{"unknown error → bubble", errors.New("mystery"), false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, ShouldFailover(tc.err, tc.pinned))
		})
	}
}
