package modelmatch

import (
	"strings"
)

// CanonicalKey normalizes a model string for fuzzy matching.
//
// The normal form:
// - trims whitespace
// - strips a single leading vendor namespace before the first slash
// - lowercases the result
// - removes all non-alphanumeric characters
//
// This makes inputs such as "qwen/qwen3.6", "QWEN3.6", and
// "Qwen-3.6-27b-MLX-8bit" comparable under the same substring matching
// rules.
func CanonicalKey(s string) string {
	s = strings.TrimSpace(s)
	if slash := strings.Index(s, "/"); slash > 0 {
		s = s[slash+1:]
	}
	s = strings.ToLower(s)
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Match returns the pool entries whose canonical key contains the canonical
// key for requested as a substring. The pool order is preserved.
func Match(requested string, pool []string) []string {
	key := CanonicalKey(requested)
	if key == "" || len(pool) == 0 {
		return nil
	}
	matches := make([]string, 0, len(pool))
	for _, candidate := range pool {
		if candidate == "" {
			continue
		}
		if strings.Contains(CanonicalKey(candidate), key) {
			matches = append(matches, candidate)
		}
	}
	if len(matches) == 0 {
		return nil
	}
	return matches
}
