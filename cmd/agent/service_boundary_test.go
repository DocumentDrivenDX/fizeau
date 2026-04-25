package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// modelRoutesParserDeprecationCycleEnded is flipped to true once the
// one-release deprecation window for `model_routes:` closes. While
// false, configs may still parse the deprecated block; once true,
// TestNoModelRoutesParserAfterDeprecation enforces removal of the
// loader entry-point in internal/config.
const modelRoutesParserDeprecationCycleEnded = false

// TestCLIRoutingProviderHasNoCoreProviderImpl asserts that
// cmd/agent/routing_provider.go contains no type that implements the
// agent core Provider surface (Chat / ChatStream methods). After
// ADR-005 step 3 the CLI's per-Chat failover wrapper was deleted; only
// route-status display helpers remain in that file.
func TestCLIRoutingProviderHasNoCoreProviderImpl(t *testing.T) {
	root := repoRootForBoundaryTest(t)
	path := filepath.Join(root, "cmd", "agent", "routing_provider.go")
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	for _, decl := range file.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok || fd.Recv == nil || len(fd.Recv.List) == 0 {
			continue
		}
		switch fd.Name.Name {
		case "Chat", "ChatStream":
			t.Fatalf("cmd/agent/routing_provider.go must not define a Chat/ChatStream method (ADR-005 removed the per-Chat failover wrapper); found method %q", fd.Name.Name)
		}
	}
	// Also reject the `routeProvider` type and `newRouteProvider`
	// constructor by name — they encode the same intent.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	src := string(data)
	for _, banned := range []string{"type routeProvider struct", "func newRouteProvider("} {
		if strings.Contains(src, banned) {
			t.Fatalf("cmd/agent/routing_provider.go must not contain %q (ADR-005 step 3)", banned)
		}
	}
}

// TestNoModelRoutesParserAfterDeprecation is gated on
// modelRoutesParserDeprecationCycleEnded. While false, this test is a
// no-op (deprecation cycle still in effect; the loader is intentionally
// kept). Once flipped to true, the test asserts that
// internal/config/legacy_model_routes.go no longer carries the
// `model_routes` YAML envelope or the `noteLegacyModelRoutes` parser —
// proving the deprecation cycle ended cleanly.
func TestNoModelRoutesParserAfterDeprecation(t *testing.T) {
	if !modelRoutesParserDeprecationCycleEnded {
		t.Skip("model_routes deprecation cycle still in effect (ADR-005); flip modelRoutesParserDeprecationCycleEnded when the cycle ends to enforce parser removal")
	}
	root := repoRootForBoundaryTest(t)
	path := filepath.Join(root, "internal", "config", "legacy_model_routes.go")
	if _, err := os.Stat(path); err == nil {
		t.Fatalf("internal/config/legacy_model_routes.go must be deleted after the deprecation cycle (ADR-005); file still present at %s", path)
	}
}

func TestCLIServiceContractUsesTypedEventDecoder(t *testing.T) {
	root := repoRootForBoundaryTest(t)
	data, err := os.ReadFile(filepath.Join(root, "cmd", "agent", "main.go"))
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}
	src := string(data)
	if !strings.Contains(src, "agent.DecodeServiceEvent(ev)") {
		t.Fatal("CLI execute path must consume typed service events via agent.DecodeServiceEvent")
	}
	if strings.Contains(src, "json.Unmarshal(ev.Data") {
		t.Fatal("CLI must not redefine private ServiceEvent payload shapes by unmarshalling ev.Data directly")
	}
}

func TestCLIMainDoesNotImportOrCallInternalCoreRun(t *testing.T) {
	root := repoRootForBoundaryTest(t)
	data, err := os.ReadFile(filepath.Join(root, "cmd", "agent", "main.go"))
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}
	src := string(data)
	if strings.Contains(src, "internal/core") {
		t.Fatal("cmd/agent/main.go must not import internal/core; execution belongs behind the service boundary")
	}
	if strings.Contains(src, "agentcore.Run(") {
		t.Fatal("cmd/agent/main.go must not call agentcore.Run directly")
	}
}

func TestCLIInternalImportBoundaryAllowlist(t *testing.T) {
	root := repoRootForBoundaryTest(t)
	entries, err := filepath.Glob(filepath.Join(root, "cmd", "agent", "*.go"))
	if err != nil {
		t.Fatalf("glob cmd files: %v", err)
	}
	approved := []string{
		"github.com/DocumentDrivenDX/agent/internal/compaction",
		"github.com/DocumentDrivenDX/agent/internal/config",
		"github.com/DocumentDrivenDX/agent/internal/core",
		"github.com/DocumentDrivenDX/agent/internal/modelcatalog",
		"github.com/DocumentDrivenDX/agent/internal/observations",
		"github.com/DocumentDrivenDX/agent/internal/productinfo",
		"github.com/DocumentDrivenDX/agent/internal/prompt",
		"github.com/DocumentDrivenDX/agent/internal/provider/openai",
		"github.com/DocumentDrivenDX/agent/internal/reasoning",
		"github.com/DocumentDrivenDX/agent/internal/safefs",
		"github.com/DocumentDrivenDX/agent/internal/session",
		"github.com/DocumentDrivenDX/agent/internal/tool",
	}
	for _, path := range entries {
		if filepath.Base(path) == "service_boundary_test.go" {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		src := string(data)
		if !strings.Contains(src, "/internal/") {
			continue
		}
		for _, line := range strings.Split(src, "\n") {
			if !strings.Contains(line, "github.com/DocumentDrivenDX/agent/internal/") {
				continue
			}
			ok := false
			for _, prefix := range approved {
				if strings.Contains(line, prefix) {
					ok = true
					break
				}
			}
			if !ok {
				t.Fatalf("unapproved internal import in %s: %s", path, strings.TrimSpace(line))
			}
		}
	}
}

func repoRootForBoundaryTest(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}
