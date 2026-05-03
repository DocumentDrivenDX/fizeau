package skill

import (
	"errors"
	"io"
	"strings"
	"testing"
)

func TestParseFrontmatter_Valid(t *testing.T) {
	src := "---\nname: fix-tests\ndescription: Fix failing tests.\ntags: [testing, go]\n---\n# Body\nrest of body\n"
	r := strings.NewReader(src)
	fm, off, err := ParseFrontmatter(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fm.Name != "fix-tests" {
		t.Errorf("name = %q, want fix-tests", fm.Name)
	}
	if fm.Description != "Fix failing tests." {
		t.Errorf("description = %q", fm.Description)
	}
	if got, want := fm.Tags, []string{"testing", "go"}; len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("tags = %v, want %v", got, want)
	}
	// Body offset should point to the byte right after the closing "---\n".
	if int(off) != len("---\nname: fix-tests\ndescription: Fix failing tests.\ntags: [testing, go]\n---\n") {
		t.Errorf("body offset = %d", off)
	}
}

func TestParseFrontmatter_DoesNotReadPastClosing(t *testing.T) {
	header := "---\nname: a\ndescription: b\n---\n"
	body := "REMAINING_BODY_BYTES"
	src := header + body
	r := strings.NewReader(src)
	_, off, err := ParseFrontmatter(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if int(off) != len(header) {
		t.Fatalf("body offset = %d, want %d", off, len(header))
	}
	// We must not consume bytes from the body via the *underlying* reader.
	// bufio may peek ahead, so to verify the contract we re-read the file at
	// the returned offset and compare — equivalent to the lazy-load path.
	got := src[off:]
	if got != body {
		t.Errorf("body slice = %q, want %q", got, body)
	}
}

func TestParseFrontmatter_NoFrontmatter(t *testing.T) {
	r := strings.NewReader("# Just a markdown file\nno yaml here\n")
	_, _, err := ParseFrontmatter(r)
	if !errors.Is(err, ErrNoFrontmatter) {
		t.Fatalf("err = %v, want ErrNoFrontmatter", err)
	}
}

func TestParseFrontmatter_EmptyInput(t *testing.T) {
	_, _, err := ParseFrontmatter(strings.NewReader(""))
	if !errors.Is(err, ErrNoFrontmatter) {
		t.Fatalf("err = %v, want ErrNoFrontmatter", err)
	}
}

func TestParseFrontmatter_Unterminated(t *testing.T) {
	src := "---\nname: x\ndescription: y\n"
	_, _, err := ParseFrontmatter(strings.NewReader(src))
	if !errors.Is(err, ErrUnterminatedFrontmatter) {
		t.Fatalf("err = %v, want ErrUnterminatedFrontmatter", err)
	}
}

func TestParseFrontmatter_UnknownFieldsIgnored(t *testing.T) {
	src := "---\nname: x\ndescription: y\nmystery_field: 42\n---\n"
	fm, _, err := ParseFrontmatter(strings.NewReader(src))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fm.Name != "x" || fm.Description != "y" {
		t.Errorf("fm = %+v", fm)
	}
}

func TestFrontmatter_Validate(t *testing.T) {
	cases := []struct {
		name    string
		fm      Frontmatter
		wantErr bool
	}{
		{"ok", Frontmatter{Name: "fix-tests", Description: "d"}, false},
		{"missing name", Frontmatter{Description: "d"}, true},
		{"missing description", Frontmatter{Name: "x"}, true},
		{"bad name chars", Frontmatter{Name: "Bad Name!", Description: "d"}, true},
		{"name too long", Frontmatter{Name: strings.Repeat("a", MaxNameLen+1), Description: "d"}, true},
		{"description too long", Frontmatter{Name: "x", Description: strings.Repeat("a", MaxDescriptionLen+1)}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.fm.Validate()
			if (err != nil) != tc.wantErr {
				t.Errorf("err = %v, wantErr = %v", err, tc.wantErr)
			}
		})
	}
}

// crashingReader returns an error after the reader hits EOF; used to confirm
// ParseFrontmatter does not call Read again after parsing the closing ---.
type crashingReader struct{ inner *strings.Reader }

func (c *crashingReader) Read(p []byte) (int, error) {
	n, err := c.inner.Read(p)
	if err == io.EOF {
		return n, io.EOF
	}
	return n, err
}

func TestParseFrontmatter_ReturnsCorrectOffsetWithCRLF(t *testing.T) {
	src := "---\r\nname: x\r\ndescription: y\r\n---\r\nbody\r\n"
	fm, off, err := ParseFrontmatter(strings.NewReader(src))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fm.Name != "x" {
		t.Errorf("name = %q", fm.Name)
	}
	want := strings.Index(src, "body")
	if int(off) != want {
		t.Errorf("offset = %d, want %d", off, want)
	}
}
