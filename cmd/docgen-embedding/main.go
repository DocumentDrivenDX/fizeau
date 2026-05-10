// Command docgen-embedding renders the embedding (Go library) reference
// page for the public fiz API into website/content/docs/embedding/_index.md.
//
// It uses go/parser + go/doc to extract exported declarations from the
// root fizeau package, then renders a curated, deterministic Hugo page.
// It is NOT a complete pkg.go.dev mirror; this is a guided tour. Run via
// `make docs-embedding`. Output is byte-identical across runs.
package main

import (
	"bytes"
	_ "embed"
	"flag"
	"fmt"
	"go/ast"
	"go/doc"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"
)

//go:embed page.tmpl
var pageTemplate string

const (
	defaultPkgDir = "."
	defaultOut    = "website/content/docs/embedding/_index.md"
	pkgGoDevURL   = "https://pkg.go.dev/github.com/easel/fizeau"
)

// curatedTypes/curatedFuncs control what appears in the highlighted
// "Core types" / "Core functions" sections. Anything exported but not
// listed still appears in the full alphabetical table below.
var curatedTypes = []string{
	"FizeauService", "ServiceOptions", "ServiceExecuteRequest",
	"ServiceEvent", "ServiceConfig", "ModelFilter", "ModelInfo",
	"ProviderInfo", "HarnessInfo", "RouteRequest", "RouteDecision",
}

var curatedFuncs = []string{"New", "RegisterConfigLoader"}

func main() {
	pkgDir := flag.String("pkg", defaultPkgDir, "Path to the root fizeau package")
	outPath := flag.String("out", defaultOut, "Output markdown path")
	flag.Parse()
	if err := run(*pkgDir, *outPath); err != nil {
		fmt.Fprintln(os.Stderr, "docgen-embedding:", err)
		os.Exit(1)
	}
}

func run(pkgDir, outPath string) error {
	absPkg, err := filepath.Abs(pkgDir)
	if err != nil {
		return err
	}
	pkg, fset, err := loadPackage(absPkg)
	if err != nil {
		return fmt.Errorf("load package: %w", err)
	}
	rendered, err := renderPage(buildPageData(pkg, fset))
	if err != nil {
		return fmt.Errorf("render: %w", err)
	}
	absOut, err := filepath.Abs(outPath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(absOut), 0o750); err != nil {
		return err
	}
	// #nosec G306 -- documentation file, world-readable is intentional.
	if err := os.WriteFile(absOut, rendered, 0o644); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "wrote %s (%d bytes)\n", absOut, len(rendered))
	return nil
}

// loadPackage parses the production .go files in dir (excluding tests
// and build-tagged seam files) and returns a go/doc.Package.
func loadPackage(dir string) (*doc.Package, *token.FileSet, error) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, dir, func(fi os.FileInfo) bool {
		name := fi.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			return false
		}
		return !hasNonDefaultBuildTag(filepath.Join(dir, name))
	}, parser.ParseComments)
	if err != nil {
		return nil, nil, err
	}
	if p, ok := pkgs["fizeau"]; ok {
		return doc.New(p, "github.com/easel/fizeau", doc.AllDecls), fset, nil
	}
	return nil, nil, fmt.Errorf("no fizeau package found in %s", dir)
}

// hasNonDefaultBuildTag returns true for files gated behind a //go:build
// tag like "testseam" (i.e. excluded from the default build).
func hasNonDefaultBuildTag(path string) bool {
	// #nosec G304 -- path comes from parser.ParseDir's filter callback.
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "//") {
			return false
		}
		if strings.HasPrefix(line, "//go:build ") {
			expr := strings.TrimSpace(strings.TrimPrefix(line, "//go:build"))
			return !strings.HasPrefix(expr, "!") &&
				!strings.Contains(expr, "&&") &&
				!strings.Contains(expr, "||")
		}
	}
	return false
}

type declRow struct{ Name, Kind, Signature, Synopsis string }
type curatedItem struct{ Name, Signature, Doc string }

type pageData struct {
	PackageDoc                 string
	CoreTypes, CoreFuncs       []curatedItem
	AllDecls                   []declRow
	PkgGoDevURL, GeneratorPath string
}

