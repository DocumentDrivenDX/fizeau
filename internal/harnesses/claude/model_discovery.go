package claude

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/DocumentDrivenDX/agent/internal/harnesses"
	"github.com/DocumentDrivenDX/agent/internal/harnesses/ptyquota"
	"github.com/DocumentDrivenDX/agent/internal/pty/cassette"
	"github.com/DocumentDrivenDX/agent/internal/pty/session"
)

const ClaudeModelDiscoveryFreshnessWindow = 24 * time.Hour

var (
	claudeFullModelPattern = regexp.MustCompile(`\bclaude-[a-z0-9][a-z0-9._-]*\b`)
	claudeAliasPattern     = regexp.MustCompile(`(?m)(?:^|[\s'"])(sonnet|opus|haiku)(?:$|[\s'"])`)
	claudeEffortPattern    = regexp.MustCompile(`--effort\s+<level>.*\(([^)]*)\)`)
)

func DefaultClaudeModelDiscovery() harnesses.ModelDiscoverySnapshot {
	return harnesses.ModelDiscoverySnapshot{
		CapturedAt:      time.Now().UTC(),
		Models:          []string{"sonnet", "opus", "claude-sonnet-4-6"},
		ReasoningLevels: []string{"low", "medium", "high", "xhigh", "max"},
		Source:          "cli-help:claude",
		FreshnessWindow: ClaudeModelDiscoveryFreshnessWindow.String(),
		Detail:          "claude --help documents --model aliases/full IDs and --effort levels; authenticated PTY record tests refresh model evidence",
	}
}

func ReadClaudeModelDiscoveryViaPTY(timeout time.Duration, opts ...QuotaPTYOption) (harnesses.ModelDiscoverySnapshot, error) {
	cfg := quotaPTYOptions{binary: "claude"}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	var snapshot harnesses.ModelDiscoverySnapshot
	_, err := ptyquota.Run(context.Background(), ptyquota.Config{
		HarnessName:  "claude",
		Binary:       cfg.binary,
		Args:         cfg.args,
		Workdir:      cfg.workdir,
		Env:          cfg.env,
		Command:      "/model\r",
		ReadyMarkers: []string{"❯", "> "},
		DoneWhen:     claudeModelDiscoveryComplete,
		Timeout:      timeout,
		Size:         session.Size{Rows: 50, Cols: 220},
		CassetteDir:  cfg.cassetteDir,
		Discovery: func(text string) (cassette.DiscoveryRecord, error) {
			snapshot = claudeDiscoveryFromText(text, "pty")
			if len(snapshot.Models) == 0 {
				return cassette.DiscoveryRecord{}, fmt.Errorf("no models found in claude /model output")
			}
			return discoveryRecord(snapshot), nil
		},
	})
	if err != nil {
		return harnesses.ModelDiscoverySnapshot{}, err
	}
	if len(snapshot.Models) == 0 {
		return harnesses.ModelDiscoverySnapshot{}, fmt.Errorf("no models found in claude /model output")
	}
	return snapshot, nil
}

func ReadClaudeModelDiscoveryFromCassette(dir string) (harnesses.ModelDiscoverySnapshot, error) {
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
	snapshot := claudeDiscoveryFromText(text, "cassette")
	if len(snapshot.Models) == 0 {
		return harnesses.ModelDiscoverySnapshot{}, fmt.Errorf("no models found in claude model cassette")
	}
	return snapshot, nil
}

func ReadClaudeReasoningFromHelp(ctx context.Context, binary string, args ...string) ([]string, error) {
	if binary == "" {
		binary = "claude"
	}
	if len(args) == 0 {
		args = []string{"--help"}
	}
	cmd := exec.CommandContext(ctx, binary, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("claude help: %w", err)
	}
	levels := parseClaudeReasoningLevels(string(out))
	if len(levels) == 0 {
		return nil, fmt.Errorf("claude help did not expose --effort levels")
	}
	return levels, nil
}

func claudeModelDiscoveryComplete(text string) bool {
	return len(parseClaudeModels(text)) > 0
}

func claudeDiscoveryFromText(text, source string) harnesses.ModelDiscoverySnapshot {
	snapshot := DefaultClaudeModelDiscovery()
	if source != "" {
		snapshot.Source = source
	}
	if models := parseClaudeModels(text); len(models) > 0 {
		snapshot.Models = models
	}
	if levels := parseClaudeReasoningLevels(text); len(levels) > 0 {
		snapshot.ReasoningLevels = levels
	}
	return snapshot
}

func parseClaudeModels(text string) []string {
	text = stripANSI(strings.ReplaceAll(text, "\r\n", "\n"))
	models := uniqueMatches(claudeFullModelPattern.FindAllString(strings.ToLower(text), -1))
	for _, match := range claudeAliasPattern.FindAllStringSubmatch(strings.ToLower(text), -1) {
		if len(match) > 1 {
			models = appendUniqueString(models, match[1])
		}
	}
	return models
}

func parseClaudeReasoningLevels(text string) []string {
	text = stripANSI(strings.ReplaceAll(text, "\n", " "))
	m := claudeEffortPattern.FindStringSubmatch(text)
	if len(m) < 2 {
		return nil
	}
	parts := strings.Split(m[1], ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		out = appendUniqueString(out, strings.TrimSpace(part))
	}
	return out
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
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = appendUniqueString(out, value)
	}
	return out
}

func appendUniqueString(values []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}
