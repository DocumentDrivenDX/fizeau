package skill

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeSkillFile(t *testing.T, dir, name, body string) {
	t.Helper()
	sub := filepath.Join(dir, name)
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	content := "---\nname: " + name + "\ndescription: Test skill " + name + ".\n---\n" + body
	if err := os.WriteFile(filepath.Join(sub, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func TestLoadSkillTool_Schema(t *testing.T) {
	tool := &LoadSkillTool{}
	if tool.Name() != "load_skill" {
		t.Fatalf("Name = %q, want load_skill", tool.Name())
	}
	if !tool.Parallel() {
		t.Fatalf("Parallel = false, want true")
	}
	var schema map[string]any
	if err := json.Unmarshal(tool.Schema(), &schema); err != nil {
		t.Fatalf("schema is invalid JSON: %v", err)
	}
	required, _ := schema["required"].([]any)
	if len(required) != 1 || required[0] != "skill_name" {
		t.Fatalf("schema required = %v, want [skill_name]", required)
	}
}

func TestLoadSkillTool_KnownSkillReturnsBody(t *testing.T) {
	dir := t.TempDir()
	writeSkillFile(t, dir, "fix-tests", "# Body for fix-tests\n\nDo the thing.\n")
	cat, _, err := ScanDir(dir)
	if err != nil {
		t.Fatalf("ScanDir: %v", err)
	}
	tool := &LoadSkillTool{Catalog: cat}
	out, err := tool.Execute(context.Background(), json.RawMessage(`{"skill_name":"fix-tests"}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "Body for fix-tests") {
		t.Fatalf("output missing body: %q", out)
	}
}

func TestLoadSkillTool_UnknownSkillListsAvailable(t *testing.T) {
	dir := t.TempDir()
	writeSkillFile(t, dir, "alpha", "alpha body\n")
	writeSkillFile(t, dir, "beta", "beta body\n")
	cat, _, err := ScanDir(dir)
	if err != nil {
		t.Fatalf("ScanDir: %v", err)
	}
	tool := &LoadSkillTool{Catalog: cat}
	_, err = tool.Execute(context.Background(), json.RawMessage(`{"skill_name":"missing"}`))
	if err == nil {
		t.Fatal("expected error for unknown skill")
	}
	msg := err.Error()
	if !strings.Contains(msg, "missing") || !strings.Contains(msg, "alpha") || !strings.Contains(msg, "beta") {
		t.Fatalf("error must mention requested name and available names; got %q", msg)
	}
}

func TestLoadSkillTool_EmptyCatalog(t *testing.T) {
	tool := &LoadSkillTool{Catalog: NewCatalog(nil)}
	_, err := tool.Execute(context.Background(), json.RawMessage(`{"skill_name":"x"}`))
	if err == nil {
		t.Fatal("expected error for empty catalog")
	}
	if !strings.Contains(err.Error(), "no skills") {
		t.Fatalf("error should explain empty catalog; got %q", err.Error())
	}
}

func TestLoadSkillTool_NilCatalog(t *testing.T) {
	tool := &LoadSkillTool{}
	_, err := tool.Execute(context.Background(), json.RawMessage(`{"skill_name":"x"}`))
	if err == nil {
		t.Fatal("expected error for nil catalog")
	}
}

func TestLoadSkillTool_MissingSkillName(t *testing.T) {
	dir := t.TempDir()
	writeSkillFile(t, dir, "alpha", "body\n")
	cat, _, _ := ScanDir(dir)
	tool := &LoadSkillTool{Catalog: cat}
	_, err := tool.Execute(context.Background(), json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error when skill_name missing")
	}
}

func TestLoadSkillTool_InvalidJSON(t *testing.T) {
	tool := &LoadSkillTool{Catalog: NewCatalog(nil)}
	_, err := tool.Execute(context.Background(), json.RawMessage(`not json`))
	if err == nil {
		t.Fatal("expected error for invalid params")
	}
}
