package tool

import (
	"context"
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

func TestAnchorEditTool_Execute(t *testing.T) {
	t.Run("basic replace", func(t *testing.T) {
		read, edit, dir := setupAnchorEditTools(t, "alpha\nbeta\ngamma")

		_, err := read.Execute(context.Background(), mustJSON(t, ReadParams{Path: "target.txt"}))
		require.NoError(t, err)

		result, err := edit.Execute(context.Background(), mustJSON(t, AnchorEditParams{
			Path:        "target.txt",
			StartAnchor: anchorwords.Anchors[1],
			EndAnchor:   anchorwords.Anchors[1],
			NewText:     "BETA",
		}))
		require.NoError(t, err)
		assert.Contains(t, result, "anchors invalidated")
		assertFileContent(t, filepath.Join(dir, "target.txt"), "alpha\nBETA\ngamma")
	})

	t.Run("delete range", func(t *testing.T) {
		read, edit, dir := setupAnchorEditTools(t, "alpha\nbeta\ngamma\ndelta")

		_, err := read.Execute(context.Background(), mustJSON(t, ReadParams{Path: "target.txt"}))
		require.NoError(t, err)

		_, err = edit.Execute(context.Background(), mustJSON(t, AnchorEditParams{
			Path:        "target.txt",
			StartAnchor: anchorwords.Anchors[1],
			EndAnchor:   anchorwords.Anchors[2],
			NewText:     "",
		}))
		require.NoError(t, err)
		assertFileContent(t, filepath.Join(dir, "target.txt"), "alpha\ndelta")
	})

	t.Run("stale error", func(t *testing.T) {
		read, edit, dir := setupAnchorEditTools(t, "alpha\nbeta")

		_, err := read.Execute(context.Background(), mustJSON(t, ReadParams{Path: "target.txt"}))
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(dir, "target.txt"), []byte("alpha\nbeta\ngamma"), 0o644))

		_, err = edit.Execute(context.Background(), mustJSON(t, AnchorEditParams{
			Path:        "target.txt",
			StartAnchor: anchorwords.Anchors[0],
			EndAnchor:   anchorwords.Anchors[0],
			NewText:     "ALPHA",
		}))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "stale anchors")
	})

	t.Run("invalidated error", func(t *testing.T) {
		read, edit, _ := setupAnchorEditTools(t, "alpha\nbeta")

		_, err := read.Execute(context.Background(), mustJSON(t, ReadParams{Path: "target.txt"}))
		require.NoError(t, err)
		_, err = edit.Execute(context.Background(), mustJSON(t, AnchorEditParams{
			Path:        "target.txt",
			StartAnchor: anchorwords.Anchors[0],
			EndAnchor:   anchorwords.Anchors[0],
			NewText:     "ALPHA",
		}))
		require.NoError(t, err)

		_, err = edit.Execute(context.Background(), mustJSON(t, AnchorEditParams{
			Path:        "target.txt",
			StartAnchor: anchorwords.Anchors[1],
			EndAnchor:   anchorwords.Anchors[1],
			NewText:     "BETA",
		}))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalidated or missing")
	})

	t.Run("start greater than end error", func(t *testing.T) {
		read, edit, _ := setupAnchorEditTools(t, "alpha\nbeta\ngamma")

		_, err := read.Execute(context.Background(), mustJSON(t, ReadParams{Path: "target.txt"}))
		require.NoError(t, err)

		_, err = edit.Execute(context.Background(), mustJSON(t, AnchorEditParams{
			Path:        "target.txt",
			StartAnchor: anchorwords.Anchors[2],
			EndAnchor:   anchorwords.Anchors[1],
			NewText:     "replacement",
		}))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "resolves after end_anchor")
	})

	t.Run("ambiguous without hint lists choices", func(t *testing.T) {
		read, edit, _ := setupAnchorEditTools(t, numberedLines(len(anchorwords.Anchors)+2))

		_, err := read.Execute(context.Background(), mustJSON(t, ReadParams{Path: "target.txt"}))
		require.NoError(t, err)

		_, err = edit.Execute(context.Background(), mustJSON(t, AnchorEditParams{
			Path:        "target.txt",
			StartAnchor: anchorwords.Anchors[0],
			EndAnchor:   anchorwords.Anchors[0],
			NewText:     "replacement",
		}))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "ambiguous")
		assert.Contains(t, err.Error(), "available choices: 0, 1024")
	})

	t.Run("ambiguous with hint resolves", func(t *testing.T) {
		read, edit, dir := setupAnchorEditTools(t, numberedLines(len(anchorwords.Anchors)+2))

		_, err := read.Execute(context.Background(), mustJSON(t, ReadParams{Path: "target.txt"}))
		require.NoError(t, err)
		hint := len(anchorwords.Anchors)

		_, err = edit.Execute(context.Background(), mustJSON(t, AnchorEditParams{
			Path:        "target.txt",
			StartAnchor: anchorwords.Anchors[0],
			EndAnchor:   anchorwords.Anchors[0],
			NewText:     "replacement",
			OffsetHint:  &hint,
		}))
		require.NoError(t, err)

		lines := strings.Split(readFileContent(t, filepath.Join(dir, "target.txt")), "\n")
		assert.Equal(t, "line 0", lines[0])
		assert.Equal(t, "replacement", lines[len(anchorwords.Anchors)])
	})
}

func setupAnchorEditTools(t *testing.T, content string) (*ReadTool, *AnchorEditTool, string) {
	t.Helper()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "target.txt"), []byte(content), 0o644))
	store := anchorstore.New()
	return &ReadTool{WorkDir: dir, AnchorStore: store}, &AnchorEditTool{WorkDir: dir, AnchorStore: store}, dir
}

func assertFileContent(t *testing.T, path string, want string) {
	t.Helper()

	assert.Equal(t, want, readFileContent(t, path))
}

func readFileContent(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(data)
}

func numberedLines(count int) string {
	lines := make([]string, count)
	for i := range lines {
		lines[i] = fmt.Sprintf("line %d", i)
	}
	return strings.Join(lines, "\n")
}
