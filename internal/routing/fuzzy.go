// Package routing implements the unified routing engine for ddx-agent.
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
