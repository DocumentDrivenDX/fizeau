package anthropic

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"

	agent "github.com/easel/fizeau/internal/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConvertToolsSetsCacheControlOnLast(t *testing.T) {
	tools := []agent.ToolDef{
		{Name: "alpha", Description: "first", Parameters: json.RawMessage(`{"type":"object"}`)},
		{Name: "bravo", Description: "second", Parameters: json.RawMessage(`{"type":"object"}`)},
		{Name: "charlie", Description: "third", Parameters: json.RawMessage(`{"type":"object"}`)},
	}

	got := convertTools(tools, agent.Options{})
	require.Len(t, got, 3)

	for i := 0; i < 2; i++ {
		require.NotNil(t, got[i].OfTool, "tool %d should be OfTool variant", i)
		assert.Empty(t, string(got[i].OfTool.CacheControl.Type),
			"tool %d should have no cache_control", i)
	}
	require.NotNil(t, got[2].OfTool)
	assert.Equal(t, "ephemeral", string(got[2].OfTool.CacheControl.Type),
		"last tool should carry cache_control: ephemeral")
}

func TestConvertToolsCachePolicyOffEmitsNoMarker(t *testing.T) {
	tools := []agent.ToolDef{
		{Name: "alpha", Description: "first", Parameters: json.RawMessage(`{"type":"object"}`)},
		{Name: "bravo", Description: "second", Parameters: json.RawMessage(`{"type":"object"}`)},
	}
	got := convertTools(tools, agent.Options{CachePolicy: "off"})
	require.Len(t, got, 2)
	for i, tu := range got {
		require.NotNil(t, tu.OfTool)
		assert.Empty(t, string(tu.OfTool.CacheControl.Type),
			"tool %d should have no cache_control when CachePolicy=off", i)
	}
}

func TestBuildSystemBlocksSetsCacheControlOnLast(t *testing.T) {
	msgs := []agent.Message{
		{Role: agent.RoleSystem, Content: "sys-a"},
		{Role: agent.RoleSystem, Content: "sys-b"},
		{Role: agent.RoleSystem, Content: "sys-c"},
		{Role: agent.RoleUser, Content: "hi"},
	}
	system, conv := buildSystemBlocks(msgs, agent.Options{})
	require.Len(t, system, 3)
	require.Len(t, conv, 1)
	assert.Empty(t, string(system[0].CacheControl.Type))
	assert.Empty(t, string(system[1].CacheControl.Type))
	assert.Equal(t, "ephemeral", string(system[2].CacheControl.Type))
}

func TestBuildSystemBlocksCachePolicyOffEmitsNoMarker(t *testing.T) {
	msgs := []agent.Message{
		{Role: agent.RoleSystem, Content: "sys-a"},
		{Role: agent.RoleSystem, Content: "sys-b"},
		{Role: agent.RoleUser, Content: "hi"},
	}
	system, _ := buildSystemBlocks(msgs, agent.Options{CachePolicy: "off"})
	require.Len(t, system, 2)
	for i, b := range system {
		assert.Empty(t, string(b.CacheControl.Type),
			"system block %d should have no cache_control when CachePolicy=off", i)
	}
}

func TestBuildSystemBlocksEmptyInput(t *testing.T) {
	system, conv := buildSystemBlocks(nil, agent.Options{})
	assert.Empty(t, system)
	assert.Empty(t, conv)

	system, conv = buildSystemBlocks([]agent.Message{
		{Role: agent.RoleUser, Content: "hi"},
	}, agent.Options{})
	assert.Empty(t, system)
	require.Len(t, conv, 1)
}

// TestAnthropicSharedSystemBuilderUsedByChatAndChatStream is an AST refactor
// guard: it parses anthropic.go and asserts that Chat and ChatStream both
// delegate to buildSystemBlocks rather than inlining the system-block
// construction loop.
func TestAnthropicSharedSystemBuilderUsedByChatAndChatStream(t *testing.T) {
	src, err := os.ReadFile("anthropic.go")
	require.NoError(t, err)

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "anthropic.go", src, parser.ParseComments)
	require.NoError(t, err)

	lines := strings.Split(string(src), "\n")
	bodySrc := func(fn *ast.FuncDecl) string {
		if fn.Body == nil {
			return ""
		}
		start := fset.Position(fn.Body.Lbrace).Line
		end := fset.Position(fn.Body.Rbrace).Line
		if start < 1 || end > len(lines) {
			return ""
		}
		return strings.Join(lines[start-1:end], "\n")
	}

	var foundChat, foundStream, foundBuilder bool
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		switch fn.Name.Name {
		case "Chat", "ChatStream":
			body := bodySrc(fn)
			assert.Containsf(t, body, "buildSystemBlocks(",
				"%s must delegate to buildSystemBlocks", fn.Name.Name)
			assert.NotContainsf(t, body, "ant.TextBlockParam{Text:",
				"%s must not inline TextBlockParam construction; use buildSystemBlocks", fn.Name.Name)
			if fn.Name.Name == "Chat" {
				foundChat = true
			} else {
				foundStream = true
			}
		case "buildSystemBlocks":
			foundBuilder = true
		}
	}
	assert.True(t, foundChat, "Chat function declaration not found")
	assert.True(t, foundStream, "ChatStream function declaration not found")
	assert.True(t, foundBuilder, "buildSystemBlocks function declaration not found")
}
