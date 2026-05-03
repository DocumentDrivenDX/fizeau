// Package skill provides discovery and lazy loading of SKILL.md files.
//
// A skill is a Markdown file beginning with a YAML frontmatter block:
//
//	---
//	name: fix-tests
//	description: Fix failing tests in a Go project.
//	tags: [testing, go]
//	---
//	# Full instructions body...
//
// Discovery reads only the frontmatter block; the body is loaded on demand
// through Catalog.LoadBody. This keeps startup cost proportional to the
// number of skills (a few hundred bytes each) rather than the total body
// size.
package skill

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// ErrNoFrontmatter is returned when a file does not begin with a `---` line.
var ErrNoFrontmatter = errors.New("skill: no frontmatter")

// ErrUnterminatedFrontmatter is returned when a `---` opener is not closed.
var ErrUnterminatedFrontmatter = errors.New("skill: unterminated frontmatter")

// MaxNameLen is the maximum permitted skill name length.
const MaxNameLen = 64

// MaxDescriptionLen is the maximum permitted skill description length.
const MaxDescriptionLen = 1024

var nameRe = regexp.MustCompile(`^[a-z0-9_-]+$`)

// Frontmatter is the YAML metadata at the top of a SKILL.md file.
type Frontmatter struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	Tags        []string `yaml:"tags,omitempty"`
	Version     string   `yaml:"version,omitempty"`
}

// Validate returns an error if required fields are missing or malformed.
func (f Frontmatter) Validate() error {
	if f.Name == "" {
		return errors.New("skill: missing required field \"name\"")
	}
	if !nameRe.MatchString(f.Name) {
		return fmt.Errorf("skill: invalid name %q (must match [a-z0-9_-]+)", f.Name)
	}
	if len(f.Name) > MaxNameLen {
		return fmt.Errorf("skill: name %q exceeds %d chars", f.Name, MaxNameLen)
	}
	if f.Description == "" {
		return errors.New("skill: missing required field \"description\"")
	}
	if len(f.Description) > MaxDescriptionLen {
		return fmt.Errorf("skill: description exceeds %d chars", MaxDescriptionLen)
	}
	return nil
}

// ParseFrontmatter consumes only the YAML frontmatter block from r and returns
// the parsed Frontmatter and the byte offset at which the body begins.
//
// The reader is advanced exactly to the byte after the closing `---` line; the
// body is not read. ErrNoFrontmatter is returned if the first non-empty line
// is not `---`. ErrUnterminatedFrontmatter is returned if EOF is reached
// before a closing `---`.
func ParseFrontmatter(r io.Reader) (Frontmatter, int64, error) {
	br := bufio.NewReader(r)
	first, err := br.ReadString('\n')
	if err != nil && err != io.EOF {
		return Frontmatter{}, 0, fmt.Errorf("skill: read frontmatter opener: %w", err)
	}
	bytesRead := int64(len(first))
	if strings.TrimRight(first, "\r\n") != "---" {
		return Frontmatter{}, 0, ErrNoFrontmatter
	}

	var yamlBuf strings.Builder
	for {
		line, err := br.ReadString('\n')
		bytesRead += int64(len(line))
		trimmed := strings.TrimRight(line, "\r\n")
		if trimmed == "---" {
			break
		}
		yamlBuf.WriteString(line)
		if err == io.EOF {
			return Frontmatter{}, 0, ErrUnterminatedFrontmatter
		}
		if err != nil {
			return Frontmatter{}, 0, fmt.Errorf("skill: read frontmatter: %w", err)
		}
	}

	var fm Frontmatter
	if err := yaml.Unmarshal([]byte(yamlBuf.String()), &fm); err != nil {
		return Frontmatter{}, 0, fmt.Errorf("skill: parse frontmatter yaml: %w", err)
	}
	return fm, bytesRead, nil
}
