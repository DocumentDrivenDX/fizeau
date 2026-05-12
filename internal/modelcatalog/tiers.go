package modelcatalog

import (
	"strconv"
	"strings"
)

// Tier classifies a model within its family by capability level.
type Tier int

const (
	TierSmart    Tier = 0 // unsuffixed gpt, opus claude, pro gemini
	TierStandard Tier = 1 // mini gpt, sonnet claude, flash gemini
	TierCheap    Tier = 2 // nano gpt, haiku claude, flash-lite gemini
	TierUnknown  Tier = 99
)

// FamilyTiers maps family name to a map of tier-suffix → Tier.
var FamilyTiers = map[string]map[string]Tier{
	"gpt":    {"": TierSmart, "mini": TierStandard, "nano": TierCheap},
	"claude": {"opus": TierSmart, "sonnet": TierStandard, "haiku": TierCheap},
	"gemini": {"pro": TierSmart, "flash": TierStandard, "flash-lite": TierCheap},
}

// ParsedModel holds the structured components of a parsed model ID.
type ParsedModel struct {
	Family     string // "gpt", "claude", "gemini", ...
	Version    []int  // [5, 5] for gpt-5.5; [4, 7] for opus-4.7; normalized so [5] -> [5, 0]
	Tier       Tier   // smart/standard/cheap/unknown
	PreRelease bool   // true for "preview", "alpha", etc.
	Raw        string // original ID
}

var preReleaseSuffixes = map[string]bool{
	"preview": true,
	"alpha":   true,
	"beta":    true,
	"rc":      true,
}

var claudeTierNames = map[string]bool{
	"opus": true, "sonnet": true, "haiku": true,
}

// Parse parses a model ID string into its structured components.
// Does not panic on any input; unknown families return {Family: "<prefix>", Tier: TierUnknown}.
func Parse(id string) ParsedModel {
	raw := id
	lower := strings.ToLower(strings.TrimSpace(id))
	if lower == "" {
		return ParsedModel{Family: "", Tier: TierUnknown, Raw: raw}
	}
	parts := strings.Split(lower, "-")
	switch parts[0] {
	case "gpt":
		return parseGPT(parts[1:], raw)
	case "claude":
		return parseClaudeWithPrefix(parts[1:], raw)
	case "gemini":
		return parseGemini(parts[1:], raw)
	default:
		if claudeTierNames[parts[0]] {
			return parseClaudeShort(parts[0], parts[1:], raw)
		}
		return ParsedModel{Family: parts[0], Tier: TierUnknown, Raw: raw}
	}
}

// Compare returns a negative int when a should rank before b ("a is better"),
// positive when b should rank before a, and 0 when equal or cross-family.
// Ranking: newer version > older; within version, TierSmart > TierStandard > TierCheap > TierUnknown;
// within tier, GA > pre-release.
func (a ParsedModel) Compare(b ParsedModel) int {
	if a.Family != b.Family {
		return 0
	}
	// Higher version ranks first → negate the slice comparison result.
	if vCmp := compareVersionSlices(a.Version, b.Version); vCmp != 0 {
		return -vCmp
	}
	if a.Tier != b.Tier {
		if a.Tier < b.Tier {
			return -1
		}
		return 1
	}
	if a.PreRelease != b.PreRelease {
		if !a.PreRelease {
			return -1 // GA ranks before pre-release
		}
		return 1
	}
	return 0
}

func parseGPT(tokens []string, raw string) ParsedModel {
	if len(tokens) == 0 {
		return ParsedModel{Family: "gpt", Tier: TierUnknown, Raw: raw}
	}
	version := parseDottedVersion(tokens[0])
	if version == nil {
		return ParsedModel{Family: "gpt", Tier: TierUnknown, Raw: raw}
	}
	tier, pre := parseGPTSuffix(tokens[1:])
	return ParsedModel{
		Family:     "gpt",
		Version:    normalizeVersion(version),
		Tier:       tier,
		PreRelease: pre,
		Raw:        raw,
	}
}

