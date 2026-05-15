package serverinstance

import (
	"net"
	"net/url"
	"strings"
)

// Normalize returns the explicit server instance when present, otherwise a
// deterministic identity derived from baseURL's host and port.
//
// The derived identity ignores URL path components so provider endpoints that
// differ only by `/v1` root normalization still collapse to the same
// server-instance label.
func Normalize(baseURL, explicit string) string {
	if trimmed := strings.TrimSpace(explicit); trimmed != "" {
		return trimmed
	}
	return FromBaseURL(baseURL)
}

// FromBaseURL derives a stable server-instance label from the URL host/port.
func FromBaseURL(baseURL string) string {
	trimmed := strings.TrimSpace(baseURL)
	if trimmed == "" {
		return ""
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return fallbackHostPort(trimmed)
	}
	host := strings.TrimSpace(parsed.Hostname())
	if host == "" {
		return fallbackHostPort(trimmed)
	}
	port := strings.TrimSpace(parsed.Port())
	if port == "" {
		return host
	}
	return host + ":" + port
}

func fallbackHostPort(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	host, port, err := net.SplitHostPort(raw)
	if err == nil && host != "" {
		if port != "" {
			return host + ":" + port
		}
		return host
	}
	raw = strings.TrimPrefix(raw, "http://")
	raw = strings.TrimPrefix(raw, "https://")
	raw = strings.TrimSuffix(raw, "/v1")
	raw = strings.TrimSuffix(raw, "/")
	return raw
}
