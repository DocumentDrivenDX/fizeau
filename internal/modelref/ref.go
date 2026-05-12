package modelref

import (
	"fmt"
	"strings"
)

// ModelRef is the canonical model identity used as the cache key and routing
// surface: <provider>/<id>.
// Provider is a lowercase slug (letters, digits, hyphens, dots; no slashes).
// ID preserves the provider's original casing and may contain slashes
// (e.g. openrouter sub-paths like "qwen/qwen3.6-27b").
type ModelRef struct {
	Provider string
	ID       string
}

// Parse splits s on the first "/" into Provider and ID.
//
//	"openrouter/qwen/qwen3.6-27b"  → {Provider:"openrouter", ID:"qwen/qwen3.6-27b"}
//	"vidar-ds4/deepseek-v4-flash"  → {Provider:"vidar-ds4", ID:"deepseek-v4-flash"}
func Parse(s string) (ModelRef, error) {
	idx := strings.IndexByte(s, '/')
	if idx < 0 {
		return ModelRef{}, fmt.Errorf("modelref: no '/' in %q", s)
	}
	r := ModelRef{
		Provider: s[:idx],
		ID:       s[idx+1:],
	}
	if err := r.Validate(); err != nil {
		return ModelRef{}, err
	}
	return r, nil
}

// String returns "<provider>/<id>".
func (r ModelRef) String() string {
	return r.Provider + "/" + r.ID
}

// Validate checks that Provider and ID are non-empty and that Provider
// satisfies the slug rules (lowercase letters, digits, hyphens, dots; no slashes).
func (r ModelRef) Validate() error {
	if r.Provider == "" {
		return fmt.Errorf("modelref: provider is empty")
	}
	if r.ID == "" {
		return fmt.Errorf("modelref: id is empty")
	}
	for i, c := range r.Provider {
		if !isProviderRune(c) {
			return fmt.Errorf("modelref: provider %q has invalid character %q at position %d", r.Provider, c, i)
		}
	}
	return nil
}

func isProviderRune(c rune) bool {
	return (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' || c == '.'
}
