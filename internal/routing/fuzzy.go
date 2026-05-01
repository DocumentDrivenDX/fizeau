// Package routing implements the unified routing engine for fizeau.
// It ranks (harness, provider, model) candidates uniformly per CONTRACT-003.
//
// The engine consolidates DDx-side harness-tier routing and agent-side
// per-provider failover into a single ranking pipeline.
package routing

import (
	"strings"
)

// canonicalizeModelKey strips a single leading vendor namespace (e.g.
// "qwen/qwen3.6" → "qwen3.6") and lowercases the result. Only the first
// slash-segment is stripped so multi-segment paths like "openai/o1/preview"
// become "o1/preview", not "preview".
//
// Fixes ddx-0486e601: case + vendor-prefix normalization so that
// "qwen/qwen3.6" can match "Qwen3.6-35B-A3B-4bit".
func canonicalizeModelKey(s string) string {
	s = strings.TrimSpace(s)
	if slash := strings.Index(s, "/"); slash > 0 {
		s = s[slash+1:]
	}
	return strings.ToLower(s)
}

// FuzzyMatch returns the best concrete model from pool for the given input,
// using canonical-form (case-insensitive, vendor-prefix-stripped) matching.
// Returns "" when no candidate matches.
//
// Algorithm:
//  1. Exact match (case-insensitive on canonical form) wins outright.
//  2. Prefix/suffix/contains match on canonical form. Among fuzzy matches,
//     prefer prefix, then suffix, then contains, and within each tier prefer
//     the shortest remaining text (most specific match).
//  3. No match returns "".
func FuzzyMatch(input string, pool []string) string {
	if input == "" || len(pool) == 0 {
		return ""
	}
	cInput := canonicalizeModelKey(input)
	if cInput == "" {
		return ""
	}
	if best := bestQwenFamilyMatch(cInput, pool); best != "" {
		return best
	}

	// Pass 1: exact canonical match.
	for _, m := range pool {
		if canonicalizeModelKey(m) == cInput {
			return m
		}
	}

	// Pass 2: canonical prefix/suffix/contains match. Track the candidate with
	// the best tier and then the shortest remaining text.
	var best string
	bestTier := 4
	bestRemainder := -1
	for _, m := range pool {
		cm := canonicalizeModelKey(m)
		if len(cm) <= len(cInput) || !strings.Contains(cm, cInput) {
			continue
		}
		tier := 3
		switch {
		case strings.HasPrefix(cm, cInput):
			tier = 1
		case strings.HasSuffix(cm, cInput):
			tier = 2
		}
		remainder := len(cm) - len(cInput)
		if tier < bestTier || (tier == bestTier && (bestRemainder < 0 || remainder < bestRemainder)) {
			best = m
			bestTier = tier
			bestRemainder = remainder
		}
	}
	return best
}

func bestQwenFamilyMatch(input string, pool []string) string {
	if input != "qwen" && !strings.HasPrefix(input, "qwen3") && !strings.HasPrefix(input, "qwen-3") {
		return ""
	}
	var best string
	var bestVersion []int
	bestParams := -1
	bestVariant := -1
	for _, model := range pool {
		candidate := canonicalizeModelKey(model)
		if !strings.Contains(candidate, input) {
			continue
		}
		version, params, variant, ok := parseQwenRank(candidate)
		if !ok {
			continue
		}
		if best == "" || compareQwenRank(version, params, variant, bestVersion, bestParams, bestVariant) > 0 {
			best = model
			bestVersion = version
			bestParams = params
			bestVariant = variant
		}
	}
	return best
}

func parseQwenRank(model string) ([]int, int, int, bool) {
	if !strings.Contains(model, "qwen") {
		return nil, 0, 0, false
	}
	idx := strings.Index(model, "qwen")
	rest := model[idx+len("qwen"):]
	rest = strings.TrimPrefix(rest, "-")
	version := make([]int, 0, 2)
	for len(rest) > 0 {
		if rest[0] < '0' || rest[0] > '9' {
			break
		}
		n := 0
		for len(rest) > 0 && rest[0] >= '0' && rest[0] <= '9' {
			n = n*10 + int(rest[0]-'0')
			rest = rest[1:]
		}
		version = append(version, n)
		if len(rest) == 0 || (rest[0] != '.' && rest[0] != '-') {
			break
		}
		if len(rest) > 1 && rest[1] >= '0' && rest[1] <= '9' {
			rest = rest[1:]
			continue
		}
		break
	}
	if len(version) == 0 {
		return nil, 0, 0, false
	}
	params := qwenParamBillions(model)
	return version, params, qwenVariantRank(model), true
}

func qwenParamBillions(model string) int {
	best := -1
	for i := 0; i < len(model); i++ {
		if model[i] < '0' || model[i] > '9' {
			continue
		}
		n := 0
		j := i
		for j < len(model) && model[j] >= '0' && model[j] <= '9' {
			n = n*10 + int(model[j]-'0')
			j++
		}
		if j < len(model) && model[j] == 'b' && n > best {
			best = n
		}
		i = j
	}
	return best
}

func qwenVariantRank(model string) int {
	switch {
	case strings.Contains(model, "4bit"):
		return 40
	case strings.Contains(model, "nvfp4"):
		return 30
	case strings.Contains(model, "mlx"):
		return 20
	default:
		return 10
	}
}

func compareQwenRank(aVersion []int, aParams, aVariant int, bVersion []int, bParams, bVariant int) int {
	n := len(aVersion)
	if len(bVersion) > n {
		n = len(bVersion)
	}
	for i := 0; i < n; i++ {
		av, bv := 0, 0
		if i < len(aVersion) {
			av = aVersion[i]
		}
		if i < len(bVersion) {
			bv = bVersion[i]
		}
		if av > bv {
			return 1
		}
		if av < bv {
			return -1
		}
	}
	if aParams != bParams {
		if aParams > bParams {
			return 1
		}
		return -1
	}
	if aVariant > bVariant {
		return 1
	}
	if aVariant < bVariant {
		return -1
	}
	return 0
}

// SameModelIntent returns true if two model strings refer to the same model
// after canonicalization. Used by capability gating and provider matching.
func SameModelIntent(a, b string) bool {
	ca := canonicalizeModelKey(a)
	cb := canonicalizeModelKey(b)
	if ca == "" || cb == "" {
		return false
	}
	return ca == cb || strings.Contains(ca, cb) || strings.Contains(cb, ca)
}
