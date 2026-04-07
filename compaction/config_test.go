package compaction

import (
	"testing"

	"github.com/DocumentDrivenDX/forge"
	"github.com/stretchr/testify/assert"
)

func TestFindCutPoint(t *testing.T) {
	t.Run("keeps recent messages", func(t *testing.T) {
		msgs := make([]forge.Message, 10)
		for i := range msgs {
			msgs[i] = forge.Message{Role: forge.RoleUser, Content: "message " + string(rune('0'+i))}
		}
		// With a tiny budget, should keep only the last few
		cut := FindCutPoint(msgs, 20)
		assert.Greater(t, cut, 0)
		assert.Less(t, cut, len(msgs))
	})

	t.Run("never cuts at tool result", func(t *testing.T) {
		msgs := []forge.Message{
			{Role: forge.RoleUser, Content: "do something"},
			{Role: forge.RoleAssistant, Content: "thinking..."},
			{Role: forge.RoleTool, Content: "tool output", ToolCallID: "tc1"},
			{Role: forge.RoleUser, Content: "thanks"},
		}
		// Force cut near the tool result
		cut := FindCutPoint(msgs, 10)
		if cut > 0 && cut < len(msgs) {
			assert.NotEqual(t, forge.RoleTool, msgs[cut].Role,
				"cut point should not be at a tool result")
		}
	})

	t.Run("empty messages", func(t *testing.T) {
		assert.Equal(t, 0, FindCutPoint(nil, 1000))
	})

	t.Run("budget exceeds total", func(t *testing.T) {
		msgs := []forge.Message{
			{Role: forge.RoleUser, Content: "short"},
		}
		// Huge budget — should keep everything (cut at 0)
		cut := FindCutPoint(msgs, 100000)
		assert.Equal(t, 0, cut)
	})
}

func TestIsCompactionSummary(t *testing.T) {
	summary := forge.Message{
		Role:    forge.RoleUser,
		Content: "The conversation history before this point was compacted into the following summary:\n\n<summary>\n## Goal\nDo stuff\n</summary>",
	}
	assert.True(t, IsCompactionSummary(summary))

	regular := forge.Message{
		Role:    forge.RoleUser,
		Content: "Read main.go please",
	}
	assert.False(t, IsCompactionSummary(regular))
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	assert.True(t, cfg.Enabled)
	assert.Equal(t, 8192, cfg.ContextWindow)
	assert.Equal(t, 8192, cfg.ReserveTokens)
	assert.Equal(t, 95, cfg.EffectivePercent)
	assert.Equal(t, 2000, cfg.MaxToolResultChars)
}
