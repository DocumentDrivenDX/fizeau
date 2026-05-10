// Command docgen-tools regenerates the Hugo "Tool reference" page at
// website/content/docs/tools/_index.md by enumerating the built-in tools
// fiz exposes to the LLM via BuiltinToolsForPreset.
//
// The generator instantiates the registry with deterministic dummy values
// (workdir "/app", an empty BashOutputFilterConfig) so the rendered page is
// byte-stable across machines. Running it twice with no source changes
// produces identical output.
//
// Run via `make docs-tools`. The page covers:
//   - per-tool name, description, parameter JSON Schema, parallel-safety
//   - a preset matrix showing which presets bundle which tools
//   - a pointer at internal/tool/builtin.go for adding new tools
package main

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"

	"github.com/easel/fizeau"
	"github.com/easel/fizeau/internal/prompt"
)

//go:embed page.tmpl
var pageTemplate string

const (
	defaultOut    = "website/content/docs/tools/_index.md"
	dummyWorkDir  = "/app"
	registryPath  = "internal/tool/builtin.go"
	parallelYes   = "yes"
	parallelNo    = "no"
	missingMarker = "—"
	presentMarker = "x"
)

// toolDoc is the per-tool data the template renders.
type toolDoc struct {
	Name        string
	Description string
	Schema      string // pretty-printed JSON
	Parallel    string // "yes" / "no"
	Anchor      string // markdown heading anchor
}

// presetRow is one row of the preset matrix.
type presetRow struct {
	Tool  string
	Cells []string // one cell per preset, "x" or "—"
}

// pageData feeds page.tmpl.
type pageData struct {
	Tools        []toolDoc
	Presets      []string
	Matrix       []presetRow
	RegistryPath string
}

func main() {
	out := flag.String("out", defaultOut, "output markdown file")
	flag.Parse()

	presets := prompt.PresetNames()
	if len(presets) == 0 {
		fail(fmt.Errorf("prompt.PresetNames() returned no presets"))
	}

	// Collect tools per preset (deterministic order: presets sorted as
	// returned by PresetNames; tools sorted by name within each preset).
	perPreset := make(map[string]map[string]bool, len(presets))
	canonical := map[string]fizeau.Tool{}
	for _, p := range presets {
		set := map[string]bool{}
		tools := fizeau.BuiltinToolsForPreset(dummyWorkDir, p, fizeau.BashOutputFilterConfig{})
		for _, t := range tools {
			set[t.Name()] = true
			if _, ok := canonical[t.Name()]; !ok {
				canonical[t.Name()] = t
			}
		}
		perPreset[p] = set
	}

	names := make([]string, 0, len(canonical))
	for n := range canonical {
		names = append(names, n)
	}
	sort.Strings(names)

	tools := make([]toolDoc, 0, len(names))
	for _, n := range names {
		t := canonical[n]
		pretty, err := prettyJSON(t.Schema())
		if err != nil {
			fail(fmt.Errorf("tool %s: schema is not valid JSON: %w", n, err))
		}
		par := parallelNo
		if t.Parallel() {
			par = parallelYes
		}
		tools = append(tools, toolDoc{
			Name:        n,
			Description: t.Description(),
			Schema:      pretty,
			Parallel:    par,
			Anchor:      anchor(n),
		})
	}

	matrix := make([]presetRow, 0, len(names))
	for _, n := range names {
		row := presetRow{Tool: n, Cells: make([]string, len(presets))}
		for i, p := range presets {
			if perPreset[p][n] {
				row.Cells[i] = presentMarker
			} else {
				row.Cells[i] = missingMarker
			}
		}
		matrix = append(matrix, row)
	}

	data := pageData{
		Tools:        tools,
		Presets:      presets,
		Matrix:       matrix,
		RegistryPath: registryPath,
	}

	rendered, err := render(data)
	if err != nil {
		fail(err)
	}

	if err := os.MkdirAll(filepath.Dir(*out), 0o750); err != nil {
		fail(err)
	}
	if err := os.WriteFile(*out, rendered, 0o600); err != nil {
		fail(err)
	}
}

func render(d pageData) ([]byte, error) {
	tmpl, err := template.New("page").Parse(pageTemplate)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, d); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// prettyJSON re-marshals raw JSON with two-space indentation. Going
// through json.Unmarshal+Marshal also normalizes key order (well: it
// preserves source order for objects via map round-trip is NOT preserved,
// so we use json.Indent which keeps the source byte order intact).
func prettyJSON(raw json.RawMessage) (string, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return "{}", nil
	}
	var buf bytes.Buffer
	if err := json.Indent(&buf, raw, "", "  "); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// anchor turns a tool name like "load_skill" into a markdown heading
// anchor "load_skill" (Hextra/Hugo lowercases and underscore-keeps).
func anchor(name string) string {
	return strings.ToLower(name)
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "docgen-tools:", err)
	os.Exit(1)
}