func parseGPTSuffix(tokens []string) (Tier, bool) {
	preRelease := false
	remaining := append([]string(nil), tokens...)
	for len(remaining) > 0 && preReleaseSuffixes[remaining[len(remaining)-1]] {
		preRelease = true
		remaining = remaining[:len(remaining)-1]
	}
	if len(remaining) == 0 {
		return TierSmart, preRelease
	}
	suffix := strings.Join(remaining, "-")
	if t, ok := FamilyTiers["gpt"][suffix]; ok {
		return t, preRelease
	}
	return TierUnknown, preRelease
}

func parseClaudeWithPrefix(tokens []string, raw string) ParsedModel {
	if len(tokens) == 0 {
		return ParsedModel{Family: "claude", Tier: TierUnknown, Raw: raw}
	}
	tier, ok := FamilyTiers["claude"][tokens[0]]
	if !ok {
		return ParsedModel{Family: "claude", Tier: TierUnknown, Raw: raw}
	}
	version, pre := parseVersionTokens(tokens[1:])
	return ParsedModel{
		Family:     "claude",
		Version:    normalizeVersion(version),
		Tier:       tier,
		PreRelease: pre,
		Raw:        raw,
	}
}

func parseClaudeShort(tierName string, tokens []string, raw string) ParsedModel {
	tier := FamilyTiers["claude"][tierName]
	version, pre := parseVersionTokens(tokens)
	return ParsedModel{
		Family:     "claude",
		Version:    normalizeVersion(version),
		Tier:       tier,
		PreRelease: pre,
		Raw:        raw,
	}
}

func parseGemini(tokens []string, raw string) ParsedModel {
	if len(tokens) == 0 {
		return ParsedModel{Family: "gemini", Tier: TierUnknown, Raw: raw}
	}
	version := parseDottedVersion(tokens[0])
	if version == nil {
		return ParsedModel{Family: "gemini", Tier: TierUnknown, Raw: raw}
	}
	remaining := append([]string(nil), tokens[1:]...)
	preRelease := false
	for len(remaining) > 0 && preReleaseSuffixes[remaining[len(remaining)-1]] {
		preRelease = true
		remaining = remaining[:len(remaining)-1]
	}
	tierName := strings.Join(remaining, "-")
	tier, ok := FamilyTiers["gemini"][tierName]
	if !ok {
		tier = TierUnknown
	}
	return ParsedModel{
		Family:     "gemini",
		Version:    normalizeVersion(version),
		Tier:       tier,
		PreRelease: preRelease,
		Raw:        raw,
	}
}

// parseDottedVersion parses a dotted version string like "5.5" or "5" into a slice.
// Returns nil if any component is not an integer.
func parseDottedVersion(s string) []int {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ".")
	result := make([]int, 0, len(parts))
	for _, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return nil
		}
		result = append(result, n)
	}
	return result
}

// parseVersionTokens extracts a version and pre-release flag from a token slice.
// Handles dotted tokens ("4.7") and consecutive hyphen-separated integers ("4", "7").
func parseVersionTokens(tokens []string) ([]int, bool) {
	if len(tokens) == 0 {
		return nil, false
	}
	preRelease := false
	// If first token contains a dot, treat as a complete dotted version.
	if strings.Contains(tokens[0], ".") {
		if v := parseDottedVersion(tokens[0]); v != nil {
			for _, t := range tokens[1:] {
				if preReleaseSuffixes[t] {
					preRelease = true
				}
			}
			return v, preRelease
		}
	}
	// Otherwise accumulate consecutive integer tokens (hyphenated version).
	var version []int
	for _, t := range tokens {
		if preReleaseSuffixes[t] {
			preRelease = true
			continue
		}
		n, err := strconv.Atoi(t)
		if err != nil {
			break
		}
		version = append(version, n)
	}
	return version, preRelease
}

// normalizeVersion pads a version slice to at least 2 components (major, minor).
func normalizeVersion(v []int) []int {
	for len(v) < 2 {
		v = append(v, 0)
	}
	return v
}

// compareVersionSlices returns 1 if a > b, -1 if a < b, 0 if equal.
func compareVersionSlices(a, b []int) int {
	n := len(a)
	if len(b) > n {
		n = len(b)
	}
	for i := range n {
		av, bv := 0, 0
		if i < len(a) {
			av = a[i]
		}
		if i < len(b) {
			bv = b[i]
		}
		if av > bv {
			return 1
		}
		if av < bv {
			return -1
		}
	}
	return 0
}
