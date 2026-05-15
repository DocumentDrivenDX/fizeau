package openai

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
)

// ErrEndpointUnreachable is the sentinel callers match via errors.Is when
// distinguishing "this endpoint is down / unreachable" from everything else.
// Wrapped by ReachabilityError.
var ErrEndpointUnreachable = errors.New("openai: endpoint unreachable")

// ReachabilityError describes a failure attributable to the endpoint being
// unreachable or returning a server-side 5xx — distinct from request-level
// client errors (4xx) or model-specific errors. Callers use errors.Is(err,
// ErrEndpointUnreachable) to detect it.
type ReachabilityError struct {
	// Endpoint is the base URL of the provider that failed.
	Endpoint string
	// Operation identifies what was being attempted when the failure
	// occurred. Typical values: "probe_models", "chat_completions".
	Operation string
	// StatusCode is the HTTP status code if the failure was HTTP-level;
	// 0 for non-HTTP failures (dial error, timeout, TLS handshake, etc.).
	StatusCode int
	// Cause is the underlying error.
	Cause error
}

func (e *ReachabilityError) Error() string {
	if e == nil {
		return ""
	}
	endpoint := e.Endpoint
	if endpoint == "" {
		endpoint = "<unknown>"
	}
	op := e.Operation
	if op == "" {
		op = "request"
	}
	if e.StatusCode > 0 {
		return fmt.Sprintf("openai: endpoint unreachable (%s %s: HTTP %d): %v", endpoint, op, e.StatusCode, e.Cause)
	}
	return fmt.Sprintf("openai: endpoint unreachable (%s %s): %v", endpoint, op, e.Cause)
}

// Unwrap returns the underlying cause so errors.As / errors.Unwrap traverse
// through the reachability wrapper.
func (e *ReachabilityError) Unwrap() error { return e.Cause }

// Is reports whether target is ErrEndpointUnreachable. This makes
// errors.Is(err, openai.ErrEndpointUnreachable) return true for any
// *ReachabilityError regardless of the underlying cause.
func (e *ReachabilityError) Is(target error) bool {
	return target == ErrEndpointUnreachable
}

// newReachabilityError wraps cause as a ReachabilityError. Returns nil
// unchanged when cause is nil. Exported via the helper functions below.
func newReachabilityError(endpoint, operation string, statusCode int, cause error) *ReachabilityError {
	if cause == nil {
		return nil
	}
	return &ReachabilityError{
		Endpoint:   endpoint,
		Operation:  operation,
		StatusCode: statusCode,
		Cause:      cause,
	}
}

// ClassifyHTTPStatus returns a ReachabilityError when the status code is
// server-side (5xx); nil otherwise. Callers combine with ClassifyNetwork for
// the full picture.
func ClassifyHTTPStatus(endpoint, operation string, statusCode int, body string) error {
	if statusCode >= 500 {
		return newReachabilityError(endpoint, operation, statusCode,
			fmt.Errorf("HTTP %d: %s", statusCode, strings.TrimSpace(body)))
	}
	return nil
}

// ClassifyNetwork wraps a transport-level error as a ReachabilityError when
// it looks like the endpoint was unreachable — connection refused, dial
// timeouts, TLS handshake failure, DNS resolution failure, mid-response reset.
// Context cancellation and context-deadline-exceeded are NOT wrapped — those
// are the caller's own deadline and should bubble as the caller's concern.
// Returns nil when cause is nil.
func ClassifyNetwork(endpoint, operation string, cause error) error {
	if cause == nil {
		return nil
	}
	if errors.Is(cause, context.Canceled) || errors.Is(cause, context.DeadlineExceeded) {
		// Caller's own context; bubble as-is.
		return cause
	}
	var netErr net.Error
	if errors.As(cause, &netErr) {
		return newReachabilityError(endpoint, operation, 0, cause)
	}
	// String-sniff for the common cases that don't surface a net.Error
	// interface — e.g. http transport errors wrapping dial failures.
	msg := cause.Error()
	markers := []string{
		"connection refused",
		"connection reset",
		"broken pipe",
		"no such host",
		"TLS handshake",
		"tls: handshake",
		"EOF",
		"i/o timeout",
	}
	for _, m := range markers {
		if strings.Contains(msg, m) {
			return newReachabilityError(endpoint, operation, 0, cause)
		}
	}
	return cause
}

// Model-family error codes observed on /v1/chat/completions 404 responses.
// A generic 404 (no body code) is likely a wrong base URL / path and should
// NOT failover; a model-not-found 404 is endpoint-says-yes-to-url but
// no-to-this-model and SHOULD failover.
var modelNotFoundCodes = []string{
	"model_not_found",
	"model_not_supported",
	"invalid_model",
	"not_found",
}

// IsModelNotFound returns true when the response body from a 404 on
// /v1/chat/completions contains a model-specific error code. Caller decides
// whether to failover based on this + the pinned flag.
func IsModelNotFound(body string) bool {
	lowered := strings.ToLower(body)
	for _, code := range modelNotFoundCodes {
		if strings.Contains(lowered, code) {
			return true
		}
	}
	return false
}

// ShouldFailover returns true iff err warrants trying the next routing
// candidate instead of bubbling up. Returns false when:
//   - the request is pinned (operator explicitly chose a provider)
//   - the error is a caller-level concern (context.Canceled /
//     context.DeadlineExceeded)
//   - the error is a request-validation error the next endpoint can't fix
//     (400 Bad Request, generic 404 likely indicating wrong URL)
//
// Returns true for:
//   - ReachabilityError (5xx, dial failures, TLS, network resets)
//   - 401 / 403 auth failures (another endpoint might have valid auth)
//   - 404 responses whose body indicates the model specifically isn't
//     served (IsModelNotFound)
//   - 429 rate-limit / quota errors
//
// pinned should be req.Provider != "" captured BEFORE the failover loop.
func ShouldFailover(err error, pinned bool) bool {
	if err == nil {
		return false
	}
	if pinned {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	if errors.Is(err, ErrEndpointUnreachable) {
		return true
	}
	var httpErr *HTTPStatusError
	if errors.As(err, &httpErr) {
		switch {
		case httpErr.StatusCode == 400:
			return false
		case httpErr.StatusCode == 401, httpErr.StatusCode == 403:
			return true
		case httpErr.StatusCode == 404:
			return IsModelNotFound(httpErr.Body)
		case httpErr.StatusCode == 429:
			return true
		case httpErr.StatusCode >= 500:
			return true
		}
	}
	return false
}

// HTTPStatusError is a typed client-level HTTP error that carries the status
// code and response body so ShouldFailover can classify beyond
// ReachabilityError (which is only for 5xx / network). Callers that surface
// 4xx errors from openai-compatible servers should wrap them as
// HTTPStatusError to participate in the failover policy.
type HTTPStatusError struct {
	Endpoint   string
	Operation  string
	StatusCode int
	Body       string
}

func (e *HTTPStatusError) Error() string {
	if e == nil {
		return ""
	}
	op := e.Operation
	if op == "" {
		op = "request"
	}
	return fmt.Sprintf("openai: %s %s returned HTTP %d: %s", e.Endpoint, op, e.StatusCode, strings.TrimSpace(e.Body))
}
