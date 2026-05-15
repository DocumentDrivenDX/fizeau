package renamecheck

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type Finding struct {
	Path    string
	Line    int
	Column  int
	Surface string
	Match   string
}

type Options struct {
	Root string
	Out  io.Writer
}

type rule struct {
	surface string
	re      *regexp.Regexp
	rootGo  bool
}

var rules = []rule{
	{surface: "github.com/DocumentDrivenDX/agent", re: regexp.MustCompile(regexp.QuoteMeta("github.com/DocumentDrivenDX/agent"))},
	{surface: "root package agent", re: regexp.MustCompile(`^\s*package\s+agent\s*$`), rootGo: true},
	{surface: "ddx-agent", re: regexp.MustCompile(regexp.QuoteMeta("ddx-agent"))},
	{surface: "DDX Agent/DDx Agent", re: regexp.MustCompile(`DD[Xx] Agent`)},
	{surface: ".agent", re: regexp.MustCompile(`(^|[^A-Za-z0-9_])\.agent([^A-Za-z0-9_]|$)`)},
	{surface: "~/.config/agent", re: regexp.MustCompile(regexp.QuoteMeta("~/.config/agent"))},
	{surface: "AGENT_*/DDX_AGENT_*", re: regexp.MustCompile(`\b(?:DDX_AGENT|AGENT)_[A-Z0-9_]+\b`)},
}

var skippedDirs = map[string]bool{
	".git":                 true,
	".ddx":                 true,
	".agents":              true,
	".claude":              true,
	".fizeau":              true,
	".helix-ratchets":      true,
	"bench/results":        true,
	"docs/helix":           true,
	"docs/research":        true,
	"internal/renamecheck": true,
	// ADRs are immutable historical decision records that legitimately
	// reference the prior project name in context.
	"website/content/docs/architecture/adr": true,
}

var skippedFiles = map[string]bool{
	"CHANGELOG.md": true,
	// Guard test that embeds "ddx-agent-" as a sentinel to assert its absence
	// from the release workflow. The rename-noise gate enforces the same
	// constraint globally; the literal in the test is the checked value, not usage.
	"release_artifact_names_test.go": true,
}

func Scan(opts Options) ([]Finding, error) {
	root := opts.Root
	if root == "" {
		root = "."
	}

	var findings []Finding
	files, gitOK := gitTrackedFiles(root)
	if gitOK {
		for _, rel := range files {
			if shouldSkipPath(rel) {
				continue
			}
			if skippedFiles[rel] {
				continue
			}
			if !isTextCandidate(rel) {
				continue
			}
			fileFindings, err := scanFile(root, rel)
			if err != nil {
				return nil, err
			}
			findings = append(findings, fileFindings...)
		}
	} else {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			rel = filepath.ToSlash(rel)
			if rel == "." {
				return nil
			}
			if d.IsDir() {
				if shouldSkipDir(rel) {
					return filepath.SkipDir
				}
				return nil
			}
			if skippedFiles[rel] {
				return nil
			}
			if !isTextCandidate(rel) {
				return nil
			}
			fileFindings, err := scanFile(root, rel)
			if err != nil {
				return err
			}
			findings = append(findings, fileFindings...)
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	sort.Slice(findings, func(i, j int) bool {
		if findings[i].Path != findings[j].Path {
			return findings[i].Path < findings[j].Path
		}
		if findings[i].Line != findings[j].Line {
			return findings[i].Line < findings[j].Line
		}
		return findings[i].Column < findings[j].Column
	})
	return findings, nil
}

func Report(w io.Writer, findings []Finding) error {
	if w == nil {
		return nil
	}
	if len(findings) == 0 {
		_, err := fmt.Fprintln(w, "rename-noise: no unallowlisted old-name hits found")
		return err
	}
	if _, err := fmt.Fprintf(w, "rename-noise: %d unallowlisted old-name hit(s)\n", len(findings)); err != nil {
		return err
	}
	for _, f := range findings {
		if _, err := fmt.Fprintf(w, "%s:%d:%d: %s: %q\n", f.Path, f.Line, f.Column, f.Surface, f.Match); err != nil {
			return err
		}
	}
	return nil
}

func Run(opts Options) ([]Finding, error) {
	findings, err := Scan(opts)
	if err != nil {
		return nil, err
	}
	return findings, Report(opts.Out, findings)
}

func gitTrackedFiles(root string) ([]string, bool) {
	cmd := exec.Command("git", "-C", root, "ls-files", "-z") // #nosec G204 -- root is the validated repo root
	out, err := cmd.Output()
	if err != nil {
		return nil, false
	}
	parts := strings.Split(strings.TrimRight(string(out), "\x00"), "\x00")
	files := make([]string, 0, len(parts))
	for _, p := range parts {
		if p == "" {
			continue
		}
		files = append(files, filepath.ToSlash(p))
	}
	return files, true
}

func shouldSkipPath(rel string) bool {
	for dir := range skippedDirs {
		if rel == dir || strings.HasPrefix(rel, dir+"/") {
			return true
		}
	}
	return false
}

func scanFile(root, rel string) ([]Finding, error) {
	file, err := os.Open(filepath.Join(root, filepath.FromSlash(rel))) // #nosec G304 -- root is a validated repo root, rel is a relative path
	if err != nil {
		// Tolerate broken symlinks and files that disappeared between
		// enumeration and open (e.g., generated benchmark artifacts).
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	defer file.Close()

	var findings []Finding
	reader := bufio.NewReader(file)
	lineNo := 0
	for {
		line, err := reader.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return nil, err
		}
		if line == "" && errors.Is(err, io.EOF) {
			break
		}
		lineNo++
		for _, r := range rules {
			if r.rootGo && !isRootGoFile(rel) {
				continue
			}
			loc := r.re.FindStringIndex(line)
			if loc == nil {
				continue
			}
			findings = append(findings, Finding{
				Path:    rel,
				Line:    lineNo,
				Column:  loc[0] + 1,
				Surface: r.surface,
				Match:   line[loc[0]:loc[1]],
			})
		}
		if errors.Is(err, io.EOF) {
			break
		}
	}
	return findings, nil
}

func shouldSkipDir(rel string) bool {
	if skippedDirs[rel] {
		return true
	}
	for dir := range skippedDirs {
		if strings.HasPrefix(rel, dir+"/") {
			return true
		}
	}
	return false
}

func isRootGoFile(rel string) bool {
	return filepath.Dir(rel) == "." && strings.HasSuffix(rel, ".go")
}

func isTextCandidate(rel string) bool {
	base := filepath.Base(rel)
	if strings.HasPrefix(base, ".") && base != ".env" {
		return false
	}
	switch filepath.Ext(rel) {
	case ".go", ".md", ".txt", ".yaml", ".yml", ".json", ".jsonl", ".toml", ".sh", ".py", ".env":
		return true
	default:
		return base == "Makefile" || base == "Dockerfile" || base == "go.mod"
	}
}
