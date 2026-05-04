package tool

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuiltinToolsForPreset_IncludesExpectedTools(t *testing.T) {
	tools := BuiltinToolsForPreset(t.TempDir(), "default", BashOutputFilterConfig{})

	var names []string
	for _, tool := range tools {
		names = append(names, tool.Name())
	}

	require.NotEmpty(t, names)
	assert.Contains(t, names, "bash")
	assert.Contains(t, names, "read")
	assert.Contains(t, names, "anchor_edit")
	assert.Contains(t, names, "write")
	assert.Contains(t, names, "edit")
	assert.Contains(t, names, "find")
	assert.Contains(t, names, "task")
}
