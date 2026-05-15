package testutil

import (
	"bytes"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/dnaeon/go-vcr.v4/pkg/cassette"
	"gopkg.in/dnaeon/go-vcr.v4/pkg/recorder"
)

const RecordProviderCassettesEnv = "FIZEAU_RECORD_PROVIDER_CASSETTES"

// ModeForEnvironment selects the cassette mode for provider tests.
//
// Replay-only is the default so tests stay offline unless an explicit record
// run is requested.
func ModeForEnvironment() recorder.Mode {
	if os.Getenv(RecordProviderCassettesEnv) == "1" {
		return recorder.ModeRecordOnly
	}
	return recorder.ModeReplayOnly
}

// NewRecorder creates a recorder rooted at cassettePath.
//
// The recorder defaults to replay-only mode, uses a stable matcher that ignores
// host/port churn and common auth/header noise, and redacts secret headers
// before saving.
func NewRecorder(cassettePath string) (*recorder.Recorder, error) {
	return recorder.New(
		cassettePath,
		recorder.WithMode(ModeForEnvironment()),
		recorder.WithMatcher(StableMatcher()),
		recorder.WithHook(RedactSensitiveHeaders, recorder.BeforeSaveHook),
	)
}

// UseDefaultTransport installs the recorder as the process-wide default
// transport for the duration of the test.
func UseDefaultTransport(t testing.TB, rec *recorder.Recorder) {
	t.Helper()
	old := http.DefaultTransport
	http.DefaultTransport = rec
	t.Cleanup(func() {
		http.DefaultTransport = old
	})
}

// StableMatcher compares requests by method, normalized URL, headers, and body.
//
// The URL normalization intentionally ignores scheme/host/port so the same
// cassette can be replayed against different local servers while still
// distinguishing /v1/models, /metrics, /slots, and /v1/chat/completions.
func StableMatcher() recorder.MatcherFunc {
	return func(r *http.Request, i cassette.Request) bool {
		clone, rawBody, err := cloneRequest(r)
		if err != nil {
			return false
		}

		clone.URL = normalizeURL(clone.URL)
		clone.Host = ""

		i.URL = normalizeURLString(i.URL)
		i.Host = ""

		clone.Body = io.NopCloser(bytes.NewReader(rawBody))
		return cassette.NewDefaultMatcher(
			cassette.WithIgnoreAuthorization(),
			cassette.WithIgnoreUserAgent(),
			cassette.WithIgnoreHeaders(
				"Accept-Encoding",
				"Api-Key",
				"X-Api-Key",
				"Openai-Api-Key",
				"Anthropic-Api-Key",
			),
		)(clone, i)
	}
}

// RedactSensitiveHeaders removes auth/API key headers before a cassette is
// written to disk.
func RedactSensitiveHeaders(i *cassette.Interaction) error {
	for key := range i.Request.Headers {
		if isSecretHeader(key) {
			delete(i.Request.Headers, key)
		}
	}
	for key := range i.Response.Headers {
		if isSecretHeader(key) {
			delete(i.Response.Headers, key)
		}
	}
	return nil
}

func cloneRequest(r *http.Request) (*http.Request, []byte, error) {
	clone := r.Clone(r.Context())
	var body []byte
	if r.Body != nil && r.Body != http.NoBody {
		var err error
		body, err = io.ReadAll(r.Body)
		if err != nil {
			return nil, nil, err
		}
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	clone.Body = io.NopCloser(bytes.NewReader(body))
	return clone, body, nil
}

func normalizeURL(raw *url.URL) *url.URL {
	if raw == nil {
		return nil
	}
	clone := *raw
	clone.Scheme = "http"
	clone.Host = "vcr.local"
	clone.Fragment = ""
	clone.RawQuery = clone.Query().Encode()
	return &clone
}

func normalizeURLString(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	normalized := normalizeURL(parsed)
	if normalized == nil {
		return raw
	}
	return normalized.String()
}

func isSecretHeader(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "authorization", "api-key", "x-api-key", "openai-api-key", "anthropic-api-key":
		return true
	}
	return strings.Contains(strings.ToLower(name), "api-key")
}

// CassettePath joins a base directory and cassette name using the file layout
// go-vcr expects.
func CassettePath(dir, name string) string {
	return filepath.Join(dir, name)
}
