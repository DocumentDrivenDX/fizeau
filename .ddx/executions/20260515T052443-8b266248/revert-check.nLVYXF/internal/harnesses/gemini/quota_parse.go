package gemini

import (
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/easel/fizeau/internal/harnesses"
)

// Gemini CLI's /model manage TUI surfaces per-tier usage as rows such as:
//
//	Flash                  4% used      Resets 9:13 PM (23h 46m)
//	Flash Lite             0% used      Resets 9:27 PM (24h)
//	Pro                    100% used
//
// Names are version-sensitive; parse_gemini_model_manage accepts a small set
// of known tier labels and tolerates surrounding decoration (box-drawing
// glyphs, leading bullets, status badges).

var geminiAnsiPattern = regexp.MustCompile(`\x1b(?:\[[0-9;?]*[a-zA-Z]|[^[])`)

func stripGeminiANSI(s string) string {
	return geminiAnsiPattern.ReplaceAllString(s, "")
}

var (
	geminiUsedPercentPattern = regexp.MustCompile(`(\d+(?:\.\d+)?)\s*%\s*used`)
	geminiResetPattern       = regexp.MustCompile(`(?i)reset[s]?\s+(.*)`)
)

// geminiTier represents a tier surfaced in the /model manage dialog.
type geminiTier struct {
	// Label is the tier label expected in the Gemini CLI UI (case-insensitive).
	Label string
	// LimitID is the stable identifier persisted into cassettes and caches.
	// Downstream consumers compare LimitIDs, not labels.
	LimitID string
	// DefaultModel is the concrete model ID mapped to this tier.
	DefaultModel string
}

var geminiTiers = []geminiTier{
	{Label: "Pro", LimitID: "gemini-pro", DefaultModel: "gemini-2.5-pro"},
	{Label: "Flash Lite", LimitID: "gemini-flash-lite", DefaultModel: "gemini-2.5-flash-lite"},
	{Label: "Flash", LimitID: "gemini-flash", DefaultModel: "gemini-2.5-flash"},
}

// ParseGeminiModelManage parses the raw text captured from `gemini` in
// /model manage. It returns one QuotaWindow per recognised tier, with the
// tier label, used percent, reset text, and a derived quota state. Tiers
// missing in the captured text are simply absent from the returned slice.
func ParseGeminiModelManage(text string) []harnesses.QuotaWindow {
	text = stripGeminiANSI(strings.ReplaceAll(text, "\r\n", "\n"))
	lines := strings.Split(text, "\n")

	windows := make([]harnesses.QuotaWindow, 0, len(geminiTiers))
	seen := map[string]bool{}

	for i := 0; i < len(lines); i++ {
		line := lines[i]
		tier := tierForLine(line)
		if tier == nil {
			continue
		}
		if seen[tier.LimitID] {
			continue
		}
		// Look at the current line and up to two following lines for the
		// "% used" token and (optionally) a "Resets ..." fragment.
		usedPct, hasUsed, resetText := harvestTierEvidence(lines, i)
		if !hasUsed {
			continue
		}
		seen[tier.LimitID] = true
		windows = append(windows, harnesses.QuotaWindow{
			Name:        tier.Label,
			LimitID:     tier.LimitID,
			UsedPercent: usedPct,
			ResetsAt:    resetText,
			State:       geminiQuotaState(usedPct),
		})
	}
	return windows
}

func tierForLine(line string) *geminiTier {
	lower := strings.ToLower(line)
	// Match longer labels first so "Flash Lite" is never matched as "Flash".
	for i := range geminiTiers {
		if containsTierLabel(lower, strings.ToLower(geminiTiers[i].Label)) {
			return &geminiTiers[i]
		}
	}
	return nil
}

// containsTierLabel checks that the lowercase tier label appears in the line
// with word-like boundaries so "flashback" never matches "flash".
func containsTierLabel(lowerLine, lowerLabel string) bool {
	idx := strings.Index(lowerLine, lowerLabel)
	if idx < 0 {
		return false
	}
	if idx > 0 {
		prev := lowerLine[idx-1]
		if isTierLabelRune(prev) {
			return false
		}
	}
	end := idx + len(lowerLabel)
	if end < len(lowerLine) {
		next := lowerLine[end]
		if isTierLabelRune(next) {
			return false
		}
	}
	return true
}

func isTierLabelRune(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= '0' && b <= '9')
}

func harvestTierEvidence(lines []string, start int) (float64, bool, string) {
	var usedPct float64
	var hasUsed bool
	var resetText string
	end := start + 3
	if end > len(lines) {
		end = len(lines)
	}
	for j := start; j < end; j++ {
		frag := lines[j]
		if !hasUsed {
			if m := geminiUsedPercentPattern.FindStringSubmatch(frag); m != nil {
				if pct, err := strconv.ParseFloat(m[1], 64); err == nil {
					usedPct = pct
					hasUsed = true
				}
			}
		}
		if resetText == "" {
			if m := geminiResetPattern.FindStringSubmatch(frag); m != nil {
				resetText = strings.TrimSpace(m[1])
			}
		}
		if hasUsed && resetText != "" {
			break
		}
	}
	return usedPct, hasUsed, resetText
}

// geminiQuotaState mirrors harnesses.QuotaStateFromUsedPercent but treats
// exhausted tiers (>=100% used) as an explicit "exhausted" state so downstream
// routing can distinguish a fully blown tier from merely approaching the cap.
func geminiQuotaState(used float64) string {
	rounded := int(math.Ceil(used))
	if rounded >= 100 {
		return "exhausted"
	}
	if rounded >= 95 {
		return "blocked"
	}
	return "ok"
}

// FindGeminiQuotaWindow returns the QuotaWindow matching limitID, or nil.
func FindGeminiQuotaWindow(windows []harnesses.QuotaWindow, limitID string) *harnesses.QuotaWindow {
	for i := range windows {
		if windows[i].LimitID == limitID {
			return &windows[i]
		}
	}
	return nil
}

// TierLimitIDForModel returns the tier limit_id that covers the given concrete
// Gemini model ID (e.g. "gemini-2.5-pro" -> "gemini-pro"). Unknown models
// return ("", false).
func TierLimitIDForModel(model string) (string, bool) {
	normalized := strings.ToLower(strings.TrimSpace(model))
	if normalized == "" {
		return "", false
	}
	// Match the more specific label first (flash-lite before flash).
	if strings.Contains(normalized, "flash-lite") || strings.Contains(normalized, "flash lite") {
		return "gemini-flash-lite", true
	}
	if strings.Contains(normalized, "pro") {
		return "gemini-pro", true
	}
	if strings.Contains(normalized, "flash") {
		return "gemini-flash", true
	}
	return "", false
}
