package gemini

import (
	"context"
	"fmt"
	"time"

	"github.com/easel/fizeau/internal/harnesses"
	"github.com/easel/fizeau/internal/harnesses/ptyquota"
	"github.com/easel/fizeau/internal/pty/cassette"
	"github.com/easel/fizeau/internal/pty/session"
)

type quotaPTYOptions struct {
	binary      string
	args        []string
	workdir     string
	env         []string
	cassetteDir string
}

// QuotaPTYOption configures a Gemini quota PTY probe.
type QuotaPTYOption func(*quotaPTYOptions)

func WithQuotaPTYCommand(binary string, args ...string) QuotaPTYOption {
	return func(opts *quotaPTYOptions) {
		opts.binary = binary
		opts.args = append([]string(nil), args...)
	}
}

func WithQuotaPTYWorkdir(workdir string) QuotaPTYOption {
	return func(opts *quotaPTYOptions) {
		opts.workdir = workdir
	}
}

func WithQuotaPTYEnv(env ...string) QuotaPTYOption {
	return func(opts *quotaPTYOptions) {
		opts.env = append([]string(nil), env...)
	}
}

func WithQuotaPTYCassetteDir(dir string) QuotaPTYOption {
	return func(opts *quotaPTYOptions) {
		opts.cassetteDir = dir
	}
}

// ReadGeminiQuotaViaPTY launches Gemini, sends /model manage, and returns the
// parsed tier quota windows. Callers must only treat non-empty, non-error
// results as authoritative — Gemini CLI renders /model manage as a version-
// sensitive TUI, so a timeout or empty parse means "quota unknown".
func ReadGeminiQuotaViaPTY(timeout time.Duration, opts ...QuotaPTYOption) ([]harnesses.QuotaWindow, error) {
	windows, _, err := captureGeminiQuotaViaPTY(context.Background(), timeout, opts...)
	return windows, err
}

// ReadGeminiQuotaFromCassette replays a previously recorded cassette and
// re-parses its final frame as Gemini /model manage output.
func ReadGeminiQuotaFromCassette(dir string) ([]harnesses.QuotaWindow, error) {
	reader, err := cassette.Open(dir)
	if err != nil {
		return nil, err
	}
	text := reader.Final().FinalText
	if text == "" {
		frames := reader.Frames()
		if len(frames) > 0 {
			text = stringsJoinLines(frames[len(frames)-1].Text)
		}
	}
	windows := ParseGeminiModelManage(text)
	if len(windows) == 0 {
		return nil, fmt.Errorf("no gemini tier quota windows found in cassette")
	}
	return windows, nil
}

func captureGeminiQuotaViaPTY(ctx context.Context, timeout time.Duration, opts ...QuotaPTYOption) ([]harnesses.QuotaWindow, ptyquota.Result, error) {
	cfg := quotaPTYOptions{binary: "gemini"}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	var windows []harnesses.QuotaWindow
	result, err := ptyquota.Run(ctx, ptyquota.Config{
		HarnessName:  "gemini",
		Binary:       cfg.binary,
		Args:         cfg.args,
		Workdir:      cfg.workdir,
		Env:          cfg.env,
		Command:      "/model manage\r",
		ReadyMarkers: []string{">", "❯"},
		DoneWhen:     geminiQuotaComplete,
		Timeout:      timeout,
		Size:         session.Size{Rows: 50, Cols: 220},
		CassetteDir:  cfg.cassetteDir,
		Quota: func(text string) (cassette.QuotaRecord, error) {
			windows = ParseGeminiModelManage(text)
			if len(windows) == 0 {
				return cassette.QuotaRecord{}, fmt.Errorf("no gemini tier quota windows found in /model manage output")
			}
			return geminiQuotaRecord(windows), nil
		},
	})
	if err != nil {
		return nil, result, err
	}
	if len(windows) == 0 {
		windows = ParseGeminiModelManage(result.Text)
	}
	if len(windows) == 0 {
		return nil, result, fmt.Errorf("no gemini tier quota windows found in /model manage output")
	}
	return windows, result, nil
}

// geminiQuotaComplete is the probe's DoneWhen predicate. Gemini CLI renders
// tiers one by one as the dialog loads. Waiting for at least one tier with a
// parsed "% used" token is enough: tiers that never appear in the rendered
// dialog were not surfaced by this CLI version.
func geminiQuotaComplete(text string) bool {
	windows := ParseGeminiModelManage(text)
	return len(windows) >= 1
}

func geminiQuotaRecord(windows []harnesses.QuotaWindow) cassette.QuotaRecord {
	records := make([]map[string]any, 0, len(windows))
	for _, window := range windows {
		records = append(records, map[string]any{
			"name":         window.Name,
			"limit_id":     window.LimitID,
			"used_percent": window.UsedPercent,
			"resets_at":    window.ResetsAt,
			"state":        window.State,
		})
	}
	return cassette.QuotaRecord{
		Source:            "pty",
		Status:            string(ptyquota.StatusOK),
		CapturedAt:        time.Now().UTC().Format(time.RFC3339),
		FreshnessWindow:   defaultGeminiQuotaStaleAfter.String(),
		StalenessBehavior: "stale gemini quota evidence keeps Gemini out of automatic routing and is treated as limited",
		Windows:           records,
	}
}

func stringsJoinLines(lines []string) string {
	out := ""
	for i, line := range lines {
		if i > 0 {
			out += "\n"
		}
		out += line
	}
	return out
}