func buildPageData(pkg *doc.Package, fset *token.FileSet) pageData {
	typeIndex := map[string]*doc.Type{}
	funcIndex := map[string]*doc.Func{}
	for _, t := range pkg.Types {
		typeIndex[t.Name] = t
	}
	for _, f := range pkg.Funcs {
		funcIndex[f.Name] = f
	}

	pd := pageData{
		PackageDoc:    strings.TrimSpace(pkg.Doc),
		PkgGoDevURL:   pkgGoDevURL,
		GeneratorPath: "cmd/docgen-embedding",
	}
	for _, name := range curatedTypes {
		if t, ok := typeIndex[name]; ok {
			pd.CoreTypes = append(pd.CoreTypes, curatedItem{
				Name: name, Signature: typeSignature(t, fset), Doc: firstSentence(t.Doc),
			})
		}
	}
	for _, name := range curatedFuncs {
		if f, ok := funcIndex[name]; ok {
			pd.CoreFuncs = append(pd.CoreFuncs, curatedItem{
				Name: name, Signature: funcSignature(f, fset), Doc: firstSentence(f.Doc),
			})
		}
	}
	for _, t := range pkg.Types {
		pd.AllDecls = append(pd.AllDecls, declRow{
			Name: t.Name, Kind: "type",
			Signature: shortTypeSig(t), Synopsis: firstSentence(t.Doc),
		})
	}
	for _, f := range pkg.Funcs {
		pd.AllDecls = append(pd.AllDecls, declRow{
			Name: f.Name, Kind: "func",
			Signature: oneLineFuncSig(f, fset), Synopsis: firstSentence(f.Doc),
		})
	}
	for _, c := range pkg.Consts {
		for _, n := range c.Names {
			pd.AllDecls = append(pd.AllDecls, declRow{Name: n, Kind: "const", Synopsis: firstSentence(c.Doc)})
		}
	}
	for _, v := range pkg.Vars {
		for _, n := range v.Names {
			pd.AllDecls = append(pd.AllDecls, declRow{Name: n, Kind: "var", Synopsis: firstSentence(v.Doc)})
		}
	}
	sort.Slice(pd.AllDecls, func(i, j int) bool {
		if pd.AllDecls[i].Name == pd.AllDecls[j].Name {
			return pd.AllDecls[i].Kind < pd.AllDecls[j].Kind
		}
		return pd.AllDecls[i].Name < pd.AllDecls[j].Name
	})
	return pd
}

// typeSignature renders the full type spec as Go source.
func typeSignature(t *doc.Type, fset *token.FileSet) string {
	if t.Decl == nil || len(t.Decl.Specs) == 0 {
		return "type " + t.Name
	}
	return printNode(fset, t.Decl, "type "+t.Name)
}

// funcSignature renders a func decl head (no body) as Go source.
func funcSignature(f *doc.Func, fset *token.FileSet) string {
	if f.Decl == nil {
		return "func " + f.Name
	}
	clone := *f.Decl
	clone.Body = nil
	return printNode(fset, &clone, "func "+f.Name)
}

func printNode(fset *token.FileSet, node any, fallback string) string {
	var buf bytes.Buffer
	cfg := &printer.Config{Mode: printer.UseSpaces, Tabwidth: 4}
	if err := cfg.Fprint(&buf, fset, node); err != nil {
		return fallback
	}
	return buf.String()
}

// shortTypeSig returns a short kind label ("interface", "struct", ...).
func shortTypeSig(t *doc.Type) string {
	if t.Decl == nil || len(t.Decl.Specs) == 0 {
		return t.Name
	}
	spec, ok := t.Decl.Specs[0].(*ast.TypeSpec)
	if !ok {
		return t.Name
	}
	switch spec.Type.(type) {
	case *ast.InterfaceType:
		return "interface"
	case *ast.StructType:
		return "struct"
	case *ast.FuncType:
		return "func"
	}
	if spec.Assign.IsValid() {
		return "alias"
	}
	return ""
}

func oneLineFuncSig(f *doc.Func, fset *token.FileSet) string {
	sig := strings.ReplaceAll(funcSignature(f, fset), "\n", " ")
	for strings.Contains(sig, "  ") {
		sig = strings.ReplaceAll(sig, "  ", " ")
	}
	return sig
}

// firstSentence returns the first sentence of a godoc comment, escaped
// for safe insertion into a markdown table cell.
func firstSentence(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if i := strings.IndexAny(s, ".\n"); i >= 0 {
		s = s[:i]
	}
	s = strings.Join(strings.Fields(s), " ")
	return strings.ReplaceAll(s, "|", `\|`)
}

func renderPage(data pageData) ([]byte, error) {
	tmpl, err := template.New("embedding").Parse(pageTemplate)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
