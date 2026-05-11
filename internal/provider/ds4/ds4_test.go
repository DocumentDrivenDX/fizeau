package ds4_test

import (
	"testing"

	"github.com/easel/fizeau/internal/provider/ds4"
	"github.com/easel/fizeau/internal/provider/openai"
	"github.com/stretchr/testify/assert"
)

// TestDS4ProtocolCapabilities verifies that ds4 declares Thinking=true and
// uses the OpenAIEffort wire format (flat top-level reasoning_effort / think:false).
// ds4's /props endpoint confirms it accepts reasoning_effort but not the
// Anthropic-shape thinking block.
func TestDS4ProtocolCapabilities(t *testing.T) {
	assert.True(t, ds4.ProtocolCapabilities.Thinking, "ds4 must declare Thinking support")
	assert.Equal(t, openai.ThinkingWireFormatOpenAIEffort, ds4.ProtocolCapabilities.ThinkingFormat,
		"ds4 must use OpenAIEffort wire format (flat reasoning_effort, not Anthropic thinking block)")
}
