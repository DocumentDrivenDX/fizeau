package tool

import (
	agent "github.com/easel/fizeau/internal/core"
)

// BuiltinToolsForPreset returns the built-in tool set used by the native
// agent harness for a prompt preset.
func BuiltinToolsForPreset(workDir, preset string, bashFilter BashOutputFilterConfig) []agent.Tool {
	tools := []agent.Tool{
		&ReadTool{WorkDir: workDir},
		&WriteTool{WorkDir: workDir},
		&EditTool{WorkDir: workDir},
		&BashTool{WorkDir: workDir, OutputFilter: bashFilter},
		&FindTool{WorkDir: workDir},
		&GrepTool{WorkDir: workDir},
		&LsTool{WorkDir: workDir},
		&PatchTool{WorkDir: workDir},
	}
	taskStore := NewTaskStore()
	tools = append(tools, &TaskTool{Store: taskStore})
	return tools
}
