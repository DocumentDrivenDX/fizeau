package config

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestConfigSchemaHasNoModelRoutes is the structural boundary test for
// ADR-005: config.go itself must define no `ModelRouteConfig` /
// `ModelRouteCandidateConfig` types, no `*Config` method named
// `ModelRouteConfig` / `ModelRouteNames`, and no `model_routes` YAML
// tag. The deprecated surface lives in legacy_model_routes.go for one
// release, after which the boundary tightens further (next release
// deletes that file outright).
func TestConfigSchemaHasNoModelRoutes(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "config.go", nil, parser.ParseComments)
	require.NoError(t, err, "parse config.go")

	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				ts, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}
				if ts.Name.Name == "ModelRouteConfig" || ts.Name.Name == "ModelRouteCandidateConfig" {
					t.Fatalf("config.go must not define %q (move to legacy_model_routes.go per ADR-005)", ts.Name.Name)
				}
			}
		case *ast.FuncDecl:
			if d.Recv == nil || len(d.Recv.List) == 0 {
				continue
			}
			recvType := exprTypeName(d.Recv.List[0].Type)
			if recvType != "Config" {
				continue
			}
			switch d.Name.Name {
			case "ModelRouteConfig", "ModelRouteNames":
				t.Fatalf("config.go must not define method (*Config).%s (move to legacy_model_routes.go per ADR-005)", d.Name.Name)
			}
		}
	}

	// Reject `yaml:"model_routes"` tag in config.go source bytes — even
	// without the type, the tag would re-introduce the deprecated
	// schema surface here.
	data, err := os.ReadFile("config.go")
	require.NoError(t, err)
	if strings.Contains(string(data), `yaml:"model_routes`) {
		t.Fatal("config.go must not declare a `yaml:\"model_routes...\"` tag (move legacy parsing to legacy_model_routes.go per ADR-005)")
	}
}

func exprTypeName(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.StarExpr:
		return exprTypeName(e.X)
	case *ast.Ident:
		return e.Name
	}
	return ""
}

// TestModelRoutesEmitsDeprecationWarning verifies the one-release
// backward compat path: configs that still set `model_routes:` parse,
// honor the configured ordering, and surface a deprecation warning
// containing "ADR-005" and the offending file path. Configs without
// `model_routes:` emit no such warning.
func TestModelRoutesEmitsDeprecationWarning(t *testing.T) {
	t.Run("warns_when_present", func(t *testing.T) {
		isolateHome(t)
		dir := t.TempDir()
		cfgDir := filepath.Join(dir, ".agent")
		require.NoError(t, os.MkdirAll(cfgDir, 0o755))
		cfgPath := filepath.Join(cfgDir, "config.yaml")
		require.NoError(t, os.WriteFile(cfgPath, []byte(`
providers:
  bragi:
    type: lmstudio
    base_url: http://127.0.0.1:1234/v1
model_routes:
  qwen3.5-27b:
    candidates:
      - provider: bragi
        priority: 100
default: bragi
`), 0o644))
		cfg, err := Load(dir)
		require.NoError(t, err)

		warnings := cfg.Warnings()
		var matched string
		for _, w := range warnings {
			if strings.Contains(w, "ADR-005") && strings.Contains(w, cfgPath) {
				matched = w
				break
			}
		}
		assert.NotEmpty(t, matched, "expected deprecation warning naming ADR-005 and the config path; got %v", warnings)
		assert.Contains(t, matched, "model_routes is deprecated")

		// Configured ordering still honored during the deprecation
		// release: the route is parsed and accessible via
		// GetModelRoute.
		route, ok := cfg.GetModelRoute("qwen3.5-27b")
		require.True(t, ok, "model_routes must still parse during the deprecation release")
		require.Len(t, route.Candidates, 1)
		assert.Equal(t, "bragi", route.Candidates[0].Provider)
	})

	t.Run("silent_when_absent", func(t *testing.T) {
		isolateHome(t)
		dir := t.TempDir()
		cfgDir := filepath.Join(dir, ".agent")
		require.NoError(t, os.MkdirAll(cfgDir, 0o755))
		cfgPath := filepath.Join(cfgDir, "config.yaml")
		require.NoError(t, os.WriteFile(cfgPath, []byte(`
providers:
  bragi:
    type: lmstudio
    base_url: http://127.0.0.1:1234/v1
default: bragi
`), 0o644))
		cfg, err := Load(dir)
		require.NoError(t, err)

		for _, w := range cfg.Warnings() {
			if strings.Contains(w, "ADR-005") || strings.Contains(w, "model_routes is deprecated") {
				t.Fatalf("expected no model_routes deprecation warning when absent; got %q", w)
			}
		}
	})
}
