package skill

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/DocumentDrivenDX/fizeau/internal/safefs"
)

// Skill describes a single discovered SKILL.md file.
type Skill struct {
	// Path is the absolute filesystem path to the SKILL.md file.
	Path string
	// Frontmatter is the parsed YAML metadata.
	Frontmatter Frontmatter
	// BodyOffset is the byte offset at which the markdown body begins
	// (i.e. the byte immediately after the closing `---` line).
	BodyOffset int64
}

// Name returns the skill's frontmatter name.
func (s Skill) Name() string { return s.Frontmatter.Name }

// Description returns the skill's frontmatter description.
func (s Skill) Description() string { return s.Frontmatter.Description }

// Catalog is an immutable collection of discovered skills.
type Catalog struct {
	skills []Skill
	byName map[string]int
}

// NewCatalog builds a Catalog from a slice of skills, deduplicating by name
// (last write wins) and sorting by name for deterministic iteration.
func NewCatalog(skills []Skill) *Catalog {
	c := &Catalog{byName: make(map[string]int, len(skills))}
	for _, s := range skills {
		if i, ok := c.byName[s.Name()]; ok {
			c.skills[i] = s
			continue
		}
		c.byName[s.Name()] = len(c.skills)
		c.skills = append(c.skills, s)
	}
	sort.SliceStable(c.skills, func(i, j int) bool {
		return c.skills[i].Name() < c.skills[j].Name()
	})
	c.byName = make(map[string]int, len(c.skills))
	for i, s := range c.skills {
		c.byName[s.Name()] = i
	}
	return c
}

// Len returns the number of skills in the catalog.
func (c *Catalog) Len() int {
	if c == nil {
		return 0
	}
	return len(c.skills)
}

// Skills returns the catalog's skills sorted by name. The returned slice is
// a copy and may be safely retained by the caller.
func (c *Catalog) Skills() []Skill {
	if c == nil {
		return nil
	}
	out := make([]Skill, len(c.skills))
	copy(out, c.skills)
	return out
}

// Names returns the skill names sorted in catalog order.
func (c *Catalog) Names() []string {
	if c == nil {
		return nil
	}
	out := make([]string, len(c.skills))
	for i, s := range c.skills {
		out[i] = s.Name()
	}
	return out
}

// ByName returns the skill with the given name, or nil if absent.
func (c *Catalog) ByName(name string) *Skill {
	if c == nil {
		return nil
	}
	i, ok := c.byName[name]
	if !ok {
		return nil
	}
	s := c.skills[i]
	return &s
}

// LoadBody reads and returns the raw markdown body of the named skill —
// everything after the closing `---` line of the frontmatter. The file is
// re-opened on each call; the body is not cached.
func (c *Catalog) LoadBody(name string) (string, error) {
	s := c.ByName(name)
	if s == nil {
		return "", fmt.Errorf("skill: unknown skill %q", name)
	}
	data, err := safefs.ReadFile(s.Path)
	if err != nil {
		return "", fmt.Errorf("skill: read %s: %w", s.Path, err)
	}
	if s.BodyOffset > int64(len(data)) {
		return "", fmt.Errorf("skill: body offset %d past file length %d for %s", s.BodyOffset, len(data), s.Path)
	}
	return string(data[s.BodyOffset:]), nil
}

// ScanDir walks dir for `SKILL.md` files, parsing frontmatter from each.
// A non-existent or empty dir returns an empty catalog and a nil error,
// allowing callers to opt in to skill discovery without branching.
//
// Files with missing/invalid frontmatter are skipped and recorded in the
// returned warnings slice; they do not abort the scan.
func ScanDir(dir string) (*Catalog, []string, error) {
	if dir == "" {
		return NewCatalog(nil), nil, nil
	}
	info, err := os.Stat(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return NewCatalog(nil), nil, nil
		}
		return nil, nil, fmt.Errorf("skill: stat %s: %w", dir, err)
	}
	if !info.IsDir() {
		return nil, nil, fmt.Errorf("skill: %s is not a directory", dir)
	}

	var skills []Skill
	var warnings []string
	walkErr := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("walk %s: %v", path, err))
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if !strings.EqualFold(d.Name(), "SKILL.md") {
			return nil
		}
		s, err := loadSkill(path)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("%s: %v", path, err))
			return nil
		}
		skills = append(skills, s)
		return nil
	})
	if walkErr != nil {
		return nil, warnings, fmt.Errorf("skill: walk %s: %w", dir, walkErr)
	}
	return NewCatalog(skills), warnings, nil
}

// loadSkill opens path, parses its frontmatter, and validates required fields.
// The body is not read.
func loadSkill(path string) (Skill, error) {
	// #nosec G304 -- callers operate on user-selected skill directories.
	f, err := os.Open(path)
	if err != nil {
		return Skill{}, fmt.Errorf("open: %w", err)
	}
	defer func() { _ = f.Close() }()

	fm, off, err := ParseFrontmatter(f)
	if err != nil {
		return Skill{}, err
	}
	if err := fm.Validate(); err != nil {
		return Skill{}, err
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	return Skill{Path: abs, Frontmatter: fm, BodyOffset: off}, nil
}
