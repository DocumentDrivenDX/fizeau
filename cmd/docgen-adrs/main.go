// Command docgen-adrs republishes Architecture Decision Records from
// docs/helix/02-design/adr/ADR-*.md into the Hugo/Hextra site under
// website/content/docs/architecture/adr/.
//
// For each source file:
//   - Strip the source `ddx:` front matter (internal tracking).
//   - Parse the `# ADR-NNN: Title` heading and the metadata table that
//     immediately follows it (Date | Status | Deciders | Related | Confidence).
//   - Emit Hextra-compatible front matter (title, linkTitle, weight, description).
//   - Demote the H1 (Hextra renders title from front matter).
//   - Skip ADRs whose Status starts with "Superseded" or "Rejected".
//
// Also writes a per-section _index.md listing every published ADR with its
// status, date, and one-line summary (the first sentence of the Context).
//
// Output is byte-stable across runs given the same source.
//
// Run via `make docs-adrs`. Generator does not parse arbitrary markdown —
// it only touches front matter, the title, the metadata table, and the first
// paragraph of the Context section.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const (
	defaultSrc = "docs/helix/02-design/adr"
	defaultOut = "website/content/docs/architecture/adr"
)

var (
	titleRe      = regexp.MustCompile(`^#\s+(ADR-\d+):\s*(.+?)\s*$`)
	titleStripRe = regexp.MustCompile(`(?m)^#\s+ADR-\d+:.*\n`)
	frontMatRe   = regexp.MustCompile(`(?s)^---\n.*?\n---\n`)
	skippedSt    = []string{"superseded", "rejected"}
)

type adr struct {
	ID         string // "ADR-001"
	Num        int    // 1
	Slug       string // "001-observability-surfaces-and-cost-attribution"
	Title      string // "Observability Surfaces and Cost Attribution"
	Status     string // "Accepted"
	Date       string // "2026-04-09"
	Summary    string // first sentence of Context
	BodyNoFM   string // source body with ddx front matter removed
	SourceFile string // basename of source file
}

func main() {
	src := flag.String("src", defaultSrc, "source ADR directory")
	out := flag.String("out", defaultOut, "output directory under Hugo content")
	flag.Parse()

	entries, err := os.ReadDir(*src)
	if err != nil {
		fail(err)
	}

	var adrs []adr
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, "ADR-") || !strings.HasSuffix(name, ".md") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(*src, name)) //nolint:gosec // generator reads from a fixed in-repo dir
		if err != nil {
			fail(err)
		}
		a, err := parseADR(name, string(raw))
		if err != nil {
			fail(fmt.Errorf("%s: %w", name, err))
		}
		if shouldSkip(a.Status) {
			continue
		}
		adrs = append(adrs, a)
	}
	sort.Slice(adrs, func(i, j int) bool { return adrs[i].Num < adrs[j].Num })

	if err := os.MkdirAll(*out, 0o750); err != nil {
		fail(err)
	}

	// Wipe stale per-ADR pages so deletes/renames don't leak.
	stale, _ := filepath.Glob(filepath.Join(*out, "ADR-*.md"))
	for _, p := range stale {
		_ = os.Remove(p)
	}

	for i, a := range adrs {
		page := renderADR(a, i+2) // weight 1 reserved for the index
		path := filepath.Join(*out, fmt.Sprintf("%s.md", a.ID))
		if err := os.WriteFile(path, []byte(page), 0o600); err != nil {
			fail(err)
		}
	}

	indexPath := filepath.Join(*out, "_index.md")
	if err := os.WriteFile(indexPath, []byte(renderIndex(adrs)), 0o600); err != nil {
		fail(err)
	}
}

