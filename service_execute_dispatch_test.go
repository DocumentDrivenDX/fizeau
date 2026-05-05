package fizeau

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

func TestExecuteDispatcherSeamsAreExplicit(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "service_execute_dispatch.go", nil, 0)
	if err != nil {
		t.Fatalf("parse service_execute_dispatch.go: %v", err)
	}

	for _, name := range []string{
		"executeRouteResolver",
		"executeSessionLogOpener",
		"executeEventFanout",
		"executeRunnerInvoker",
	} {
		spec := findTypeSpec(file, name)
		if spec == nil {
			t.Fatalf("missing %s seam type", name)
		}
		if _, ok := spec.Type.(*ast.InterfaceType); !ok {
			t.Fatalf("%s is %T, want interface type", name, spec.Type)
		}
	}
}

func TestExecuteDispatcherMovesConcreteRunnerSelectionOutOfExecuteLoop(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "service_execute.go", nil, 0)
	if err != nil {
		t.Fatalf("parse service_execute.go: %v", err)
	}

	for _, imp := range file.Imports {
		path := imp.Path.Value
		switch path {
		case `"github.com/DocumentDrivenDX/fizeau/internal/harnesses/claude"`,
			`"github.com/DocumentDrivenDX/fizeau/internal/harnesses/codex"`,
			`"github.com/DocumentDrivenDX/fizeau/internal/harnesses/gemini"`,
			`"github.com/DocumentDrivenDX/fizeau/internal/harnesses/opencode"`,
			`"github.com/DocumentDrivenDX/fizeau/internal/harnesses/pi"`:
			t.Fatalf("service_execute.go imports concrete runner package %s; selection belongs behind executeRunnerInvoker", path)
		}
	}
}

func findTypeSpec(file *ast.File, name string) *ast.TypeSpec {
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.TYPE {
			continue
		}
		for _, spec := range gen.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if ok && typeSpec.Name.Name == name {
				return typeSpec
			}
		}
	}
	return nil
}
