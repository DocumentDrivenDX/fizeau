package codex

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/easel/fizeau/internal/harnesses"
)

var ansiPattern = regexp.MustCompile(`\x1b(?:\[[0-9;?]*[a-zA-Z]|[^[])`)

func stripANSI(s string) string {
	return ansiPattern.ReplaceAllString(s, "")
}

// tmuxRun invokes the tmux binary with the given args and waits for it to
// complete. tmuxOutput captures stdout. Both helpers exist to localize the
// gosec G204 (subprocess launched with variable) annotation: tmux is a fixed
// binary name resolved via PATH, and the variable args are tmux subcommands
// plus a service-generated session identifier (`ddx-codex-quota-<pid>`),
// never raw external input.
func tmuxRun(args ...string) error {
	// #nosec G204 -- "tmux" is a fixed binary; args are tmux subcommands and a
	// service-generated session identifier, not external input.
	return exec.Command("tmux", args...).Run()
}

func tmuxOutput(args ...string) ([]byte, error) {
	// #nosec G204 -- "tmux" is a fixed binary; args are tmux subcommands and a
	// service-generated session identifier, not external input.
	return exec.Command("tmux", args...).Output()
}

// ReadCodexQuotaViaTmux starts codex in a detached tmux session, sends /status,
// captures the output, and returns parsed quota windows.
//
// Deprecated: this is a diagnostic-only legacy path. Supported quota probes use
// ReadCodexQuotaViaPTY so accepted evidence passes through direct PTY cassettes.
func ReadCodexQuotaViaTmux(timeout time.Duration) ([]harnesses.QuotaWindow, error) {
	if _, err := exec.LookPath("tmux"); err != nil {
		return nil, fmt.Errorf("tmux not found in PATH: %w", err)
	}
	if _, err := exec.LookPath("codex"); err != nil {
		return nil, fmt.Errorf("codex not found in PATH: %w", err)
	}
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	sessName := fmt.Sprintf("ddx-codex-quota-%d", os.Getpid())
	if err := tmuxRun("new-session", "-d", "-s", sessName, "-x", "220", "-y", "50", "codex"); err != nil {
		return nil, fmt.Errorf("start tmux session: %w", err)
	}
	defer func() { _ = tmuxRun("kill-session", "-t", sessName) }()

	// Poll until codex shows its "›" interactive prompt.
	deadline := time.Now().Add(timeout)
	ready := false
	for !ready && time.Now().Before(deadline) {
		time.Sleep(500 * time.Millisecond)
		if err := tmuxRun("has-session", "-t", sessName); err != nil {
			return nil, fmt.Errorf("codex session exited before initialization")
		}
		out, err := tmuxOutput("capture-pane", "-t", sessName, "-p")
		if err == nil {
			text := stripANSI(string(out))
			if strings.Contains(text, "›") {
				ready = true
			}
		}
	}
	if !ready {
		return nil, fmt.Errorf("timed out waiting for codex to initialize")
	}

	if err := tmuxRun("send-keys", "-t", sessName, "/status", "Enter"); err != nil {
		return nil, fmt.Errorf("send /status: %w", err)
	}

	// Poll until /status output appears ("% left" in output).
	var captured string
	for time.Now().Before(deadline) {
		time.Sleep(500 * time.Millisecond)
		out, err := tmuxOutput("capture-pane", "-t", sessName, "-p", "-S", "-100")
		if err == nil {
			text := stripANSI(string(out))
			if strings.Contains(text, "% left") {
				captured = text
				break
			}
		}
	}
	if captured == "" {
		return nil, fmt.Errorf("timed out waiting for /status output")
	}

	windows := parseCodexStatusOutput(captured)
	if len(windows) == 0 {
		return nil, fmt.Errorf("no quota windows found in /status output")
	}
	return windows, nil
}

var (
	codexModelLinePattern  = regexp.MustCompile(`\b(gpt-[A-Za-z0-9][A-Za-z0-9._-]*)\b.*?\b(\d{1,3})%\s+(?:left|remaining)\b`)
	codexWeeklyWarnPattern = regexp.MustCompile(`(?i)(less than\s+)?(\d{1,3})%\s+of your weekly limit\s+(?:left|remaining)`)
)

// parseCodexStatusOutput parses the text captured from a codex /status pane.
// The primary format is: "  gpt-5.4 high · 100% left · /path"
// Weekly warning: "Heads up, you have less than 5% of your weekly limit left."
func parseCodexStatusOutput(text string) []harnesses.QuotaWindow {
	text = stripANSI(strings.ReplaceAll(text, "\r\n", "\n"))
	lines := strings.Split(text, "\n")

	var windows []harnesses.QuotaWindow

	// Extract primary window from "model effort · X% left · path" lines.
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if m := codexModelLinePattern.FindStringSubmatch(line); m != nil {
			pctLeft, _ := strconv.Atoi(m[2])
			if pctLeft < 0 || pctLeft > 100 {
				continue
			}
			usedPct := 100 - pctLeft
			windows = append(windows, harnesses.QuotaWindow{
				Name:          "5h",
				LimitID:       "codex",
				WindowMinutes: 300,
				UsedPercent:   float64(usedPct),
				State:         harnesses.QuotaStateFromUsedPercent(usedPct),
			})
			break
		}
	}

	// Extract weekly warning if present.
	for _, line := range lines {
		if m := codexWeeklyWarnPattern.FindStringSubmatch(line); m != nil {
			pctLeft, _ := strconv.Atoi(m[2])
			if pctLeft < 0 || pctLeft > 100 {
				continue
			}
			// "less than X%" remaining implies the used percentage is a lower
			// bound, so state is computed at the next integer.
			usedFloor := 100 - pctLeft
			statePercent := usedFloor
			if strings.TrimSpace(m[1]) != "" {
				statePercent++
			}
			windows = append(windows, harnesses.QuotaWindow{
				Name:          "7d",
				LimitID:       "codex",
				WindowMinutes: 10080,
				UsedPercent:   float64(usedFloor), // lower bound
				State:         harnesses.QuotaStateFromUsedPercent(statePercent),
				ResetsAt:      "",
			})
			break
		}
	}

	return windows
}
