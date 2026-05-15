package openai

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestProtocolSupport_DefaultOpenAI(t *testing.T) {
	p := New(Config{BaseURL: "https://api.openai.com/v1"})
	assert.True(t, p.SupportsTools())
	assert.True(t, p.SupportsStream())
	assert.True(t, p.SupportsStructuredOutput())
	assert.False(t, p.SupportsThinking())
}

func TestProtocolSupport_ProviderCapabilitiesOverride(t *testing.T) {
	caps := ProtocolCapabilities{
		Tools:            true,
		Stream:           true,
		StructuredOutput: false,
		Thinking:         true,
	}

	p := New(Config{
		BaseURL:      "http://localhost:1234/v1",
		Capabilities: &caps,
	})

	assert.True(t, p.SupportsTools())
	assert.True(t, p.SupportsStream())
	assert.False(t, p.SupportsStructuredOutput())
	assert.True(t, p.SupportsThinking())
}
