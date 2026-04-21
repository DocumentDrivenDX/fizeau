package codex

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/DocumentDrivenDX/agent/internal/harnesses"
	"github.com/DocumentDrivenDX/agent/internal/harnesses/ptyquota"
	"github.com/DocumentDrivenDX/agent/internal/pty/cassette"
	"github.com/DocumentDrivenDX/agent/internal/pty/session"
)

const CodexModelDiscoveryFreshnessWindow = 24 * time.Hour

var codexModelPattern = regexp.MustCompile(`\bgpt-[A-Za-z0-9][A-Za-z0-9._-]*\b`)

func DefaultCodexModelDiscovery() harnesses.ModelDiscoverySnapshot {
	return harnesses.ModelDiscoverySnapshot{
		CapturedAt:      time.Now().UTC(),
		Models:          []string{"gpt-5.4"},
		ReasoningLevels: []string{"low", "medium", "high", "xhigh", "max"},
		Source:          "compatibility-table:codex-cli",
		FreshnessWindow: CodexModelDiscoveryFreshnessWindow.String(),
		Detail:          "codex CLI exposes exact model pins with -m/--model; model IDs are refreshed by authenticated PTY record tests",
	}
}

func ReadCodexModelDiscoveryViaPTY(timeout time.Duration, opts ...QuotaPTYOption) (harnesses.ModelDiscoverySnapshot, error) {
	cfg := quotaPTYOptions{binary: "codex", args: []string{"--no-alt-screen"}}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	var snapshot harnesses.ModelDiscoverySnapshot
	_, err := ptyquota.Run(context.Background(), ptyquota.Config{
		HarnessName:        "codex",
		Binary:             cfg.binary,
		Args:               cfg.args,
		Workdir:            cfg.workdir,
		Env:                cfg.env,
		Command:            "/model\r",
		ReadyMarkers:       []string{"›", "> "},
		DoneWhen:           codexModelDiscoveryComplete,
		ResetBeforeCommand: true,
		Timeout:            timeout,
		Size:               session.Size{Rows: 50, Cols: 220},
		CassetteDir:        cfg.cassetteDir,
		Discovery: func(text string) (cassette.DiscoveryRecord, error) {
			snapshot = codexDiscoveryFromText(text, "pty")
			if len(snapshot.Models) == 0 {
				return cassette.DiscoveryRecord{}, fmt.Errorf("no models found in codex /model output")
			}
			return discoveryRecord(snapshot), nil
		},
	})
	if err != nil {
		return harnesses.ModelDiscoverySnapshot{}, err
	}
	if len(snapshot.Models) == 0 {
		return harnesses.ModelDiscoverySnapshot{}, fmt.Errorf("no models found in codex /model output")
	}
	return snapshot, nil
}

func ReadCodexModelDiscoveryFromCassette(dir string) (harnesses.ModelDiscoverySnapshot, error) {
	reader, err := cassette.Open(dir)
	if err != nil {
		return harnesses.ModelDiscoverySnapshot{}, err
	}
	if rec := reader.Discovery(); rec != nil && len(rec.Models) > 0 {
		return snapshotFromDiscoveryRecord(*rec), nil
	}
	text := reader.Final().FinalText
	if text == "" {
		frames := reader.Frames()
		if len(frames) > 0 {
			text = strings.Join(frames[len(frames)-1].Text, "\n")
		}
	}
	snapshot := codexDiscoveryFromText(text, "cassette")
	if len(snapshot.Models) == 0 {
		return harnesses.ModelDiscoverySnapshot{}, fmt.Errorf("no models found in codex model cassette")
	}
	return snapshot, nil
}

func codexModelDiscoveryComplete(text string) bool {
	return len(parseCodexModels(text)) > 0
}

func codexDiscoveryFromText(text, source string) harnesses.ModelDiscoverySnapshot {
	snapshot := DefaultCodexModelDiscovery()
	if source != "" {
		snapshot.Source = source
	}
	if models := parseCodexModels(text); len(models) > 0 {
		snapshot.Models = models
	}
	return snapshot
}

func parseCodexModels(text string) []string {
	text = stripANSI(strings.ReplaceAll(text, "\r\n", "\n"))
	return uniqueMatches(codexModelPattern.FindAllString(text, -1))
}

func discoveryRecord(snapshot harnesses.ModelDiscoverySnapshot) cassette.DiscoveryRecord {
	return cassette.DiscoveryRecord{
		Source:          snapshot.Source,
		Status:          string(ptyquota.StatusOK),
		Models:          append([]string(nil), snapshot.Models...),
		ReasoningLevels: append([]string(nil), snapshot.ReasoningLevels...),
		CapturedAt:      snapshot.CapturedAt.UTC().Format(time.RFC3339),
		FreshnessWindow: snapshot.FreshnessWindow,
		Metadata:        map[string]any{"detail": snapshot.Detail},
	}
}

func snapshotFromDiscoveryRecord(rec cassette.DiscoveryRecord) harnesses.ModelDiscoverySnapshot {
	capturedAt, _ := time.Parse(time.RFC3339, rec.CapturedAt)
	detail, _ := rec.Metadata["detail"].(string)
	return harnesses.ModelDiscoverySnapshot{
		CapturedAt:      capturedAt,
		Models:          append([]string(nil), rec.Models...),
		ReasoningLevels: append([]string(nil), rec.ReasoningLevels...),
		Source:          rec.Source,
		FreshnessWindow: rec.FreshnessWindow,
		Detail:          detail,
	}
}

func uniqueMatches(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}
