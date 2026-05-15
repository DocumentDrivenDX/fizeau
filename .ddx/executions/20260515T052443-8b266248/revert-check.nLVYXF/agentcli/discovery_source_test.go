package agentcli_test

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/easel/fizeau/internal/serverinstance"
)

func testDiscoverySourceName(providerName, endpointName, baseURL, serverInstance string) string {
	name := strings.TrimSpace(providerName)
	if strings.TrimSpace(serverInstance) == "" {
		serverInstance = serverinstance.FromBaseURL(baseURL)
	}
	trimmedEndpoint := strings.TrimSpace(endpointName)
	if trimmedEndpoint == "" || trimmedEndpoint == "default" || trimmedEndpoint == name {
		name = sanitizeDiscoveryName(name)
	} else {
		name = sanitizeDiscoveryName(name + "-" + trimmedEndpoint)
	}
	identity := strings.TrimSpace(baseURL) + "|" + strings.TrimSpace(serverInstance)
	if strings.TrimSpace(identity) != "|" {
		sum := sha256.Sum256([]byte(identity))
		name = sanitizeDiscoveryName(name + "-" + hex.EncodeToString(sum[:4]))
	}
	return name
}

func sanitizeDiscoveryName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return "discovery"
	}
	var b strings.Builder
	b.Grow(len(name))
	lastDash := false
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
			lastDash = false
		case r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "discovery"
	}
	return out
}
