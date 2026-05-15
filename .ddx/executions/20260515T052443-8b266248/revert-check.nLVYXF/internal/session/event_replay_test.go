package session

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReplaysPreV011SessionLog(t *testing.T) {
	src := filepath.Join("testdata", "pre_v011_session.jsonl")
	raw, err := os.ReadFile(src)
	require.NoError(t, err)
	require.Contains(t, string(raw), "requested_model_ref")
	require.Contains(t, string(raw), "resolved_model_ref")

	path := filepath.Join(t.TempDir(), "pre_v011_session.jsonl")
	require.NoError(t, os.WriteFile(path, raw, 0o600))

	events, err := ReadEvents(path)
	require.NoError(t, err)
	require.Len(t, events, 3)

	var replay bytes.Buffer
	require.NoError(t, Replay(path, &replay))
	output := replay.String()
	require.Contains(t, output, "Session pre-v011")
	require.Contains(t, output, "old replay response")
	if strings.Contains(output, "model_ref") {
		t.Fatalf("replay output re-emitted legacy model_ref keys:\n%s", output)
	}
}
