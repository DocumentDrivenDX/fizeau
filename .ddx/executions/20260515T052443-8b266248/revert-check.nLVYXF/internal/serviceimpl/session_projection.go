package serviceimpl

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/easel/fizeau/internal/session"
)

// SessionLogEntry is an API-neutral projection of one service session log.
type SessionLogEntry struct {
	SessionID string
	ModTime   time.Time
	Size      int64
}

// UsageReport aggregates session usage from the service-owned log directory.
func UsageReport(ctx context.Context, logDir string, opts session.UsageOptions) (*session.UsageReport, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return session.AggregateUsage(logDir, opts)
}

// ListSessionLogs returns JSONL session logs sorted by session ID.
func ListSessionLogs(ctx context.Context, logDir string) ([]SessionLogEntry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if logDir == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(logDir)
	if err != nil {
		return nil, err
	}
	out := make([]SessionLogEntry, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		id := strings.TrimSuffix(e.Name(), ".jsonl")
		entry := SessionLogEntry{SessionID: id}
		if info, err := e.Info(); err == nil && info != nil {
			entry.ModTime = info.ModTime()
			entry.Size = info.Size()
		}
		out = append(out, entry)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SessionID < out[j].SessionID })
	return out, nil
}

// WriteSessionLog writes the raw session events as indented JSON objects.
func WriteSessionLog(ctx context.Context, path string, w io.Writer) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	events, err := session.ReadEvents(path)
	if err != nil {
		return err
	}
	for _, e := range events {
		data, err := json.MarshalIndent(e, "", "  ")
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintln(w, string(data)); err != nil {
			return err
		}
	}
	return nil
}

// ReplaySession renders a human-readable transcript for one session log path.
func ReplaySession(ctx context.Context, path string, w io.Writer) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return session.Replay(path, w)
}