func parseADR(filename, raw string) (adr, error) {
	body := frontMatRe.ReplaceAllString(raw, "")
	a := adr{SourceFile: filename, BodyNoFM: body}

	scanner := bufio.NewScanner(strings.NewReader(body))
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	var inMetaTable bool
	var metaSeparatorSeen bool
	var contextLines []string
	var inContext bool
	var contextProblem string
	var inDecision bool
	var decisionLines []string

	for scanner.Scan() {
		line := scanner.Text()
		if a.Title == "" {
			if m := titleRe.FindStringSubmatch(line); m != nil {
				a.ID = m[1]
				a.Title = m[2]
				num := strings.TrimPrefix(a.ID, "ADR-")
				fmt.Sscanf(num, "%d", &a.Num)
				a.Slug = strings.ToLower(strings.ReplaceAll(strings.TrimSuffix(filename, ".md"), "ADR-", ""))
				continue
			}
		}
		if a.Title != "" && a.Date == "" {
			if strings.HasPrefix(line, "| Date") {
				inMetaTable = true
				continue
			}
			if inMetaTable && !metaSeparatorSeen && strings.HasPrefix(line, "|---") {
				metaSeparatorSeen = true
				continue
			}
			if inMetaTable && metaSeparatorSeen && strings.HasPrefix(line, "|") {
				cells := splitTableRow(line)
				if len(cells) >= 2 {
					a.Date = cells[0]
					a.Status = cells[1]
				}
				inMetaTable = false
				continue
			}
		}
		if strings.HasPrefix(line, "## Context") {
			inContext = true
			inDecision = false
			continue
		}
		if strings.HasPrefix(line, "## Decision") {
			inContext = false
			inDecision = true
			continue
		}
		if inContext {
			if strings.HasPrefix(line, "## ") {
				inContext = false
				continue
			}
			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				if len(contextLines) > 0 {
					inContext = false
				}
				continue
			}
			if strings.HasPrefix(trimmed, "|") {
				// Capture the "Problem" row from a leading Context table.
				if contextProblem == "" {
					cells := splitTableRow(trimmed)
					if len(cells) >= 2 && strings.EqualFold(cells[0], "Problem") {
						contextProblem = cells[1]
					}
				}
				continue
			}
			contextLines = append(contextLines, trimmed)
			continue
		}
		if inDecision {
			if strings.HasPrefix(line, "## ") {
				inDecision = false
				continue
			}
			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				if len(decisionLines) > 0 {
					inDecision = false
				}
				continue
			}
			if strings.HasPrefix(trimmed, "|") || strings.HasPrefix(trimmed, "###") {
				continue
			}
			decisionLines = append(decisionLines, trimmed)
		}
	}
	if a.Title == "" {
		return a, fmt.Errorf("no `# ADR-NNN: Title` heading found")
	}
	switch {
	case len(contextLines) > 0:
		a.Summary = firstSentence(strings.Join(contextLines, " "))
	case contextProblem != "":
		a.Summary = firstSentence(contextProblem)
	case len(decisionLines) > 0:
		a.Summary = firstSentence(strings.Join(decisionLines, " "))
	}
	return a, nil
}

func splitTableRow(line string) []string {
	parts := strings.Split(strings.Trim(line, "| \t"), "|")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		out = append(out, strings.TrimSpace(p))
	}
	return out
}

func firstSentence(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if i := strings.Index(s, ". "); i > 0 {
		return s[:i+1]
	}
	if strings.HasSuffix(s, ".") {
		return s
	}
	return s + "."
}

func shouldSkip(status string) bool {
	low := strings.ToLower(status)
	for _, s := range skippedSt {
		if strings.HasPrefix(low, s) {
			return true
		}
	}
	return false
}

func renderADR(a adr, weight int) string {
	desc := strings.ReplaceAll(a.Summary, `"`, `\"`)
	body := titleStripRe.ReplaceAllString(a.BodyNoFM, "")
	body = strings.TrimLeft(body, "\n")
	return fmt.Sprintf(`---
title: "%s: %s"
linkTitle: "%s"
weight: %d
description: "%s"
# Generated by `+"`make docs-adrs`"+` from %s — do not edit by hand.
---

%s`, a.ID, a.Title, a.ID, weight, desc, a.SourceFile, body)
}

func renderIndex(adrs []adr) string {
	var b strings.Builder
	b.WriteString(`---
title: "Architecture Decision Records"
linkTitle: "ADR Index"
weight: 1
description: "Numbered, dated decisions about Fizeau's architecture."
# Generated by ` + "`make docs-adrs`" + ` — do not edit by hand.
---

Each ADR captures one architectural decision: the context that forced the
choice, the alternatives considered, what was decided, and the consequences.
ADRs are append-only — superseded ones stay in the source tree but are hidden
from this index.

| ADR | Status | Date | Decision |
|-----|--------|------|----------|
`)
	for _, a := range adrs {
		b.WriteString(fmt.Sprintf("| [%s](%s) | %s | %s | %s |\n",
			a.ID, a.ID, a.Status, a.Date, escapePipes(a.Title)))
	}
	return b.String()
}

func escapePipes(s string) string { return strings.ReplaceAll(s, "|", `\|`) }

func fail(err error) {
	fmt.Fprintln(os.Stderr, "docgen-adrs:", err)
	os.Exit(1)
}
