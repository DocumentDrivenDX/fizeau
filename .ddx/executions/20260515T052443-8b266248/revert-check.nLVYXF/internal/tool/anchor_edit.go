package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"unicode/utf8"

	agent "github.com/easel/fizeau/internal/core"
	"github.com/easel/fizeau/internal/safefs"
	"github.com/easel/fizeau/internal/tool/anchorstore"
)

// AnchorEditParams are the parameters for the anchor_edit tool.
type AnchorEditParams struct {
	Path        string `json:"path"`
	StartAnchor string `json:"start_anchor"`
	EndAnchor   string `json:"end_anchor"`
	NewText     string `json:"new_text"`
	OffsetHint  *int   `json:"offset_hint,omitempty"`
}

// AnchorEditTool performs line-range edits using anchors assigned by ReadTool.
type AnchorEditTool struct {
	WorkDir     string
	AnchorStore *anchorstore.AnchorStore
}

func (t *AnchorEditTool) Name() string { return "anchor_edit" }
func (t *AnchorEditTool) Description() string {
	return "Replace or delete a line range using anchors returned by read."
}
func (t *AnchorEditTool) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"additionalProperties": false,
		"properties": {
			"path": {"type": "string", "description": "File path (relative to working directory or absolute)"},
			"start_anchor": {"type": "string", "description": "Anchor for the first line to replace"},
			"end_anchor": {"type": "string", "description": "Anchor for the last line to replace"},
			"new_text": {"type": "string", "description": "Replacement text. Empty string deletes the range."},
			"offset_hint": {"type": "integer", "description": "Approximate 0-based line offset used to resolve ambiguous anchors"}
		},
		"required": ["path", "start_anchor", "end_anchor", "new_text"]
	}`)
}

func (t *AnchorEditTool) Execute(_ context.Context, params json.RawMessage) (string, error) {
	var p AnchorEditParams
	if err := json.Unmarshal(params, &p); err != nil {
		return "", fmt.Errorf("anchor_edit: invalid params: %w", err)
	}
	if p.StartAnchor == "" {
		return "", fmt.Errorf("anchor_edit: start_anchor required")
	}
	if p.EndAnchor == "" {
		return "", fmt.Errorf("anchor_edit: end_anchor required")
	}
	if t.AnchorStore == nil {
		return "", fmt.Errorf("anchor_edit: anchors unavailable; read file first")
	}

	resolved := resolvePath(t.WorkDir, p.Path)

	data, err := safefs.ReadFile(resolved)
	if err != nil {
		return "", fmt.Errorf("anchor_edit: %w", err)
	}
	if !utf8.Valid(data) {
		return "", fmt.Errorf("anchor_edit: file appears to be binary: %s", resolved)
	}

	content := string(data)
	lines := splitFileLines(content)

	start, expectedCount, err := t.resolveAnchor(resolved, p.StartAnchor, p.OffsetHint)
	if err != nil {
		return "", err
	}
	end, endExpectedCount, err := t.resolveAnchor(resolved, p.EndAnchor, p.OffsetHint)
	if err != nil {
		return "", err
	}
	if expectedCount != endExpectedCount {
		return "", fmt.Errorf("anchor_edit: anchor assignment for %s is inconsistent; read file again", resolved)
	}
	if len(lines) != expectedCount {
		return "", fmt.Errorf("anchor_edit: stale anchors for %s: file has %d lines, anchors cover %d lines; read file again", resolved, len(lines), expectedCount)
	}
	if start > end {
		return "", fmt.Errorf("anchor_edit: start_anchor resolves after end_anchor (%d > %d)", start, end)
	}
	if start < 0 || end >= len(lines) {
		return "", fmt.Errorf("anchor_edit: anchor range %d..%d outside %s line count %d; read file again", start, end, resolved, len(lines))
	}

	replacement := splitReplacementLines(p.NewText)
	next := make([]string, 0, len(lines)-(end-start+1)+len(replacement))
	next = append(next, lines[:start]...)
	next = append(next, replacement...)
	next = append(next, lines[end+1:]...)

	if err := safefs.WriteFile(resolved, []byte(strings.Join(next, "\n")), 0o600); err != nil {
		return "", fmt.Errorf("anchor_edit: writing: %w", err)
	}
	t.AnchorStore.Invalidate(resolved)

	return fmt.Sprintf("Replaced lines %d-%d in %s; anchors invalidated", start, end, resolved), nil
}

func (t *AnchorEditTool) Parallel() bool { return false }

var _ agent.Tool = (*AnchorEditTool)(nil)

func (t *AnchorEditTool) resolveAnchor(path, anchor string, offsetHint *int) (int, int, error) {
	choices, count, ok := t.AnchorStore.Resolve(path, anchor)
	if !ok {
		return 0, 0, fmt.Errorf("anchor_edit: anchor %q for %s is invalidated or missing; read file first", anchor, path)
	}
	if len(choices) == 1 {
		return choices[0], count, nil
	}
	if offsetHint == nil {
		return 0, 0, fmt.Errorf("anchor_edit: anchor %q is ambiguous in %s; available choices: %s; provide offset_hint", anchor, path, formatLineChoices(choices))
	}
	return nearestLine(choices, *offsetHint), count, nil
}

func splitFileLines(content string) []string {
	if content == "" {
		return nil
	}
	return strings.Split(content, "\n")
}

func splitReplacementLines(text string) []string {
	if text == "" {
		return nil
	}
	return strings.Split(text, "\n")
}

func nearestLine(lines []int, hint int) int {
	best := lines[0]
	bestDistance := math.Abs(float64(best - hint))
	for _, line := range lines[1:] {
		distance := math.Abs(float64(line - hint))
		if distance < bestDistance {
			best = line
			bestDistance = distance
		}
	}
	return best
}

func formatLineChoices(lines []int) string {
	parts := make([]string, len(lines))
	for i, line := range lines {
		parts[i] = fmt.Sprintf("%d", line)
	}
	return strings.Join(parts, ", ")
}
