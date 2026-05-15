package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/easel/fizeau/internal/tool/anchorstore"
	"github.com/easel/fizeau/internal/tool/anchorwords"
)

func TestReadTool_Execute(t *testing.T) {
	dir := t.TempDir()
	tool := &ReadTool{WorkDir: dir}

	t.Run("reads file contents", func(t *testing.T) {
		path := filepath.Join(dir, "hello.txt")
		require.NoError(t, os.WriteFile(path, []byte("hello world\n"), 0o644))

		result, err := tool.Execute(context.Background(), mustJSON(t, ReadParams{Path: "hello.txt"}))
		require.NoError(t, err)
		assert.Equal(t, "hello world\n", result)
	})

	t.Run("reads absolute path", func(t *testing.T) {
		path := filepath.Join(dir, "abs.txt")
		require.NoError(t, os.WriteFile(path, []byte("absolute"), 0o644))

		result, err := tool.Execute(context.Background(), mustJSON(t, ReadParams{Path: path}))
		require.NoError(t, err)
		assert.Equal(t, "absolute", result)
	})

	t.Run("errors on missing file", func(t *testing.T) {
		_, err := tool.Execute(context.Background(), mustJSON(t, ReadParams{Path: "nonexistent.txt"}))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no such file")
	})

	t.Run("errors on binary content", func(t *testing.T) {
		path := filepath.Join(dir, "binary.bin")
		require.NoError(t, os.WriteFile(path, []byte{0x00, 0xFF, 0xFE, 0x01}, 0o644))

		_, err := tool.Execute(context.Background(), mustJSON(t, ReadParams{Path: "binary.bin"}))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "binary")
	})

	t.Run("supports offset and limit", func(t *testing.T) {
		path := filepath.Join(dir, "lines.txt")
		require.NoError(t, os.WriteFile(path, []byte("line0\nline1\nline2\nline3\nline4"), 0o644))

		result, err := tool.Execute(context.Background(), mustJSON(t, ReadParams{
			Path:   "lines.txt",
			Offset: 1,
			Limit:  2,
		}))
		require.NoError(t, err)
		assert.Equal(t, "line1\nline2", result)
	})

	t.Run("offset beyond file length returns empty", func(t *testing.T) {
		path := filepath.Join(dir, "short.txt")
		require.NoError(t, os.WriteFile(path, []byte("one\ntwo"), 0o644))

		result, err := tool.Execute(context.Background(), mustJSON(t, ReadParams{
			Path:   "short.txt",
			Offset: 100,
		}))
		require.NoError(t, err)
		assert.Equal(t, "", result)
	})

	t.Run("errors on invalid params", func(t *testing.T) {
		_, err := tool.Execute(context.Background(), json.RawMessage(`{invalid`))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid params")
	})
}

func TestReadTool_ExecuteAnchorMode(t *testing.T) {
	dir := t.TempDir()
	store := anchorstore.New()
	tool := &ReadTool{WorkDir: dir, AnchorStore: store}

	t.Run("prefixes lines and assigns anchors with file offset", func(t *testing.T) {
		path := filepath.Join(dir, "anchored.txt")
		require.NoError(t, os.WriteFile(path, []byte("line0\nline1\nline2\nline3"), 0o644))

		result, err := tool.Execute(context.Background(), mustJSON(t, ReadParams{
			Path:   "anchored.txt",
			Offset: 1,
			Limit:  2,
		}))
		require.NoError(t, err)

		assert.Equal(t, anchorwords.Anchors[1]+": line1\n"+anchorwords.Anchors[2]+": line2", result)

		line, ambiguous := store.Lookup(path, anchorwords.Anchors[1])
		require.False(t, ambiguous)
		assert.Equal(t, 1, line)

		line, ambiguous = store.Lookup(path, anchorwords.Anchors[2])
		require.False(t, ambiguous)
		assert.Equal(t, 2, line)

		line, ambiguous = store.Lookup(path, anchorwords.Anchors[0])
		assert.False(t, ambiguous)
		assert.Equal(t, -1, line)
	})

	t.Run("truncation marker uses ellipsis prefix", func(t *testing.T) {
		path := filepath.Join(dir, "long.txt")
		lines := make([]string, truncMaxLines+1)
		for i := range lines {
			lines[i] = fmt.Sprintf("line%d", i)
		}
		require.NoError(t, os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644))

		result, err := tool.Execute(context.Background(), mustJSON(t, ReadParams{Path: "long.txt"}))
		require.NoError(t, err)

		outLines := strings.Split(result, "\n")
		require.Len(t, outLines, truncMaxLines+1)
		assert.Equal(t, anchorwords.Anchors[0]+": line0", outLines[0])
		assert.Equal(t, "...: [Truncated: 1 lines omitted]", outLines[len(outLines)-1])

		line, ambiguous := store.Lookup(path, "...")
		assert.False(t, ambiguous)
		assert.Equal(t, -1, line)
	})
}

func TestReadTool_LegacyModeOutput(t *testing.T) {
	dir := t.TempDir()
	tool := &ReadTool{WorkDir: dir}
	path := filepath.Join(dir, "legacy.txt")
	lines := make([]string, truncMaxLines+1)
	for i := range lines {
		lines[i] = fmt.Sprintf("legacy%d", i)
	}
	content := strings.Join(lines, "\n")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))

	result, err := tool.Execute(context.Background(), mustJSON(t, ReadParams{Path: "legacy.txt"}))
	require.NoError(t, err)

	assert.Equal(t, TruncateHead(content, truncMaxLines, truncMaxBytes), result)
}

func mustJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(v)
	require.NoError(t, err)
	return data
}
