package ollama

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestProtocolCapabilities(t *testing.T) {
	p := New(Config{BaseURL: "http://localhost:11434/v1"})
	assert.True(t, p.SupportsTools())
	assert.True(t, p.SupportsStream())
	assert.False(t, p.SupportsStructuredOutput())
	assert.False(t, p.SupportsThinking())
}
