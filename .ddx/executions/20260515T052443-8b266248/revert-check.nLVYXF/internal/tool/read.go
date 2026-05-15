package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	agent "github.com/easel/fizeau/internal/core"
	"github.com/easel/fizeau/internal/safefs"
	"github.com/easel/fizeau/internal/tool/anchorstore"
	"github.com/easel/fizeau/internal/tool/anchorwords"
)

// ReadParams are the parameters for the read tool.
type ReadParams struct {
	Path   string `json:"path"`
	Offset int    `json:"offset,omitempty"` // 0-based line offset
	Limit  int    `json:"limit,omitempty"`  // max lines to return
}

// ReadTool reads file contents relative to a working directory.
type ReadTool struct {
	WorkDir     string
	AnchorStore *anchorstore.AnchorStore
}

func (t *ReadTool) Name() string { return "read" }
func (t *ReadTool) Description() string {
	return "Read file contents. Use instead of cat/head/tail."
}
func (t *ReadTool) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path":   {"type": "string", "description": "File path (relative to working directory or absolute)"},
			"offset": {"type": "integer", "description": "0-based line offset to start reading from"},
			"limit":  {"type": "integer", "description": "Maximum number of lines to return"}
		},
		"required": ["path"]
	}`)
}

func (t *ReadTool) Execute(ctx context.Context, params json.RawMessage) (string, error) {
	var p ReadParams
	if err := json.Unmarshal(params, &p); err != nil {
		return "", fmt.Errorf("read: invalid params: %w", err)
	}

	resolved := resolvePath(t.WorkDir, p.Path)

	data, err := safefs.ReadFile(resolved)
	if err != nil {
		return "", fmt.Errorf("read: %w", err)
	}

	if !utf8.Valid(data) {
		return "", fmt.Errorf("read: file appears to be binary: %s", resolved)
	}

	content := string(data)

	if t.AnchorStore == nil {
		if p.Offset > 0 || p.Limit > 0 {
			lines := strings.Split(content, "\n")
			start := p.Offset
			if start > len(lines) {
				start = len(lines)
			}
			end := len(lines)
			if p.Limit > 0 && start+p.Limit < end {
				end = start + p.Limit
			}
			content = strings.Join(lines[start:end], "\n")
		}

		return TruncateHead(content, truncMaxLines, truncMaxBytes), nil
	}

	return t.executeAnchoredRead(resolved, content, p), nil
}

func (t *ReadTool) Parallel() bool { return true }

// Verify ReadTool implements agent.Tool at compile time.
var _ agent.Tool = (*ReadTool)(nil)

func (t *ReadTool) executeAnchoredRead(resolved string, content string, p ReadParams) string {
	if content == "" {
		t.AnchorStore.Assign(resolved, 0, nil)
		return ""
	}

	lines := strings.Split(content, "\n")
	start := p.Offset
	if start > len(lines) {
		start = len(lines)
	}
	end := len(lines)
	if p.Limit > 0 && start+p.Limit < end {
		end = start + p.Limit
	}

	lines = lines[start:end]
	keptLines, truncated := truncateAnchoredLines(lines, truncMaxLines, truncMaxBytes)
	t.AnchorStore.Assign(resolved, start, anchorSlice(start, len(keptLines)))

	if len(keptLines) == 0 {
		if truncated {
			return fmt.Sprintf("...: [Truncated: %d lines omitted]", len(lines))
		}
		return ""
	}

	out := make([]string, 0, len(keptLines)+1)
	for i, line := range keptLines {
		out = append(out, fmt.Sprintf("%s: %s", anchorwords.Anchors[(start+i)%len(anchorwords.Anchors)], line))
	}
	if truncated {
		out = append(out, fmt.Sprintf("...: [Truncated: %d lines omitted]", len(lines)-len(keptLines)))
	}
	return strings.Join(out, "\n")
}

func anchorSlice(fileOffset int, count int) []string {
	anchors := make([]string, count)
	for i := range anchors {
		anchors[i] = anchorwords.Anchors[(fileOffset+i)%len(anchorwords.Anchors)]
	}
	return anchors
}

func truncateAnchoredLines(lines []string, maxLines int, maxBytes int) ([]string, bool) {
	kept := 0
	size := 0
	for _, line := range lines {
		lineSize := len(line) + 1
		if kept >= maxLines || size+lineSize > maxBytes {
			break
		}
		kept++
		size += lineSize
	}
	return lines[:kept], kept < len(lines)
}
