package anchorwords

import (
	"testing"
	"unicode"
)

func TestLength(t *testing.T) {
	if got := len(Anchors); got != 1024 {
		t.Fatalf("len(Anchors) = %d, want 1024", got)
	}
}

func TestNoEmptyWords(t *testing.T) {
	for i, w := range Anchors {
		if w == "" {
			t.Errorf("Anchors[%d] is empty", i)
		}
	}
}

func TestNoDuplicates(t *testing.T) {
	seen := make(map[string]int, len(Anchors))
	for i, w := range Anchors {
		if prev, ok := seen[w]; ok {
			t.Errorf("Anchors[%d] = %q duplicates Anchors[%d]", i, w, prev)
		}
		seen[w] = i
	}
}

func TestNoKeywords(t *testing.T) {
	// Go reserved keywords (per the language spec). Anchors are capital-first,
	// so a case-insensitive comparison catches any accidental collision.
	keywords := []string{
		"break", "case", "chan", "const", "continue", "default", "defer",
		"else", "fallthrough", "for", "func", "go", "goto", "if", "import",
		"interface", "map", "package", "range", "return", "select",
		"struct", "switch", "type", "var",
	}
	kw := make(map[string]struct{}, len(keywords))
	for _, k := range keywords {
		kw[k] = struct{}{}
	}
	for i, w := range Anchors {
		lower := lowerASCII(w)
		if _, bad := kw[lower]; bad {
			t.Errorf("Anchors[%d] = %q is a Go keyword", i, w)
		}
	}
}

func TestShape(t *testing.T) {
	for i, w := range Anchors {
		n := len(w)
		if n < 6 || n > 12 {
			t.Errorf("Anchors[%d] = %q has length %d, want 6..12", i, w, n)
		}
		if !unicode.IsUpper(rune(w[0])) {
			t.Errorf("Anchors[%d] = %q is not capital-first", i, w)
		}
		for j := 0; j < len(w); j++ {
			c := w[j]
			if c >= 0x80 || !((c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')) {
				t.Errorf("Anchors[%d] = %q contains non-ASCII-letter byte %#x", i, w, c)
				break
			}
		}
	}
}

func lowerASCII(s string) string {
	b := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		b[i] = c
	}
	return string(b)
}
